# 本地 LLM Web 聊天系统：功能需求与技术设计规格书

> 2026-10-15：业务实现已按作者要求清空，仓库只保留依赖、框架和可启动的空壳。本文是上一版设计记录，不要据此把功能补回去，除非作者明确要求。
>
> 文档版本：V1.1  
> 文档用途：上一版设计记录，不是当前代码  
> 当前阶段：单机单后端，多用户，管理员预创建账号，PostgreSQL 16  
> 设计原则：先确保数据可靠、用户隔离、任务生命周期可控；避免过早引入分布式架构

---

## 0. 给 Cursor 的执行说明

本文描述已经落地的系统。后续修改以当前代码和本文为准，不要把现有工程当成空目录重做，也不要未经确认删除或覆盖已有代码。若实现方案与本文冲突，先说明冲突、影响和建议，再改代码或回写本文。

### 0.1 必须遵守的实现原则

1. **按阶段实现，不要一次性输出一个未经验证的大型代码补丁。** 每个阶段完成后运行测试、检查错误并总结变更。
2. **先实现可靠的端到端最小闭环。** 优先完成登录、用户隔离、会话与消息持久化、模型调用、流式返回、任务取消和断线恢复，再增加记忆管理及管理后台。
3. **数据库是持久化事实来源。** 内存缓存、进程内任务注册表和浏览器状态都不能成为唯一数据来源。
4. **所有用户资源都必须在后端校验归属。** 不能只依靠前端隐藏按钮或不可猜测的 ID。
5. **模型服务凭证只存在于后端配置。** 任何前端代码、前端环境变量、API 响应和日志均不得泄露 llama-server API Key。
6. **不假设关闭 HTTP 流就一定停止模型计算。** 必须通过当前实际运行的 llama-server 版本进行取消集成测试；资源槽位只能在上游生成已结束或已确认释放后归还。
7. **不假设所有 OpenAI 兼容参数都被当前 llama-server 版本支持。** 模型调用适配器需要集中管理参数，并对实际接口进行测试。
8. 不得把模型的 `reasoning_content`、内部推理字段或其他非最终答案内容默认展示给普通用户。V1 只显示正常回答内容。
9. 不实现公开注册、多模型路由、向量数据库、复杂 Agent、MCP、文件上传或分布式任务队列，除非后续明确提出需求。
10. 新增配置必须有合理默认值、类型校验、示例配置和文档。不要在代码中硬编码密码、API Key、生产域名或模型绝对路径。
11. 所有数据库结构变更必须通过迁移管理，不得依赖手工修改生产数据库。
12. 写入、删除、取消、重试等操作必须考虑重复请求和并发竞态，并提供相应测试。

### 0.2 每阶段交付格式

每一阶段完成时请提供：

- 已实现的功能和未实现的功能；
- 修改或新增的关键文件；
- 数据库迁移变化；
- 测试命令及实际结果；
- 尚存风险和下一阶段计划。

如果实现方案与本文冲突，请先说明冲突、影响和建议，再进行修改；不要静默改变关键架构决策。

---

## 1. 产品概述

### 1.1 产品目标

构建一个运行在个人服务器上的 Web 聊天系统。浏览器访问应用后，用户通过登录使用本地部署的 LLM。应用后端通过 llama-server 提供的 OpenAI 兼容接口请求模型，管理用户、会话、聊天历史、记忆、生成任务及访问权限。

本系统不是 llama-server 官方 Web UI 的完整复刻。它是一个由应用后端控制身份、数据、记忆和任务状态的独立聊天产品。

### 1.2 当前已确定的部署和产品假设

- 单机部署、单个应用后端进程；
- 多用户，但不公开注册；账号由管理员提前创建；
- 第一版使用一个已经运行的 llama-server 和一个默认模型；
- llama-server 对应用后端提供 OpenAI 兼容的 `/v1/chat/completions` 接口；
- PostgreSQL 16 作为持久化数据库；连接串必须是 PostgreSQL，启动时拒绝其他数据库；
- 进程内缓存可用于优化活跃会话访问，但可随时丢弃并从数据库重建；
- 正常聊天以前台使用为主；浏览器短暂断线可以恢复，超过恢复期限后取消任务；
- 单个会话默认最多存在一个未结束的生成任务；
- 用户之间的会话、消息和记忆严格隔离；
- 暂不提供公开 API Token、组织/团队、公开注册和付费计费功能。

### 1.3 V1 的成功标准

1. 管理员可以创建、禁用用户以及重置用户密码。
2. 用户可以登录、退出并恢复有效登录会话。
3. 用户可以新建会话、重命名会话、查看历史、归档和删除会话。
4. 用户消息与助手回答能可靠保存，不因页面刷新而丢失已持久化内容。
5. 模型回答支持实时流式显示和停止生成。
6. 浏览器短暂断线后能恢复同一个任务，不能因重连重复生成第二次回答。
7. 超过断线恢复期限后，系统请求停止生成，并正确记录取消结果；不能虚报取消成功。
8. 长对话有摘要、关键决策和用户偏好机制，模型上下文有明确大小限制。
9. 一个用户不能读取或操作其他用户的聊天和记忆。
10. 后端重启后，未结束任务能够被识别并安全标记为中断，不会被静默当成仍在运行。
11. llama-server 不需要直接暴露给浏览器；模型 API Key 不向前端泄露。
12. 有自动化测试覆盖认证、资源隔离、幂等请求、任务状态和断线恢复的关键规则。

---

## 2. V1 范围与非目标

### 2.1 V1 必须包含

- 登录、退出、当前用户信息、会话过期处理；
- 管理员用户管理；
- 多用户资源隔离；
- 会话和消息管理；
- llama-server 连接配置和连通性检查；
- 流式生成、停止、失败提示、重试；
- 生成任务生命周期、队列和并发限制；
- 短暂断线恢复和输出快照；
- 历史消息持久化；
- 会话摘要、关键决策、用户偏好；
- 提示词组装和上下文裁剪；
- 基础生成参数设置；
- 基础系统状态与日志；
- 数据库迁移、备份说明、测试、运行文档。

### 2.2 V1 暂不包含

- 用户自行注册或公开邀请码；
- 微信、Google、OIDC 等第三方登录；
- 多模型动态路由、自动模型切换和分布式 GPU 调度；
- 多个后端实例共享任务队列；
- 向量数据库、语义检索、RAG 知识库；
- 自动浏览网页、执行外部工具、Agent/MCP 工作流；
- 文件、图片、音频和视频上传；
- 用户自助 API Token 管理；
- 计费、额度结算、团队和企业级组织管理；
- 完整复刻 llama-server 的 `/props`、`/tools` 等官方 Web UI 辅助端点。

以上并不意味着架构必须阻止未来扩展；只是不应为了未实现的功能增加当前复杂度。

---

## 3. 建议技术栈

以下技术栈是已落地的决策。后续修改不要改用其他框架。

### 3.1 前端

**固定使用 React 技术栈，不要改用 Vue，也不要混用不同框架。**

- React + TypeScript；
- Vite 作为开发服务器和生产构建工具；
- React Router 管理页面路由和登录保护；
- Zustand 只管理确实需要跨组件共享的客户端状态，例如当前用户、当前选中会话、活跃任务和连接状态；表单输入等局部状态优先使用 React 本地状态；
- `fetch` + `ReadableStream` 接收和解析后端的 SSE 风格事件流，不要因为使用 React 就引入额外的流式通信框架；
- Tailwind CSS + shadcn/ui 作为基础样式和组件方案；可复用其可访问性组件，聊天消息区、会话侧栏和任务状态应按产品需求组合，不要机械套用后台模板；
- Markdown 使用 `react-markdown` 和 `remark-gfm`；代码块可加语法高亮；默认不启用原始 HTML 渲染，避免模型输出的 HTML 直接执行；
- 前端 API 请求集中放在 `src/api/`，共享类型放在 `src/types/`；避免组件直接散落拼接 URL 或重复定义响应类型；
- 不引入 Redux、React Query 等额外状态框架，除非实现过程中出现明确且有文档依据的需求。

### 3.2 后端

