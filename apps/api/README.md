# AdInsight Node API

REST + LibreChat OpenAI-compatible chat. Proxies investigations to the Go engine and narrates evidence with Gemini.

```bash
cp .env.example .env
npm install
npm run dev
```

Requires `apps/engine` on `ENGINE_URL` (default `http://127.0.0.1:4100`) when `USE_ENGINE=true`.

## Endpoints

- `GET /health`
- `GET /api/alerts`
- `POST /api/investigate`
- `GET /api/investigations/:id`
- `POST /v1/chat/completions` — LibreChat custom endpoint
