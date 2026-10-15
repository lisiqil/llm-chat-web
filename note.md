
test脚本启动
uv run --with openai python test.py

后端启动
cd backend
uv run uvicorn app.main:app --host 127.0.0.1 --port 8000

前端启动
cd frontend
npm run dev