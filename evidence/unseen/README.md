# Unseen incident bundle (mandatory)

When the Click-a-thon unseen dataset is released:

1. Load it into your ClickHouse Cloud `insightiq` service (follow release notes).
2. Refresh / recompute cascade tables if the release instructions require it (`alerts_live`, contributors, observations).
3. Run the system — do **not** hand-write the diagnosis.

## Produce and commit

```bash
# List alerts from the live API (or local)
node scripts/export-investigation.mjs --list

# Export the graded alert(s)
node scripts/export-investigation.mjs --alertId=<UUID> --out=./evidence/unseen
```

Also from the UI: Investigation → **Export**.

### Required contents (per graded incident)

| Artifact | File | Must include |
|----------|------|----------------|
| Diagnosis | Inside `*-export.json` → `investigation.diagnosis` | Plain language + named segments + citations |
| Numbers | Same JSON + reproducible via CH / engine | actual, expected, z, deltas, contributions |
| Trace | `investigation.trace` + `evidenceHash` | Ordered steps proving the system ran |
| Langfuse | `../langfuse/` share link or JSON | Narration/chat turn for that incident |

### Naming

```
evidence/unseen/<alertId>-export.json
evidence/unseen/NOTES.md   # optional: which alert IDs were graded
```

**No trace → no credit** on this criterion.
