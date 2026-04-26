package httpserver

import (
	"fmt"

	"govatars/internal/pkg/config"
)

// EchoBodyLimit returns an Echo [middleware.BodyLimit] string derived from the same upload cap as the domain layer.
func EchoBodyLimit(cfg *config.App) string {
	if cfg == nil || cfg.Avatars.MaxUploadBytes <= 0 {
		return "12M"
	}
	return formatByteLimit(cfg.Avatars.MaxUploadBytes)
}

func formatByteLimit(n int64) string {
	switch {
	case n <= 0:
		return "1M"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		k := (n + 1023) / 1024
		return fmt.Sprintf("%dK", k)
	default:
		m := (n + 1024*1024 - 1) / (1024 * 1024)
		return fmt.Sprintf("%dM", m)
	}
}
