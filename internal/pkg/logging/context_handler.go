package logging

import (
	"context"
	"log/slog"

	"govatars/internal/pkg/contextlib"
)

// ContextHandler enriches each log record with HTTP request fields from [contextlib.RequestInfo]
// when the log is emitted with a request context (e.g. [slog.Logger.ErrorContext]) on the server logger from [NewServerLogger].
//
// WithAttrs and WithGroup are overridden to keep the wrapper alive after [slog.Logger.With] / [slog.Logger.WithGroup];
// without these overrides, method promotion would unwrap the inner handler and silently drop request enrichment.
type ContextHandler struct {
	slog.Handler
}

// Handle implements [slog.Handler].
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx == nil {
		return h.Handler.Handle(ctx, r)
	}

	reqInfo, ok := contextlib.GetRequestInfo(ctx)
	if !ok {
		return h.Handler.Handle(ctx, r)
	}

	attrs := []slog.Attr{
		slog.String("request_id", reqInfo.RequestID),
		slog.String("remote_ip", reqInfo.RemoteIP),
		slog.String("host", reqInfo.Host),
		slog.String("method", reqInfo.Method),
		slog.String("path", reqInfo.Path),
		slog.String("user_agent", reqInfo.UserAgent),
	}
	if userID, ok := contextlib.GetUserID(ctx); ok {
		attrs = append(attrs, slog.String("user_id", userID))
	}
	r.AddAttrs(attrs...)

	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a [ContextHandler] wrapping the inner handler's WithAttrs result.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a [ContextHandler] wrapping the inner handler's WithGroup result.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}
