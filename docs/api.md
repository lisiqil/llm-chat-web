# 接口说明

所有接口都在 `/api/v1` 下。浏览器开发时由前端把 `/api` 代理到后端。请求和响应都是 JSON，时间是带时区的 UTC。除健康检查和登录外，都要带登录 Cookie。

写请求（`POST`、`PATCH`、`DELETE`）还要带 CSRF。登录成功后，服务端设置两个 Cookie：`llm_session`（HttpOnly，会话令牌）和 `llm_csrf`（页面可读）。之后的写请求必须同时带上 `llm_csrf`，以及请求头 `X-CSRF-Token`，两者内容相同，并且要和当前登录会话算出的令牌一致。`GET /api/v1/auth/csrf` 可以重新取这个令牌。登录接口本身不校验 CSRF。生产环境的写请求还必须带允许列表里的 `Origin`。

请求体里出现未声明的字段会直接拒绝。业务错误统一是：

```json
{"error": {"code": "VALIDATION_ERROR", "message": "请求参数无效。", "request_id": "..."}}
```

| code | HTTP | 含义 |
|---|---|---|
| `AUTH_REQUIRED` | 401 | 未登录或会话失效 |
| `PERMISSION_DENIED` | 403 | 不是管理员、来源不被允许，或 CSRF 失败 |
| `RESOURCE_NOT_FOUND` | 404 | 资源不存在，或不属于当前用户 |
| `VALIDATION_ERROR` | 422 | 参数不合法；请求体过大时是 413 |
| `CONVERSATION_BUSY` | 409 | 这个会话已经有进行中的生成 |
| `IDEMPOTENCY_KEY_CONFLICT` | 409 | 同一个 `client_request_id` 配了不同内容 |
| `TASK_NOT_CANCELLABLE` | 409 | 任务已经结束，不能再取消 |
| `RATE_LIMITED` | 429 | 登录或发消息过于频繁 |
| `QUEUE_FULL` | 503 | 全局队列或该用户的排队数已满 |
| `LLM_UNAVAILABLE` | 503 | 模型服务不可用；生成过程中的失败更多记在任务上 |
| `INTERNAL_ERROR` | 500 | 未处理的服务器错误 |

别人的会话、消息和任务一律返回 404，不区分“不存在”和“不属于你”。

## 健康检查

`GET /api/v1/health` 不需要登录。数据库能执行 `SELECT 1` 时返回 `{"status": "ok"}`。

## 登录

`POST /api/v1/auth/login` 的请求体是 `username`、`password`。成功时写入上面两个 Cookie，并返回当前用户和 `csrf_token`。用户名会转成小写再查找。失败、账号停用，都返回同一句“用户名或密码错误”，状态码 401。

`POST /api/v1/auth/logout` 吊销当前会话并清掉 Cookie，返回 `{"ok": true}`。

`GET /api/v1/auth/me` 返回当前用户：`id`、`username`、`role`（`admin` 或 `user`）、`status`（`active` 或 `disabled`）、`created_at`、`updated_at`、`last_login_at`。

## 会话与消息

会话列表 `GET /api/v1/conversations`。查询参数：`archived`（默认 false）、`q`（按标题）、`limit`（1 到 100，默认 30）、`cursor`。响应是 `{items, next_cursor}`。游标形如 `时间~会话ID`，按最后消息时间往更早翻。

创建 `POST /api/v1/conversations`，体里可以只给 `title`。不传时标题是“新对话”，发出第一条消息后会用消息开头自动改标题。

读取 `GET /api/v1/conversations/{conversation_id}`。修改 `PATCH /api/v1/conversations/{conversation_id}`，体里可以改 `title` 或 `archived`。归档后不能再发消息，先把 `archived` 设回 false。

删除 `DELETE /api/v1/conversations/{conversation_id}`。会话里如果还有正在跑的生成，返回 409，需要等它停下来。还在排队的任务会先取消，再删除会话、消息、任务和记忆。

