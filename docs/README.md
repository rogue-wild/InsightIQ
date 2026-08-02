# InsightIQ Documentation

InsightIQ is a ClickHouse-native analytics control plane for ad-tech event streams: a reactive cascade inside ClickHouse detects anomalies and attributes root causes; the Go/Node/React apps investigate and narrate evidence-backed answers.

| Doc | What it covers |
|-----|----------------|
| [Architecture](./architecture.md) | System design, request paths, principles |
| [Native pipeline](./pipeline.md) | ClickHouse cascade, seasonality baseline, noise-floored Z-score, multi-dim RCA |
| [Setup & run](./setup.md) | Local environment, ports, credentials |
| [Deploy (public demo)](./deploy.md) | Railway engine+API, Vercel web, ClickHouse/Langfuse Cloud |
| [Data model](./data-model.md) | Tables, engines, query patterns |
| [API reference](./api-reference.md) | Engine + Node endpoints |
| [Product guide](./product-guide.md) | Dashboard, Alerts, Investigation, Chat |

Metrics glossary: [`../metrics_glossary.md`](../metrics_glossary.md)
