export default function AskInChatButton({ alertId, question }) {
  const base = (import.meta.env.VITE_LIBRECHAT_URL || '').replace(/\/$/, '')
  const enabled = Boolean(base)

  const href = enabled
    ? `${base}/?q=${encodeURIComponent(
        question ||
          `What else could explain alert ${alertId}? Use get_investigation and only cite evidence numbers.`,
      )}&alertId=${encodeURIComponent(alertId || '')}`
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
