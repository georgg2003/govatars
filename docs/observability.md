# Observability (Govatars)

Stack in `docker compose`: **OpenTelemetry Collector** → **Jaeger** (traces), **Prometheus** (metrics), **Loki** (logs), **Grafana** (UI).

HTTP middleware (`otelecho`) runs when `otel.enabled` and **either** `tracer_provider.enabled` or `metrics_provider.enabled` (RED metrics for Grafana). Postgres (`otelpgx`) and distributed traces need `tracer_provider.enabled`. Logs use `logger_provider.enabled`.

## Ports


| Service    | URL                                              |
| ---------- | ------------------------------------------------ |
| Grafana    | [http://localhost:3000](http://localhost:3000)   |
| Jaeger UI  | [http://localhost:16686](http://localhost:16686) |
| Prometheus | [http://localhost:9090](http://localhost:9090)   |
| Loki       | [http://localhost:3100](http://localhost:3100)   |


## End-to-end trace (acceptance check)

1. `docker compose up` (full stack).
2. Upload an avatar (`POST /api/v1/avatars` with `X-User-ID`).
3. Open Jaeger → service `govatars_server` or `govatars_worker` → find the trace.

Expected spans in **one** trace:

- HTTP request (`otelecho`)
- `avatar.upload` → `rabbitmq.publish`
- `rabbitmq.process` → `avatar.process_upload` → `s3.`* / Postgres (`otelpgx`)

## Logs (Loki)

Logs are structured JSON with `trace_id`, `span_id`, `request_id`, `user_id` (HTTP), `level`, `msg`.

**Dashboard:** Grafana → folder **Govatars** → **Govatars Logs** (`govatars-logs`). Variables: service, level, trace ID, request ID, free-text search. Panels: volume by level/service, full log stream, HTTP access logs, errors/warnings.

1. Copy **Trace ID** from Jaeger.
2. Paste it into the **Trace ID** variable on the logs dashboard (or use Explore → Loki).
3. Example LogQL:

```logql
{service_name="govatars_server"} | trace_id="<paste-trace-id>"
```

## Metrics (Prometheus + Grafana)

- Scrape: Prometheus → `otel-collector:8889`.
- Dashboards: Grafana → folder **Govatars** → **Govatars** (metrics), **Govatars Logs** (Loki).
- Business counters: `govatars_avatar_uploads_total`, `govatars_avatar_deletes_total`, `govatars_thumbnail_jobs_total`, `govatars_queue_retries_total`, `govatars_queue_dlq_total`.

Quick check:

```bash
curl -s 'http://localhost:9090/api/v1/query?query=govatars_avatar_uploads_total'
```