会话对象字段：`id`、`title`、`created_at`、`updated_at`、`last_message_at`、`archived_at`（未归档为 null）、`version`（当前固定为 1）、`active_task`。`active_task` 有进行中的生成时是任务对象，否则为 null。

消息列表 `GET /api/v1/conversations/{conversation_id}/messages`。参数 `limit`（默认 50）、`before_sequence_no`。响应 `{items, next_before_sequence_no}`，按序号从早到晚。助手消息会带上对应任务的 `task_id` 和 `error_code`。

消息字段：`id`、`conversation_id`、`sequence_no`、`role`（`user` 或 `assistant`）、`content`、`status`、`task_id`、`created_at`、`updated_at`、`error_code`。`input_tokens` 和 `output_tokens` 目前总是 null，用量在任务对象上。数据库里的 `streaming` 在这里显示为 `generating`。其余状态是 `completed`、`pending`、`interrupted`、`cancelled`、`failed`。

发送 `POST /api/v1/conversations/{conversation_id}/messages`：

```json
{
  "content": "你好",
  "client_request_id": "uuid",
  "generation_params": {"temperature": 0.7, "top_p": 0.9, "max_tokens": 256}
}
```

`client_request_id` 必须是 UUID。同一个用户重复提交相同的 ID 和相同内容时，返回已经创建的那次任务，不会再插一条消息。内容不同则 409。`generation_params` 可省略；`presence_penalty` 和 `frequency_penalty` 只有服务端打开惩罚系数后才接受。

成功响应：

```json
{
  "conversation_id": "...",
  "user_message_id": "...",
  "assistant_message_id": "...",
  "task_id": "...",
  "task_status": "queued"
}
```

接着用 `task_id` 订阅下面的事件流。归档会话、空消息、队列满、本会话已有活动任务，都会在这一步被拒绝。

## 任务

`GET /api/v1/tasks/{task_id}` 返回任务快照。字段包括 `id`、`conversation_id`、`user_message_id`、`assistant_message_id`、`status`、`task_kind`（聊天任务固定为 `chat`）、`output_text`、`error_code`、`safe_error_message`、`terminal`、`prompt_tokens`、`completion_tokens`、`duration_ms`、`model_name`，以及创建、开始、结束时间。状态含义见 `docs/database.md`。`cancel_requested` 时会补上“正在等待模型确认结束”，这时 `terminal` 仍是 false。

事件流 `GET /api/v1/tasks/{task_id}/events`，响应类型是 `text/event-stream`。先发一条 `snapshot`，内容是当前状态、已有正文和 `terminal`。如果当时已经结束，紧接着再发一条与终态同名的事件，然后关闭。否则继续推送：

| event | data |
|---|---|
| `snapshot` | `task_id`、`status`、`output_text`、`output_seq`、`terminal` |
| `delta` | `task_id`、`seq`、`text`（这一小段新增文本） |
| `status` | `task_id`、`status` |
| `heartbeat` | `task_id` |
| `completed` / `cancelled` / `failed` / `interrupted` / `rejected` | `task_id`、`status`、`assistant_message_id`、`error_code`、`safe_error_message`、`output_seq`、`output_text` |

终态事件发出后连接结束。断线重连会重新从已保存的助手正文做 `snapshot`，再接后续增量。

取消 `POST /api/v1/tasks/{task_id}/cancel`，没有请求体。任务先变成 `cancel_requested`，模型流确认结束后才是 `cancelled`。已结束的任务返回 409。

重试 `POST /api/v1/tasks/{task_id}/retry`，重新生成 `POST /api/v1/tasks/{task_id}/regenerate`。体和发送消息一样，只保留 `client_request_id` 和可选的 `generation_params`。重试只接受 `failed`、`interrupted`、`cancelled`。重新生成在此之外还接受 `completed`。两者都会新建一条助手消息和一条任务，返回字段与发送消息相同。

## 记忆

