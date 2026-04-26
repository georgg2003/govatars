package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// AccessLog emits one structured log per HTTP request (status, bytes, latency, optional error).
// Log level follows status: 5xx -> Error, 4xx -> Warn, else Info.
//
// Register after [RequestContext] so [logging.ContextHandler] enriches records with request fields.
// Register before [github.com/labstack/echo/v4/middleware.RecoverWithConfig] (wrap it) so every
// request still produces one access line, including panics (final status e.g. 500).
func AccessLog(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			res := c.Response()

			err := next(c)
			stop := time.Now()

			status := res.Status
			if status == 0 {
				status = http.StatusOK
			}
			if err != nil && status == http.StatusOK {
				var he *echo.HTTPError
				if errors.As(err, &he) {
					status = he.Code
				}
			}

			logCtx := c.Request().Context()
			attrs := []any{
				slog.Int("status", status),
				slog.Int64("bytes_out", res.Size),
				slog.String("latency", fmt.Sprint(stop.Sub(start))),
			}

			level := levelForStatus(status)
			msg := "request completed"
			if err != nil {
				msg = "request completed with error"
				attrs = append(attrs, slog.String("error", err.Error()))
			}
			logger.Log(logCtx, level, msg, attrs...)
			return err
		}
	}
}

func levelForStatus(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
