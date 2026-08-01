#!/usr/bin/env node
/**
 * Unseen-incident export CLI
 *
 * Usage:
 *   node scripts/export-unseen.mjs --alertId=<uuid>
 *   node scripts/export-unseen.mjs --investigationId=inv-<uuid>
 *   node scripts/export-unseen.mjs --alertId=<uuid> --out=./unseen-out
 *
 * Writes diagnosis.json + evidence hash for hackathon submission.
 */
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'

const ENGINE = (process.env.ENGINE_URL || 'http://127.0.0.1:4100').replace(/\/$/, '')
const API = (process.env.API_URL || 'http://127.0.0.1:4000').replace(/\/$/, '')

function arg(name, fallback = '') {
  const hit = process.argv.find((a) => a.startsWith(`--${name}=`))
  return hit ? hit.slice(name.length + 3) : fallback
}

async function fetchJSON(url, options) {
  const res = await fetch(url, options)
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${url} -> ${res.status} ${body.slice(0, 300)}`)
  }
  return res.json()
}

async function main() {
  const alertId = arg('alertId')
  let investigationId = arg('investigationId')
  const outDir = path.resolve(arg('out', './unseen-out'))

  let inv
  if (alertId) {
    inv = await fetchJSON(`${ENGINE}/investigate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ alertId }),
    })
    investigationId = inv.id
  } else if (investigationId) {
    try {
      inv = await fetchJSON(`${API}/api/investigations/${encodeURIComponent(investigationId)}`)
    } catch {
      inv = await fetchJSON(`${ENGINE}/investigations/${encodeURIComponent(investigationId)}`)
    }
  } else {
    console.error('Pass --alertId=... or --investigationId=...')
    process.exit(1)
  }

  let bundle
  try {
    bundle = await fetchJSON(
      `${API}/api/investigations/${encodeURIComponent(investigationId)}/export`,
    )
  } catch {
    bundle = {
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

  await mkdir(outDir, { recursive: true })
  const file = path.join(outDir, `${investigationId}-unseen-export.json`)
  await writeFile(file, JSON.stringify(bundle, null, 2))
  console.log('wrote', file)
  console.log('evidenceHash', bundle.evidenceHash)
  console.log('seasonality', bundle.seasonality?.status)
  console.log('culprit', bundle.investigation?.decomposition?.find((d) => d.status === 'culprit')?.factor)
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
