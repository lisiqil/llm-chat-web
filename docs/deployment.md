# 部署

第一版是单机、单个后端进程。不要启动多个 uvicorn worker，否则每个进程会有自己的任务队列。

## 生产配置

- `APP_ENV=production`
- `COOKIE_SECURE=true`
- `APP_SECRET_KEY` 使用新的随机值
- `ALLOWED_ORIGINS` 只填写实际站点来源，例如 `https://chat.example.com`
- `PUBLIC_BASE_URL` 填写同一来源
- `TRUST_PROXY=true` 仅当请求经过你自己的反向代理
- llama-server 绑定 `127.0.0.1`，不要把模型端口暴露到公网

前端执行 `npm run build` 后，由反向代理提供 `frontend/dist`，并把 `/api` 转到后端。

## Nginx 示例

```nginx
server {
    listen 443 ssl;
    server_name chat.example.com;

    root /srv/llm-web/frontend/dist;
    index index.html;

    location /api/ {
        proxy_pass http://127.0.0.1:8000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        add_header X-Accel-Buffering no;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

`/api/v1/tasks/` 的事件流必须关闭代理缓冲，否则浏览器要等生成结束后才能看到文字。

## llama-server

在同一台机器上启动，并只监听回环地址。模型 ID 以 `/v1/models` 的实际返回为准，写进 `LLM_MODEL`。上下文长度写进 `LLM_CONTEXT_WINDOW`，需要和启动 llama-server 时的上下文参数一致。

升级应用前先备份数据库，再执行：

```bash
cd backend
uv run python -m app.cli upgrade
```

迁移失败时停止启动，用备份恢复，不要手工改生产库结构。
