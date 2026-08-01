import './instrumentation.js'
import cors from 'cors'
import express from 'express'
import {
  propagateAttributes,
  startActiveObservation,
} from '@langfuse/tracing'
import {
  alerts as mockAlerts,
  getAlertById as getMockAlert,
  getInvestigationById as getMockInvestigation,
  getInvestigationForAlert as getMockInvestigationForAlert,
  investigations as mockInvestigations,
} from '../../../packages/contracts/sample-investigations.js'
import { flushLangfuse } from './instrumentation.js'

const app = express()
const port = Number(process.env.PORT || 4000)
const geminiKey = process.env.GEMINI_API_KEY || ''
const geminiModel = process.env.GEMINI_MODEL || 'gemini-flash-lite-latest'
const engineUrl = (process.env.ENGINE_URL || 'http://127.0.0.1:4100').replace(/\/$/, '')
const useEngine = (process.env.USE_ENGINE || 'true') === 'true'
const langfuseEnabled = Boolean(
  process.env.LANGFUSE_PUBLIC_KEY && process.env.LANGFUSE_SECRET_KEY,
)

const invCache = new Map()

app.use(cors())
app.use(express.json({ limit: '1mb' }))

app.get('/health', async (_req, res) => {
  let engine = null
  if (useEngine) {
    try {
      engine = await fetchJSON(`${engineUrl}/health`)
    } catch (err) {
      engine = { ok: false, error: err.message }
    }
  }
  res.json({
    ok: true,
    service: 'insightiq-api',
    gemini: Boolean(geminiKey),
    model: geminiKey ? geminiModel : null,
    useEngine,
    engine,
    langfuse: langfuseEnabled,
    langfuseBaseUrl: process.env.LANGFUSE_BASE_URL || null,
  })
})

app.get('/api/alerts', async (_req, res) => {
  if (!useEngine) return res.json(mockAlerts)
  try {
    const live = await fetchJSON(`${engineUrl}/alerts`)
    if (Array.isArray(live) && live.length) return res.json(live)
  } catch (err) {
    console.warn('engine alerts failed, using mocks', err.message)
  }
  res.json(mockAlerts)
})

app.get('/api/alerts/:alertId', async (req, res) => {
  const list = useEngine
    ? await fetchJSON(`${engineUrl}/alerts`).catch(() => mockAlerts)
    : mockAlerts
  const alert = (list || []).find((a) => a.id === req.params.alertId) || getMockAlert(req.params.alertId)
  if (!alert) return res.status(404).json({ error: 'alert_not_found' })
  res.json(alert)
})

app.get('/api/alerts/:alertId/investigation', async (req, res) => {
  try {
    const inv = await investigationForAlert(req.params.alertId)
    if (!inv) return res.status(404).json({ error: 'investigation_not_found' })
    res.json(inv)
  } catch (err) {
    console.error(err)
    res.status(500).json({ error: 'investigation_failed', detail: err.message })
  }
})

app.get('/api/investigations/:investigationId', async (req, res) => {
  const id = req.params.investigationId
  if (invCache.has(id)) return res.json(invCache.get(id))
  if (useEngine) {
    try {
      const inv = await fetchJSON(`${engineUrl}/investigations/${id}`)
      invCache.set(id, inv)
      return res.json(inv)
    } catch (err) {
      // Engine may 404 on cold cache; rebuild via POST /investigate from id.
      try {
        const body = requestFromInvestigationId(id)
        if (body) {
          const inv = await runEngineInvestigate(body)
          return res.json(inv)
        }
      } catch (rebuildErr) {
        console.warn('investigation rebuild failed', rebuildErr.message || err.message)
      }
    }
  }
  const mock = getMockInvestigation(id)
  if (!mock) return res.status(404).json({ error: 'investigation_not_found' })
  res.json(mock)
})

