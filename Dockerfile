# Multi-stage build: HTTP API and worker binaries.
FROM golang:1.26-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
	&& CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
	&& adduser -D -H -u 65532 appuser

WORKDIR /app

COPY --from=builder --chown=appuser:appuser /out/server /out/worker ./
COPY --chown=appuser:appuser config/config.yaml ./config/config.yaml
COPY --chown=appuser:appuser web/static ./web/static

USER appuser

EXPOSE 8080

CMD ["./server"]
