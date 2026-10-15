# 本地 LLM Web 应用

前后端依赖和框架还在，业务实现已清空，方便从头写聊天。

模型连通性见 `backend/test.py`，服务器情况见 `note.md`。`SPEC.md` 和 `docs/` 是上一版完整设计的记录，不是当前代码。

## 启动

后端：

```bash
cd backend
uv run uvicorn app.main:app --host 127.0.0.1 --port 8000
```

前端：

```bash
cd frontend
npm run dev
```

浏览器打开 `http://127.0.0.1:5173`，页面是一条路由上的 Hello World。开发服务器把 `/api` 代理到 `127.0.0.1:8000`。

接口只有两个：`GET /api/v1/health` 和 `GET /api/v1/hello`。
