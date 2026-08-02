import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { diversifySegments } from './investigate.js'

describe('diversifySegments', () => {
  it('keeps one row per category before filling by contribution', () => {
    const segs = [
      { dimension: 'vertical', value: 'entertainment', contributionPct: 100 },
      { dimension: 'campaign_type', value: 'CPM', contributionPct: 100 },
      { dimension: 'category', value: 'ecommerce', contributionPct: 53 },
      { dimension: 'publisher_tier', value: 'tier_3', contributionPct: 52 },
      { dimension: 'os_version', value: 'Android 12', contributionPct: 44 },
      { dimension: 'country', value: 'US', contributionPct: 42 },
      { dimension: 'region', value: 'NAM', contributionPct: 40 },
      { dimension: 'ad_format', value: 'video', contributionPct: 30 },
      { dimension: 'category', value: 'gaming', contributionPct: 20 },
    ]
    const out = diversifySegments(segs, 6)
    const dims = out.map((s) => s.dimension)
    assert.ok(dims.includes('country') || dims.includes('region'))
    assert.ok(dims.includes('os_version'))
    assert.ok(dims.includes('campaign_type'))
    assert.equal(out.length, 6)
  })
})
