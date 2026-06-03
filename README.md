# Govatars (GophProfile)

Go module: `**govatars**`.

**Govatars** is a Go microservice for managing user avatars: upload once, store processed images, and serve them over HTTP. Third-party apps can resolve an avatar by user identifier; if none exists, a default placeholder may be returned (planned for later milestones).

This repository implements the **MVP** of the **GophProfile** course project: REST API, PostgreSQL metadata, S3-compatible object storage (MinIO), asynchronous image processing via **RabbitMQ**, and a bundled web UI (upload and gallery under `/web`).

## Features (target architecture)

- **REST API** described in `[api/swagger.yaml](api/swagger.yaml)`, server types generated with **[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)** and served with **Echo**.
- **Configuration** via **Viper**: YAML file, environment variables, and CLI flags (precedence: defaults → file → env → flags).
- **Persistence**: PostgreSQL for metadata; **MinIO** (or AWS S3) for originals and thumbnails.
- **Messaging**: RabbitMQ with a **retry** topology (multiple delay queues, DLX/DLQ, custom `X-Retry-Count` header for backoff routing).
- **Worker** binary for thumbnail generation and asynchronous deletes.
- **Testing**: `testify/suite`, **gomock** for interfaces, optional testcontainers for integration tests.
- **SOLID**: dependency interfaces are declared **at the point of use** (e.g. each use case defines the small interfaces it needs); concrete implementations live under `internal/repository/...` and satisfy those interfaces implicitly.

## Repository layout (planned)

```
cmd/
  server/          # HTTP API entrypoint
  worker/          # Background consumer
api/
  swagger.yaml     # OpenAPI spec (source for codegen)
internal/
  models/          # Domain and application models
  usecase/         # Business logic; defines interfaces it depends on
  repository/
    postgres/
    s3/
    rabbitmq/
  delivery/http/   # Echo handlers; server.gen.go / models.gen.go from oapi-codegen
  pkg/config/      # Viper-based loading (YAML + GOVATARS_* env + flags)
config/
  config.yaml      # Example local config (ports match docker-compose.deps.yml)
web/static/        # HTML/CSS assets served by the API at `/web` (Echo static + form routes)
migrations/        # SQL migrations
```

## API specification

The HTTP contract is the single source of truth: `**[api/swagger.yaml](api/swagger.yaml)**`.

After changing the spec, regenerate code (from `internal/delivery/http` or project root, depending on `go:generate` directives):

```bash
go generate ./...
```

## Requirements

- Go 1.26+ (see `go.mod` for the exact toolchain)
- PostgreSQL, RabbitMQ, and S3-compatible storage (e.g. MinIO) for full local runs

## Configuration

- Default file: `config/config.yaml`. Override path with `--config` / `-c`.
- Environment: prefix `**GOVATARS_**`, nested keys use `_` (e.g. `GOVATARS_POSTGRES_DSN`, `GOVATARS_HTTP_ADDRESS`).
- **Postgres**: set `postgres.dsn`, or omit `dsn` and set `postgres.host`, `postgres.user`, `postgres.database` (and optional `port`, `password`, `sslmode`) to build a URL.

## Logging & observability

- **Structured logs**: JSON via `slog` (`internal/pkg/logging`). HTTP requests get `request_id`, `user_id`, and (when OTEL is on) `trace_id` / `span_id` for correlation with Jaeger.
- **OTEL** (optional): set `otel.enabled: true` or `GOVATARS_OTEL_ENABLED=true`. Exports traces → Jaeger, metrics → Prometheus, logs → Loki. See [docs/observability.md](docs/observability.md) for ports, LogQL examples, and the acceptance checklist.
- **Full stack**: `docker compose up` starts API, worker, Postgres, MinIO, RabbitMQ, OTEL Collector, Jaeger (`:16686`), Prometheus (`:9090`), Loki, Grafana (`:3000` — **Govatars** and **Govatars Logs** dashboards).
- **Local dev without collector**: default `otel.enabled: false` in `config/config.yaml`; logs stay on stderr only.
- **Log level**: `logging.level` / `GOVATARS_LOGGING_LEVEL` — `debug`, `info`, `warn`, `error` (default `info`).

## Development

1. Start dependencies: `make deps-up` (see `[docker-compose.deps.yml](docker-compose.deps.yml)`).
2. Apply migrations: `make migrate-up` (runs `[golang-migrate](https://github.com/golang-migrate/migrate)` via `go run -tags postgres …@version`; the CLI must be built with the `postgres` tag so the driver is linked — same idea as `go install -tags postgres …` to `GOPATH/bin`. It is not listed under `tool` in `[go.mod](go.mod)`: `go tool migrate` does not apply build tags, and adding the binary as a `tool` pulls a very large indirect dependency graph).
3. Run API from the repository root (`api/swagger.yaml` is read at startup): `make run-server` or `go run ./cmd/server`.
4. Run the worker (thumbnail + S3 cleanup consumers): `make run-worker`.

