package contextlib

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetGetRequestInfo(t *testing.T) {
	ctx := context.Background()
	_, ok := GetRequestInfo(ctx)
	require.False(t, ok)

	ri := RequestInfo{
		RequestID: "rid-1",
		RemoteIP:  "127.0.0.1",
		Host:      "example.com",
		Method:    "GET",
		Path:      "/api/v1/health",
		UserAgent: "test-agent",
		UserID:    "user-42",
	}
	ctx = SetRequestInfo(ctx, ri)
	got, ok := GetRequestInfo(ctx)
	require.True(t, ok)
	require.Equal(t, ri, got)

	uid, ok := GetUserID(ctx)
	require.True(t, ok)
	require.Equal(t, "user-42", uid)
}
