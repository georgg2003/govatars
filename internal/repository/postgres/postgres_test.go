package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/config"
	"govatars/internal/repository/postgres"
)

func TestNew_InvalidPostgresConfig(t *testing.T) {
	_, err := postgres.New(context.Background(), config.Postgres{}, false)
	require.Error(t, err)
}
