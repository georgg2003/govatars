package rabbitmq_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/config"
	"govatars/internal/repository/rabbitmq"
)

func TestNewPublisher_EmptyURL(t *testing.T) {
	_, err := rabbitmq.NewPublisher(context.Background(), slog.New(slog.DiscardHandler), config.RabbitMQ{})
	require.Error(t, err)
}

func TestNewPublisher_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rabbitmq.NewPublisher(ctx, slog.New(slog.DiscardHandler), config.RabbitMQ{URL: "amqp://guest:guest@127.0.0.1:5672/"})
	require.ErrorIs(t, err, context.Canceled)
}