app.get('/api/investigations/:investigationId/export', async (req, res) => {
  const id = req.params.investigationId
  try {
    if (useEngine) {
      try {
        const bundle = await fetchJSON(`${engineUrl}/investigations/${encodeURIComponent(id)}/export`)
        res.setHeader(
          'Content-Disposition',
          `attachment; filename="${id}-unseen-export.json"`,
        )
        return res.json(bundle)
      } catch (err) {
        console.warn('engine export failed, building locally', err.message)
      }
    }
    let inv = invCache.get(id) || getMockInvestigation(id)
    if (!inv && useEngine) {
      const body = requestFromInvestigationId(id)
      if (body) inv = await runEngineInvestigate(body)
    }
    if (!inv) return res.status(404).json({ error: 'investigation_not_found' })
    const bundle = {
      exportedAt: new Date().toISOString(),
      purpose: 'unseen-incident-submission',
      investigation: inv,
      immutableTrace: inv.trace || [],
      evidenceHash: inv.evidence?.hash || null,
      evidence: inv.evidence || null,
      seasonality: inv.seasonality || null,
      waterfall: inv.waterfall || [],
      counterfactual: inv.counterfactual || null,
      hypotheses: inv.hypotheses || [],
    }
    res.setHeader('Content-Disposition', `attachment; filename="${id}-unseen-export.json"`)
    res.json(bundle)
  } catch (err) {
    console.error(err)
    res.status(500).json({ error: 'export_failed', detail: err.message })
  }
})

app.post('/api/investigate', async (req, res) => {
  try {
    const inv = await startActiveObservation(
      'investigate-alert',
      async (span) => {
        span.update({
          input: {
            alertId: req.body?.alertId || null,
            metric: req.body?.metric || null,
          },
        })
        const out = await runEngineInvestigate(req.body || {})
        span.update({
          output: {
            investigationId: out?.id,
            advertiserId: out?.alert?.advertiserId,
            metric: out?.alert?.metric,
            status: out?.status,
          },
        })
        return out
      },
      { asType: 'span' },
    )
    await flushLangfuse()
    res.json(inv)
  } catch (err) {
    console.error(err)
    res.status(500).json({ error: 'investigate_failed', detail: err.message })
  }
})

app.get('/api/dashboard/meta', async (_req, res) => {
  if (!useEngine) {
    return res.json({
      metrics: [
        { id: 'revenue', label: 'Revenue' },
        { id: 'requests', label: 'Requests' },
        { id: 'fill_rate', label: 'Fill rate' },
        { id: 'ecpm', label: 'eCPM' },
        { id: 'ctr', label: 'CTR' },
      ],
      dimensions: [
        { id: 'ad_format', label: 'Ad format' },
        { id: 'region', label: 'Region' },
        { id: 'country', label: 'Country' },
        { id: 'os_version', label: 'OS' },
        { id: 'campaign_type', label: 'Campaign type' },
        { id: 'publisher_tier', label: 'Publisher tier' },
      ],
    })
  }
  try {
    res.json(await fetchJSON(`${engineUrl}/dashboard/meta`))
  } catch (err) {
    res.status(502).json({ error: 'dashboard_meta_failed', detail: err.message })
  }
})

app.post('/api/dashboard/query', async (req, res) => {
  if (!useEngine) {
    return res.status(503).json({ error: 'engine_required' })
  }
  try {
    const out = await fetchJSON(`${engineUrl}/dashboard/query`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req.body || {}),
    })
    res.json(out)
  } catch (err) {
    console.error(err)
    res.status(502).json({ error: 'dashboard_query_failed', detail: err.message })
  }
})

app.get('/api/dashboard/filters', async (req, res) => {
  if (!useEngine) return res.json({ dimension: req.query.dimension, values: [] })
  try {
    const qs = new URLSearchParams({
      dimension: String(req.query.dimension || ''),
      start: String(req.query.start || ''),
      end: String(req.query.end || ''),
    }).toString()
    res.json(await fetchJSON(`${engineUrl}/dashboard/filters?${qs}`))
  } catch (err) {
    res.status(502).json({ error: 'dashboard_filters_failed', detail: err.message })
  }
})

app.get('/v1/models', (_req, res) => {
  res.json({
    object: 'list',
    data: [{ id: 'insightiq-rca', object: 'model', owned_by: 'insightiq' }],
  })
})

