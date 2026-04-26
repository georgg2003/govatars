package logging

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLevelFromString(t *testing.T) {
	require.Equal(t, slog.LevelDebug, LevelFromString("debug"))
	require.Equal(t, slog.LevelInfo, LevelFromString("info"))
	require.Equal(t, slog.LevelInfo, LevelFromString(""))
	require.Equal(t, slog.LevelWarn, LevelFromString("warn"))
	require.Equal(t, slog.LevelWarn, LevelFromString("warning"))
	require.Equal(t, slog.LevelError, LevelFromString("error"))
	require.Equal(t, slog.LevelInfo, LevelFromString("unknown"))
}
