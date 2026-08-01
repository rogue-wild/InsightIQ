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

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