app.post('/v1/chat/completions', async (req, res) => {
  try {
    const messages = req.body?.messages || []
    const lastUser = [...messages].reverse().find((m) => m.role === 'user')
    const content = typeof lastUser?.content === 'string' ? lastUser.content : ''
    const investigationId = req.body?.investigationId || ''
    const alertId = req.body?.alertId || ''
    const sessionId =
      req.body?.sessionId ||
      req.headers['x-session-id'] ||
      `insightiq-chat-${Date.now()}`

    const payload = await propagateAttributes(
      {
        traceName: 'chat-completion',
        sessionId: String(sessionId),
        tags: ['insightiq', 'chat'],
        metadata: {
          feature: 'chat',
          hasInvestigationContext: investigationId || alertId ? 'true' : 'false',
        },
      },
      async () =>
        startActiveObservation(
          'handle-chat-completion',
          async (span) => {
            span.update({
              input: {
                question: content,
                investigationId: investigationId || null,
                alertId: alertId || null,
              },
            })

            const slice = detectDashboardIntent(content)
            let reply
            let mode = 'investigation'
            if (slice) {
              mode = 'dashboard'
              const dash = await fetchDashboardEvidence(slice)
              reply = await narrateFromEvidence(content, null, {
                kind: 'dashboard',
                label: slice.label,
                filters: slice.filters,
                window: dash.window,
                granularity: dash.granularity,
                totals: dash.totals,
                deltas: dash.deltas,
                breakdown: dash.breakdown,
                query: dash.query,
              })
            } else {
              const investigation = await resolveInvestigation(content, {
                investigationId,
                alertId,
              })
              reply = await narrateFromEvidence(content, investigation)
            }

            span.update({
              output: {
                mode,
                replyPreview: String(reply || '').slice(0, 400),
              },
              metadata: {
                mode,
                filters: slice?.filters || null,
              },
            })

            return {
              id: `chatcmpl-${Date.now()}`,
              object: 'chat.completion',
              created: Math.floor(Date.now() / 1000),
              model: req.body?.model || 'insightiq-rca',
              choices: [
                {
                  index: 0,
                  message: { role: 'assistant', content: reply },
                  finish_reason: 'stop',
                },
              ],
              usage: { prompt_tokens: 0, completion_tokens: 0, total_tokens: 0 },
              _reply: reply,
            }
          },
          { asType: 'span' },
        ),
    )

    await flushLangfuse()

    const reply = payload._reply
    delete payload._reply

    if (req.body?.stream) {
      res.setHeader('Content-Type', 'text/event-stream')
      res.write(
        `data: ${JSON.stringify({
          id: payload.id,
          object: 'chat.completion.chunk',
          created: payload.created,
          model: payload.model,
          choices: [
            { index: 0, delta: { role: 'assistant', content: reply }, finish_reason: null },
          ],
        })}\n\n`,
      )
      res.write(
        `data: ${JSON.stringify({
          id: payload.id,
          object: 'chat.completion.chunk',
          created: payload.created,
          model: payload.model,
          choices: [{ index: 0, delta: {}, finish_reason: 'stop' }],
        })}\n\n`,
      )
      res.write('data: [DONE]\n\n')
      return res.end()
    }

    res.json(payload)
  } catch (err) {
    console.error('chat completions error', err)
    res.status(500).json({ error: { message: err.message || 'chat_failed' } })
  }
})

const REGION_ALIASES = [
  { re: /\bapac\b|\basia[-\s]?pacific\b/i, value: 'APAC', label: 'APAC' },
  { re: /\bnam\b|\bnorth\s+america\b/i, value: 'NAM', label: 'NAM' },
  { re: /\beu\b|\beurope\b/i, value: 'EU', label: 'EU' },
  { re: /\blatam\b|\blatin\s+america\b/i, value: 'LATAM', label: 'LATAM' },
  { re: /\bmea\b|\bmiddle\s+east\b/i, value: 'MEA', label: 'MEA' },
]

const COUNTRY_ALIASES = [
  { re: /\bindia\b|\bcountry\s*[:=]?\s*in\b/i, value: 'IN', label: 'India (IN)' },
  { re: /\bphilippines?\b|\bcountry\s*[:=]?\s*ph\b/i, value: 'PH', label: 'Philippines (PH)' },
  { re: /\bindonesia\b|\bcountry\s*[:=]?\s*id\b/i, value: 'ID', label: 'Indonesia (ID)' },
  { re: /\bjapan\b|\bcountry\s*[:=]?\s*jp\b/i, value: 'JP', label: 'Japan (JP)' },
  { re: /\bunited\s+states\b|\b\busa\b|\bcountry\s*[:=]?\s*us\b/i, value: 'US', label: 'United States (US)' },
]

