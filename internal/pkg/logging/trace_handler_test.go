package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceContextHandler_addsTraceFields(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	h := newTraceContextHandler(inner)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:  trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	logger := slog.New(h)
	logger.InfoContext(ctx, "hello")

	require.Contains(t, buf.String(), `"trace_id":"0102030405060708090a0b0c0d0e0f10"`)
	require.Contains(t, buf.String(), `"span_id":"0102030405060708"`)
}
