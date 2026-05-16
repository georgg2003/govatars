package worker

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestRetryCountFromHeaders(t *testing.T) {
	require.Equal(t, 0, retryCountFromHeaders(nil))
	require.Equal(t, 0, retryCountFromHeaders(amqp.Table{}))
	require.Equal(t, 2, retryCountFromHeaders(amqp.Table{"x-retry-count": int32(2)}))
	require.Equal(t, 3, retryCountFromHeaders(amqp.Table{"x-retry-count": int64(3)}))
	require.Equal(t, 4, retryCountFromHeaders(amqp.Table{"x-retry-count": 4}))
}