const OS_VERSIONS = [
  'iOS 18.1',
  'iOS 17.5',
  'iOS 17.2',
  'iOS 16.4',
  'Android 15',
  'Android 14',
  'Android 13',
  'Android 12',
]

const AD_FORMATS = ['video', 'native', 'interstitial', 'rewarded', 'banner']
const CAMPAIGN_TYPES = ['CPM', 'CPC', 'CPI']
const PUBLISHER_TIERS = ['tier_1', 'tier_2', 'tier_3']

/** Build combined dashboard filters from a natural-language question (all matches, not first-only). */
function detectDashboardIntent(text) {
  const q = String(text || '')
  if (!q.trim()) return null

  const filters = {}
  const labels = []

  for (const r of REGION_ALIASES) {
    if (r.re.test(q)) {
      filters.region = [r.value]
      labels.push(`region=${r.label}`)
      break
    }
  }
  for (const c of COUNTRY_ALIASES) {
    if (c.re.test(q)) {
      filters.country = [c.value]
      labels.push(`country=${c.label}`)
      break
    }
  }

  const qLower = q.toLowerCase()
  for (const os of OS_VERSIONS) {
    if (qLower.includes(os.toLowerCase())) {
      filters.os_version = [os]
      labels.push(`os_version=${os}`)
      break
    }
  }

  for (const fmt of AD_FORMATS) {
    const re = new RegExp(`\\b${fmt}\\b`, 'i')
    if (re.test(q)) {
      filters.ad_format = [fmt]
      labels.push(`ad_format=${fmt}`)
      break
    }
  }

  for (const ct of CAMPAIGN_TYPES) {
    const re = new RegExp(`\\b${ct}\\b`, 'i')
    if (re.test(q)) {
      filters.campaign_type = [ct]
      labels.push(`campaign_type=${ct}`)
      break
    }
  }

  for (const tier of PUBLISHER_TIERS) {
    const pretty = tier.replace('_', ' ')
    if (qLower.includes(tier) || qLower.includes(pretty)) {
      filters.publisher_tier = [tier]
      labels.push(`publisher_tier=${tier}`)
      break
    }
  }

  if (!Object.keys(filters).length) return null

  const filterKeys = Object.keys(filters)
  const narrow = filterKeys.length > 1 || Boolean(filters.os_version || filters.ad_format || filters.campaign_type || filters.publisher_tier)

  // Match Dashboard default demo day for precise filter questions; keep a week window for broad geo-only asks.
  const window = narrow ? demoDayWindow() : demoWeekWindow()
  const dimensions = ['ad_format', 'country', 'os_version', 'campaign_type', 'publisher_tier', 'category'].filter(
    (d) => !filters[d],
  )

  return {
    filters,
    label: labels.join(', '),
    dimensions: dimensions.length ? dimensions : ['ad_format'],
    window,
    granularity: narrow ? 'hour' : 'day',
  }
}

function demoWeekWindow() {
  return {
    start: '2026-06-15T00:00:00Z',
    end: '2026-06-21T23:59:59Z',
    compare: {
      start: '2026-06-08T00:00:00Z',
      end: '2026-06-14T23:59:59Z',
    },
  }
}

function demoDayWindow() {
  return {
    start: '2026-06-21T00:00:00Z',
    end: '2026-06-21T23:59:59Z',
    compare: {
      start: '2026-06-20T00:00:00Z',
      end: '2026-06-20T23:59:59Z',
    },
  }
}

