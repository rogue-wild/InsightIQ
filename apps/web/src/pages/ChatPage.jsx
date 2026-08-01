import { useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { sendChatMessage } from '../api/client.js'
import ChatMarkdown from '../components/ChatMarkdown.jsx'

function bubbleKey(role, index) {
  return `${role}-${index}`
}

const LIBRECHAT_URL = (import.meta.env.VITE_LIBRECHAT_URL || 'http://localhost:3080').replace(
  /\/$/,
  '',
)

function libreChatHref(prompt) {
  const params = new URLSearchParams()
  params.set('endpoint', 'InsightIQ')
  params.set('model', 'insightiq-rca')
  if (prompt?.trim()) {
    params.set('prompt', prompt.trim())
    params.set('submit', 'true')
  }
  return `${LIBRECHAT_URL}/c/new?${params.toString()}`
}

export default function ChatPage() {
  const [params] = useSearchParams()
  const initialQ = params.get('q') || params.get('prompt') || ''
  const investigationId = params.get('investigationId') || ''
  const alertId = params.get('alertId') || ''

  const [input, setInput] = useState('')
  const [messages, setMessages] = useState([])
  const [sending, setSending] = useState(false)
  const [error, setError] = useState(null)
  const bootstrapped = useRef(false)
  const bottomRef = useRef(null)
  const inputRef = useRef(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, sending])

  useEffect(() => {
    if (bootstrapped.current) return
    if (!initialQ.trim()) return
    bootstrapped.current = true
    void submit(initialQ.trim(), [])
  }, [initialQ])

  async function submit(text, prior) {
    const content = text.trim()
    if (!content || sending) return

    const nextUser = { role: 'user', content }
    const history = [...prior, nextUser]
    setMessages(history)
    setInput('')
    setSending(true)
    setError(null)

    try {
      const reply = await sendChatMessage(history, {
        investigationId,
        alertId,
      })
      setMessages([...history, { role: 'assistant', content: reply }])
    } catch (err) {
      setError(err.message || 'Chat failed')
      setMessages(history)
    } finally {
      setSending(false)
      inputRef.current?.focus()
    }
  }

  function onSubmit(e) {
    e.preventDefault()
    void submit(input, messages)
  }

  const contextBits = [investigationId, alertId].filter(Boolean)
  const handoffPrompt =
    input.trim() ||
    messages.filter((m) => m.role === 'user').at(-1)?.content ||
    initialQ ||
    'How is APAC revenue doing this week?'

  return (
    <div className="chat-page fade-in">
      <div className="chat-toolbar">
        <div>
          <h1 className="page-title">Chat</h1>
          <p className="page-subtitle" style={{ marginBottom: 0 }}>
            Ask InsightIQ why a metric moved — answers stay grounded in investigation evidence.
          </p>
        </div>
        <div className="chat-toolbar-actions">
          {contextBits.length > 0 ? (
            <span className="mono muted chat-context">{investigationId || alertId}</span>
          ) : null}
          <a
            className="btn btn-primary"
            href={libreChatHref(handoffPrompt)}
            target="_blank"
            rel="noreferrer"
            title="Continue in LibreChat with MCP tools"
          >
            Open in LibreChat
          </a>
          <Link to="/alerts" className="btn">
            Alerts
          </Link>
        </div>
      </div>

      <section className="panel chat-shell">
        <div className="chat-transcript" role="log" aria-live="polite">
          {messages.length === 0 && !sending ? (
            <div className="chat-empty">
              <p className="chat-empty-title">Start with a question</p>
              <p className="muted">
                Try “How is India doing?” or “How is APAC revenue?” — or open an investigation and use
                Ask in chat. Use <strong>Open in LibreChat</strong> for MCP-powered exploration.
              </p>
            </div>
          ) : null}

          {messages.map((m, i) => (
            <div
              key={bubbleKey(m.role, i)}
              className={`chat-bubble ${m.role === 'user' ? 'is-user' : 'is-assistant'}`}
            >
              <div className="chat-bubble-role">{m.role === 'user' ? 'You' : 'InsightIQ'}</div>
              <div className="chat-bubble-body">
                {m.role === 'assistant' ? <ChatMarkdown>{m.content}</ChatMarkdown> : m.content}
              </div>
            </div>
          ))}

          {sending ? (
            <div className="chat-bubble is-assistant is-typing">
              <div className="chat-bubble-role">InsightIQ</div>
              <div className="chat-bubble-body muted">Analyzing evidence…</div>
            </div>
          ) : null}
          <div ref={bottomRef} />
        </div>

        {error ? <div className="chat-error">{error}</div> : null}

        <form className="chat-composer" onSubmit={onSubmit}>
          <textarea
            ref={inputRef}
            className="chat-input"
            rows={2}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void submit(input, messages)
              }
            }}
            placeholder="Ask about a region (India, APAC) or why a metric moved…"
            disabled={sending}
          />
          <button type="submit" className="btn btn-primary" disabled={sending || !input.trim()}>
            Send
          </button>
        </form>
      </section>
    </div>
  )
}
