# llama-server 集成与取消限制

后端通过 `LLM_BASE_URL` 调用：

- `GET /models`：健康检查和管理员连通性测试，不生成文本
- `POST /chat/completions`：`stream: true`

默认只发送 `model`、`messages`、`temperature`、`top_p`、`max_tokens` 和 `stream`。`presence_penalty` 与 `frequency_penalty` 只有在 `LLM_ENABLE_PENALTIES=true` 时才会发送。不要假定当前 llama-server 支持全部 OpenAI 参数。

模型返回的 `reasoning_content` 不会写入助手正文。出现 `tool_calls` 时不会执行任何工具；如果整段回复只有工具调用，任务会失败并提示模型返回无法识别。

## 停止生成

用户点击停止后，适配器会关闭这次 HTTP 响应流，任务先进入 `cancel_requested`。只有读取循环确认结束后，才标记为 `cancelled` 并释放推理槽位。

如果在 `CANCEL_CONFIRM_TIMEOUT_SECONDS` 内流没有结束，状态保持 `cancel_requested`，槽位继续占用，直到上游连接真正结束。界面会显示“正在停止”，不会提前显示已经取消。

这个确认只表示应用侧的 HTTP 流结束。它不能证明 llama-server 已经停止 GPU 计算或释放了并行槽位。当前环境没有可用的 llama-server，因此这项行为尚未做真实集成验证。

验证步骤：

```bash
cd backend
LLM_INTEGRATION=1 uv run pytest -q -m integration
uv run python scripts/probe_llama.py
```

探针会请求 `/v1/models`，再发起一次流式补全并在约 1 秒后关闭连接。输出里的“客户端流已结束”仍然不是 GPU 已停止的证据。需要在 llama-server 自己的槽位或日志里确认生成是否停止，再调整 `CANCEL_CONFIRM_TIMEOUT_SECONDS`。
