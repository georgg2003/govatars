package httpserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/config"
)

func TestEchoBodyLimit(t *testing.T) {
	cfg := &config.App{}
	cfg.Normalize()
	require.Equal(t, "12M", EchoBodyLimit(nil))
	require.Contains(t, EchoBodyLimit(cfg), "M")
}
