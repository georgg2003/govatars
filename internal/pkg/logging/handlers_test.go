package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLevelHandler_filtersBelowMinLevel(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := newLevelHandler(inner, slog.LevelWarn)

	require.False(t, h.Enabled(context.Background(), slog.LevelInfo))

	logger := slog.New(h)
	logger.Info("skipped")
	logger.Warn("kept")

	require.NotContains(t, buf.String(), "skipped")
	require.Contains(t, buf.String(), "kept")
}

func TestMultiHandler_writesToAllHandlers(t *testing.T) {
	var a, b bytes.Buffer
	h := newMultiHandler(
		slog.NewJSONHandler(&a, nil),
		slog.NewJSONHandler(&b, nil),
	)
	logger := slog.New(h)
	logger.Info("both")

	require.Contains(t, a.String(), "both")
	require.Contains(t, b.String(), "both")
}

func TestMultiHandler_WithAttrsPropagates(t *testing.T) {
	var buf bytes.Buffer
	h := newMultiHandler(slog.NewJSONHandler(&buf, nil))
	logger := slog.New(h).With("scope", "x")
	logger.Info("msg")

	require.Contains(t, buf.String(), `"scope":"x"`)
	require.Contains(t, buf.String(), "msg")
}