async function fetchDashboardEvidence(slice) {
  return startActiveObservation(
    'retrieve-dashboard-evidence',
    async (obs) => {
      const window = slice.window || demoDayWindow()
      const body = {
        start: window.start,
        end: window.end,
        compare: window.compare,
        granularity: slice.granularity || 'hour',
        metrics: ['revenue', 'requests', 'fill_rate', 'ecpm'],
        dimensions: slice.dimensions || ['ad_format'],
        filters: slice.filters,
        limit: 10,
      }
      obs.update({ input: { filters: body.filters, window, granularity: body.granularity } })
      const out = await fetchJSON(`${engineUrl}/dashboard/query`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const breakdown = {}
      for (const dim of slice.dimensions || []) {
        breakdown[dim] = (out.tables?.[dim] || []).slice(0, 5)
      }
      const result = {
        window,
        granularity: body.granularity,
        totals: out.totals || {},
        deltas: out.deltas || {},
        breakdown,
        query: body,
      }
      obs.update({ output: { totals: result.totals, deltas: result.deltas } })
      return result
    },
    { asType: 'retriever' },
  )
}

let defaultInvestigationPromise = null

async function getDefaultInvestigation() {
  if (!useEngine) return mockInvestigations['inv-001']
  if (!defaultInvestigationPromise) {
    defaultInvestigationPromise = (async () => {
      try {
        const alerts = await fetchJSON(`${engineUrl}/alerts`)
        // Prefer an alert that already has category/segment narration (usually adv_0000 demo).
        const pick =
          (Array.isArray(alerts) && alerts.find((a) => a.advertiserId === 'adv_0000')) ||
          (Array.isArray(alerts) && alerts[0])
        if (pick?.id) {
          return await runEngineInvestigate({ alertId: pick.id })
        }
      } catch (err) {
        console.warn('default investigation failed', err.message)
      }
      return mockInvestigations['inv-001']
    })()
  }
  return defaultInvestigationPromise
}

async function investigationForAlert(alertId) {
  const mock = getMockInvestigationForAlert(alertId)
  if (!useEngine) return mock

  const uuid = String(alertId || '').replace(/^inv-/i, '')
  const isUUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(uuid)
  if (isUUID) {
    try {
      return await runEngineInvestigate({ alertId: uuid })
    } catch (err) {
      console.warn('engine investigate failed, mock fallback', err.message)
      return mock
    }
  }

  // Legacy demo ids: alert-{metric}-{YYYY-MM-DD}
  const dateMatch = alertId.match(/(\d{4}-\d{2}-\d{2})/)
  const metricMatch = alertId.match(/alert-([a-z_]+)-/)
  const body = {
    alertId,
    metric: metricMatch?.[1] || mock?.alert?.metric || 'revenue',
    windowStart: dateMatch ? `${dateMatch[1]}T00:00:00Z` : mock?.alert?.windowStart,
    windowEnd: dateMatch ? `${dateMatch[1]}T23:59:59Z` : mock?.alert?.windowEnd,
    baselineKind: mock?.alert?.baselineKind || 'same_hour_4w_seasonality',
  }
  try {
    return await runEngineInvestigate(body)
  } catch (err) {
    console.warn('engine investigate failed, mock fallback', err.message)
    return mock
  }
}

async function runEngineInvestigate(body) {
  if (!useEngine) {
    const id = body.alertId || 'alert-rev-2026-06-28'
    return getMockInvestigationForAlert(id) || mockInvestigations['inv-001']
  }
  const inv = await fetchJSON(`${engineUrl}/investigate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  invCache.set(inv.id, inv)
  return inv
}

/** Parse inv-{uuid} or inv-{metric}-{YYYYMMDD} into an investigate request body. */
function requestFromInvestigationId(id) {
  const raw = String(id || '')
  const uuid = raw.replace(/^inv-/i, '')
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(uuid)) {
    return { alertId: uuid }
  }
  const m = raw.match(/^inv-(.+)-(\d{8})$/)
  if (!m) return null
  const metric = m[1]
  const y = m[2].slice(0, 4)
  const mo = m[2].slice(4, 6)
  const d = m[2].slice(6, 8)
  const day = `${y}-${mo}-${d}`
  return {
    alertId: `alert-${metric}-${day}`,
    metric,
    windowStart: `${day}T00:00:00Z`,
    windowEnd: `${day}T23:59:59Z`,
    baselineKind: 'same_hour_4w_seasonality',
  }
}

async function resolveInvestigation(text, opts = {}) {
  const investigationId = opts.investigationId || ''
  const alertId = opts.alertId || ''

  if (investigationId) {
    if (invCache.has(investigationId)) return invCache.get(investigationId)
    try {
      if (useEngine) {
        const inv = await fetchJSON(`${engineUrl}/investigations/${investigationId}`)
        invCache.set(inv.id || investigationId, inv)
        return inv
      }
    } catch {
      /* fall through to rebuild */
    }
    try {
      const body = requestFromInvestigationId(investigationId)
      if (body) return await runEngineInvestigate(body)
    } catch {
      /* fall through */
    }
  }

  if (alertId) {
    const inv = await investigationForAlert(alertId)
    if (inv) return inv
  }

  const invMatch = text.match(/inv-[0-9a-f-]{36}|inv-[a-z0-9-]+/i)
  if (invMatch) {
    const id = invMatch[0]
    if (invCache.has(id)) return invCache.get(id)
    try {
      const body = requestFromInvestigationId(id)
      if (body) return await runEngineInvestigate(body)
    } catch {
      /* fall through */
    }
    const mock = getMockInvestigation(id)
    if (mock) return mock
  }
  const uuidMatch = text.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i)
  if (uuidMatch) {
    try {
      return await runEngineInvestigate({ alertId: uuidMatch[0] })
    } catch {
      /* fall through */
    }
  }
  const alertMatch = text.match(/alert-[a-z0-9-]+/i)
  if (alertMatch) {
    const inv = await investigationForAlert(alertMatch[0].toLowerCase())
    if (inv) return inv
  }

  // Free-form RCA questions: reuse one cached default investigation (do not re-run /alerts + investigate each turn).
  return getDefaultInvestigation()
}

function fallbackNarration(question, inv, extraEvidence) {
  if (extraEvidence?.kind === 'dashboard' || extraEvidence?.kind === 'geo') {
    const lines = [
      `Dashboard snapshot for **${extraEvidence.label}** (${extraEvidence.window?.start?.slice(0, 10)} → ${extraEvidence.window?.end?.slice(0, 10)}).`,
      `Filters: ${JSON.stringify(extraEvidence.filters || {})}`,
      '',
      `Totals: revenue=${extraEvidence.totals?.revenue ?? 'n/a'}, requests=${extraEvidence.totals?.requests ?? 'n/a'}, fill_rate=${extraEvidence.totals?.fill_rate ?? 'n/a'}, ecpm=${extraEvidence.totals?.ecpm ?? 'n/a'}.`,
    ]
    if (extraEvidence.deltas) {
      lines.push(
        `vs prior period: revenue ${fmtDelta(extraEvidence.deltas.revenue)}, requests ${fmtDelta(extraEvidence.deltas.requests)}.`,
      )
    }
    for (const [dim, rows] of Object.entries(extraEvidence.breakdown || {})) {
      if (!rows?.length) continue
      lines.push('', `Top ${dim}:`)
      for (const row of rows.slice(0, 5)) {
        lines.push(`- ${row.value ?? row[dim]}: revenue=${row.revenue ?? row.metrics?.revenue ?? 'n/a'}`)
      }
    }
    lines.push('', '_Numbers above are from metric_hourly_snapshot via the dashboard query API._')
    return lines.join('\n')
  }
  if (!inv) {
    return 'No investigation evidence found. Ask about a known alert id or investigation id.'
  }
  const lines = [
    `Investigation \`${inv.id}\` (${inv.status}).`,
    '',
    inv.diagnosis?.text || '',
    '',
    'Citations (from evidence only):',
    ...(inv.diagnosis?.citations || []).map((c) => `- ${c.label}: ${c.value}`),
  ]
  if (inv.ruledOut?.length) {
    lines.push('', 'Ruled out:')
    for (const item of inv.ruledOut) lines.push(`- ${item.reason}: ${item.detail}`)
  }
  if (/what else|follow|more|trace/i.test(question || '')) {
    lines.push('', 'Trace:')
    for (const step of inv.trace || []) {
      lines.push(`- ${step.step}: ${step.detail}${step.durationMs ? ` (${step.durationMs} ms)` : ''}`)
    }
  }
  lines.push('', '_Numbers above are copied from the investigation evidence JSON._')
  return lines.join('\n')
}

function fmtDelta(d) {
  if (!d || d.deltaPct == null) return 'n/a'
  const sign = d.deltaPct > 0 ? '+' : ''
  return `${sign}${Number(d.deltaPct).toFixed(1)}%`
}

async function narrateFromEvidence(question, inv, extraEvidence = null) {
  if (!inv && !extraEvidence) return fallbackNarration(question, inv, extraEvidence)
  if (!geminiKey) return fallbackNarration(question, inv, extraEvidence)

  return startActiveObservation(
    'narrate-with-gemini',
    async (generation) => {
      const isDash = extraEvidence?.kind === 'dashboard' || extraEvidence?.kind === 'geo'
      const evidence = isDash
        ? {
            kind: extraEvidence.kind,
            label: extraEvidence.label,
            filters: extraEvidence.filters,
            window: extraEvidence.window,
            totals: extraEvidence.totals,
            deltas: extraEvidence.deltas,
          }
        : {
            id: inv.id,
            status: inv.status,
            alert: inv.alert,
            decomposition: inv.decomposition,
            segments: inv.segments,
            ruledOut: inv.ruledOut,
            diagnosis: inv.diagnosis,
          }

      const system = [
        'You are InsightIQ, an automated analytics narrator.',
        'You MUST only use numbers and facts present in the provided evidence JSON.',
        'Never invent metrics, segments, or percentages.',
        isDash
          ? 'This evidence is a dashboard query result for the exact filters listed. Answer using totals.revenue / totals.requests / etc. for that filter intersection. Do not ignore filters like os_version. Cite the filter set and date window.'
          : 'This evidence is a root-cause investigation package.',
        'Keep the answer concise and plain-English for a hackathon demo.',
      ].join(' ')

      const userPrompt = `User question: ${question || 'Explain this investigation.'}\n\nEvidence JSON:\n${JSON.stringify(evidence, null, 2)}`
      generation.update({
        input: [
          { role: 'system', content: system },
          { role: 'user', content: userPrompt },
        ],
        model: geminiModel,
        metadata: {
          evidenceKind: isDash ? 'dashboard' : 'investigation',
          investigationId: inv?.id || null,
          filters: extraEvidence?.filters || null,
        },
      })

      const url = `https://generativelanguage.googleapis.com/v1beta/models/${geminiModel}:generateContent?key=${encodeURIComponent(geminiKey)}`
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          systemInstruction: { parts: [{ text: system }] },
          contents: [{ role: 'user', parts: [{ text: userPrompt }] }],
          generationConfig: { temperature: 0.2, maxOutputTokens: 512 },
        }),
      })

      if (!response.ok) {
        const body = await response.text()
        console.error('Gemini error', response.status, body.slice(0, 500))
        const fallback = `${fallbackNarration(question, inv, extraEvidence)}\n\n_(Gemini unavailable — showed evidence fallback.)_`
        generation.update({
          output: fallback,
          level: 'ERROR',
          statusMessage: `gemini_http_${response.status}`,
        })
        return fallback
      }

      const data = await response.json()
      const textOut = data?.candidates?.[0]?.content?.parts?.map((p) => p.text).filter(Boolean).join('\n')
      const reply = textOut?.trim() || fallbackNarration(question, inv, extraEvidence)
      const usage = data?.usageMetadata || {}
      generation.update({
        output: reply,
        usageDetails: {
          input: usage.promptTokenCount || 0,
          output: usage.candidatesTokenCount || 0,
          total: usage.totalTokenCount || 0,
        },
      })
      return reply
    },
    { asType: 'generation' },
  )
}

async function fetchJSON(url, options) {
  const res = await fetch(url, options)
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${url} -> ${res.status} ${body.slice(0, 200)}`)
  }
  return res.json()
}

app.listen(port, () => {
  console.log(
    `InsightIQ API listening on http://localhost:${port} (gemini=${geminiKey ? geminiModel : 'off'}, engine=${useEngine ? engineUrl : 'off'}, langfuse=${langfuseEnabled ? 'on' : 'off'})`,
  )
})

process.on('SIGTERM', async () => {
  await flushLangfuse()
  process.exit(0)
})
process.on('SIGINT', async () => {
  await flushLangfuse()
  process.exit(0)
})
