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

## Logging

- `**internal/pkg/logging.New()**` returns a `***slog.Logger**` (JSON to stderr). `**cmd/server**` and `**cmd/worker**` create it once and pass it where needed (worker `**NewProcessor` / `NewApp**`, server `**main**` and panic handler). There is **no** `slog.SetDefault` — dependencies stay explicit.
- **HTTP access logs**: Echo `RequestID` (`X-Request-ID`), then `internal/pkg/middleware` attaches request metadata to `context` and logs one JSON line per request (`request_id`, `host`, `remote_ip`, `method`, `path`, `status`, `latency`, `user_id` from `X-User-ID` — empty string when the header is absent, etc.).
- Set `**LOG_LEVEL`** to `debug`, `info`, `warn`, or `error` to tune slog verbosity (default `**info`**).

## Development

1. Start dependencies: `make deps-up` (see `[docker-compose.deps.yml](docker-compose.deps.yml)`).
2. Apply migrations: `make migrate-up` (runs `[golang-migrate](https://github.com/golang-migrate/migrate)` via `go run -tags postgres …@version`; the CLI must be built with the `postgres` tag so the driver is linked — same idea as `go install -tags postgres …` to `GOPATH/bin`. It is not listed under `tool` in `[go.mod](go.mod)`: `go tool migrate` does not apply build tags, and adding the binary as a `tool` pulls a very large indirect dependency graph).
3. Run API from the repository root (`api/swagger.yaml` is read at startup): `make run-server` or `go run ./cmd/server`.
4. Run the worker (thumbnail + S3 cleanup consumers): `make run-worker`.

Other **Makefile** targets: `build`, `test`, `install-lint` (installs **golangci-lint** into `**$(go env GOPATH)/bin`**), `lint` (runs that binary by full path — **no `PATH` setup required**), `generate` (OpenAPI codegen), `deps-down`, `migrate-down`.

## License

See [LICENSE](LICENSE).