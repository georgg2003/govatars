package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/contextlib"
)

func TestContextHandler_enrichesFromContext(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(&ContextHandler{Handler: inner})

	ctx := contextlib.SetRequestInfo(context.Background(), contextlib.RequestInfo{
		RequestID: "r1",
		RemoteIP:  "10.0.0.1",
		Host:      "api.example.com",
		Method:    "POST",
		Path:      "/api/v1/avatars",
		UserAgent: "curl",
		UserID:    "u99",
	})

	log.InfoContext(ctx, "hello", "k", "v")
	s := buf.String()
	require.Contains(t, s, `"request_id":"r1"`)
	require.Contains(t, s, `"path":"/api/v1/avatars"`)
	require.Contains(t, s, `"host":"api.example.com"`)
	require.Contains(t, s, `"user_id":"u99"`)
}

func TestContextHandler_noRequestInfo(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(&ContextHandler{Handler: inner})

	log.InfoContext(context.Background(), "hello")
	s := buf.String()
	require.NotContains(t, s, `"request_id"`)
}

func TestContextHandler_omitsEmptyUserID(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(&ContextHandler{Handler: inner})

	ctx := contextlib.SetRequestInfo(context.Background(), contextlib.RequestInfo{
		RequestID: "r-anon",
		Path:      "/anon",
	})

	log.InfoContext(ctx, "anon hello")
	s := buf.String()
	require.Contains(t, s, `"request_id":"r-anon"`)
	require.NotContains(t, s, `"user_id"`, "empty user_id must not appear in logs")
}

func TestContextHandler_WithAttrsKeepsEnrichment(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(&ContextHandler{Handler: inner}).With("scope", "test")

	ctx := contextlib.SetRequestInfo(context.Background(), contextlib.RequestInfo{
		RequestID: "r2",
		Path:      "/scoped",
	})

	log.InfoContext(ctx, "scoped hello")
	s := buf.String()
	require.Contains(t, s, `"scope":"test"`)
	require.Contains(t, s, `"request_id":"r2"`, "ContextHandler must remain wrapped after With(...)")
	require.Contains(t, s, `"path":"/scoped"`)
}

func TestContextHandler_WithGroupKeepsEnrichment(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(&ContextHandler{Handler: inner}).WithGroup("g")

	ctx := contextlib.SetRequestInfo(context.Background(), contextlib.RequestInfo{
		RequestID: "r3",
	})

	log.InfoContext(ctx, "grouped hello", "k", "v")
	s := buf.String()
	require.Contains(t, s, `"request_id":"r3"`, "ContextHandler must remain wrapped after WithGroup(...)")
}
