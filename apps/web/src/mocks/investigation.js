/** Sample alerts + investigations matching packages/contracts/investigation.schema.json */

export const alerts = [
  {
    id: 'alert-rev-2026-06-28',
    metric: 'revenue',
    direction: 'down',
    pctChange: -14.2,
    windowStart: '2026-06-28T00:00:00Z',
    windowEnd: '2026-06-28T23:59:59Z',
    baselineKind: 'same_weekday_trailing',
    severity: 'critical',
    investigationId: 'inv-001',
    status: 'complete',
    summary: 'Revenue drop localized to rewarded ads on Android 15 in India',
  },
  {
    id: 'alert-fill-2026-06-21',
    metric: 'fill_rate',
    direction: 'down',
    pctChange: -8.6,
    windowStart: '2026-06-21T00:00:00Z',
    windowEnd: '2026-06-21T23:59:59Z',
    baselineKind: 'same_weekday_trailing',
    severity: 'high',
    investigationId: 'inv-002',
    status: 'complete',
    summary: 'Fill-rate dip concentrated in LATAM interstitial inventory',
  },
  {
    id: 'alert-rev-2026-06-14',
    metric: 'revenue',
    direction: 'down',
    pctChange: -6.1,
    windowStart: '2026-06-14T00:00:00Z',
    windowEnd: '2026-06-14T23:59:59Z',
    baselineKind: 'same_weekday_trailing',
    severity: 'medium',
    investigationId: 'inv-003',
    status: 'complete',
    summary: 'Weekend seasonality — ruled out as anomaly',
  },
  {
    id: 'alert-ctr-2026-07-02',
    metric: 'ctr',
    direction: 'up',
    pctChange: 11.4,
    windowStart: '2026-07-02T00:00:00Z',
    windowEnd: '2026-07-02T23:59:59Z',
    baselineKind: 'trailing_7d',
    severity: 'low',
    investigationId: 'inv-004',
    status: 'running',
    summary: 'CTR spike under investigation',
  },
]