- Python 3.11 或项目现有受支持版本；
- FastAPI；
- Pydantic v2 / `pydantic-settings`；
- SQLAlchemy 2；
- Alembic 数据库迁移；
- PostgreSQL 16，通过 SQLAlchemy 与 `psycopg` 访问；
- `httpx` 异步 HTTP 客户端，用于调用 llama-server；
- `argon2id` 密码哈希实现；
- 标准 Python logging 或结构化日志库。

不要为了 V1 引入 Redis、Celery、Kafka 或微服务架构。

### 3.3 推理服务

模型适配层通过配置中的 `LLM_BASE_URL` 调用 llama-server 的 `/v1/chat/completions`。模型名、API Key、超时、上下文预算和生成参数均来自后端配置或后端管理设置。

开发环境可使用本机地址；生产环境优先使用服务器内部地址，例如 `http://127.0.0.1:8080/v1`，而不是让浏览器访问公网代理地址。实际地址必须通过环境变量配置，不得写死。

llama-server 官方接口文档：<https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md>。以当前安装的 llama-server 版本和实际测试为准，不应把主分支文档中的所有功能都假定为已安装版本支持。

---

## 4. 系统架构和模块职责

采用**单后端进程、模块化单体应用**。模块之间通过明确的服务接口协作，不拆成多个微服务。

### 4.1 逻辑模块

1. **Authentication / Authorization**：登录验证、会话、角色权限和资源归属检查。
2. **User Management**：用户创建、禁用、密码重置和会话撤销。
3. **Conversation Service**：会话创建、列表、重命名、归档、删除和消息历史查询。
4. **Message Service**：用户消息、助手消息、状态、顺序和正文持久化。
5. **Task Manager**：创建任务、状态流转、队列、单会话并发限制、超时、取消和恢复。
6. **Inference Adapter**：封装 llama-server 的请求、SSE 解析、错误映射、上游取消和能力检测。
7. **Context Builder**：收集系统提示词、摘要、关键决策、用户偏好和最近历史，生成最终模型输入。
8. **Memory Service**：摘要、决策和偏好的保存、更新、来源追踪与上下文选择。
9. **Settings Service**：管理非机密的模型生成设置和系统提示词；机密配置只从服务器环境读取。
10. **Audit / Observability**：记录重要管理事件、任务错误、耗时和模型连接状态。

### 4.2 模块依赖规则

- API 路由负责解析请求、认证、权限检查和调用服务，不放入大量业务逻辑。
- Task Manager 不自行拼接复杂提示词，调用 Context Builder。
- Inference Adapter 不决定用户权限、不操作会话归属、不直接写用户记忆。
- Memory Service 不得更改原始消息正文。
- 前端不得直接访问 PostgreSQL、llama-server、内部任务注册表或服务器文件。
- 不允许模块通过全局可变变量共享业务状态；任务运行时状态必须集中到 Task Manager。
- 数据库访问逻辑应通过服务/仓储层组织，避免路由文件中出现大量 SQL 细节。

### 4.3 建议目录结构

目录结构可按现有项目调整，但职责应保持清晰。

```text
project-root/
  backend/
    app/
      main.py
      core/
        config.py
        security.py
        errors.py
        logging.py
      api/v1/
        auth.py
        users.py
        conversations.py
        messages.py
        tasks.py
        settings.py
        health.py
      db/
        base.py
        session.py
        models/
        migrations/
      schemas/
      services/
        auth_service.py
        user_service.py
        conversation_service.py
        message_service.py
        task_manager.py
        context_builder.py
        memory_service.py
        settings_service.py
      inference/
        base.py
        llama_server.py
        stream_parser.py
      tests/
    alembic.ini
    pyproject.toml
  frontend/
    src/
      api/
      components/
      layouts/
      pages/
      stores/
      router/
      types/
      utils/
    package.json
  docs/
  .env.example
  .gitignore
  README.md
```

不要机械地为了满足目录结构而拆成过多空文件；以清晰职责为目标。

---

## 5. 核心数据模型

### 5.1 总体关系

- 一个 `User` 拥有多个 `Conversation`。
- 一个 `Conversation` 拥有多条 `Message`。
- 一条用户消息可以发起一个或多个历史 `GenerationTask`，以支持重试/重新生成；但同一时间一个会话只能有一个未终结任务。
- 一个 `GenerationTask` 关联一条用户消息和一条助手消息。
- 一个会话最多有一条当前摘要，存在 `conversation_memories`，可通过版本号更新。
- 一个会话可以有多条关键决策。
- 一个用户可以有多条跨会话偏好。
- 登录会话属于一个用户，服务端只保存会话令牌的哈希。

所有 ID 使用 UUID 或等价的随机唯一 ID；不要使用递增数组下标充当资源标识。时间统一以 UTC 保存，API 返回 ISO 8601 时间；前端转换为本地时间显示。

### 5.2 `users`

字段：

- `id`：主键；
- `username`：唯一，规范化后比较大小写；
- `password_hash`：Argon2id 哈希；
- `role`：`admin` 或 `user`；
- `is_active`：是否启用。接口通过派生字段 `status` 返回 `active` 或 `disabled`，不单独存状态列；
- `created_at`、`updated_at`；
- `last_login_at`；
- `password_changed_at`。

规则：

- 不保存明文密码；
- 不允许公开注册；
- 至少有一个活动管理员；
- 不允许管理员通过 UI 禁用或删除最后一个活动管理员；
- 禁用用户后立即撤销其所有登录会话，并取消其活动任务；
- 禁用用户不会自动删除其历史聊天数据；
- 不建议物理删除用户，优先禁用并保留数据关联完整性。

### 5.3 `auth_sessions`

字段：

- `id`；
- `user_id`；
- `token_hash`：唯一。库里只存会话令牌哈希；
- `created_at`、`last_seen_at`、`expires_at`；
- `revoked_at`；
- `user_agent`：长度受限的浏览器标识，不用于唯一身份识别。

CSRF 不入库。写操作使用由会话 ID 和密钥算出的 HMAC，通过 `X-CSRF-Token` 提交。

规则：

- 浏览器获得高熵随机会话令牌，数据库只保存令牌哈希；
- Cookie 使用 `HttpOnly`、合适的 `SameSite`；HTTPS 部署时必须设置 `Secure`；
- 写操作应进行 CSRF 防护，至少验证允许的 `Origin` 并要求自定义 CSRF Header/Token；
- 登录退出、密码重置、用户禁用时撤销相应会话；
- 建议默认空闲超时 12 小时、绝对过期 7 天，均应可配置；
- 不把登录会话令牌写入日志或前端持久化存储。

### 5.4 `conversations`

字段：

- `id`；
- `user_id`：所属用户。接口里的 `owner_user_id` 是它的别名，不是单独的列；
- `title`；
- `status`：`active` 或 `archived`。接口里的 `archived_at` 在已归档时取 `updated_at`，否则为空；
- `next_message_seq`：下一条消息要用的序号；
- `created_at`、`updated_at`、`last_message_at`。

不使用 `deleted_at`。删除会话是物理删除。接口里的 `version` 固定返回 `1`，当前不作为乐观锁。

规则：

- 创建会话时标题可先使用“新对话”；
- 用户发出第一条有效消息后，可根据其内容生成简单标题；标题生成失败不影响聊天；
- 用户可重命名和归档。归档后不能继续发送，需要先取消归档；
- 删除必须通过二次确认。删除前取消活动任务；若无法确认任务已终止，应拒绝删除并提示稍后重试；
- 删除会话后，级联删除消息、摘要、决策、生成任务和事件；审计日志只保留会话 ID，不保留正文；
- 普通用户只能操作自己的会话。

### 5.5 `messages`

字段：

- `id`；
- `conversation_id`；
- `sequence_no`：在会话内单调递增，由 `conversations.next_message_seq` 分配；
- `role`：只允许 `user` 和 `assistant`；
- `content`：消息正文；
- `status`：`completed`、`pending`、`streaming`、`interrupted`、`cancelled`、`failed`。接口把 `streaming` 显示为 `generating`；
- `reply_to_message_id`：助手消息指回同一会话里的用户消息，可空；
- `created_at`、`updated_at`。

消息表不存 `task_id`、`owner_user_id`、token 数和 `error_code`，避免和 `generation_tasks` 循环引用。接口响应可以带上关联任务的 ID 和错误码；token 用量记在任务上。

规则：