Other **Makefile** targets: `build`, `test`, `install-lint` (installs **golangci-lint** into `**$(go env GOPATH)/bin`**), `lint` (runs that binary by full path — **no `PATH` setup required**), `generate` (OpenAPI codegen), `deps-down`, `migrate-down`.

## Local development in Kubernetes

The full stack (Postgres, MinIO, RabbitMQ, observability, API + worker) can be deployed with **[Helmfile](https://github.com/helmfile/helmfile)** from [`deploy/helmfile.yaml`](deploy/helmfile.yaml). This mirrors `docker-compose.yaml` but runs inside a local cluster (tested with **Rancher Desktop** + built-in **Traefik** ingress).

### Prerequisites

- A running Kubernetes cluster (`kubectl cluster-info` succeeds).
- **Docker** (or the container runtime your cluster uses) to build `govatars-server` / `govatars-worker` images.
- **Helm 3** and **Helmfile**.

### Install Helmfile

**macOS (Homebrew):**

```bash
brew install helm helmfile
```

**Linux / manual install:** see [Helmfile releases](https://github.com/helmfile/helmfile/releases) and [Helm install docs](https://helm.sh/docs/intro/install/).

Helmfile uses the **helm-diff** plugin for `apply`. Install it once:

```bash
helm plugin install https://github.com/databus23/helm-diff
```

### One-time: Helm chart repositories

External charts (Grafana, Prometheus, OpenTelemetry, Jaeger) are listed in the commented `repositories:` block at the top of [`deploy/helmfile.yaml`](deploy/helmfile.yaml). Either uncomment that block, or add repos manually:

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm repo update
```

### Build application images

The Helm chart expects local tags `govatars-server:latest` and `govatars-worker:latest` (see [`deploy/govatars/values.yaml`](deploy/govatars/values.yaml)).

From the repository root:

```bash
docker build -f Dockerfile.server -t govatars-server:latest .
docker build -f Dockerfile.worker -t govatars-worker:latest .
```

**Rancher Desktop:** build images with the **Rancher Desktop** Docker context (`docker context use rancher-desktop`), not Docker Desktop — otherwise the cluster will not see the images (`ImagePullBackOff`). If you already built elsewhere:

```bash
docker save govatars-server:latest govatars-worker:latest | docker --context rancher-desktop load
```

### Deploy the stack

```bash
cd deploy
helmfile apply
```

Release order is handled by `needs:` dependencies: **secrets** → infra → observability → **govatars** → **network-policies**.

First run takes several minutes (chart downloads, PVC provisioning, migration Job).

Check status:

```bash
kubectl get pods -n govatars
helmfile -l name=govatars status
```

### Credentials (K8s Secrets)

Passwords are **not** stored in git. The local chart [`deploy/secrets`](deploy/secrets) creates:

| Secret | Used by |
|--------|---------|
| `govatars-postgres` | Postgres |
| `govatars-rabbitmq` | RabbitMQ |
| `govatars-minio` | MinIO |
| `govatars-grafana` | Grafana admin |
| `govatars-app` | server / worker (DSN, RabbitMQ URL, S3 keys) |

On first install passwords are generated randomly; on upgrade existing values are kept (`lookup` + `helm.sh/resource-policy: keep`).

Example — Grafana admin password:

```bash
kubectl get secret govatars-grafana -n govatars \
  -o jsonpath='{.data.admin-password}' | base64 -d; echo
```

DB migrations run automatically as a Helm pre-upgrade hook (`govatars-migrate` Job).

### Ingress URLs

Traefik routes `*.localhost` to services (add hosts to `/etc/hosts` only if your environment does not resolve them automatically):

| URL | Service |
|-----|---------|
| http://govatars.localhost | API + `/web` UI |
| http://grafana.localhost | Grafana (dashboards from `deploy/grafana-dashboards`) |
| http://prometheus.localhost | Prometheus |
| http://jaeger.localhost | Jaeger UI |
| http://rabbitmq.localhost | RabbitMQ management |

### Day-to-day workflow

**After Go code changes** — rebuild images and roll out the app:

```bash
docker build -f Dockerfile.server -t govatars-server:latest .
docker build -f Dockerfile.worker -t govatars-worker:latest .
cd deploy && helmfile -l name=govatars apply
```

**After Helm/value changes** in `deploy/`:

```bash
cd deploy && helmfile apply
```

**Deploy or tear down a single release:**

```bash
cd deploy
helmfile -l name=grafana apply
helmfile -l name=grafana destroy
```

**Render manifests without applying:**

```bash
cd deploy && helmfile template
```
   
## License

See [LICENSE](LICENSE).