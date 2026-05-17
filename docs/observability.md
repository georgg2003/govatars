# Observability (Govatars)

Stack in `docker compose`: **OpenTelemetry Collector** → **Jaeger** (traces), **Prometheus** (metrics), **Loki** (logs), **Grafana** (UI).

Enable in Compose via `GOVATARS_OTEL_ENABLED=true` on `server` and `worker`. Local runs without Compose keep `otel.enabled: false` in `config/config.yaml`.

## Ports

| Service    | URL |
|-----------|-----|
| Grafana   | http://localhost:3000 |
| Jaeger UI | http://localhost:16686 |
| Prometheus | http://localhost:9090 |
| Loki      | http://localhost:3100 |

## End-to-end trace (acceptance check)

1. `docker compose up` (full stack).
2. Upload an avatar (`POST /api/v1/avatars` with `X-User-ID`).
3. Open Jaeger → service `govatars_server` or `govatars_worker` → find the trace.

Expected spans in **one** trace:

- HTTP request (`otelecho`)
- `avatar.upload` → `rabbitmq.publish`
- `rabbitmq.process` → `avatar.process_upload` → `s3.*` / Postgres (`otelpgx`)

## Logs (Loki — ELK-equivalent workflow)

Logs are structured JSON with `trace_id`, `span_id`, `request_id`, `user_id` (HTTP), `level`, `msg`.

**Dashboard:** Grafana → folder **Govatars** → **Govatars Logs** (`govatars-logs`). Variables: service, level, trace ID, request ID, free-text search. Panels: volume by level/service, full log stream, HTTP access logs, errors/warnings.

1. Copy **Trace ID** from Jaeger.
2. Paste it into the **Trace ID** variable on the logs dashboard (or use Explore → Loki).
3. Example LogQL:

```logql
{service_name="govatars_server"} | json | trace_id="<paste-trace-id>"
```

```logql
{service_name="govatars_worker"} | json | trace_id="<paste-trace-id>"
```

Filter by request:

```logql
{service_name="govatars_server"} | json | request_id="<x-request-id>"
```

## Metrics (Prometheus + Grafana)

- Scrape: Prometheus → `otel-collector:8889`.
- Dashboards: Grafana → folder **Govatars** → **Govatars** (metrics), **Govatars Logs** (Loki).
- Business counters: `govatars_avatar_uploads_total`, `govatars_avatar_deletes_total`, `govatars_thumbnail_jobs_total`, `govatars_queue_retries_total`, `govatars_queue_dlq_total`.

Quick check:

```bash
curl -s 'http://localhost:9090/api/v1/query?query=govatars_avatar_uploads_total'
```

## Self-check before review

- [ ] Jaeger: one upload → single trace with HTTP + queue + worker + S3/DB spans
- [ ] Loki: same `trace_id` on server and worker log lines
- [ ] Grafana dashboard: HTTP rate/latency and business panels non-empty after traffic
- [ ] Prometheus: business counters increase after upload/delete
