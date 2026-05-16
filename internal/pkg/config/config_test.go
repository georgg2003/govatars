package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/config"
)

func TestLoad_OverrideFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
http:
  address: ":9999"
postgres:
  dsn: "postgres://test:test@localhost:5432/db?sslmode=disable"
`), 0o600)
	require.NoError(t, err)

	old := os.Args
	t.Cleanup(func() { os.Args = old })
	// Use short flag -config/-c to avoid pflag misparsing combined -config + path on some Go test runners.
	os.Args = []string{"test", "-c", p}

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, ":9999", cfg.HTTP.Address)
	require.Contains(t, cfg.Postgres.DSN, "postgres://")
	require.Equal(t, "info", cfg.Logging.Level)
}

func TestLoad_RabbitMQConsumerTags_ViperDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
http:
  address: ":8080"
postgres:
  dsn: "postgres://test:test@localhost:5432/db?sslmode=disable"
`), 0o600)
	require.NoError(t, err)

	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"test", "-c", p}

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "govatars-upload", cfg.RabbitMQ.UploadConsumerTag)
	require.Equal(t, "govatars-delete", cfg.RabbitMQ.DeleteConsumerTag)
}

func TestAvatars_Catalog_Defaults(t *testing.T) {
	var cfg config.App
	cfg.Normalize()
	require.Equal(t, "max-age=86400", cfg.Avatars.ImageCacheControl)
	cat, err := cfg.Avatars.Catalog()
	require.NoError(t, err)
	require.NotEmpty(t, cat.Labels)
	for _, label := range cat.Labels {
		require.Positive(t, cat.Sides[label])
	}
}

func TestPostgres_ResolveDSN_FromFields(t *testing.T) {
	p := config.Postgres{
		Host:     "db.example",
		Port:     5433,
		User:     "u",
		Password: "p@ss",
		Database: "appdb",
		SSLMode:  "require",
	}
	dsn, err := p.ResolveDSN()
	require.NoError(t, err)
	require.Contains(t, dsn, "db.example:5433")
	require.Contains(t, dsn, "appdb")
	require.Contains(t, dsn, "sslmode=require")
}
