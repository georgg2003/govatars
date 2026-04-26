package logging

import (
	"log/slog"
	"strings"
)

// LevelFromString maps a config value to [slog.Level]. Recognized: debug, info, warn, warning, error.
// Empty or unknown values default to info (same behavior as the former LOG_LEVEL env default).
func LevelFromString(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}
