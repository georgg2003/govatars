package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"govatars/internal/pkg/contextlib"
	"govatars/internal/pkg/logging"
)

type AccessLogSuite struct {
	suite.Suite

	buf  *bytes.Buffer
	echo *echo.Echo
}

func (s *AccessLogSuite) SetupTest() {
	s.buf = &bytes.Buffer{}
	inner := slog.NewJSONHandler(s.buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(&logging.ContextHandler{Handler: inner})

	s.echo = echo.New()
	s.echo.Use(echomw.RequestID())
	s.echo.Use(RequestUserID())
	s.echo.Use(RequestContext())
	s.echo.Use(AccessLog(log))
}

func (s *AccessLogSuite) TestAccessLog_OK() {
	s.echo.GET("/ok", func(c echo.Context) error {
		ri, ok := contextlib.GetRequestInfo(c.Request().Context())
		s.Require().True(ok)
		s.Equal("/ok", ri.Path)
		s.Equal("GET", ri.Method)
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/ok?token=secret", nil)
	req.Header.Set(contextlib.HeaderXUserID, "alice")
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	body := s.buf.String()
	s.Contains(body, "request completed")
	s.Contains(body, `"level":"INFO"`)
	s.Contains(body, `"user_id":"alice"`)
	s.NotContains(body, "secret", "raw query must not be in access log")
}

func (s *AccessLogSuite) TestAccessLog_ClientErrorIsWarn() {
	s.echo.GET("/boom", func(_ echo.Context) error {
		return echo.NewHTTPError(http.StatusTeapot, "nope")
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)

	s.Equal(http.StatusTeapot, rec.Code)
	body := s.buf.String()
	s.Contains(body, "request completed with error")
	s.Contains(body, `"level":"WARN"`, "4xx must be logged at WARN, not ERROR")
	s.Contains(body, `"status":418`)
	s.Contains(body, "nope")
}

func (s *AccessLogSuite) TestAccessLog_ServerErrorIsError() {
	s.echo.GET("/oops", func(_ echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "kaboom")
	})

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/oops", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)

	s.Equal(http.StatusInternalServerError, rec.Code)
	body := s.buf.String()
	s.Contains(body, `"level":"ERROR"`)
	s.Contains(body, `"status":500`)
}

func TestAccessLogSuite(t *testing.T) {
	suite.Run(t, new(AccessLogSuite))
}

// TestAccessLog_wrapsRecover documents production order: AccessLog wraps Recover so the access line
// is always emitted after the request (including panic -> 500), with RequestContext on the log ctx.
func TestAccessLog_wrapsRecover(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(&logging.ContextHandler{Handler: inner})

	e := echo.New()
	e.Use(echomw.RequestID())
	e.Use(RequestUserID())
	e.Use(RequestContext())
	e.Use(AccessLog(log))
	e.Use(echomw.RecoverWithConfig(echomw.RecoverConfig{DisablePrintStack: true}))
	e.GET("/panic", func(c echo.Context) error {
		panic("boom")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	body := buf.String()
	// Echo Recover handles the panic and writes 500 without returning a non-nil error to AccessLog.
	require.Contains(t, body, "request completed")
	require.Contains(t, body, `"level":"ERROR"`)
	require.Contains(t, body, `"status":500`)
}

