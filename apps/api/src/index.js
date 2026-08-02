import './instrumentation.js'
import cors from 'cors'
import express from 'express'
import {
  propagateAttributes,
  startActiveObservation,
} from '@langfuse/tracing'
import { flushLangfuse } from './instrumentation.js'
import { createClickHouse, createEngine, queryMaps } from './engine/index.js'

const app = express()
const port = Number(process.env.PORT || 4000)
const geminiKey = process.env.GEMINI_API_KEY || ''
const geminiModel = process.env.GEMINI_MODEL || 'gemini-flash-lite-latest'
const langfuseEnabled = Boolean(
  process.env.LANGFUSE_PUBLIC_KEY && process.env.LANGFUSE_SECRET_KEY,
)

/** @type {Awaited<ReturnType<typeof createEngine>> | null} */
let engine = null
const invCache = new Map()

app.use(cors())
app.use(express.json({ limit: '1mb' }))

function requireEngine(_req, res, next) {
  if (!engine) return res.status(503).json({ error: 'engine_not_ready' })
  next()
}

app.get('/health', async (_req, res) => {
  let clickhouse = { ok: false }
  try {
    if (engine?.client) {
      const rows = await queryMaps(engine.client, 'SELECT count() AS n FROM alerts_live')
      clickhouse = {
        ok: true,
        database: process.env.CLICKHOUSE_DATABASE || 'insightiq',
        alerts: Number(rows?.[0]?.n || 0),
      }
    }
  } catch (err) {
    clickhouse = { ok: false, error: err.message }
  }
  res.json({
    ok: Boolean(engine) && clickhouse.ok,
    service: 'insightiq-api',
    gemini: Boolean(geminiKey),
    model: geminiKey ? geminiModel : null,
    clickhouse,
    langfuse: langfuseEnabled,
    langfuseBaseUrl: process.env.LANGFUSE_BASE_URL || null,
  })
})

function alertsGranularityQuery(req) {
  const g = String(req.query.granularity || 'day').toLowerCase()
  if (g === 'hour' || g === 'hourly') return 'hour'
  return 'day'
}

app.get('/api/alerts', requireEngine, async (req, res) => {
  try {
    const granularity = alertsGranularityQuery(req)
    const live = await engine.detectAlerts(granularity)
    res.json(Array.isArray(live) ? live : [])
  } catch (err) {
    console.error('alerts failed', err.message)
    res.status(502).json({ error: 'alerts_unavailable', detail: err.message })
  }
})

app.get('/api/alerts/:alertId', requireEngine, async (req, res) => {
  try {
    const granularity = alertsGranularityQuery(req)
    const list = await engine.detectAlerts(granularity)
    const alert = (list || []).find((a) => a.id === req.params.alertId)
    if (!alert) return res.status(404).json({ error: 'alert_not_found' })
    res.json(alert)
  } catch (err) {
    console.error(err)
    res.status(502).json({ error: 'alerts_unavailable', detail: err.message })
  }
})

app.get('/api/alerts/:alertId/investigation', requireEngine, async (req, res) => {
  try {
    const inv = await investigationForAlert(req.params.alertId)
    if (!inv) return res.status(404).json({ error: 'investigation_not_found' })
    res.json(inv)
  } catch (err) {
    console.error(err)
    res.status(500).json({ error: 'investigation_failed', detail: err.message })
  }
})

app.get('/api/investigations/:investigationId', requireEngine, async (req, res) => {
  const id = req.params.investigationId
  if (invCache.has(id)) return res.json(invCache.get(id))
  try {
    const inv = await engine.getInvestigation(id)
    invCache.set(inv.id || id, inv)
    return res.json(inv)
  } catch (err) {
    console.warn('investigation failed', err.message)
    return res.status(404).json({ error: 'investigation_not_found', detail: err.message })
  }
})