- 每条用户消息独立持久化；
- 创建任务时同时创建关联的助手占位消息；
- 助手消息正文在生成过程中累积，按检查点定期保存；
- 正常结束时保存最终完整正文并设为 `completed`；
- 用户取消且模型停止时，保存已生成的部分正文并设为 `cancelled`；
- 生成失败时，已生成的部分正文可保留，状态设为 `failed`；
- 后端重启时无法恢复的任务，其助手消息设为 `interrupted`；
- 已完成消息正文不能被迟到的旧流事件覆盖；
- 消息顺序由后端生成，客户端不能自行指定 `sequence_no`。

### 5.6 `generation_tasks`

字段：

- `id`；
- `user_id`、`conversation_id`。接口里的 `owner_user_id` 是 `user_id` 的别名；外键同时约束 `conversation_id` 与 `user_id`，保证任务属于该会话的拥有者；
- `user_message_id`、`assistant_message_id`：都限定在同一会话；
- `idempotency_key`：前端首次提交时生成，同一用户内唯一。接口里叫 `client_request_id`；
- `request_fingerprint`：用来识别同一个幂等键是否对应不同正文；
- `retry_of_task_id`：这次重试的来源任务，可空；
- `status`：见第 9 节；
- `model_name`；
- `generation_params`：JSONB，已校验、允许公开记录的参数；不得包含 API Key；
- `created_at`、`started_at`、`finished_at`、`updated_at`；
- `heartbeat_at`、`disconnect_deadline_at`；
- `cancel_requested_at`、`cancel_reason`；
- `prompt_tokens`、`completion_tokens`、`queue_wait_ms`、`generation_duration_ms`（可空）；
- `error_code`、`error_message`（可空）。接口里的安全错误文案来自 `error_message`。

摘要维护不写入这张表，避免造出虚假的用户消息和助手消息。

规则：

- 同一用户的 `idempotency_key` 必须唯一；重复提交相同 ID 时返回原任务，不能再次生成；
- 同一个幂等 ID 对应不同正文时返回 `409 IDEMPOTENCY_KEY_CONFLICT`；
- 任务状态只能通过 Task Manager 转换；
- 任务状态变更和对应消息状态应以事务或可恢复的操作顺序更新；
- 任务日志不应保存完整系统提示词或秘密配置；必要的 prompt 调试必须是显式的管理员开发模式，默认关闭。

### 5.7 `conversation_memories`

每个会话一行。

字段：

- `conversation_id`：主键；
- `summary`：摘要正文；
- `summary_version`：版本号；
- `covered_through_sequence`：摘要已经覆盖到的消息序号；
- `updated_at`。

规则：

- 摘要是压缩理解，不替代原始消息；
- 更新摘要时要保存覆盖范围；
- 摘要生成失败不影响正常聊天；
- 不得把一个会话的摘要用于其他会话；
- 摘要更新不得覆盖比它更新的版本；
- 摘要走同一套推理槽位，但不是 `generation_tasks` 里的一行，也不受「10 秒内没有浏览器订阅就取消」这条规则约束；
- 不得旁路全局推理并发限制。

### 5.8 `conversation_decisions`

字段：

- `id`；
- `conversation_id`；
- `title`；
- `content`；
- `source_message_id`：可空。来源消息被删除时置空，决定本身保留；
- `status`：`active`、`superseded` 或 `archived`；
- `created_at`、`updated_at`。

不单独存 `confirmed_by_user`。决定由用户写入和维护，接口上该字段恒为已确认。模型不会把猜测写成已确认决定。

规则：

- 记录已确定方案、限制、重要结论和待办，不保存每句普通对话；
- 记录来源消息，便于追溯；
- 新决策与旧决策冲突时，不直接无痕覆盖，应将旧决策标记为 `superseded`；
- 不确定的模型推断不能直接升级为已确认决策；
- 用户可以查看、编辑和删除决策；
- 只向当前会话的提示词注入该会话有效决策。

### 5.9 `user_preferences`

字段：

- `id`；
- `user_id`；
- `key`：同一用户内唯一；
- `value`：JSONB；
- `source`：`user` 或 `assistant_suggested`。接口上的 `confirmed_by_user` 在来源为 `user` 时为真；
- `is_active`：停用时关掉这一列，不删除记录。接口上的 `status` 对应 `active` 或 `inactive`；
- `created_at`、`updated_at`。

规则：

- 仅保存跨会话有价值的稳定偏好；
- 会话中的临时项目选择不能自动变成全局用户偏好；
- 对模型推断出的偏好，优先标记为候选或要求用户确认；
- 用户能够查看、修改和删除自己的偏好；
- 仅注入与当前任务相关、长度受控的偏好。

### 5.10 `generation_settings`

全局生成配置，整张表只有一行，主键固定为 `1`。

字段：

- `model_name`、`temperature`、`top_p`、`max_tokens`；
- `top_k`、`repeat_penalty`：可空；
- `system_prompt`：管理端维护的系统提示词；
- `updated_by`：最后修改的用户，该用户被删除后置空；
- `updated_at`。

机密配置（API Key、数据库连接串）不进这张表，只从环境变量读取。环境变量里的 `LLM_SYSTEM_PROMPT` 是初始默认值；管理端保存后以这张表为准。

### 5.11 `audit_logs` 和 `generation_task_events`

`audit_logs` 记录管理员创建/禁用用户、重置密码、登录安全事件、设置更改和会话删除等操作。字段包括自增 `id`、`actor_user_id`、`action`、`target_type`、`target_id`、`details`、`ip_address`、`created_at`。操作人被删除后日志保留，`actor_user_id` 置空。默认不记录消息正文、密码、Cookie 或 API Key。

`generation_task_events` 是某次生成的事件流，主键是 `task_id` 加 `sequence_no`。`event_type` 为 `delta`、`status`、`error` 或 `done`。`delta_text` 保存这一小段新增文本，`payload` 保存其余 JSON。刷新或重连时靠这些事件补回已经吐出的内容。任务删除时事件一起删除。不要为每个 token 再另建消息行。

### 5.12 数据库约束和索引

至少包含：

- `users.username` 唯一；`role` 只能是 `admin` 或 `user`；
- `auth_sessions.token_hash` 唯一；按 `(user_id, expires_at)` 查询；
- `conversations` 上 `(id, user_id)` 唯一，供其他表做复合外键；按 `(user_id, status, updated_at)` 和 `(user_id, last_message_at)` 查询；
- `messages(conversation_id, sequence_no)` 唯一；`(id, conversation_id)` 唯一，供回复和任务引用；
- `generation_tasks(user_id, idempotency_key)` 唯一；同一会话在 `queued`、`running`、`cancel_requested` 上最多一行；
- `user_preferences(user_id, key)` 唯一；
- `generation_settings.id` 必须为 `1`；
- 所有外键启用相应约束。用户删除级联其会话、消息、偏好和登录会话。会话删除级联消息、任务、事件、摘要和决定。

数据库使用迁移工具维护 schema。生产运行不要自动删除数据库或在启动时重建表。

---

## 6. 用户角色和权限矩阵

| 功能 | 管理员 | 普通用户 |
|---|---|---|
| 登录/退出/查看自己的资料 | 是 | 是 |
| 创建、禁用用户 | 是 | 否 |
| 重置其他用户密码 | 是 | 否 |
| 查看用户列表 | 是 | 否 |
| 查看自己的会话和消息 | 是 | 是 |
| 查看他人的聊天正文 | 默认否 | 否 |
| 创建/重命名/归档/删除自己的会话 | 是 | 是 |
| 操作自己的推理任务 | 是 | 是 |
| 取消他人的推理任务 | 可以允许用于系统维护，但不可借此读取正文 | 否 |
| 更改全局模型生成配置 | 是 | 否 |
| 查看系统健康状态 | 是 | 否或只展示简化状态 |
| 管理自己的长期偏好 | 是 | 是 |
| 管理他人的长期偏好 | 默认否 | 否 |

所有 API 必须验证认证身份、角色、资源归属和资源状态。管理员访问普通用户数据的权限应明确授权，不应把管理员角色当成无条件读取所有聊天内容的许可。

---

## 7. 页面与交互设计

### 7.1 登录页

功能：

- 用户名、密码输入；
- 登录中状态；
- 通用错误提示，避免通过错误文案暴露用户名是否存在；
- 登录失败限流；
- 登录成功后进入聊天页；
- 登录会话失效时跳转登录页，保留必要的安全返回路径。

