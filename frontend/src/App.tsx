import { FormEvent, useState } from "react"
import { Route, Routes } from "react-router-dom"

export function App() {
  return (
    <Routes>
      <Route path="/" element={<ChatPage />} />
    </Routes>
  )
}

function ChatPage() {
  const [message, setMessage] = useState("")
  const [answer, setAnswer] = useState("")
  const [error, setError] = useState("")
  const [sending, setSending] = useState(false)

  async function onSubmit(event: FormEvent) {
    event.preventDefault()
    const text = message.trim()
    if (!text || sending) return
    setSending(true)
    setAnswer("")
    setError("")
    try {
      // 后端约定：POST /api/v1/chat，body 为 { message }，响应体是模型回复的纯文本流。
      const response = await fetch("/api/v1/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: text }),
      })
      if (!response.ok) {
        const detail = await response.text()
        throw new Error(detail || `请求失败（${response.status}）`)
      }
      if (!response.body) throw new Error("响应没有内容。")
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let next = ""
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        next += decoder.decode(value, { stream: true })
        setAnswer(next)
      }
      next += decoder.decode()
      setAnswer(next)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "发送失败。")
    } finally {
      setSending(false)
    }
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col gap-4 p-8">
      <h1 className="text-2xl font-semibold">本地 LLM</h1>
      <form className="flex flex-col gap-3" onSubmit={(event) => void onSubmit(event)}>
        <textarea
          className="min-h-28 rounded border border-neutral-300 p-3"
          value={message}
          placeholder="输入一句话"
          disabled={sending}
          onChange={(event) => setMessage(event.target.value)}
        />
        <button
          className="w-fit rounded bg-neutral-900 px-4 py-2 text-white disabled:opacity-50"
          type="submit"
          disabled={sending || message.trim() === ""}
        >
          {sending ? "生成中" : "发送"}
        </button>
      </form>
      {error ? <p className="text-red-700">{error}</p> : null}
      <section className="min-h-40 whitespace-pre-wrap rounded border border-neutral-200 p-3" aria-live="polite">
        {answer || (sending ? "正在等待回答" : "回答会显示在这里")}
      </section>
    </main>
  )
}