app.get('/api/investigations/:investigationId/export', requireEngine, async (req, res) => {
  const id = req.params.investigationId
  try {
    const bundle = await engine.exportInvestigation(id)
    const inv = bundle.investigation
    if (inv?.id) invCache.set(inv.id, inv)
    res.setHeader('Content-Disposition', `attachment; filename="${id}-export.json"`)
    return res.json(bundle)
  } catch (err) {
    console.error(err)
    res.status(500).json({ error: 'export_failed', detail: err.message })
  }
})

app.post('/api/investigate', requireEngine, async (req, res) => {
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

app.get('/api/dashboard/meta', requireEngine, async (_req, res) => {
  try {
    res.json(await engine.getDashboardMeta())
  } catch (err) {
    res.status(502).json({ error: 'dashboard_meta_failed', detail: err.message })
  }
})

app.post('/api/dashboard/query', requireEngine, async (req, res) => {
  try {
    res.json(await engine.queryDashboard(req.body || {}))
  } catch (err) {
    console.error(err)
    res.status(502).json({ error: 'dashboard_query_failed', detail: err.message })
  }
})

app.get('/api/dashboard/filters', requireEngine, async (req, res) => {
  try {
    res.json(
      await engine.getDashboardFilters({
        dimension: String(req.query.dimension || ''),
        start: String(req.query.start || ''),
        end: String(req.query.end || ''),
      }),
    )
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
              try {
                const dash = await fetchDashboardEvidence(slice, content)
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
              } catch (err) {
                reply = [
                  `I couldn't run a live dashboard query for **${slice.label}**.`,
                  '',
                  String(err.message || err),
                ].join('\n')
              }
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
  const narrow =
    filterKeys.length > 1 ||
    Boolean(filters.os_version || filters.ad_format || filters.campaign_type || filters.publisher_tier)

  const dimensions = ['ad_format', 'country', 'os_version', 'campaign_type', 'publisher_tier', 'category'].filter(
    (d) => !filters[d],
  )

  return {
    filters,
    label: labels.join(', '),
    dimensions: dimensions.length ? dimensions : ['ad_format'],
    narrow,
    granularity: narrow ? 'hour' : 'day',
  }
}

const MONTH_INDEX = {
  jan: 0,
  january: 0,
  feb: 1,
  february: 1,
  mar: 2,
  march: 2,
  apr: 3,
  april: 3,
  may: 4,
  jun: 5,
  june: 5,
  jul: 6,
  july: 6,
  aug: 7,
  august: 7,
  sep: 8,
  sept: 8,
  september: 8,
  oct: 9,
  october: 9,
  nov: 10,
  november: 10,
  dec: 11,
  december: 11,
}

/** Parse "2026-06-21", "21 June 2026", "June 21, 2026", "21/06/2026" → YYYY-MM-DD. */
function parseOneDayToken(text) {
  const q = String(text || '')

  const iso = q.match(/\b(\d{4})-(\d{2})-(\d{2})\b/)
  if (iso) return `${iso[1]}-${iso[2]}-${iso[3]}`

  const dmySlash = q.match(/\b(\d{1,2})[\/\-.](\d{1,2})[\/\-.](\d{4})\b/)
  if (dmySlash) {
    const d = Number(dmySlash[1])
    const m = Number(dmySlash[2])
    const y = Number(dmySlash[3])
    // Prefer D/M/Y for slash-separated dates.
    if (m >= 1 && m <= 12 && d >= 1 && d <= 31) {
      return `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    }
  }

  const dMonthY = q.match(
    /\b(\d{1,2})(?:st|nd|rd|th)?\s+(Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s*,?\s*(\d{4})\b/i,
  )
  if (dMonthY) {
    const d = Number(dMonthY[1])
    const m = MONTH_INDEX[dMonthY[2].toLowerCase()]
    const y = Number(dMonthY[3])
    if (m != null && d >= 1 && d <= 31) {
      return `${y}-${String(m + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    }
  }

  const monthDY = q.match(
    /\b(Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s+(\d{1,2})(?:st|nd|rd|th)?\s*,?\s*(\d{4})\b/i,
  )
  if (monthDY) {
    const m = MONTH_INDEX[monthDY[1].toLowerCase()]
    const d = Number(monthDY[2])
    const y = Number(monthDY[3])
    if (m != null && d >= 1 && d <= 31) {
      return `${y}-${String(m + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    }
  }

  return null
}

/** Parse absolute or relative date hints from the user question. */
function parseDateWindowFromText(text) {
  const q = String(text || '')

  const isoRange = q.match(/(\d{4}-\d{2}-\d{2}).{0,48}?(\d{4}-\d{2}-\d{2})/)
  if (isoRange) {
    return windowFromDayBounds(isoRange[1], isoRange[2])
  }

  // "from 21 June 2026 to 28 June 2026" / "between June 21 and June 28, 2026"
  const naturalRange = q.match(
    /\b(?:from|between)\s+(.+?)\s+(?:to|and|through|-)\s+(.+?)(?:\?|$)/i,
  )
  if (naturalRange) {
    const a = parseOneDayToken(naturalRange[1])
    const b = parseOneDayToken(naturalRange[2])
    if (a && b) return windowFromDayBounds(a, b)
  }

  const single = parseOneDayToken(q)
  if (single) return windowFromDayBounds(single, single)

  if (/\btoday\b|\bthis day\b|\blast day\b|\byesterday\b/i.test(q)) {
    return { kind: 'day' }
  }
  if (/\bthis week\b|\blast\s*7\s*days\b|\bpast week\b|\blast week\b/i.test(q)) {
    return { kind: 'week' }
  }
  return null
}

function windowFromDayBounds(startDay, endDay) {
  const start = `${startDay}T00:00:00.000Z`
  const end = `${endDay}T23:59:59.000Z`
  return {
    start,
    end,
    compare: compareWindowFor(start, end),
  }
}

function compareWindowFor(startISO, endISO) {
  const start = new Date(startISO)
  const end = new Date(endISO)
  const ms = Math.max(end.getTime() - start.getTime(), 0)
  const cEnd = new Date(start.getTime() - 1000)
  const cStart = new Date(cEnd.getTime() - ms)
  return {
    start: cStart.toISOString(),
    end: cEnd.toISOString(),
  }
}

let liveRangeCache = { at: 0, value: null }

async function fetchLiveDataRange() {
  if (liveRangeCache.value && Date.now() - liveRangeCache.at < 60_000) {
    return liveRangeCache.value
  }
  if (!engine) throw new Error('engine_not_ready')
  const meta = await engine.getDashboardMeta()
  const range = meta?.dataRange || null
  liveRangeCache = { at: Date.now(), value: range }
  return range
}

/**
 * Prefer dates mentioned in the question; otherwise use the latest day/week
 * available in metric_hourly_snapshot.
 */
async function resolveChatWindow(text, narrow) {
  const parsed = parseDateWindowFromText(text)
  if (parsed?.start && parsed?.end) return parsed

  const kind = parsed?.kind || (narrow ? 'day' : 'week')
  const range = await fetchLiveDataRange()
  if (!range?.max) {
    throw new Error('no snapshot data available to choose a date window')
  }

  const max = new Date(range.max)
  const min = range.min ? new Date(range.min) : null
  const maxDay = new Date(Date.UTC(max.getUTCFullYear(), max.getUTCMonth(), max.getUTCDate()))

  let start
  let end
  if (kind === 'day') {
    start = maxDay
    end = new Date(maxDay.getTime() + 24 * 3600 * 1000 - 1000)
  } else {
    end = new Date(maxDay.getTime() + 24 * 3600 * 1000 - 1000)
    start = new Date(maxDay.getTime() - 6 * 24 * 3600 * 1000)
  }

  if (end > max) end = max
  if (min && start < min) start = min

  return {
    start: start.toISOString(),
    end: end.toISOString(),
    compare: compareWindowFor(start.toISOString(), end.toISOString()),
  }
}

async function fetchDashboardEvidence(slice, question = '') {
  return startActiveObservation(
    'retrieve-dashboard-evidence',
    async (obs) => {
      const window = await resolveChatWindow(question, Boolean(slice.narrow))
      const body = {
        start: window.start,
        end: window.end,
        compare: window.compare,
        granularity: slice.granularity || (slice.narrow ? 'hour' : 'day'),
        metrics: ['revenue', 'requests', 'fill_rate', 'ecpm', 'ctr'],
        dimensions: slice.dimensions || ['ad_format'],
        filters: slice.filters,
        limit: 10,
      }
      obs.update({ input: { filters: body.filters, window, granularity: body.granularity } })
      if (!engine) throw new Error('engine_not_ready')
      const out = await engine.queryDashboard(body)
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
  if (!defaultInvestigationPromise) {
    defaultInvestigationPromise = (async () => {
      if (!engine) throw new Error('engine_not_ready')
      const alerts = await engine.detectAlerts('day')
      const pick =
        (Array.isArray(alerts) && alerts.find((a) => a.advertiserId === 'adv_0000')) ||
        (Array.isArray(alerts) && alerts[0])
      if (!pick?.id) {
        throw new Error('no live alerts available for default investigation')
      }
      return await runEngineInvestigate({ alertId: pick.id })
    })().catch((err) => {
      defaultInvestigationPromise = null
      throw err
    })
  }
  return defaultInvestigationPromise
}

async function investigationForAlert(alertId) {
  const uuid = String(alertId || '').replace(/^inv-/i, '')
  const isUUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(uuid)
  if (isUUID) {
    return await runEngineInvestigate({ alertId: uuid })
  }

  // Legacy ids: alert-{metric}-{YYYY-MM-DD}
  const dateMatch = alertId.match(/(\d{4}-\d{2}-\d{2})/)
  const metricMatch = alertId.match(/alert-([a-z_]+)-/)
  const metricAlias = {
    rev: 'revenue',
    revenue: 'revenue',
    fill: 'fill_rate',
    fill_rate: 'fill_rate',
    ctr: 'ctr',
    ecpm: 'ecpm',
  }
  if (!dateMatch) {
    throw new Error(`cannot investigate alert without window: ${alertId}`)
  }
  const rawMetric = metricMatch?.[1] || 'revenue'
  const body = {
    alertId,
    metric: metricAlias[rawMetric] || rawMetric,
    windowStart: `${dateMatch[1]}T00:00:00Z`,
    windowEnd: `${dateMatch[1]}T23:59:59Z`,
    baselineKind: 'same_hour_4w_seasonality',
  }
  return await runEngineInvestigate(body)
}

async function runEngineInvestigate(body) {
  if (!engine) throw new Error('engine_not_ready')
  const inv = await engine.runInvestigation(body || {})
  invCache.set(inv.id, inv)
  return inv
}

/** Parse inv-{uuid} or inv-{metric}-{YYYYMMDD} into an investigate request body. */
function requestFromInvestigationId(id) {
  try {
    if (!engine) {
      // Fallback parser when engine not ready (startup race).
      const raw = String(id || '')
      const uuid = raw.replace(/^inv-/i, '')
      if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(uuid)) {
        return { alertId: uuid }
      }
      return null
    }
    return engine.requestFromInvestigationID(id)
  } catch {
    return null
  }
}

async function resolveInvestigation(text, opts = {}) {
  const investigationId = opts.investigationId || ''
  const alertId = opts.alertId || ''

  if (investigationId) {
    if (invCache.has(investigationId)) return invCache.get(investigationId)
    try {
      if (engine) {
        const inv = await engine.getInvestigation(investigationId)
        invCache.set(inv.id || investigationId, inv)
        return inv
      }
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
    try {
      const inv = await investigationForAlert(alertMatch[0].toLowerCase())
      if (inv) return inv
    } catch {
      /* fall through */
    }
  }

  // Free-form RCA questions: reuse one cached default investigation (do not re-run /alerts + investigate each turn).
  try {
    return await getDefaultInvestigation()
  } catch (err) {
    console.warn('default investigation unavailable', err.message)
    return null
  }
}

function formatHumanDay(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return String(iso).slice(0, 10)
  return d.toLocaleDateString('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  })
}

function formatHumanWindow(window) {
  if (!window?.start) return ''
  const start = formatHumanDay(window.start)
  const end = formatHumanDay(window.end || window.start)
  return start === end ? start : `${start} – ${end}`
}

function roundNice(n, digits = 2) {
  const x = Number(n)
  if (!Number.isFinite(x)) return n
  return Number(x.toFixed(digits))
}

function fallbackNarration(question, inv, extraEvidence) {
  if (extraEvidence?.kind === 'dashboard' || extraEvidence?.kind === 'geo') {
    const day = formatHumanWindow(extraEvidence.window)
    const lines = [
      `For **${extraEvidence.label}** on **${day}**:`,
      '',
      `Revenue **${roundNice(extraEvidence.totals?.revenue)}** · requests ${roundNice(extraEvidence.totals?.requests, 0)} · fill rate ${roundNice((extraEvidence.totals?.fill_rate || 0) * 100, 1)}% · eCPM ${roundNice(extraEvidence.totals?.ecpm)}.`,
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
            date: formatHumanWindow(extraEvidence.window),
            windowISO: extraEvidence.window,
            totals: {
              revenue: roundNice(extraEvidence.totals?.revenue),
              requests: roundNice(extraEvidence.totals?.requests, 0),
              fill_rate: roundNice(extraEvidence.totals?.fill_rate, 4),
              ecpm: roundNice(extraEvidence.totals?.ecpm),
            },
            deltas: extraEvidence.deltas,
          }
        : {
            id: inv.id,
            status: inv.status,
            alert: inv.alert,
            decomposition: inv.decomposition,
            segments: (inv.segments || []).slice(0, 5),
            ruledOut: inv.ruledOut,
            diagnosis: {
              text: inv.diagnosis?.text,
              citations: (inv.diagnosis?.citations || []).slice(0, 6),
            },
          }

      const system = [
        'You are InsightIQ, an automated analytics narrator.',
        'You MUST only use numbers and facts present in the provided evidence JSON.',
        'Never invent metrics, segments, or percentages.',
        isDash
          ? 'This evidence is a dashboard query result for the exact filters listed. Answer using totals.revenue / totals.requests / etc. for that filter intersection. Do not ignore filters like os_version.'
          : 'This evidence is a root-cause investigation package. Summarize diagnosis + top 2-3 segments only.',
        'Keep the answer to 2-4 short sentences.',
        'Format dates as human calendar dates (e.g. 21 Jun 2026). Never paste raw ISO timestamps like 2026-06-21T00:00:00.000Z.',
        'Round revenue to 2 decimals. Do not print floating-point noise.',
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

async function main() {
  const client = await createClickHouse()
  engine = createEngine(client)
  console.log(
    `connected to ClickHouse database=${process.env.CLICKHOUSE_DATABASE || 'insightiq'}`,
  )
  app.listen(port, () => {
    console.log(
      `InsightIQ API listening on http://localhost:${port} (gemini=${geminiKey ? geminiModel : 'off'}, engine=in-process, langfuse=${langfuseEnabled ? 'on' : 'off'})`,
    )
  })
}

main().catch((err) => {
  console.error('failed to start API', err)
  process.exit(1)
})

async function shutdown() {
  await flushLangfuse()
  try {
    await engine?.client?.close?.()
  } catch {
    /* ignore */
  }
  process.exit(0)
}
process.on('SIGTERM', shutdown)
process.on('SIGINT', shutdown)