不包含：注册、忘记密码邮件、自助找回账号。密码重置由管理员执行。

### 7.2 主聊天页

建议布局：

- 左侧：新建聊天、会话搜索、会话列表、归档入口；
- 中央：消息列表、Markdown 渲染、代码块、复制、生成状态、错误状态；
- 底部：多行输入框、发送按钮、停止按钮；
- 顶部或用户菜单：当前用户、设置、记忆管理、退出登录。

交互规则：

1. 发送按钮按当前状态显示；提交后先显示用户消息，再显示待生成的助手占位消息。
2. 生成期间允许停止，不允许同一会话再提交第二个生成任务；可禁用发送按钮并显示原因。
3. 切换到另一个会话时，旧任务按产品策略继续处于恢复窗口；UI 不应误将它归属到新会话。
4. 浏览器刷新后，从后端重新加载会话和消息，不依赖只存在于前端内存的数据。
5. 流式文本必须按序追加；接收 `snapshot` 时以快照全文替换本地对应助手消息，避免重复拼接。
6. 显示停止、已取消、失败、中断和完成等不同状态。
7. 网络断线后显示“正在恢复连接”，不立即创建新任务；恢复失败或超时后刷新任务状态。
8. Markdown 渲染必须防 XSS；不允许未经净化地渲染模型输出中的任意 HTML。
9. 代码块支持复制；复制失败时提示用户；页面不因长代码撑破布局。
10. 错误消息应使用可理解的用户文案，详细堆栈仅写服务端日志。

### 7.3 会话管理

支持：

- 新建会话；
- 列出当前用户的会话；
- 搜索标题；
- 重命名；
- 归档与取消归档；
- 查看历史消息；
- 删除（明确确认）。

默认按 `last_message_at` 倒序排列。归档会话默认不显示在主列表，但可通过归档过滤查看。分页或游标方式加载历史，不应一次加载用户全部消息正文。

### 7.4 记忆管理页或抽屉

至少提供：

- 查看当前会话摘要；
- 查看、编辑、删除当前会话关键决策；
- 查看、编辑、删除用户级偏好；
- 显示摘要覆盖到的消息范围/更新时间；
- 记忆更新失败时提示状态，不影响聊天历史。

V1 不需要展示每次模型选择上下文的全部内部细节，但应提供足够信息帮助用户发现错误记忆并修正。

### 7.5 管理后台

#### 用户管理

- 用户列表、搜索、角色、状态、创建时间和最近登录时间；
- 创建用户；
- 设置初始密码或生成一次性初始密码；
- 禁用/启用用户；
- 重置密码；
- 撤销该用户所有登录会话；
- 禁止把密码明文存入数据库或日志；
- 管理操作必须记录审计事件。

#### 系统状态

显示：

- 后端运行状态；
- 数据库连接状态；
- llama-server 连接状态；
- 当前可识别的模型名称（不显示密钥）；
- 活跃生成任务数、排队任务数；
- 最近失败任务的摘要信息（不显示秘密信息）。

#### 模型参数设置

V1 只开放经过后端校验的白名单参数，例如 `temperature`、`top_p`、`max_tokens`、`presence_penalty`、`frequency_penalty`（仅在当前服务支持时启用）。设置范围必须在服务端验证，不能接受任意 JSON 参数直接透传。

`LLM_BASE_URL`、`LLM_API_KEY` 等敏感连接信息只通过环境变量或服务器秘密管理，不提供在普通 UI 中读取明文密钥的功能。

### 7.6 响应式与可访问性

- 桌面是主要使用场景；窄屏时会话侧栏可折叠；
- 键盘可完成发送、停止、切换输入焦点等常用操作；
- 按钮需有明确的可访问名称；
- 加载中、失败、断线状态不能只通过颜色表达；
- 不依赖浏览器刷新才能恢复任务状态。

---

## 8. 聊天请求与模型上下文处理

### 8.1 用户发送消息的业务流程

1. 前端创建稳定的 `client_request_id`，在请求重试时继续使用同一个 ID。
2. 后端认证用户，并验证会话归属、请求正文长度和会话状态。
3. 在一个数据库事务中：创建用户消息、创建助手占位消息、创建生成任务、更新会话活动时间。
4. 返回 `task_id`、用户消息 ID、助手消息 ID 和初始状态。
5. Task Manager 将任务加入有界队列。
6. 前端使用任务 ID 打开事件流。
7. Context Builder 查询合适的摘要、决策、偏好和最近历史。
8. Inference Adapter 调用 llama-server。
9. 后端接收模型数据、解析最终回答文本，向订阅者发送 `delta` 事件。
10. 以受控频率检查点保存助手消息正文，生成结束后保存最终正文和终态。
11. 更新会话最后活动时间和可用的 token/耗时统计。
12. 需要更新摘要时，安排一次不写入 `generation_tasks` 的摘要整理；维护失败不能使本轮回答失败。决策不由模型自动写入。

### 8.2 上下文组装规则

输入模型的消息由 Context Builder 统一组装，至少包括：

1. 全局系统提示词；
2. 适用的用户偏好（仅相关且长度有限）；
3. 当前会话摘要；
4. 当前会话有效关键决策；
5. 最近历史消息；
6. 当前用户消息。

约束：

- 不把所有历史无上限地发送给模型；
- 先保留系统提示词、当前用户消息和核心约束，再按需要添加记忆与近期历史；
- 超长时优先裁剪最旧的完整消息对，避免只留下没有上下文的助手答复；
- 不截断当前用户消息；如果用户消息自身超出允许长度，明确拒绝并提示；
- 当前没有接入 tokenizer。上下文按大约 2 个字符折算 1 个 token，故意偏保守，并保留配置中的安全预算；
- 提示词预算不得超过配置中的上下文窗口扣除生成预留后的上限；
- 应预留最大输出 token 数和安全余量；
- 每轮提示词中不要自动插入其他会话的摘要或项目决策；
- 不默认把模型的 `reasoning_content` 写进助手可见正文或长期记忆。

### 8.3 摘要生成策略

建议通过低优先级维护任务更新摘要，而不是每发一条消息就额外调用一次模型。

可配置的触发条件：

- 新消息数超过阈值；
- 摘要覆盖范围落后于当前会话较多；
- 上下文长度接近限制；
- 用户明确请求整理/更新摘要。

默认初值可以是每累计 12 条新消息或接近上下文预算时触发，但该值应配置化并以实际使用情况调整。

摘要整理占用同一个推理槽位，但不是 `generation_tasks` 的一行，也不会因为没有浏览器订阅而被 10 秒初始超时取消。它应：

- 指明本次摘要覆盖到的消息序号；
- 从上一个摘要和未覆盖的新消息生成更新；
- 不因旧的摘要任务晚到而覆盖新版本；
- 保存成功后再更新缓存；
- 失败后保留旧摘要并允许重试；
- 不自动把模型的猜测写成确定事实。

### 8.4 关键决策提取

决定由用户维护。模型不会把猜测写成已确认决定。

- 每条决策尽量绑定源消息；
- 更新与历史决策冲突时保留替代关系，旧决定标为 `superseded`，也可以归档为 `archived`；
- 仅把当前有效且相关的决策放入上下文。

### 8.5 用户偏好

- 只有跨会话稳定且有价值的信息才作为用户偏好；
- 例如回答语言、希望的详略程度、长期工作流；
- 不把某个项目的技术选型自动当成用户的全局偏好；
- 明确的用户修改应更新/取代旧值；
- 用户可以关闭或删除偏好；
- 不把敏感个人信息作为默认长期记忆。若业务将来需要处理敏感记忆，应单独设计权限、明示和删除机制。

### 8.6 输出处理

- 保存模型返回的正常回答文本；
- 正常处理 `data: ...` SSE 块、结束标记、空 delta 和非文本控制块；
- 对格式不正确的单个块应记录受控错误并尽量避免崩溃；
- 不把 SSE 包装内容直接展示在聊天中；
- 不默认暴露 `reasoning_content`；
- V1 不执行工具调用，即使模型返回 `tool_calls` 也不能擅自执行外部动作；应显示受控的不支持错误或按适配器策略安全结束；
- 不通过简单正则任意删除模型正文内容。Markdown 渲染净化与业务层文本改写必须分开。

---

## 9. 推理任务、状态机和并发控制

