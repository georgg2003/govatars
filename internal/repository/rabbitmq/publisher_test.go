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