export const investigations = {
  'inv-001': {
    id: 'inv-001',
    status: 'complete',
    alert: {
      id: 'alert-rev-2026-06-28',
      metric: 'revenue',
      direction: 'down',
      pctChange: -14.2,
      windowStart: '2026-06-28T00:00:00Z',
      windowEnd: '2026-06-28T23:59:59Z',
      baselineKind: 'same_weekday_trailing',
      severity: 'critical',
    },
    decomposition: [
      {
        factor: 'requests',
        label: 'Requests',
        status: 'ruled_out',
        baseline: 4_820_000,
        observed: 4_791_000,
        deltaPct: -0.6,
      },
      {
        factor: 'fill_rate',
        label: 'Fill rate',
        status: 'culprit',
        baseline: 0.912,
        observed: 0.781,
        deltaPct: -14.4,
      },
      {
        factor: 'render_rate',
        label: 'Render rate',
        status: 'ruled_out',
        baseline: 0.987,
        observed: 0.984,
        deltaPct: -0.3,
      },
      {
        factor: 'ecpm',
        label: 'eCPM',
        status: 'neutral',
        baseline: 2.84,
        observed: 2.79,
        deltaPct: -1.8,
      },
      {
        factor: 'ctr',
        label: 'CTR',
        status: 'ruled_out',
        baseline: 0.0182,
        observed: 0.0179,
        deltaPct: -1.6,
      },
    ],
    segments: [
      {
        dimension: 'ad_format',
        value: 'rewarded',
        metric: 'fill_rate',
        deltaPct: -30.9,
        contributionPct: 61.2,
      },
      {
        dimension: 'os_version',
        value: 'Android 15',
        metric: 'fill_rate',
        deltaPct: -28.4,
        contributionPct: 48.7,
      },
      {
        dimension: 'country',
        value: 'IN',
        metric: 'fill_rate',
        deltaPct: -26.1,
        contributionPct: 42.3,
      },
      {
        dimension: 'region',
        value: 'APAC',
        metric: 'fill_rate',
        deltaPct: -18.2,
        contributionPct: 55.0,
      },
      {
        dimension: 'publisher_tier',
        value: 'tier_2',
        metric: 'fill_rate',
        deltaPct: -12.4,
        contributionPct: 19.8,
      },
    ],
    ruledOut: [
      {
        reason: 'Seasonality',
        detail:
          'Same-weekday trailing baseline (prior 3 Saturdays) already accounts for weekend softness; residual revenue gap remains −14.2%.',
      },
      {
        reason: 'Request volume',
        detail: 'Requests changed only −0.6% vs baseline (4.82M → 4.79M).',
      },
      {
        reason: 'eCPM / price',
        detail: 'eCPM moved −1.8% ($2.84 → $2.79), too small to explain the revenue drop.',
      },
      {
        reason: 'CTR',
        detail: 'CTR −1.6% and is not a direct revenue factor in the CPM identity.',
      },
    ],
    diagnosis: {
      text: 'Revenue fell 14.2% because fill rate for rewarded ads on Android 15 in India decreased from 92.3% to 61.8%, reducing impressions by ~31%. Request volume and eCPM remained within normal seasonal ranges and were ruled out.',
      citations: [
        { label: 'Revenue Δ', value: '−14.2%' },
        { label: 'Fill rate (rewarded · Android 15 · IN)', value: '92.3% → 61.8%' },
        { label: 'Impressions Δ (segment)', value: '≈ −31%' },
        { label: 'Requests Δ (global)', value: '−0.6%' },
        { label: 'eCPM Δ (global)', value: '−1.8%' },
      ],
    },
    trace: [
      {
        step: 'baseline',
        detail: 'Compared 2026-06-28 against same-weekday trailing Saturdays',
        durationMs: 84,
      },
      {
        step: 'decompose',
        detail: 'Walked revenue identity: requests × fill × render × eCPM/1000',
        durationMs: 126,
      },
      {
        step: 'slice',
        detail: 'Ranked dimensions by contribution to fill-rate decline',
        durationMs: 312,
      },
      {
        step: 'evidence',
        detail: 'Built evidence JSON with culprit segments and ruled-out factors',
        durationMs: 41,
      },
      {
        step: 'narrate',
        detail: 'LLM narrated diagnosis from evidence only (no raw row dump)',
        durationMs: 690,
      },
    ],
  },
  'inv-002': {
    id: 'inv-002',
    status: 'complete',
    alert: {
      id: 'alert-fill-2026-06-21',
      metric: 'fill_rate',
      direction: 'down',
      pctChange: -8.6,
      windowStart: '2026-06-21T00:00:00Z',
      windowEnd: '2026-06-21T23:59:59Z',
      baselineKind: 'same_weekday_trailing',
      severity: 'high',
    },
    decomposition: [
      {
        factor: 'requests',
        label: 'Requests',
        status: 'ruled_out',
        baseline: 5_010_000,
        observed: 5_040_000,
        deltaPct: 0.6,
      },
      {
        factor: 'fill_rate',
        label: 'Fill rate',
        status: 'culprit',
        baseline: 0.894,
        observed: 0.817,
        deltaPct: -8.6,
      },
      {
        factor: 'render_rate',
        label: 'Render rate',
        status: 'ruled_out',
        baseline: 0.981,
        observed: 0.979,
        deltaPct: -0.2,
      },
      {
        factor: 'ecpm',
        label: 'eCPM',
        status: 'neutral',
        baseline: 2.41,
        observed: 2.38,
        deltaPct: -1.2,
      },
      {
        factor: 'ctr',
        label: 'CTR',
        status: 'ruled_out',
        baseline: 0.0164,
        observed: 0.0161,
        deltaPct: -1.8,
      },
    ],
    segments: [
      {
        dimension: 'region',
        value: 'LATAM',
        metric: 'fill_rate',
        deltaPct: -22.1,
        contributionPct: 58.4,
      },
      {
        dimension: 'ad_format',
        value: 'interstitial',
        metric: 'fill_rate',
        deltaPct: -19.7,
        contributionPct: 44.1,
      },
      {
        dimension: 'country',
        value: 'BR',
        metric: 'fill_rate',
        deltaPct: -24.3,
        contributionPct: 31.6,
      },
    ],
    ruledOut: [
      {
        reason: 'Seasonality',
        detail: 'Same-weekday baseline already applied; residual fill-rate gap −8.6%.',
      },
      {
        reason: 'Request volume',
        detail: 'Requests slightly up (+0.6%); not a volume-driven fill drop.',
      },
    ],
    diagnosis: {
      text: 'Fill rate fell 8.6%, driven mainly by interstitial inventory in LATAM (especially BR), where fill rate dropped ~22–24% vs the same-weekday baseline. Request volume and render rate were ruled out.',
      citations: [
        { label: 'Fill rate Δ', value: '−8.6%' },
        { label: 'LATAM fill rate Δ', value: '−22.1%' },
        { label: 'Interstitial contribution', value: '44.1%' },
      ],
    },
    trace: [
      { step: 'baseline', detail: 'Same-weekday trailing baseline', durationMs: 72 },
      { step: 'decompose', detail: 'Isolated fill_rate as primary mover', durationMs: 98 },
      { step: 'slice', detail: 'Ranked region / format / country', durationMs: 245 },
      { step: 'evidence', detail: 'Packaged LATAM interstitial evidence', durationMs: 33 },
      { step: 'narrate', detail: 'Narrated from evidence JSON', durationMs: 540 },
    ],
  },
  'inv-003': {
    id: 'inv-003',
    status: 'complete',
    alert: {
      id: 'alert-rev-2026-06-14',
      metric: 'revenue',
      direction: 'down',
      pctChange: -6.1,
      windowStart: '2026-06-14T00:00:00Z',
      windowEnd: '2026-06-14T23:59:59Z',
      baselineKind: 'same_weekday_trailing',
      severity: 'medium',
    },
    decomposition: [
      {
        factor: 'requests',
        label: 'Requests',
        status: 'ruled_out',
        baseline: 4_200_000,
        observed: 3_980_000,
        deltaPct: -5.2,
      },
      {
        factor: 'fill_rate',
        label: 'Fill rate',
        status: 'ruled_out',
        baseline: 0.901,
        observed: 0.898,
        deltaPct: -0.3,
      },
      {
        factor: 'render_rate',
        label: 'Render rate',
        status: 'ruled_out',
        baseline: 0.985,
        observed: 0.986,
        deltaPct: 0.1,
      },
      {
        factor: 'ecpm',
        label: 'eCPM',
        status: 'ruled_out',
        baseline: 2.72,
        observed: 2.7,
        deltaPct: -0.7,
      },
      {
        factor: 'ctr',
        label: 'CTR',
        status: 'ruled_out',
        baseline: 0.0175,
        observed: 0.0174,
        deltaPct: -0.6,
      },
    ],
    segments: [],
    ruledOut: [
      {
        reason: 'Seasonality (primary)',
        detail:
          'Vs flat weekly average this looked like −6.1%, but same-weekday trailing Saturdays show expected weekend softness. Flag cleared.',
      },
      {
        reason: 'Segment localization',
        detail: 'No dimension contributed >15% residual after seasonality adjustment.',
      },
    ],
    diagnosis: {
      text: 'The apparent revenue drop of 6.1% matches expected Saturday seasonality against the same-weekday trailing baseline. No segment-level anomaly was localized; this movement should be ruled out, not alerted.',
      citations: [
        { label: 'Vs flat average', value: '−6.1%' },
        { label: 'Vs same-weekday baseline', value: 'within noise' },
        { label: 'Max segment contribution', value: '< 15%' },
      ],
    },
    trace: [
      { step: 'baseline', detail: 'Compared flat avg vs same-weekday', durationMs: 90 },
      { step: 'decompose', detail: 'No single factor outside seasonal band', durationMs: 110 },
      { step: 'slice', detail: 'No dominant segment contribution', durationMs: 200 },
      { step: 'evidence', detail: 'Marked as seasonality false positive', durationMs: 25 },
      { step: 'narrate', detail: 'Explained ruled-out outcome', durationMs: 480 },
    ],
  },
  'inv-004': {
    id: 'inv-004',
    status: 'running',
    alert: {
      id: 'alert-ctr-2026-07-02',
      metric: 'ctr',
      direction: 'up',
      pctChange: 11.4,
      windowStart: '2026-07-02T00:00:00Z',
      windowEnd: '2026-07-02T23:59:59Z',
      baselineKind: 'trailing_7d',
      severity: 'low',
    },
    decomposition: [
      {
        factor: 'requests',
        label: 'Requests',
        status: 'neutral',
        baseline: 5_100_000,
        observed: 5_150_000,
        deltaPct: 1.0,
      },
      {
        factor: 'fill_rate',
        label: 'Fill rate',
        status: 'neutral',
        baseline: 0.905,
        observed: 0.908,
        deltaPct: 0.3,
      },
      {
        factor: 'render_rate',
        label: 'Render rate',
        status: 'neutral',
        baseline: 0.984,
        observed: 0.985,
        deltaPct: 0.1,
      },
      {
        factor: 'ecpm',
        label: 'eCPM',
        status: 'neutral',
        baseline: 2.9,
        observed: 2.92,
        deltaPct: 0.7,
      },
      {
        factor: 'ctr',
        label: 'CTR',
        status: 'culprit',
        baseline: 0.0171,
        observed: 0.0191,
        deltaPct: 11.4,
      },
    ],
    segments: [
      {
        dimension: 'ad_format',
        value: 'native',
        metric: 'ctr',
        deltaPct: 18.2,
        contributionPct: 40.1,
      },
    ],
    ruledOut: [],
    diagnosis: {
      text: 'Investigation still running. Preliminary signal: CTR up 11.4%, with native format contributing ~40% of the lift.',
      citations: [
        { label: 'CTR Δ', value: '+11.4%' },
        { label: 'Native contribution', value: '40.1%' },
      ],
    },
    trace: [
      { step: 'baseline', detail: 'Trailing 7-day baseline computed', durationMs: 60 },
      { step: 'decompose', detail: 'CTR isolated as mover', durationMs: 95 },
      { step: 'slice', detail: 'Partial dimension ranking…', durationMs: 0 },
    ],
  },
}

export function getAlertById(alertId) {
  return alerts.find((a) => a.id === alertId) ?? null
}

export function getInvestigationById(investigationId) {
  return investigations[investigationId] ?? null
}

export function getInvestigationForAlert(alertId) {
  const alert = getAlertById(alertId)
  if (!alert) return null
  return getInvestigationById(alert.investigationId)
}
