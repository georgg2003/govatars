package contextlib

import (
	"context"
	"strings"
)

// HeaderXUserID is the HTTP header carrying the trusted caller user id (API gateway / auth proxy).
const HeaderXUserID = "X-User-Id"

type userIDKey struct{}

var uidKey = userIDKey{}

// SetUserID attaches a trimmed user id to ctx. Empty input is a no-op so [GetUserID] can still fall back to [RequestInfo].
func SetUserID(ctx context.Context, userID string) context.Context {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, uidKey, userID)
}

// GetUserID returns the user id from [SetUserID] when set, otherwise from [RequestInfo].UserID.
func GetUserID(ctx context.Context) (string, bool) {
	if v, ok := ctx.Value(uidKey).(string); ok && v != "" {
		return v, true
	}
	ri, ok := GetRequestInfo(ctx)
	if !ok || ri.UserID == "" {
		return "", false
	}
	return ri.UserID, true
}
