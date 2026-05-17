// Package logging builds JSON slog loggers for binaries. Callers keep *slog.Logger and pass it into constructors (no global SetDefault).
package logging

import (
	"govatars/internal/pkg/otelpkg"
	"io"
	"log/slog"
	"os"
)

// NewJSONHandlerOptions returns handler options for JSON output at the given level.
func NewJSONHandlerOptions(level slog.Level) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level: level,
	}
}

func newBaseHandler(w io.Writer, level slog.Level) slog.Handler {
	return slog.NewJSONHandler(w, NewJSONHandlerOptions(level))
}

// NewServerLogger returns a logger for the API process: JSON to stderr, plus HTTP fields when using LogContext / InfoContext / ErrorContext / etc. with a request context (see [ContextHandler]).
func NewServerLogger(level slog.Level) *slog.Logger {
	return slog.New(&ContextHandler{Handler: newBaseHandler(os.Stderr, level)})
}

// NewOTELServerLogger exports logs to OTLP and mirrors JSON logs to stderr at level.
func NewOTELServerLogger(otelLoggerProvider *otelpkg.OTELLoggerProvider, level slog.Level) *slog.Logger {
	otelH := newLevelHandler(otelLoggerProvider.NewSlogHandler(), level)
	stderrH := newBaseHandler(os.Stderr, level)
	return slog.New(&ContextHandler{Handler: newMultiHandler(otelH, stderrH)})
}

// NewWorkerLogger returns a logger for the worker process: JSON to stderr without HTTP request enrichment (no [ContextHandler]).
func NewWorkerLogger(level slog.Level) *slog.Logger {
	return slog.New(newBaseHandler(os.Stderr, level))
}

// NewOTELWorkerLogger exports logs to OTLP and mirrors JSON logs to stderr at level.
func NewOTELWorkerLogger(otelLoggerProvider *otelpkg.OTELLoggerProvider, level slog.Level) *slog.Logger {
	otelH := newLevelHandler(otelLoggerProvider.NewSlogHandler(), level)
	stderrH := newBaseHandler(os.Stderr, level)
	return slog.New(newMultiHandler(otelH, stderrH))
}

// DiscardLogger returns a logger that discards all records. Use as the default for optional loggers (e.g. functional options).
func DiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
