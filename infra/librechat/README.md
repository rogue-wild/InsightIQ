# LibreChat for AdInsight

Your running LibreChat install lives at `/Users/geospot/Developer/LibreChat` (port **3080**).

AdInsight wires into it as a **custom OpenAI-compatible endpoint** backed by the Node API on port **4000**.

## Already configured

1. [`LibreChat/librechat.yaml`](/Users/geospot/Developer/LibreChat/librechat.yaml) — `AdInsight` endpoint → `http://host.docker.internal:4000/v1/`
2. [`LibreChat/docker-compose.override.yml`](/Users/geospot/Developer/LibreChat/docker-compose.override.yml) — mounts that YAML into the container
3. `apps/web/.env` — `VITE_LIBRECHAT_URL=http://localhost:3080` enables **Ask in chat**

## Run checklist

```bash
# Terminal 1 — AdInsight API (must be up for chat answers)
cd /Users/geospot/Developer/AdInsight/apps/api && npm run dev

# Terminal 2 — dashboard
cd /Users/geospot/Developer/AdInsight/apps/web && npm run dev

# LibreChat (already dockerized)
cd /Users/geospot/Developer/LibreChat && docker compose up -d
```

1. Open http://localhost:5173 → open an investigation → **Ask in chat**
2. Or open http://localhost:3080 directly, pick endpoint **AdInsight** / model **adinsight-rca**
3. Try: `Why did revenue drop? Use inv-001`

## Smoke test from inside Docker

```bash
docker exec LibreChat wget -qO- http://host.docker.internal:4000/health
```

If that fails, restart with host gateway: `extra_hosts: ["host.docker.internal:host-gateway"]` (already in the stock compose).
