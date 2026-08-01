# InsightIQ Node API

REST + LibreChat OpenAI-compatible chat. Proxies investigations to the Go engine and narrates evidence with Gemini. Traces LLM + evidence steps to **Langfuse**.

```bash
cp .env.example .env
npm install
npm run dev
```

Requires `apps/engine` on `ENGINE_URL` (default `http://127.0.0.1:4100`) when `USE_ENGINE=true`.

## Langfuse

Set in `.env` (Japan cloud example):

```bash
LANGFUSE_SECRET_KEY=sk-lf-...
LANGFUSE_PUBLIC_KEY=pk-lf-...
LANGFUSE_BASE_URL=https://jp.cloud.langfuse.com
```

Each chat turn emits a trace tree:

- `handle-chat-completion` (span)
  - `retrieve-dashboard-evidence` (retriever) *or* investigation resolve
  - `narrate-with-gemini` (generation) — model, prompt, tokens, reply

Sessions are grouped via `sessionId` from the web client.

## Endpoints

- `GET /health` — includes `langfuse: true/false`
- `GET /api/alerts`
- `POST /api/investigate` — traced
- `GET /api/investigations/:id`
- `POST /v1/chat/completions` — LibreChat + in-app chat (traced)
