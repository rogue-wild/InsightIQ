# Unseen / graded incident bundle — 404Duos

System-generated export (not hand-written).

## Bundle

| Field | Value |
|-------|--------|
| File | [`inv-b82a676d-5e80-f884-0f1a-78a4b44f9b07-export.json`](./inv-b82a676d-5e80-f884-0f1a-78a4b44f9b07-export.json) |
| Alert | `b82a676d-5e80-f884-0f1a-78a4b44f9b07` |
| Investigation | `inv-b82a676d-5e80-f884-0f1a-78a4b44f9b07` |
| Evidence hash | `37466a08f1e49bc1ea635bd2e1c62f749fa72f160c2e7f54ba06aac7a02b727c` |

Contains: diagnosis + citations, segments, ruled-out, seasonality, waterfall, counterfactual, hypotheses, immutable `trace[]`, evidence lock.

## Reproduce

```bash
API_URL=https://insightiq-production-be0e.up.railway.app \
  node scripts/export-investigation.mjs \
  --alertId=b82a676d-5e80-f884-0f1a-78a4b44f9b07 \
  --out=./evidence/unseen
```

Or: hosted UI → Alerts → open alert → **Export**.
