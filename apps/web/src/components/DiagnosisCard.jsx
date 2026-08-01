import { polishSummary } from '../utils/format.js'

export default function DiagnosisCard({ diagnosis, evidence, animate = true }) {
  if (!diagnosis) return null

  return (
    <article className={`diagnosis-card ${animate ? 'fade-in' : ''}`}>
      <p className="diagnosis-text">{polishSummary(diagnosis.text)}</p>
      {diagnosis.citations?.length > 0 && (
        <ul className="citation-list">
          {diagnosis.citations.map((c) => (
            <li key={c.label} className="citation-chip" title="Computed from ClickHouse evidence — not LLM-invented">
              <span className="citation-label">{polishSummary(c.label)}</span>
              <span className="citation-value mono">{polishSummary(c.value)}</span>
            </li>
          ))}
        </ul>
      )}
      {evidence?.hash ? (
        <p className="evidence-lock mono muted" title={(evidence.sources || []).join(', ')}>
          Evidence lock · sha256:{evidence.hash.slice(0, 12)}…
        </p>
      ) : null}
    </article>
  )
}
