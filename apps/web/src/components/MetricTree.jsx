import StatusPill from './StatusPill.jsx'
import { formatFactorValue, formatSignedPct } from '../utils/format.js'

const STATUS_LABEL = {
  culprit: 'Culprit',
  ruled_out: 'Ruled out',
  neutral: 'Neutral',
}

export default function MetricTree({ decomposition = [], animate = true }) {
  return (
    <div className={`metric-tree ${animate ? 'fade-in' : ''}`}>
      <div className="metric-tree-rail" aria-hidden />
      <ul className="metric-tree-list">
        {decomposition.map((node, index) => (
          <li
            key={node.factor}
            className={`metric-node status-${node.status}`}
            style={{ animationDelay: `${index * 70}ms` }}
          >
            <div className="metric-node-top">
              <span className="metric-label">{node.label}</span>
              <StatusPill status={node.status}>{STATUS_LABEL[node.status]}</StatusPill>
            </div>
            <div className="metric-node-stats mono">
              <span>
                {formatFactorValue(node.factor, node.baseline)} to{' '}
                {formatFactorValue(node.factor, node.observed)}
              </span>
              <span className={node.deltaPct < 0 ? 'neg' : 'pos'}>
                {formatSignedPct(node.deltaPct)}
              </span>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}
