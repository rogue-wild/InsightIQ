# Langfuse evidence (InMobi / Click-a-thon)

## What’s here

| File | Purpose |
|------|---------|
| `1785643387197-lf-events-export-….csv` | Bulk export of live chat/narrate observations |
| `SHARE_LINKS.md` | Public per-trace URLs + notes |

## Project-wide share?

**Not available.** Langfuse only public-shares **single traces**. For “whole project” evidence:

1. Commit the CSV/JSON export (done), **and**
2. Public-share **1–2 key traces** (demo + unseen) via Share → make public

See [SHARE_LINKS.md](./SHARE_LINKS.md).

## Wiring (in product code)

- `apps/api/src/instrumentation.js` — OTEL → Langfuse  
- `apps/api/src/index.js` — chat / investigate / narrate spans  
- `apps/api/.env.example` — `LANGFUSE_*` (secrets redacted)

## Live check

```bash
curl -s https://insightiq-production-be0e.up.railway.app/health
# langfuse: true, langfuseBaseUrl: https://jp.cloud.langfuse.com
```
