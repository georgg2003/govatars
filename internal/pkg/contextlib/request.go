package contextlib

import "context"

// RequestInfo is attached to each HTTP request context for structured logging (see gophermart pattern).
type RequestInfo struct {
	RequestID string
	Host      string
	RemoteIP  string
	Method    string
	Path      string
	UserAgent string
	UserID    string // optional: e.g. X-User-ID when present
}

type reqInfoKey struct{}

var rik = reqInfoKey{}

// SetRequestInfo returns ctx with request metadata for handlers and downstream code.
func SetRequestInfo(ctx context.Context, ri RequestInfo) context.Context {
	return context.WithValue(ctx, rik, ri)
}

// GetRequestInfo returns metadata set by HTTP middleware, if any.
func GetRequestInfo(ctx context.Context) (RequestInfo, bool) {
	ri, ok := ctx.Value(rik).(RequestInfo)
	return ri, ok
}
