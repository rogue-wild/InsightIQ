import 'dotenv/config'
import cors from 'cors'
import express from 'express'
import {
  alerts as mockAlerts,
  getAlertById as getMockAlert,
  getInvestigationById as getMockInvestigation,
  getInvestigationForAlert as getMockInvestigationForAlert,
  investigations as mockInvestigations,
} from '../../../packages/contracts/sample-investigations.js'

const app = express()
const port = Number(process.env.PORT || 4000)
const geminiKey = process.env.GEMINI_API_KEY || ''
const geminiModel = process.env.GEMINI_MODEL || 'gemini-flash-lite-latest'
const engineUrl = (process.env.ENGINE_URL || 'http://127.0.0.1:4100').replace(/\/$/, '')
const useEngine = (process.env.USE_ENGINE || 'true') === 'true'

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

app.post('/api/investigate', async (req, res) => {
  try {
    const inv = await runEngineInvestigate(req.body || {})
    res.json(inv)
  } catch (err) {
    console.error(err)
    res.status(500).json({ error: 'investigate_failed', detail: err.message })
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
    const investigation = await resolveInvestigation(content)
    const reply = await narrateFromEvidence(content, investigation)

    const payload = {
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
    }

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

async function resolveInvestigation(text) {
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
  // Default: first InsightIQ alert with observations (engine /alerts prefers those)
  try {
    const alerts = await fetchJSON(`${engineUrl}/alerts`)
    if (Array.isArray(alerts) && alerts[0]?.id) {
      return await runEngineInvestigate({ alertId: alerts[0].id })
    }
  } catch {
    /* fall through */
  }
  return mockInvestigations['inv-001']
}

function fallbackNarration(question, inv) {
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

async function narrateFromEvidence(question, inv) {
  if (!inv) return fallbackNarration(question, inv)
  if (!geminiKey) return fallbackNarration(question, inv)

  const evidence = {
    id: inv.id,
    status: inv.status,
    alert: inv.alert,
    decomposition: inv.decomposition,
    segments: inv.segments,
    ruledOut: inv.ruledOut,
    diagnosis: inv.diagnosis,
    trace: inv.trace,
  }

  const system = [
    'You are InsightIQ, an automated root-cause analyst narrator.',
    'You MUST only use numbers and facts present in the provided evidence JSON.',
    'Never invent metrics, segments, or percentages.',
    'Keep the answer concise and plain-English for a hackathon demo.',
  ].join(' ')

  const userPrompt = `User question: ${question || 'Explain this investigation.'}\n\nEvidence JSON:\n${JSON.stringify(evidence, null, 2)}`
  const url = `https://generativelanguage.googleapis.com/v1beta/models/${geminiModel}:generateContent?key=${encodeURIComponent(geminiKey)}`
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      systemInstruction: { parts: [{ text: system }] },
      contents: [{ role: 'user', parts: [{ text: userPrompt }] }],
      generationConfig: { temperature: 0.2 },
    }),
  })

  if (!response.ok) {
    const body = await response.text()
    console.error('Gemini error', response.status, body.slice(0, 500))
    return `${fallbackNarration(question, inv)}\n\n_(Gemini unavailable — showed evidence fallback.)_`
  }

  const data = await response.json()
  const text = data?.candidates?.[0]?.content?.parts?.map((p) => p.text).filter(Boolean).join('\n')
  return text?.trim() || fallbackNarration(question, inv)
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
    `InsightIQ API listening on http://localhost:${port} (gemini=${geminiKey ? geminiModel : 'off'}, engine=${useEngine ? engineUrl : 'off'})`,
  )
})
