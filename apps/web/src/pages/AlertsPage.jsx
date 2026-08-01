import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { listAlerts } from '../api/client.js'
import StatusPill from '../components/StatusPill.jsx'
import { formatMetric, formatWindow, polishSummary } from '../utils/format.js'

export default function AlertsPage() {
  const [alerts, setAlerts] = useState([])
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const data = await listAlerts()
        if (!cancelled) setAlerts(data)
      } catch (err) {
        if (!cancelled) setError(err.message || 'Failed to load alerts')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  if (loading) return <div className="loading">Loading alerts…</div>
  if (error) return <div className="error-box">{error}</div>

  return (
    <div>
      <h1 className="page-title">Alerts</h1>
      <p className="page-subtitle">
        Metric movements ready for automated root-cause investigation. Open an alert to
        inspect the metric tree, segments, and evidence-backed diagnosis.
      </p>

      <div className="alert-list panel">
        {alerts.map((alert) => (
          <Link
            key={alert.id}
            to={`/investigations/${alert.investigationId}`}
            className="alert-row"
          >
            <div className="alert-main">
              <div className="alert-title-row">
                <span className="alert-metric">{formatMetric(alert.metric)}</span>
                <StatusPill status={alert.severity}>{alert.severity}</StatusPill>
                <StatusPill status={alert.status}>{alert.status}</StatusPill>
              </div>
              <p className="alert-summary">{polishSummary(alert.summary)}</p>
              <div className="alert-meta mono muted">
                <span>{formatWindow(alert.windowStart, alert.windowEnd)}</span>
                <span>·</span>
                <span>{alert.baselineKind.replaceAll('_', ' ')}</span>
              </div>
            </div>
            <div className={`alert-delta ${alert.direction === 'down' ? 'neg' : 'pos'}`}>
              {alert.direction === 'down' ? '↓' : '↑'}{' '}
              {Math.abs(alert.pctChange).toFixed(1)}%
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
