package rabbitmq

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDialContext_CancelledBeforeDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := DialContext(ctx, "amqp://guest:guest@127.0.0.1:5672/")
	require.ErrorIs(t, err, context.Canceled)
}

func TestDialContext_DoesNotHangOnUnreachableBroker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	// Closed port: fast TCP refusal; must not block for OS-level long timeouts.
	_, err := DialContext(ctx, "amqp://guest:guest@google.com/", WithConnectionTimeout(300*time.Millisecond))
	t.Log(err)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 250*time.Millisecond, "dial should respect context and fail quickly")
}

func TestDialContext_WithConnectionTimeout(t *testing.T) {
	start := time.Now()
	_, err := Dial("amqp://guest:guest@google.com/", WithConnectionTimeout(300*time.Millisecond))
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 350*time.Millisecond, "dial should respect context and fail quickly")
}