### 9.1 任务状态

持久化状态：

- `queued`：已接受，等待资源/执行；
- `running`：上游推理正在执行；
- `cancel_requested`：已发出取消请求，尚未确认上游停止；
- `completed`：正常完成；
- `cancelled`：已确认终止；
- `failed`：执行失败；
- `interrupted`：后端退出或重启后无法继续原任务；
- `rejected`：由于权限、容量或限流规则未接受执行。

`completed`、`cancelled`、`failed`、`interrupted`、`rejected` 是终态。终态不得被迟到的 token 事件改写。

### 9.2 允许的基本状态转换

- `queued -> running`；
- `queued -> cancelled`（尚未启动时可以直接取消）；
- `queued -> rejected`；
- `running -> completed`；
- `running -> cancel_requested`；
- `running -> failed`；
- `running -> interrupted`（只由启动恢复/崩溃恢复逻辑产生）；
- `cancel_requested -> cancelled`（确认结束后）；
- `cancel_requested -> failed`（取消失败或上游异常，且任务清理已完成）；
- `cancel_requested -> interrupted`（进程退出后无法确认原上游请求）。

禁止任意终态之间转换。状态转换由 Task Manager 的单一入口执行，并记录状态变更事件。

### 9.3 幂等和并发竞态

- 每个发送请求必须带 `client_request_id`；
- 相同用户和相同 ID 重试时返回同一个用户消息/任务，不得再次发起推理；
- 同一会话有未终结任务时，新的发送请求返回 `409 CONVERSATION_BUSY`；
- 创建任务、关联消息与占用会话活动任务的操作必须通过事务或可靠的唯一约束防止竞态；
- 多个停止请求应幂等；
- 删除会话、禁用用户和取消任务同时发生时，应有明确的锁/状态检查和结果；
- 所有资源额度只能释放一次，使用明确的任务最终清理逻辑，避免 semaphore 泄漏或重复释放。

### 9.4 默认并发与队列策略

采用配置项，不散落写死在业务代码中。建议初始值：

- `MAX_ACTIVE_GENERATIONS=1`：默认一次只允许一个活动模型生成；
- `MAX_QUEUE_SIZE=20`；
- `MAX_QUEUED_TASKS_PER_USER=3`；
- `MAX_ACTIVE_TASKS_PER_USER=1`；
- `RECONNECT_GRACE_SECONDS=30`；
- `INITIAL_STREAM_ATTACH_TIMEOUT_SECONDS=10`；
- `OUTPUT_CHECKPOINT_INTERVAL_SECONDS=1`；
- `OUTPUT_CHECKPOINT_MIN_CHARS=1024`；
- `UPSTREAM_CONNECT_TIMEOUT_SECONDS=10`；
- `UPSTREAM_READ_TIMEOUT_SECONDS` 必须适合长时间生成，不得使用过短的普通请求超时；
- `CANCEL_CONFIRM_TIMEOUT_SECONDS` 必须配置，并通过实际 llama-server 测试调整。

这些只是安全的初始配置建议，不代表 GPU 已验证支持这些吞吐量。所有上限都应可调且在启动时打印不含秘密的生效配置摘要。

队列满时返回明确错误，不允许无界堆积。模型维护任务（摘要等）优先级低于用户聊天任务，不得绕过全局推理并发限制。

### 9.5 断线恢复机制

浏览器和后端的连接是任务的订阅关系；连接断开本身不等于用户取消任务。

规则：

1. 前端创建任务后立即建立事件流订阅。
2. 如果聊天任务从未建立过事件流，在 `INITIAL_STREAM_ATTACH_TIMEOUT_SECONDS` 到期后停止/取消该任务，避免没有前端观察者的请求长期占用 GPU。摘要整理不是这条规则的对象。
3. 已连接客户端断线时，记下 `disconnect_deadline = now + RECONNECT_GRACE_SECONDS`。
4. 如果期限内重新订阅，清除断线期限并恢复相同的任务 ID；不得重新调用模型。
5. 如果期限内没有重新连接，则请求取消任务。
6. 若任务已完成，重连直接返回最终快照和终态。
7. 若用户主动点击停止，立即发起取消，不使用断线恢复宽限期。
8. 只在确认上游流已结束、任务已清理或尚未启动时，标记 `cancelled` 并释放活动推理槽位。
9. 取消无法及时确认时，保持 `cancel_requested` 状态并继续清理/监控；不得虚报已取消。
10. 进程重启后，活动生成不能被假设为可续跑。将原 `running`/`cancel_requested` 任务标记为 `interrupted`，保存可用的部分输出并让用户选择重试。尚未调用模型的 `queued` 任务继续执行，不标记为中断，也不当成新的一次生成。

### 9.6 输出检查点与快照

增量文本写入 `generation_task_events`，供刷新和重连补回；不要为每个 token 再另建消息行。助手正文仍按检查点写入 `messages`。

- 运行时内存保存当前完整输出、单调递增的 `seq` 和短期订阅缓冲；
- 每一段新增文本作为 `delta` 事件落库，并带上 `output_seq`；
- 满足时间间隔或新增字符数阈值时，将当前助手正文保存为检查点；
- 任务完成、取消、失败时立即保存最终可用输出；
- 同进程重新连接时，从 Task Manager 原子地获取当前完整输出快照与 `seq`，再订阅后续 `seq` 更大的 delta；
- 前端收到 snapshot 时替换当前正文，收到 delta 时仅处理比本地最后序号更新的事件；
- 进程重启后，`running` 和 `cancel_requested` 标为 `interrupted`，已落库的事件和最后一个检查点仍然可读；`queued` 继续执行；
- 快照和订阅注册必须避免竞态，不能在获取快照和注册订阅的空隙丢失新 delta。

### 9.7 llama-server 取消风险

不得直接假定取消应用侧的 `httpx` 请求一定停止 llama-server 内部的 GPU 生成。当前安装的 llama-server 版本必须进行实验验证。

要求：

- 将上游请求取消封装在 Inference Adapter；
- 检查关闭响应流后上游是否停止生成、是否释放并行槽位；
- 若无法确认模型已经停止，应继续持有调度额度直到上游结束，或者采用经过验证的专用控制机制；
- 不允许为追求 UI 上立即显示“已停止”，提前释放推理容量；
- 将实际取消行为、等待时间和失败情况写入集成测试及运维文档。

---

## 10. 流式 API 事件协议

### 10.1 推荐协议

`GET /api/v1/tasks/{task_id}/events` 返回 `text/event-stream`。建议事件类型：

#### `snapshot`

在每次订阅开始时发送，正文为任务当前完整输出快照：

```json
{
  "task_id": "uuid",
  "status": "running",
  "output_text": "当前完整的助手回答",
  "output_seq": 25,
  "terminal": false
}
```

前端必须用 `output_text` 替换当前任务的助手正文，而不是追加。

#### `delta`

```json
{
  "task_id": "uuid",
  "seq": 26,
  "text": "新增文本"
}
```

前端仅当 `seq` 大于最后处理序号时追加；重复或过期事件应忽略。

#### `status`

```json
{
  "task_id": "uuid",
  "status": "queued|running|cancel_requested"
}
```

#### 终态事件

事件名按终态确定：`completed`、`cancelled`、`failed` 或 `interrupted`。数据至少包含任务 ID、终态、助手消息 ID、错误代码（如有）和最终输出序号。

#### `heartbeat`

定期发送心跳，避免代理或连接层把长时间无 token 输出的请求当成死连接。建议初始间隔 15 秒，可配置。

### 10.2 SSE 传输要求

- 设置 `Content-Type: text/event-stream`；
- 设置 `Cache-Control: no-cache`；
- 反向代理环境下关闭该路径的响应缓冲，例如适用时设置 `X-Accel-Buffering: no`；
- 事件流中不得包含秘密配置；
- 结束时发送明确终态事件并关闭流；
- 客户端重连必须使用相同 `task_id`；
- 事件流端点要检查任务归属；
- 不允许用户凭任务 ID 访问他人的事件流；
- 若任务为终态，不要一直保持空连接，应返回快照、终态事件后结束。

---

## 11. API 设计

统一前缀：`/api/v1`。JSON 请求与响应使用 UTF-8。ID 使用 UUID。返回统一错误结构，详见第 12 节。

### 11.1 认证

