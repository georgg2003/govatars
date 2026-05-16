package worker

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/config"
)

func TestNewApp_nilProcessor(t *testing.T) {
	log := slog.Default()
	_, err := NewApp(context.Background(), log, nil, config.RabbitMQ{})
	require.Error(t, err)
}
