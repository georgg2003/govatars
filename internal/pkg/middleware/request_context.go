package middleware

import (
	"github.com/labstack/echo/v4"

	"govatars/internal/pkg/contextlib"
)

// RequestContext attaches [contextlib.RequestInfo] to the request context (request id, IP, path, user id, etc.).
//
// Register after [github.com/labstack/echo/v4/middleware.RequestID] and [RequestUserID], and before
// [AccessLog] and [github.com/labstack/echo/v4/middleware.RecoverWithConfig] so panic logs and handlers
// see the same enriched context.
func RequestContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()

			reqID := req.Header.Get(echo.HeaderXRequestID)
			if reqID == "" {
				reqID = res.Header().Get(echo.HeaderXRequestID)
			}

			ctx := req.Context()
			userID, _ := contextlib.GetUserID(ctx)

			ctx = contextlib.SetRequestInfo(ctx, contextlib.RequestInfo{
				RequestID: reqID,
				RemoteIP:  c.RealIP(),
				Host:      req.Host,
				Method:    req.Method,
				Path:      req.URL.Path,
				UserAgent: req.UserAgent(),
				UserID:    userID,
			})
			c.SetRequest(req.WithContext(ctx))

			return next(c)
		}
	}
}