#### `POST /auth/login`

请求：`username`、`password`。  
结果：设置安全 Cookie，返回当前用户的公开资料。  
失败：统一返回登录失败文案，并限流。

#### `POST /auth/logout`

撤销当前登录会话并清除 Cookie。

#### `GET /auth/me`

返回 `id`、`username`、`role`、`status`、安全偏好的公开信息；不能返回密码哈希或会话令牌。

#### `GET /auth/csrf`

如采用独立 CSRF Token 机制，返回当前会话对应的 CSRF Token；写操作必须按约定带入请求 Header。

### 11.2 管理员用户管理

- `GET /admin/users`：分页、搜索、筛选状态；
- `POST /admin/users`：创建用户；
- `GET /admin/users/{user_id}`：用户基本资料，不含敏感认证数据；
- `PATCH /admin/users/{user_id}`：修改允许字段，如角色、状态；
- `POST /admin/users/{user_id}/reset-password`：重置密码；
- `POST /admin/users/{user_id}/revoke-sessions`：撤销所有登录会话。

所有接口必须管理员权限。创建/重置密码时，初始密码应只返回一次或由管理员安全设置；不能在后续列表接口中读回。

### 11.3 会话

- `GET /conversations`：分页列表，可按归档状态筛选；
- `POST /conversations`：创建会话；
- `GET /conversations/{conversation_id}`：读取会话元数据；
- `PATCH /conversations/{conversation_id}`：重命名、归档、取消归档；
- `DELETE /conversations/{conversation_id}`：删除前处理活动任务，确认安全后删除相关内容；
- `GET /conversations/{conversation_id}/messages`：游标分页读取消息，可按 `before_sequence_no` 读取更早记录。

### 11.4 消息和任务创建

#### `POST /conversations/{conversation_id}/messages`

请求示例：

```json
{
  "content": "用户输入内容",
  "client_request_id": "uuid",
  "generation_params": {
    "temperature": 0.7,
    "top_p": 0.9,
    "max_tokens": 2048
  }
}
```

要求：

- `content` 去除首尾纯空白后不能为空；
- 限制正文长度；
- `client_request_id` 必填；
- `generation_params` 可选，只接受白名单字段和有效范围；
- 普通用户不能指定任意 `model`、`base_url`、API Key 或任意上游 URL；
- 成功时返回用户消息、助手占位消息及任务的 ID 与状态；
- 重复幂等请求返回原有资源；
- 会话存在活动任务时返回 `409 CONVERSATION_BUSY`；
- 队列满时返回 `429` 或明确的 `503` 业务错误，不生成重复消息。

响应示例：

```json
{
  "conversation_id": "uuid",
  "user_message_id": "uuid",
  "assistant_message_id": "uuid",
  "task_id": "uuid",
  "task_status": "queued"
}
```

### 11.5 任务

- `GET /tasks/{task_id}`：获取任务状态和安全的进度信息；
- `GET /tasks/{task_id}/events`：SSE 流及重连；
- `POST /tasks/{task_id}/cancel`：主动取消；
- `POST /tasks/{task_id}/retry`：对失败、中断或已取消的尝试发起新任务；
- `POST /tasks/{task_id}/regenerate`：可选，基于同一用户消息重新生成一条新助手回答。

重试与重新生成必须创建新的任务记录和新的助手消息记录，不能重用已终结任务 ID。需要确保同一会话同一时刻只有一个活动任务。

### 11.6 记忆

- `GET /conversations/{conversation_id}/memory`：读取当前会话摘要和决策；
- `PATCH /conversations/{conversation_id}/memory/summary`：用户手动编辑摘要；
- `POST /conversations/{conversation_id}/memory/summary/refresh`：请求立即整理摘要；
- `POST /conversations/{conversation_id}/decisions`：新增或确认决策；
- `PATCH /conversations/{conversation_id}/decisions/{decision_id}`：编辑/标记过期；
- `DELETE /conversations/{conversation_id}/decisions/{decision_id}`：删除决策；
- `GET /user/preferences`：读取当前用户偏好；
- `POST /user/preferences`：新增当前用户偏好；
- `PATCH /user/preferences/{preference_id}`：编辑偏好；
- `DELETE /user/preferences/{preference_id}`：删除偏好。

所有记忆接口必须验证资源归属。

### 11.7 设置与健康

- `GET /admin/settings/generation`：读取已公开的生成默认值；
- `PATCH /admin/settings/generation`：更新经验证的生成参数；
- `GET /admin/system/health`：检查后端、数据库和 llama-server 状态；
- `POST /admin/system/llm-test`：执行轻量连通性检查，不应消耗过多生成资源。

健康接口不得返回密钥、环境变量全集、绝对服务器敏感路径或数据库凭证。

---

## 12. API 错误格式与错误代码

统一 JSON 错误结构：

```json
{
  "error": {
    "code": "CONVERSATION_BUSY",
    "message": "当前会话已有生成任务正在执行。",
    "request_id": "uuid"
  }
}
```

建议至少覆盖：

- `AUTH_REQUIRED`：未登录或会话过期；
- `PERMISSION_DENIED`：权限不足；
- `RESOURCE_NOT_FOUND`：资源不存在或对当前用户不可见；
- `VALIDATION_ERROR`：参数无效；
- `IDEMPOTENCY_KEY_CONFLICT`：幂等 ID 与既有请求正文冲突；
- `CONVERSATION_BUSY`：会话有活动任务；
- `QUEUE_FULL`：任务队列已满；
- `RATE_LIMITED`：触发限流；
- `TASK_NOT_CANCELLABLE`：任务已终结或不允许取消；
- `LLM_UNAVAILABLE`：模型服务不可用；
- `LLM_TIMEOUT`：上游超时；
- `LLM_BAD_RESPONSE`：上游响应格式无效；
- `CANCEL_UNCONFIRMED`：取消已请求但暂未确认；
- `INTERNAL_ERROR`：未预期的内部错误。

规则：

- 返回给用户的 `message` 不包含堆栈、API Key 或内部路径；
- 详细异常仅记录在服务端受控日志；
- 根据语义选择 HTTP 状态码：401、403、404、409、422、429、502、503、504 等；
- 为请求分配 request ID，方便日志关联。

---

## 13. 安全设计

### 13.1 网络与模型凭证

- 浏览器只访问 Web 应用，不直接请求 llama-server；
- llama-server 尽可能绑定 `127.0.0.1` 或受防火墙保护的内部地址；
- 不应让公网用户绕过应用层直接访问模型端口；
- `LLM_API_KEY` 只能保存在后端环境变量/服务器秘密中；
- 已暴露过的 API Key 应视为失效风险并轮换；
- 公网访问应使用 HTTPS 或受控 VPN/访问网关；
- CORS 只允许实际前端来源；若同源部署，不要开放通配符；
- 反向代理要正确支持 SSE 并关闭相关路径的响应缓冲。

### 13.2 认证与授权

- 密码仅保存 Argon2id 哈希；
- 对登录接口限速，并记录失败次数；
- 会话 Cookie 使用 `HttpOnly`、`SameSite`，HTTPS 下设置 `Secure`；
- 写请求有 CSRF 防护和 Origin 校验；
- 用户被禁用或密码重置后撤销旧会话；
- 服务端每次请求都重新验证当前用户和资源归属；
- 不能信任客户端提交的 `owner_user_id`、role 或 conversation ID。

### 13.3 输入和输出

- 对用户名、消息正文、标题、摘要、决策等字段限制长度；
- 防止 SQL 注入，使用 ORM 参数化查询；
- Markdown HTML 需要禁用或净化；
- 不允许模型输出自动执行 JavaScript、Shell、SQL 或外部工具；
- V1 不开放任意 URL 的服务器端抓取，以避免 SSRF；
- 上传文件功能暂不实现；
- 日志不记录密码、Cookie、会话令牌、API Key 或完整敏感 prompt。

### 13.4 限流与资源限制

- 登录接口有独立限流；
- 消息提交按用户限速；
- 队列有全局和用户级上限；
- 消息正文、参数 JSON、请求体大小受限；
- 单任务设最大生成 token 和最长运行时间；
- 进程内缓存有 TTL 和最大容量，避免无限增长；
- 数据库查询分页，避免一次性返回全部历史。

### 13.5 管理员安全

