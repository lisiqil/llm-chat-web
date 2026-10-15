import asyncio
import os
from pathlib import Path

from openai import AsyncOpenAI


def _load_env(path: Path) -> None:
    if not path.exists():
        return
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        os.environ.setdefault(key.strip(), value.strip().strip('"').strip("'"))


_load_env(Path(__file__).resolve().parents[1] / ".env")

client = AsyncOpenAI(
    base_url=os.environ.get("LLM_BASE_URL", "http://127.0.0.1:8080/v1"),
    api_key=os.environ.get("LLM_API_KEY", ""),
)


async def main() -> None:
    stream = await client.chat.completions.create(
        model="local-model",
        messages=[{"role": "user", "content": "你好,介绍一下自己"}],
        stream=True,
    )
    async for chunk in stream:
        content = chunk.choices[0].delta.content or ""
        print(content, end="", flush=True)
    print()


if __name__ == "__main__":
    asyncio.run(main())
