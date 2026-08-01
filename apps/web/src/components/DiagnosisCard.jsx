import { polishSummary } from '../utils/format.js'

export default function DiagnosisCard({ diagnosis, animate = true }) {
  if (!diagnosis) return null

  return (
    <article className={`diagnosis-card ${animate ? 'fade-in' : ''}`}>
      <p className="diagnosis-text">{polishSummary(diagnosis.text)}</p>
      {diagnosis.citations?.length > 0 && (
        <ul className="citation-list">
          {diagnosis.citations.map((c) => (
            <li key={c.label}>
              <span className="citation-label">{polishSummary(c.label)}</span>
              <span className="citation-value mono">{polishSummary(c.value)}</span>
            </li>
          ))}
        </ul>
      )}
    </article>
  )
}
