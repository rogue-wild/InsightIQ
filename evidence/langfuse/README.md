# Langfuse evidence (InMobi / Click-a-thon)

Judges must not need login to your Langfuse project. For every graded run (especially the **unseen incident** chat/investigation narration):

## What to put here

1. **Public share links** — in Langfuse UI open the trace → **Share** → public link. Paste into `SHARE_LINKS.md`.
2. **and/or JSON exports** — export the trace JSON and save as:
   - `chat-<session-or-timestamp>.json`
   - `investigate-<alertId>.json`

## Expected trace shape (live wiring)

From `apps/api` with Langfuse enabled:

- Trace / session: in-app chat (`sessionId` from web)
- Span: `handle-chat-completion`
  - Retriever or investigate resolve
  - Generation: `narrate-with-gemini` (model, prompt, tokens, reply)

Also: `investigate-alert` spans when `/api/investigate` is called.

## Config (redacted)

See `../../apps/api/.env.example`:

```bash
LANGFUSE_PUBLIC_KEY=pk-lf-...
LANGFUSE_SECRET_KEY=sk-lf-...
LANGFUSE_BASE_URL=https://jp.cloud.langfuse.com
```

## Live check

```bash
curl -s https://insightiq-production-be0e.up.railway.app/health
# → "langfuse": true, "langfuseBaseUrl": "https://jp.cloud.langfuse.com"
```