- 首个管理员通过命令行初始化，不使用固定默认账号密码；
- 管理员操作写入审计日志；
- 不可禁用最后一个活动管理员；
- 管理界面不显示密码哈希、Cookie 或 API Key；
- 具有管理员角色不等于默认允许读取所有用户聊天内容。

---

## 14. 配置管理

所有部署配置应集中定义，并提供 `.env.example`。以下为建议项，不要把示例秘密当成真实生产值。

### 14.1 应用配置

- `APP_ENV`：development / production；
- `APP_HOST`、`APP_PORT`；
- `APP_SECRET_KEY`：随机高熵值，用于必要的安全签名；
- `DATABASE_URL`：PostgreSQL 连接串，例如 `postgresql+psycopg://user:password@127.0.0.1:5432/llmAppDb`；
- `LOG_LEVEL`；
- `PUBLIC_BASE_URL`；
- `ALLOWED_ORIGINS`；
- `SESSION_IDLE_TIMEOUT_SECONDS`；
- `SESSION_ABSOLUTE_TIMEOUT_SECONDS`。

### 14.2 llama-server 配置

- `LLM_BASE_URL`：如 `http://127.0.0.1:8080/v1`；
- `LLM_API_KEY`：服务器端私密配置；若服务启用了 API Key 则必须提供；
- `LLM_MODEL`：模型 ID/别名；可通过 `/v1/models` 检查实际 ID；
- `LLM_CONNECT_TIMEOUT_SECONDS`；
- `LLM_READ_TIMEOUT_SECONDS`；
- `LLM_CONTEXT_WINDOW`：必须与实际 llama-server 上下文设置相符；
- `LLM_MAX_OUTPUT_TOKENS`；
- `LLM_TEMPERATURE`；
- `LLM_TOP_P`；
- `LLM_PRESENCE_PENALTY`（可选）；
- `LLM_FREQUENCY_PENALTY`（可选）；
- `LLM_SYSTEM_PROMPT` 或受版本管理的数据库设置。

### 14.3 任务配置

- `MAX_ACTIVE_GENERATIONS`；
- `MAX_QUEUE_SIZE`；
- `MAX_QUEUED_TASKS_PER_USER`；
- `MAX_ACTIVE_TASKS_PER_USER`；
- `RECONNECT_GRACE_SECONDS`；
- `INITIAL_STREAM_ATTACH_TIMEOUT_SECONDS`；
- `OUTPUT_CHECKPOINT_INTERVAL_SECONDS`；
- `OUTPUT_CHECKPOINT_MIN_CHARS`；
- `CANCEL_CONFIRM_TIMEOUT_SECONDS`；
- `TASK_MAX_RUNTIME_SECONDS`；
- `SUMMARY_TRIGGER_MESSAGE_COUNT`；
- `MAX_CONTEXT_MESSAGES`；
- `MAX_INPUT_CHARS`。

配置读取必须做类型和范围验证；启动时若关键配置非法，应明确报错并停止，不应静默使用危险默认值。

---

## 15. 日志、监控和故障恢复

### 15.1 结构化日志字段

建议每条日志包含：

- 时间戳；
- 日志级别；
- request ID；
- user ID（内部 ID，不输出用户名或敏感资料）；
- conversation ID；
- task ID；
- 操作阶段；
- 错误代码；
- 请求耗时或模型耗时。

日志不默认保存消息正文、完整提示词或模型 API 返回的敏感内容。

### 15.2 最低限度监控指标

- 当前活动生成任务数；
- 排队任务数；
- 队列等待时间；
- 模型首 token 延迟；
- 生成耗时和可获得的 tokens/s；
- 用户取消率、断线后取消率；
- llama-server 连接失败/超时次数；
- 任务失败和中断数量；
- 数据库错误和缓存命中情况。

不要为了 V1 引入复杂监控平台；日志和管理员状态页即可。后续可按需增加外部监控。

### 15.3 后端启动恢复

启动时执行恢复检查：

1. 清理已过期登录会话；
2. 找出此前 `running` 或 `cancel_requested` 的任务；
3. 因无法确认原始模型请求是否存活，将这些任务转为 `interrupted`，保留最后可用的输出检查点和事件；
4. 更新关联助手消息的状态；
5. 找出此前 `queued`、尚未调用模型的任务，在新进程里继续执行；
6. 释放本进程内残留状态（新进程中应为空）；
7. 不把已经 `running` 的请求自动再提交一次，避免重复生成；
8. 在日志中记录中断数量和继续执行的排队任务。

被中断任务由用户在 UI 中选择重试。若未来需要真正的后台续跑，应作为单独需求重新设计，而不能靠重启时盲目重发请求。

### 15.4 数据备份

- 提供数据库备份和恢复说明；
- 备份应包含数据库和必要配置说明，但不应把秘密配置随意打包；
- 使用 `pg_dump` 备份。不要把 `DATABASE_URL` 和 API Key 打进备份包。操作步骤见 `docs/backup.md`；
- 说明历史消息删除策略及备份保留期限；
- 升级前备份数据库，执行迁移失败应清晰报错并可回滚/恢复。

---

## 16. 前端状态管理和错误恢复

### 16.1 最少需要的前端状态

- 当前登录用户；
- 当前选中的会话 ID；
- 当前会话消息；
- 每个活动任务的 task ID、状态、最后输出序号；
- 当前 SSE 连接状态；
- 当前消息发送请求的 `client_request_id`；
- 连接重试计数与最后错误。

不应把长期历史当作 Zustand 或其他前端状态存储的唯一数据源；切换会话、刷新页面时要重新向后端获取。

### 16.2 刷新和重新进入页面

1. 登录态有效时加载会话列表；
2. 选中会话后加载最近消息和任务状态；
3. 如发现该会话存在非终态任务，查询任务详情；
4. 若任务仍在恢复窗口或连接可恢复，订阅该任务的事件流；
5. 若任务已经终结，显示后端返回的最终内容和状态；
6. 若任务为 `interrupted`，显示已保存的部分回答，并提供重新生成/重试入口；
7. 前端不能因页面刷新再次创建相同的生成任务。

### 16.3 断线重试

- 仅对当前已知的 `task_id` 重连；
- 重连使用指数退避或短间隔递增策略，并有最大重试时间；
- 重连期间不重复 POST 新消息；
- 收到 `snapshot` 时覆盖本地任务正文；
- 收到 delta 时按序号去重；
- 超出后端恢复窗口后，停止无限重试，查询任务终态；
- 明确向用户展示连接已恢复、任务已取消、任务中断或任务失败。

---

## 17. 测试要求

### 17.1 单元测试

至少包括：

- 密码哈希和登录验证；
- 会话 Cookie 和过期逻辑；
- 角色权限判断；
- 用户/会话资源归属验证；
- 上下文组装顺序和上下文预算；
- 任务合法状态转换；
- 幂等 ID 重复提交；
- 输出序号去重；
- 取消请求幂等性；
- 摘要旧版本不能覆盖新版本；
- 设置参数范围验证。

### 17.2 API 集成测试

至少包括：

1. 未登录访问会话接口应返回 401；
2. 用户 A 不能读取、删除或订阅用户 B 的会话/任务；
3. 普通用户不能创建或禁用账号；
4. 禁用用户后，旧 Cookie 不再有效；
5. 相同 `client_request_id` 重试不会重复创建消息或任务；
6. 同一会话有活动任务时第二个请求被拒绝；
7. 队列满时正确拒绝并不产生孤立消息；
8. 模型超时后任务和助手消息状态一致；
9. 删除有活动任务的会话时，必须先确认任务安全终止；
10. 缓存丢失后可以从数据库重建历史和摘要。

### 17.3 流式与任务测试

至少包括：

- 多个 delta 按序拼接；
- 收到重复或旧 `seq` 不重复追加；
- 重连时 snapshot 替换完整正文，不产生重复文本；
- 任务完成后重连直接返回快照和终态；
- 断线在宽限期内恢复，不重复推理；
- 超时未恢复时请求取消；
- 主动取消与网络断开采用不同策略；
- 上游取消未确认时不提前释放并发额度；
- 后端重启后运行中任务被标记为 `interrupted`；
- SSE 连接收到心跳且代理配置不会缓冲所有输出。

### 17.4 llama-server 真实集成测试

使用当前实际运行的模型服务验证：