`GET /api/v1/conversations/{conversation_id}/memory` 返回 `{summary, decisions}`。还没有摘要时 `summary` 为 null。摘要字段：`conversation_id`、`summary_text`、`through_sequence_no`、`version`、`updated_at`。

手动保存 `PATCH /api/v1/conversations/{conversation_id}/memory/summary`，体为 `summary_text` 和 `base_version`。正文不能为空。`base_version` 必须等于当前 `version`，第一次保存传 `0`。成功后覆盖范围会记到该会话当前最后一条消息，`version` 加 1。版本不一致时返回 422，需要先重新读取。

让模型整理 `POST /api/v1/conversations/{conversation_id}/memory/summary/refresh`，没有请求体。返回 `{task_id, task_status}`。这个 `task_id` 只表示这次整理，不会出现在生成任务表里，也不能拿去订阅 `/tasks/{id}/events`。整理完成后再次读取 memory，才能看到新正文。已有非空摘要且没有新消息时返回 422。会话正在生成，或队列没有空位时返回 409。空白摘要不算已经覆盖，可以重新整理全部消息。

决定 `POST /api/v1/conversations/{conversation_id}/decisions`，体为 `title`、`content`，可选 `source_message_id`、`status`（`active` 或 `superseded`）、`confirmed_by_user`。来源消息必须属于同一会话。修改 `PATCH .../decisions/{decision_id}`，删除 `DELETE .../decisions/{decision_id}`。读取时 `confirmed_by_user` 固定为 true，决定是否有效看 `status`。

偏好是用户级的，不挂在某个会话上。

`GET /api/v1/user/preferences` 返回 `{items}`。每项有 `id`、`key`、`value`、`status`（`active` 或 `inactive`）、`confirmed_by_user`、`created_at`、`updated_at`。`source_message_id` 目前总是 null。

创建或覆盖 `POST /api/v1/user/preferences`，体为 `key`、`value`，可选 `confirmed_by_user`。同一个 key 再次提交会更新原记录。修改 `PATCH /api/v1/user/preferences/{preference_id}`，可改 `value`、`status`、`confirmed_by_user`。停用只把状态改成 `inactive`。删除 `DELETE /api/v1/user/preferences/{preference_id}`。

## 管理

以下接口要求当前用户角色是 `admin`，否则 403。

用户列表 `GET /api/v1/admin/users`，参数 `q`、`status`（`active` 或 `disabled`）、`limit`、`offset`。响应 `{items, total}`。

创建 `POST /api/v1/admin/users`，体为 `username`、`role`，`password` 可省略。省略时响应里多一个 `initial_password`，只返回这一次。用户名是 3 到 32 位字母、数字、点、下划线或连字符。密码长度 10 到 128，首尾不能是空白。

读取 `GET /api/v1/admin/users/{user_id}`。修改 `PATCH /api/v1/admin/users/{user_id}`，体里可改 `role`、`status`。停用会吊销该用户的登录会话，并取消其进行中的生成。不能停用或降级最后一个活动管理员。

重置密码 `POST /api/v1/admin/users/{user_id}/reset-password`，体为 `new_password`，返回 `{"ok": true}`。响应里没有新密码。

吊销会话 `POST /api/v1/admin/users/{user_id}/revoke-sessions`，返回 `{"ok": true, "revoked": 数量}`。

系统状态 `GET /api/v1/admin/system/health`。包含应用、数据库、模型连通性、活动任务数、排队数、占用的推理槽位、进程内计数，以及最近最多 8 条失败或中断任务。模型连通性测试 `POST /api/v1/admin/system/llm-test` 只请求模型的 `/models`，不生成文本。

生成默认参数 `GET /api/v1/admin/settings/generation`，修改 `PATCH /api/v1/admin/settings/generation`。可改 `temperature`、`top_p`、`max_tokens`、`system_prompt`，以及在启用惩罚系数时的 `presence_penalty`、`frequency_penalty`。响应里还有 `model`、`supported_params`、`penalties_enabled`。已经排队的任务用的是创建时复制下来的参数，不会跟着这次修改变。
