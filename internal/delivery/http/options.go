package httphandler

import (
	"log/slog"

	"govatars/internal/pkg/logging"
)

// ServerOption configures [Server].
type ServerOption func(*Server)

// WithLogger sets the structured logger. The default is [logging.DiscardLogger].
func WithLogger(log *slog.Logger) ServerOption {
	return func(s *Server) {
		if log != nil {
			s.log = log
		}
	}
}

func defaultServerLogger() *slog.Logger {
	return logging.DiscardLogger()
}