- `/v1/models` 的模型 ID；
- `/v1/chat/completions` 非流式和流式响应格式；
- 实际支持的生成参数；
- 长时间无输出时的超时行为；
- 取消上游请求后，推理是否真正停止以及资源槽位何时可用；
- API Key 错误时的错误映射；
- 上游重启时应用的恢复行为。

不能只用模拟响应就宣布取消机制已验证。

### 17.5 前端人工验收

- 登录/退出；
- 新建、切换、重命名、归档、删除会话；
- 多轮流式对话；
- 停止生成；
- 在生成过程中刷新页面；
- 在生成过程中断网并恢复；
- 在网络不恢复时观察任务按期取消；
- 同时使用两个账号确认数据隔离；
- 管理员禁用正在登录的用户；
- Markdown 与代码块不会执行恶意 HTML。

---

## 18. 分阶段实现计划

以下阶段是当初的实现顺序，现有代码已按这些阶段落地。后续改动不要重新从阶段 0 初始化工程。

### 阶段 0：从零初始化项目

- 阅读并落实本文的技术栈决策，前端必须使用 React + TypeScript + Vite；
- 创建前后端目录、依赖配置、环境变量示例、`.gitignore` 和 README；
- 建立前端基础页面、路由、API 客户端和统一错误提示骨架；
- 建立 FastAPI 应用、配置加载、日志和健康检查骨架；
- 添加项目运行与测试命令，记录开发阶段实施计划，但不能只输出计划而不创建文件；
- 对关键依赖和版本使用当前稳定兼容版本，并保存锁文件；
- 不引入 LangChain、LangGraph、LlamaIndex、Redis、Celery 或微服务。

验收：前后端均可按 README 启动；前端能显示基础应用页面；后端健康检查可访问；依赖安装、前端构建和基础测试实际运行通过。

### 阶段 1：后端基础和数据库

- FastAPI 应用、配置加载、日志、数据库会话；
- SQLAlchemy 模型与 Alembic 迁移；
- 初始化管理员的 CLI 命令；
- 健康检查；
- `.env.example` 和 README。

验收：空数据库能从迁移创建，管理员可通过 CLI 建立，不存在默认密码。

### 阶段 2：认证与管理员用户管理

- 登录、退出、当前用户；
- 安全 Cookie、CSRF、过期和撤销；
- 管理员用户列表、创建、禁用、重置密码；
- 审计事件。

验收：权限测试和用户隔离测试通过。

### 阶段 3：会话、消息和非流式模型调用

- 会话 CRUD；
- 消息持久化；
- llama-server Adapter；
- 最小提示词上下文；
- 先完成非流式测试，再增加流式逻辑。

验收：一个用户可以登录、创建会话并得到持久化的模型回复。

### 阶段 4：流式输出与任务调度

- 任务状态机和有界队列；
- 前端流式 UI；
- SSE 事件协议；
- 输出序号和检查点；
- 停止生成；
- 同一会话并发限制。

验收：取消和资源释放语义通过单元测试；真实 llama-server 取消行为完成验证或明确记录限制。

### 阶段 5：断线恢复和启动恢复

- 短暂断线恢复窗口；
- SSE 重新订阅和 snapshot；
- 幂等消息请求；
- 后端启动时处理遗留任务；
- 对重连重复文本、任务重发等情况进行集成测试。

验收：刷新和断网恢复均不会重复执行同一任务。

### 阶段 6：记忆系统

- 会话摘要；
- 关键决策 CRUD；
- 用户偏好 CRUD；
- Context Builder 整合记忆；
- 低优先级摘要维护任务；
- 缓存抽象和缓存失效后的数据库恢复。

验收：不同会话/用户记忆不串用，摘要更新失败不影响聊天。

### 阶段 7：前端和管理功能完善

- 会话搜索、归档、删除确认；
- 管理后台；
- 模型参数安全设置；
- 系统状态；
- 错误状态、窄屏适配和可访问性。

验收：主要用户路径端到端可用。

### 阶段 8：部署与安全验收

- 生产配置说明；
- HTTPS/反向代理/SSE 缓冲配置示例；
- llama-server 仅内部可访问的部署建议；
- 数据备份与恢复文档；
- 端到端测试、安全检查、依赖检查。

验收：能按 README 从干净环境部署；没有默认口令和前端模型 API Key。

---

## 19. 完成定义（Definition of Done）

每项功能只有满足以下要求才算完成：

- 有后端权限和输入校验；
- 对需要持久化的业务状态有数据库实现和迁移；
- 有成功路径、失败路径和重复请求处理；
- 对状态机变化有测试；
- 前端状态与后端最终状态一致；
- 不泄露秘密信息；
- 日志足以定位问题，但不会默认记录正文或凭证；
- README/配置文档同步更新；
- 测试在实际项目环境运行过，不能只说“理论上通过”。

整个项目的 V1 完成标准：新用户由管理员创建后可安全登录、独立管理聊天；本地模型能流式回复；任务可以停止；断线可在恢复窗口内继续订阅同一任务；历史和记忆可靠隔离；服务重启后状态不含糊；部署、备份和测试说明齐备。

---

## 20. 当前不应做的过度设计

- 不要先搭 Redis、Celery、消息队列集群或微服务；
- 不要为每个 token 另建消息行；重连所需的增量文本写入 `generation_task_events`；
- 不要把整个聊天历史放进缓存或每轮都塞给模型；
- 不要每条消息都额外调用模型生成摘要；
- 不要让多个后端 worker 同时启动同一份本地任务队列；
- 不要默认模型 API 一定会支持全部 OpenAI 参数；
- 不要把 HTTP 断开等同于上游模型已经停止；
- 不要让管理员 UI 读取任何用户的所有聊天正文成为默认行为；
- 不要为了未来扩展过早实现向量数据库、Agent、工具执行或计费系统；
- 不要把前端按钮禁用当作唯一的并发保护；
- 不要只靠内存保存任务状态而没有持久化和启动恢复策略。

---

## 21. 关键产品决策记录

在生成代码时应保持以下默认决策，除非用户后续明确修改：

- 工程已经落地。修改时对照当前代码和本文，不要重建一套新工程。
- 前端固定为 React + TypeScript + Vite + React Router + Zustand + Tailwind CSS + shadcn/ui。不得使用 Vue 3、Pinia 或 Element Plus。
- 不引入 LangChain、LangGraph 或 LlamaIndex；模型调用直接通过后端 Inference Adapter 完成。

1. 单机、单应用后端进程，模块化单体；
2. 多用户，管理员预创建账号，不开放公开注册；
3. 使用 PostgreSQL 16 持久化；进程内缓存可以丢弃和重建；
4. llama-server 由后端通过 OpenAI 兼容接口调用；前端不直连模型服务；
5. 历史消息、会话摘要、关键决策和用户偏好分开保存；
6. 每个会话同一时间最多一个未终结生成任务；
7. 消息提交采用幂等 ID，避免网络重试重复生成；
8. 短暂断线可恢复，默认宽限期 30 秒，逾期请求取消；
9. 助手输出周期性持久化；进程崩溃最多损失最近一个未检查点区间；
10. 后端重启后不自动重复提交旧的运行中任务，改为 `interrupted` 并允许用户手动重试；
11. 模型取消行为必须在当前实际 llama-server 版本上验证；
12. 默认模型并发数为 1，后续通过压测和显存情况调整；
13. 不默认把内部推理字段展示给用户；
14. 不开放任意工具执行、公开 API Token 或公开注册。

---

## 22. 给 Cursor 的启动指令

本文位于项目根目录，对应已经实现的代码。表结构以 `backend/app/db/models/entities.py` 为准，各表作用见 `docs/database.md`。

> 修改本系统时，先阅读 `SPEC.md` 和相关代码，再改实现。不要从零初始化，不要覆盖或删除无关文件。行为变化若偏离本文，先说明冲突再改代码或回写规格。严格保持 React + TypeScript + Vite 前端和 FastAPI 后端。必须真实接入配置的 llama-server，不得用模拟回答代替模型调用。对当前 llama-server 无法真实验证的取消或恢复行为，必须明确记录限制，不得声称验证通过。

如果一次执行受上下文或环境限制，先交付当前阶段真实可运行的结果，并在 README 中记录明确的下一步；不得将空文件、占位实现或未经执行的测试描述为已完成。
