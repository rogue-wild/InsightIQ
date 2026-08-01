#!/usr/bin/env node
/**
 * Unseen-incident export CLI
 *
 * Run from repo root:
 *   cd /Users/geospot/Developer/InsightIQ
 *
 * List live alerts (real UUIDs):
 *   node scripts/export-unseen.mjs --list
 *
 * Export by ClickHouse alert UUID (no angle brackets):
 *   node scripts/export-unseen.mjs --alertId=a1b2c3d4-e5f6-7890-abcd-ef1234567890 --out=./unseen-submission
 *
 * Export by investigation id:
 *   node scripts/export-unseen.mjs --investigationId=inv-a1b2c3d4-e5f6-7890-abcd-ef1234567890
 *
 * NOTE: Do NOT paste the literal text <NEW_ALERT_UUID> — zsh treats <...> as redirection.
 * Prefer live alert UUIDs from --list (or API /api/alerts).
 */
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'

const ENGINE = (process.env.ENGINE_URL || 'http://127.0.0.1:4100').replace(/\/$/, '')
const API = (process.env.API_URL || 'http://127.0.0.1:4000').replace(/\/$/, '')

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function arg(name, fallback = '') {
  const hit = process.argv.find((a) => a.startsWith(`--${name}=`))
  return hit ? hit.slice(name.length + 3) : fallback
}

function hasFlag(name) {
  return process.argv.includes(`--${name}`)
}

function stripPlaceholders(s) {
  return String(s || '')
    .trim()
    .replace(/^<|>$/g, '')
    .replace(/^NEW_ALERT_UUID$/i, '')
}

async function fetchJSON(url, options) {
  const res = await fetch(url, options)
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${url} -> ${res.status} ${body.slice(0, 400)}`)
  }
  return res.json()
}

function normalizeAlertId(raw) {
  let id = stripPlaceholders(raw)
  if (!id) return ''
  if (id.startsWith('inv-')) id = id.slice(4)
  return id
}

async function listAlerts() {
  try {
    return await fetchJSON(`${API}/api/alerts`)
  } catch {
    return fetchJSON(`${ENGINE}/alerts`)
  }
}

async function investigate(alertId) {
  // Prefer API; fall back to engine for UUID lookups.
  try {
    return await fetchJSON(`${API}/api/investigate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ alertId }),
    })
  } catch (apiErr) {
    const msg = apiErr.message || String(apiErr)
    if (!UUID_RE.test(alertId)) {
      throw new Error(
        [
          `Investigate failed for "${alertId}".`,
          'Use a live UUID from: node scripts/export-unseen.mjs --list',
          `Detail: ${msg}`,
        ].join('\n'),
      )
    }
    return fetchJSON(`${ENGINE}/investigate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ alertId }),
    })
  }
}

async function exportBundle(investigationId, inv) {
  try {
    return await fetchJSON(
      `${API}/api/investigations/${encodeURIComponent(investigationId)}/export`,
    )
  } catch {
    return {
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
  }
}

async function main() {
  if (hasFlag('help') || hasFlag('h')) {
    console.log(`Usage (from repo root):
  node scripts/export-unseen.mjs --list
  node scripts/export-unseen.mjs --alertId=UUID --out=./unseen-submission
  node scripts/export-unseen.mjs --investigationId=inv-UUID

Do not wrap the id in <angle brackets>.
Live alerts use UUIDs from alerts_live.`)
    return
  }

  if (hasFlag('list')) {
    const alerts = await listAlerts()
    if (!Array.isArray(alerts) || alerts.length === 0) {
      console.error(
        'No alerts returned. Engine/API may be down, or alerts_live is empty (load data / check CLICKHOUSE_DATABASE).',
      )
      process.exit(1)
    }
    console.log(`Found ${alerts.length} alerts:\n`)
    for (const a of alerts.slice(0, 20)) {
      const uuid = String(a.id || '').replace(/^inv-/i, '')
      console.log(
        `- ${uuid}  ${a.metric || '?'}  ${a.advertiserId || ''}  ${a.pctChange ?? ''}%  inv=${a.investigationId || 'inv-' + uuid}`,
      )
    }
    console.log(`\nExample:
  node scripts/export-unseen.mjs --alertId=${String(alerts[0].id).replace(/^inv-/i, '')} --out=./unseen-submission`)
    return
  }

  const alertId = normalizeAlertId(arg('alertId'))
  let investigationId = stripPlaceholders(arg('investigationId'))
  const outDir = path.resolve(arg('out', './unseen-submission'))

  let inv
  if (alertId) {
    console.log(`Investigating alertId=${alertId} via API…`)
    inv = await investigate(alertId)
    investigationId = inv.id
  } else if (investigationId) {
    console.log(`Loading investigationId=${investigationId}…`)
    try {
      inv = await fetchJSON(`${API}/api/investigations/${encodeURIComponent(investigationId)}`)
    } catch {
      inv = await fetchJSON(`${ENGINE}/investigations/${encodeURIComponent(investigationId)}`)
    }
  } else {
    console.error(`Missing --alertId or --investigationId.

First list live UUIDs:
  node scripts/export-unseen.mjs --list

Then export (example):
  node scripts/export-unseen.mjs --alertId=YOUR-UUID-HERE --out=./unseen-submission`)
    process.exit(1)
  }

  const bundle = await exportBundle(investigationId, inv)
  if (!bundle.evidenceHash && !bundle.evidence?.hash) {
    console.warn('Warning: export has no evidenceHash.')
  }

  await mkdir(outDir, { recursive: true })
  const file = path.join(outDir, `${investigationId}-unseen-export.json`)
  await writeFile(file, JSON.stringify(bundle, null, 2))
  console.log('wrote', file)
  console.log('evidenceHash', bundle.evidenceHash || bundle.evidence?.hash || '(none)')
  console.log('seasonality', bundle.seasonality?.status || '(none)')
  console.log(
    'culprit',
    bundle.investigation?.decomposition?.find((d) => d.status === 'culprit')?.factor || '(none)',
  )
}

main().catch((err) => {
  console.error(err.message || err)
  process.exit(1)
})
