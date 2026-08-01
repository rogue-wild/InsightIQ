/** Shared display helpers — avoid raw Unicode math symbols and scientific notation. */

const UTC_DAY = {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
  timeZone: 'UTC',
}

const UTC_DAY_TIME = {
  ...UTC_DAY,
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
}

/** Format an investigation window as a single UTC calendar day when possible. */
export function formatWindow(start, end, { withTime = false } = {}) {
  const s = new Date(start)
  const e = new Date(end)
  const opts = withTime ? UTC_DAY_TIME : UTC_DAY
  const sDay = s.toLocaleDateString('en-GB', { ...UTC_DAY })
  const eDay = e.toLocaleDateString('en-GB', { ...UTC_DAY })
  if (!withTime || sDay === eDay) {
    // Same UTC day (or date-only mode): show one date
    if (!withTime) return sDay
    return `${s.toLocaleString('en-GB', opts)} – ${e.toLocaleString('en-GB', opts)} UTC`
  }
  return `${s.toLocaleString('en-GB', opts)} – ${e.toLocaleString('en-GB', opts)} UTC`
}

export function formatMetric(metric) {
  const labels = {
    revenue: 'Revenue',
    fill_rate: 'Fill rate',
    render_rate: 'Render rate',
    requests: 'Requests',
    impressions: 'Impressions',
    ctr: 'CTR',
    ecpm: 'eCPM',
    rpr: 'Revenue per request',
  }
  return labels[metric] || metric.replaceAll('_', ' ')
}

/**
 * Human labels for how “expected” was computed.
 * - same weekday: compare this Saturday to prior Saturdays (avoids weekend false alarms)
 * - trailing 7d: compare to the average of the last 7 days
 * - same hour 4w: compare this hour to the same hour over the prior 4 weeks
 */
const BASELINE_KINDS = {
  same_weekday_trailing: {
    label: 'vs prior same weekdays',
    hint: 'Compared to the same weekday in recent weeks (e.g. this Saturday vs prior Saturdays), so normal weekend softness is not treated as an incident.',
  },
  trailing_7d: {
    label: 'vs last 7 days',
    hint: 'Compared to the average of the trailing 7 calendar days.',
  },
  same_hour_4w_seasonality: {
    label: 'vs same hour, prior 4 weeks',
    hint: 'Compared to the same hour-of-day over the previous 4 weeks (seasonality-aware).',
  },
}

export function formatBaselineKind(kind) {
  if (!kind) return { label: 'baseline', hint: '' }
  const key = String(kind)
  if (BASELINE_KINDS[key]) return BASELINE_KINDS[key]
  return {
    label: key.replaceAll('_', ' '),
    hint: 'How the expected/baseline value was computed for this alert.',
  }
}

export const ALERT_CATEGORIES = [
  { id: 'all', label: 'All' },
  { id: 'geo', label: 'Geo', hint: 'Region and country' },
  { id: 'os', label: 'OS', hint: 'OS version' },
  { id: 'campaign_type', label: 'Campaign type' },
  { id: 'ad_format', label: 'Ad format' },
  { id: 'publisher_tier', label: 'Publisher tier' },
]

export function categoryLabel(id) {
  return ALERT_CATEGORIES.find((c) => c.id === id)?.label || String(id || '').replaceAll('_', ' ')
}

export function formatSignedPct(value) {
  const n = Number(value) || 0
  const sign = n > 0 ? '+' : ''
  return `${sign}${n.toFixed(1)}%`
}

export function formatFactorValue(factor, value) {
  const n = Number(value) || 0
  if (factor === 'requests' || factor === 'impressions' || factor === 'clicks') {
    return Math.round(n).toLocaleString('en-US')
  }
  if (factor === 'ecpm') return `$${n.toFixed(2)}`
  if (factor === 'fill_rate' || factor === 'render_rate' || factor === 'ctr') {
    return `${(n * 100).toFixed(1)}%`
  }
  if (Math.abs(n) >= 1000) return Math.round(n).toLocaleString('en-US')
  return n.toFixed(2)
}

/** Soften backend text that still contains Δ / → / scientific notation. */
export function polishSummary(text) {
  if (!text) return ''
  return String(text)
    .replaceAll('Δ', 'change')
    .replaceAll('→', 'to')
    .replace(/(\d+\.\d+)e\+(\d+)/gi, (_, coeff, exp) => {
      const n = Number(`${coeff}e+${exp}`)
      return Number.isFinite(n) ? Math.round(n).toLocaleString('en-US') : _
    })
    .replace(/(\d+\.\d+)e-(\d+)/gi, (_, coeff, exp) => {
      const n = Number(`${coeff}e-${exp}`)
      return Number.isFinite(n) ? n.toFixed(4) : _
    })
}
