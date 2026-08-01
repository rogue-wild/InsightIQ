import {
  alerts as mockAlerts,
  getInvestigationById,
  getInvestigationForAlert,
} from '../mocks/investigation.js'

const useMock = (import.meta.env.VITE_USE_MOCK ?? 'true') !== 'false'
const apiBase = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '')

/** In-flight + short TTL cache — React StrictMode remounts effects in dev and would otherwise double-fetch. */
const inflight = new Map()
const responseCache = new Map()
const CACHE_TTL_MS = 30_000

async function fetchJson(path) {
  const cached = responseCache.get(path)
  if (cached && Date.now() - cached.at < CACHE_TTL_MS) {
    return structuredClone(cached.data)
  }

  if (inflight.has(path)) {
    const data = await inflight.get(path)
    return structuredClone(data)
  }

  const promise = (async () => {
    const res = await fetch(`${apiBase}${path}`)
    if (!res.ok) {
      throw new Error(`API ${path} failed: ${res.status}`)
    }
    const data = await res.json()
    responseCache.set(path, { at: Date.now(), data })
    return data
  })()

  inflight.set(path, promise)
  try {
    const data = await promise
    return structuredClone(data)
  } finally {
    inflight.delete(path)
  }
}

/** @returns {Promise<typeof mockAlerts>} */
export async function listAlerts() {
  if (useMock) {
    await delay(180)
    return structuredClone(mockAlerts)
  }
  return fetchJson('/api/alerts')
}

/** @param {string} alertId */
export async function getAlert(alertId) {
  if (useMock) {
    await delay(120)
    const alert = mockAlerts.find((a) => a.id === alertId)
    if (!alert) throw new Error(`Alert not found: ${alertId}`)
    return structuredClone(alert)
  }
  return fetchJson(`/api/alerts/${alertId}`)
}

/** @param {string} investigationId */
export async function getInvestigation(investigationId) {
  if (useMock) {
    await delay(220)
    const inv = getInvestigationById(investigationId)
    if (!inv) throw new Error(`Investigation not found: ${investigationId}`)
    return structuredClone(inv)
  }
  return fetchJson(`/api/investigations/${investigationId}`)
}

/** Downloadable unseen-incident bundle (diagnosis + immutable trace + evidence hash). */
export async function exportInvestigationBundle(investigationId) {
  if (useMock) {
    await delay(120)
    const inv = getInvestigationById(investigationId)
    if (!inv) throw new Error(`Investigation not found: ${investigationId}`)
    return {
      exportedAt: new Date().toISOString(),
      purpose: 'unseen-incident-submission',
      investigation: structuredClone(inv),
      immutableTrace: inv.trace || [],
      evidenceHash: inv.evidence?.hash || 'mock',
      evidence: inv.evidence || { hash: 'mock', sources: ['mock'] },
      seasonality: inv.seasonality || null,
      waterfall: inv.waterfall || [],
      counterfactual: inv.counterfactual || null,
      hypotheses: inv.hypotheses || [],
    }
  }
  if (!apiBase) throw new Error('VITE_API_URL is not set')
  const res = await fetch(`${apiBase}/api/investigations/${encodeURIComponent(investigationId)}/export`)
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Export failed: ${res.status} ${body.slice(0, 180)}`)
  }
  return res.json()
}

/** @param {string} alertId */
export async function getInvestigationByAlert(alertId) {
  if (useMock) {
    await delay(220)
    const inv = getInvestigationForAlert(alertId)
    if (!inv) throw new Error(`Investigation not found for alert: ${alertId}`)
    return structuredClone(inv)
  }
  return fetchJson(`/api/alerts/${alertId}/investigation`)
}

export function isMockMode() {
  return useMock
}

export function clearApiCache() {
  inflight.clear()
  responseCache.clear()
}

/**
 * OpenAI-compatible chat against the InsightIQ API.
 * @param {{role: string, content: string}[]} messages
 * @param {{ investigationId?: string, alertId?: string }} [context]
 */
export async function sendChatMessage(messages, context = {}) {
  if (useMock) {
    await delay(400)
    const last = [...messages].reverse().find((m) => m.role === 'user')
    return [
      'Mock InsightIQ reply (set `VITE_USE_MOCK=false` for live narration).',
      '',
      `You asked: ${last?.content || '(empty)'}`,
      context.investigationId ? `Investigation: ${context.investigationId}` : null,
      context.alertId ? `Alert: ${context.alertId}` : null,
    ]
      .filter(Boolean)
      .join('\n')
  }

  if (!apiBase) {
    throw new Error('VITE_API_URL is not set')
  }

  const seeded = [...messages]
  const contextId = context.investigationId || context.alertId
  if (contextId) {
    const hint = [
      context.investigationId ? `investigation ${context.investigationId}` : null,
      context.alertId ? `alert ${context.alertId}` : null,
    ]
      .filter(Boolean)
      .join(' / ')
    const firstUser = seeded.findIndex((m) => m.role === 'user')
    if (firstUser >= 0 && !seeded[firstUser].content.includes(contextId)) {
      seeded[firstUser] = {
        ...seeded[firstUser],
        content: `${seeded[firstUser].content}\n\n(Context: ${hint})`,
      }
    }
  }

  let sessionId = context.sessionId
  if (!sessionId && typeof window !== 'undefined') {
    sessionId = window.sessionStorage.getItem('insightiq-chat-session')
    if (!sessionId) {
      sessionId = `insightiq-web-${crypto.randomUUID()}`
      window.sessionStorage.setItem('insightiq-chat-session', sessionId)
    }
  }

  const res = await fetch(`${apiBase}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model: 'insightiq-rca',
      messages: seeded,
      stream: false,
      investigationId: context.investigationId || undefined,
      alertId: context.alertId || undefined,
      sessionId: sessionId || undefined,
    }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`Chat failed: ${res.status} ${body.slice(0, 180)}`)
  }
  const data = await res.json()
  const text = data?.choices?.[0]?.message?.content
  if (!text) throw new Error('Empty chat response')
  return text
}

import { FALLBACK_META } from '../dashboard/config.js'

export async function getDashboardMeta() {
  if (useMock || !apiBase) {
    return FALLBACK_META
  }
  return fetchJson('/api/dashboard/meta')
}

export async function queryDashboard(body) {
  if (!apiBase) throw new Error('VITE_API_URL is not set')
  const res = await fetch(`${apiBase}/api/dashboard/query`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`Dashboard query failed: ${res.status} ${text.slice(0, 200)}`)
  }
  return res.json()
}

export async function getDashboardFilterValues({ dimension, start, end }) {
  if (!apiBase) return []
  const qs = new URLSearchParams({ dimension, start, end }).toString()
  const data = await fetchJson(`/api/dashboard/filters?${qs}`)
  return data.values || []
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
