package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceContextHandler adds trace_id and span_id from the active span when present.
type traceContextHandler struct {
	slog.Handler
}

func newTraceContextHandler(h slog.Handler) slog.Handler {
	return traceContextHandler{Handler: h}
}

func (h traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		sc := trace.SpanFromContext(ctx).SpanContext()
		if sc.IsValid() {
			r.AddAttrs(
				slog.String("trace_id", sc.TraceID().String()),
				slog.String("span_id", sc.SpanID().String()),
			)
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h traceContextHandler) WithGroup(name string) slog.Handler {
	return traceContextHandler{Handler: h.Handler.WithGroup(name)}
}
