# Deploy a public demo (Railway + Vercel)

Target layout:

```
Browser → Vercel (web) → Railway (API, public) → Railway private network (engine) → ClickHouse Cloud
                              ↘ Gemini + Langfuse Cloud (optional)
```

Keep ClickHouse Cloud and Langfuse Cloud as they are. Do not put ClickHouse credentials in the web app.

| Piece | Host | Public? |
|-------|------|---------|
| Web | Vercel | Yes — **demo link** |
| API | Railway | Yes — generate a public domain |
| Engine | Railway | No public domain — private only |
| ClickHouse | ClickHouse Cloud | Existing |
| Langfuse | Langfuse Cloud | Existing |

Dockerfiles live at `apps/engine/Dockerfile` and `apps/api/Dockerfile`.

---

## 0. Prerequisites

- [Railway](https://railway.app) account + [Railway CLI](https://docs.railway.com/guides/cli) optional (`railway login`)
- [Vercel](https://vercel.com) (GitHub import or CLI)
- ClickHouse Cloud credentials already working locally
- Gemini + Langfuse keys if you want narration / traces

---

## 1. Railway project — two services

Create one Railway **project** (e.g. `insightiq`) with two services from the same GitHub repo.

### Service A — `engine`

| Setting | Value |
|---------|--------|
| Root Directory | `apps/engine` |
| Builder | Dockerfile |
| Dockerfile path | `Dockerfile` |
| Public networking | **Off** (no domain) |

Variables:

```bash
CLICKHOUSE_HOST=your.host.clickhouse.cloud
CLICKHOUSE_PORT=8443
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=***
CLICKHOUSE_DATABASE=insightiq
CLICKHOUSE_SECURE=true
CLICKHOUSE_LOG_QUERIES=true
# Do not set ENGINE_PORT — Railway injects PORT; the engine listens on it.
```

Health check path: `/health` (also in `apps/engine/railway.toml`).

### Service B — `api`

| Setting | Value |
|---------|--------|
| Root Directory | `apps/api` |
| Builder | Dockerfile |
| Dockerfile path | `Dockerfile` |
| Public networking | **On** — Generate Domain |

Variables:

```bash
GEMINI_API_KEY=***
GEMINI_MODEL=gemini-flash-lite-latest
LANGFUSE_PUBLIC_KEY=pk-***
LANGFUSE_SECRET_KEY=sk-***
LANGFUSE_BASE_URL=https://jp.cloud.langfuse.com

# Private URL to the engine service (names must match your Railway service name).
# Prefer Railway variable references in the dashboard:
ENGINE_URL=http://${{engine.RAILWAY_PRIVATE_DOMAIN}}:${{engine.PORT}}
```

If the engine service is named differently (e.g. `insightiq-engine`), use that name in `${{...}}`.

After deploy, copy the API public URL, e.g. `https://api-production-xxxx.up.railway.app`.

Smoke test:

```bash
curl -s https://YOUR-API.up.railway.app/health
curl -s 'https://YOUR-API.up.railway.app/api/alerts?granularity=day' | head -c 400
```

`health` should show the engine reachable. If `engine` is null/down, fix `ENGINE_URL` and private networking (both services in the **same project**).

---

## 2. CLI alternative (optional)

```bash
# From apps/engine — create/link service, set vars, deploy
cd apps/engine
railway link          # select project + engine service
railway variables set CLICKHOUSE_HOST=... CLICKHOUSE_PASSWORD=... # etc.
railway up

cd ../api
railway link          # same project, api service
railway variables set GEMINI_API_KEY=... LANGFUSE_PUBLIC_KEY=... LANGFUSE_SECRET_KEY=...
# Set ENGINE_URL in the Railway dashboard with ${{engine.RAILWAY_PRIVATE_DOMAIN}} references
railway up
railway domain        # ensure API has a public domain
```

---

## 3. Vercel — web (demo link)

`apps/web/vercel.json` configures Vite SPA rewrites.

1. Import the repo → **Root Directory** = `apps/web`
2. Environment variables (Production):
   - `VITE_API_URL` = `https://YOUR-API.up.railway.app` (no trailing slash)
   - `VITE_LIBRECHAT_URL` = optional; leave unset if LibreChat is not hosted
3. Deploy → use the `*.vercel.app` URL as the public demo link

CLI:

```bash
cd apps/web
vercel link
vercel env add VITE_API_URL production   # paste Railway API URL
vercel --prod
```

`VITE_*` values are **build-time**. Redeploy web whenever the API hostname changes.

---

## 4. Demo checklist

1. Open the Vercel URL → Dashboard loads
2. **Alerts** → Daily list non-empty
3. Open one investigation → metric tree + diagnosis + ruled-out
4. **Export** once (evidence hash / trace)
5. Chat: ask a follow-up; confirm Langfuse shows a turn if keys are set

---

## Cloudflare Pages alternative (web)

```bash
cd apps/web
npm ci
VITE_API_URL=https://YOUR-API.up.railway.app npm run build
npx wrangler pages deploy dist --project-name=insightiq
```

---

## Ops notes

| Topic | Guidance |
|-------|----------|
| Private engine | No public domain on `engine`; API talks over Railway private network |
| CORS | API uses open `cors()` — fine for a separate Vercel origin |
| Secrets | Never put ClickHouse or Gemini keys in `VITE_*` |
| PORT | Railway injects `PORT`; both Docker images honor it |
| Sleep / cold start | On trial plans, wake latency can add a few seconds — hit `/health` before a live demo |
| LibreChat | Optional; not required for Alerts → Investigation → in-app Chat |

---

## Local reference

See [setup.md](./setup.md) for local `.env` shapes. Deploy variables should match the same ClickHouse Cloud database you validated locally.
