export default function AskInChatButton({ alertId, question }) {
  const base = (import.meta.env.VITE_LIBRECHAT_URL || '').replace(/\/$/, '')
  const enabled = Boolean(base)
  const endpoint = import.meta.env.VITE_LIBRECHAT_ENDPOINT || 'InsightIQ'
  const model = import.meta.env.VITE_LIBRECHAT_MODEL || 'insightiq-rca'

  const prompt =
    question ||
    `What else could explain alert ${alertId}? Use get_investigation and only cite evidence numbers.`

  const href = enabled
    ? `${base}/c/new?${new URLSearchParams({
        endpoint,
        model,
        prompt,
        submit: 'true',
      }).toString()}`
    : undefined

  if (!enabled) {
    return (
      <button
        type="button"
        className="btn"
        disabled
        title="Set VITE_LIBRECHAT_URL to enable LibreChat follow-ups"
      >
        Ask in chat
      </button>
    )
  }

  return (
    <a className="btn btn-primary" href={href} target="_blank" rel="noreferrer">
      Ask in chat
    </a>
  )
}
