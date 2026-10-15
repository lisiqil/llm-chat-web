# 数据库表说明

数据库是 PostgreSQL 16。表结构以 `backend/app/db/models/entities.py` 为准，变更只走 Alembic，初始迁移是 `backend/alembic/versions/0001_initial.py`。一共 11 张业务表。

用户和登录是入口。`users` 存账号：用户名、密码哈希、角色（`admin` 或 `user`）、是否启用，以及创建、更新、最近登录、改密时间。`auth_sessions` 是登录会话。登录成功后存的是 token 的哈希，不是明文 token，并记录过期时间、最近访问、吊销时间和浏览器标识。用户被删除时，会话一起删掉。

对话本身是三张表。`conversations` 是一条会话：属于哪个用户、标题、状态（进行中 `active` 或已归档 `archived`）、下一条消息序号 `next_message_seq`，以及最后一条消息时间。`messages` 是会话里的每一句话，角色只有 `user` 和 `assistant`，按 `sequence_no` 排顺序。内容、状态（完成、等待、流式输出中、中断、取消、失败）都在这里。助手消息通过 `reply_to_message_id` 指回同一会话里的用户消息。`generation_tasks` 是一次模型生成任务，把这次请求的用户消息和正在写的助手消息绑在一起，并记录模型名、生成参数、状态、token 用量、排队和生成耗时、错误信息。同一会话同时只能有一条进行中的任务（`queued`、`running`、`cancel_requested`）。`idempotency_key` 用来防止同一用户重复提交；`retry_of_task_id` 表示这次是重试哪一次。客户端断开后，任务可以继续到 `disconnect_deadline_at`，服务重启中断会记成 `interrupted`，和模型执行失败分开。

`generation_task_events` 是这次生成过程中的事件流。主键是任务 ID 加序号，每条可以带增量文本 `delta_text` 和一份 JSON 载荷。刷新页面或重连时，靠这些事件把已经吐出的内容补回去。

长期记忆有两张表，都挂在会话上。`conversation_memories` 每个会话一行，存滚动摘要：摘要正文、版本号，以及这份摘要已经覆盖到第几条消息（`covered_through_sequence`）。后续生成不必把整段历史都塞进上下文。`conversation_decisions` 存这次对话里确认下来的决定，有标题、内容和状态（有效、被新决定取代、已归档），还可以指回来源消息。

偏好和全局配置分开存。`user_preferences` 是用户自己的键值偏好，值是 JSON。来源可以是用户自己设的，也可以是助手建议的；停用时只把 `is_active` 关掉，不删记录。同一用户同一个 key 只能有一条。`generation_settings` 是全局单例，主键固定为 `1`，存默认模型名、`temperature`、`top_p`、`max_tokens`、可选的 `top_k` 和 `repeat_penalty`，以及系统提示词。谁在什么时候改的，记在 `updated_by` 和 `updated_at`。

`audit_logs` 记管理操作：谁做的、做了什么动作、目标类型和 ID、详情 JSON、来源 IP、时间。操作人被删后，日志还在，只是操作人字段变成空。

## 补充约定

删除用户会级联删除其会话、消息、偏好和登录会话。删除会话会级联删除其中的消息、生成任务、事件、摘要和决定。会话删除是物理删除，审计日志只保留会话 ID，不保留正文。归档会话不能继续发送，需要先取消归档。

`generation_tasks.status` 的取值：

| 状态 | 含义 |
|---|---|
| `queued` | 已入队，还没调用模型 |
| `running` | 正在生成 |
| `cancel_requested` | 已请求取消，等待上游流结束 |
| `completed` | 生成完成 |
| `cancelled` | 已取消 |
| `failed` | 模型执行失败 |
| `interrupted` | 服务重启等原因中断，需要用户手动重试 |
| `rejected` | 请求被拒绝，没有开始生成 |

摘要维护不写入 `generation_tasks`，避免造出虚假的用户消息和助手消息。进程重启后，还没调用模型的 `queued` 任务会继续执行；`running` 和 `cancel_requested` 会被标成 `interrupted`，不会自动重发。关键决定由用户维护，模型不会把猜测写成已确认决定。
