package logging

import (
	"context"
	"log/slog"
)

// levelHandler drops records below minLevel before delegating to the inner handler.
type levelHandler struct {
	slog.Handler
	minLevel slog.Level
}

func newLevelHandler(h slog.Handler, minLevel slog.Level) slog.Handler {
	return levelHandler{Handler: h, minLevel: minLevel}
}

func (h levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.minLevel && h.Handler.Enabled(ctx, level)
}

// multiHandler fan-outs each record to every child handler (stderr + OTLP, etc.).
type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	return multiHandler{handlers: handlers}
}

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for i, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		rec := r
		if i > 0 {
			rec = r.Clone()
		}
		if err := h.Handle(ctx, rec); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return multiHandler{handlers: next}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return multiHandler{handlers: next}
}
