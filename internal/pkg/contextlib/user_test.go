package contextlib

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetUserID_GetUserID(t *testing.T) {
	ctx := SetUserID(context.Background(), "  spaced  ")
	uid, ok := GetUserID(ctx)
	require.True(t, ok)
	require.Equal(t, "spaced", uid)

	ctx2 := SetUserID(context.Background(), "")
	_, ok = GetUserID(ctx2)
	require.False(t, ok)
}

func TestGetUserID_missing(t *testing.T) {
	ctx := context.Background()
	_, ok := GetUserID(ctx)
	require.False(t, ok)

	ctx = SetRequestInfo(ctx, RequestInfo{Path: "/"})
	_, ok = GetUserID(ctx)
	require.False(t, ok)
}
