import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { getInvestigation } from '../api/client.js'
import AskInChatButton from '../components/AskInChatButton.jsx'
import DiagnosisCard from '../components/DiagnosisCard.jsx'
import MetricTree from '../components/MetricTree.jsx'
import SegmentTable from '../components/SegmentTable.jsx'
import StatusPill from '../components/StatusPill.jsx'
import TraceTimeline from '../components/TraceTimeline.jsx'
import { formatMetric, formatWindow } from '../utils/format.js'

export default function InvestigationPage() {
  const { investigationId } = useParams()
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    ;(async () => {
      try {
        const inv = await getInvestigation(investigationId)
        if (!cancelled) setData(inv)
      } catch (err) {
        if (!cancelled) setError(err.message || 'Failed to load investigation')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [investigationId])

  if (loading) return <div className="loading">Running investigation…</div>
  if (error) return <div className="error-box">{error}</div>
  if (!data) return null

  const { alert, decomposition, segments, ruledOut, diagnosis, trace } = data

  return (
    <div className="investigation">
      <div className="inv-nav">
        <Link to="/" className="back-link muted">
          ← Alerts
        </Link>
      </div>

      <header className="inv-header panel fade-in">
        <div>
          <div className="inv-title-row">
            <h1 className="page-title" style={{ marginBottom: 0 }}>
              {formatMetric(alert.metric)} {alert.direction === 'down' ? '↓' : '↑'}{' '}
              {Math.abs(alert.pctChange).toFixed(1)}%
            </h1>
            <StatusPill status={alert.severity}>{alert.severity}</StatusPill>
            <StatusPill status={data.status}>{data.status}</StatusPill>
          </div>
          <p className="inv-window mono muted">
            {formatWindow(alert.windowStart, alert.windowEnd, { withTime: true })}
          </p>
          <p className="muted" style={{ margin: '0.35rem 0 0' }}>
            Baseline: {alert.baselineKind.replaceAll('_', ' ')}
          </p>
        </div>
        <AskInChatButton
          alertId={alert.id}
          question={`Why did ${formatMetric(alert.metric)} move ${alert.pctChange}% on ${alert.windowStart.slice(0, 10)}? Use investigation ${data.id}.`}
        />
      </header>

      <div className="grid-2" style={{ marginTop: '1rem' }}>
        <section className="panel">
          <div className="panel-header">
            <h2 className="panel-title">Metric tree</h2>
            <span className="muted" style={{ fontSize: '0.8rem' }}>
              Revenue identity walk
            </span>
          </div>
          <div className="panel-body">
            <MetricTree decomposition={decomposition} />
          </div>
        </section>

        <section className="panel">
          <div className="panel-header">
            <h2 className="panel-title">Diagnosis</h2>
          </div>
          <div className="panel-body">
            <DiagnosisCard diagnosis={diagnosis} />
          </div>
        </section>
      </div>

      <div className="grid-2" style={{ marginTop: '1rem' }}>
        <section className="panel">
          <div className="panel-header">
            <h2 className="panel-title">Dimension drill-down</h2>
          </div>
          <div className="panel-body">
            <SegmentTable segments={segments} />
          </div>
        </section>

        <div className="grid-stack">
          <section className="panel">
            <div className="panel-header">
              <h2 className="panel-title">Ruled out</h2>
            </div>
            <div className="panel-body">
              {ruledOut.length === 0 ? (
                <p className="muted">Still checking…</p>
              ) : (
                <ul className="ruled-list">
                  {ruledOut.map((item) => (
                    <li key={item.reason}>
                      <strong>{item.reason}</strong>
                      <p className="muted">{item.detail}</p>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </section>

          <section className="panel">
            <div className="panel-header">
              <h2 className="panel-title">Investigation trace</h2>
            </div>
            <div className="panel-body">
              <TraceTimeline trace={trace} />
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
