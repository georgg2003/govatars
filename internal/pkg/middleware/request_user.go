package middleware

import (
	"github.com/labstack/echo/v4"

	"govatars/internal/pkg/contextlib"
)

// RequestUserID reads [contextlib.HeaderXUserID] and stores the value with [contextlib.SetUserID].
// Register before [RequestContext] so [contextlib.RequestInfo].UserID matches this value.
func RequestUserID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			ctx := contextlib.SetUserID(req.Context(), req.Header.Get(contextlib.HeaderXUserID))
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}
