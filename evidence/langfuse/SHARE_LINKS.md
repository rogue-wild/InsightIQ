# Langfuse evidence — 404Duos / InsightIQ

## Project-wide public link?

**Langfuse does not support a public share link for an entire project.**  
Only **individual traces** can be marked public (no login). For the bulk of runs, commit an **export** (CSV/JSON) into this folder — judges can open that without your Langfuse account.

| Method | Scope | Login required? |
|--------|--------|-----------------|
| Trace → Share → make public | One trace | No |
| CSV / JSON export in repo | Many traces/observations | No (in git) |
| Project URL (`/project/...`) | Whole project | Yes — **not** valid judge evidence alone |

Your project (private UI, members only):  
https://jp.cloud.langfuse.com/project/cmsaj0vmd00bcad0k7vvthm8x/traces

## Bulk export (committed)

- [`1785643387197-lf-events-export-cmsaj0vmd00bcad0k7vvthm8x.csv`](./1785643387197-lf-events-export-cmsaj0vmd00bcad0k7vvthm8x.csv)  
  - ~94 observation rows / multiple `chat-completion` traces  
  - Includes `narrate-with-gemini` generations, sessions, latency, I/O  

This satisfies the guidelines’ “export them as JSON into your submission folder” spirit (CSV export from Langfuse UI is fine).

## Public share links (still add for graded runs)

Open each important trace in Langfuse → **Share** → enable public access → paste URL here.

Pattern (JP):  
`https://jp.cloud.langfuse.com/project/cmsaj0vmd00bcad0k7vvthm8x/traces/<traceId>`

| Run | Trace ID | Public share URL |
|-----|----------|------------------|
| Demo chat (example) | `3794b0dbc8e300ccf9998c0d55bbf0c9` | Make public, then: `https://jp.cloud.langfuse.com/project/cmsaj0vmd00bcad0k7vvthm8x/traces/3794b0dbc8e300ccf9998c0d55bbf0c9` |
| Unseen incident | after release | TODO |

Opening that URL **without** making the trace public will still require login. Use Share → public first.

### How to make a trace public (UI)

1. https://jp.cloud.langfuse.com → InsightIQ → **Traces**
2. Open a `chat-completion` trace
3. **Share** (or lock/globe control) → **Make public** / copy link
4. Open the link in an **incognito** window to confirm no login
5. Paste below

```
<!-- paste public URLs here -->
```
