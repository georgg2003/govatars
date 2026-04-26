package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/contextlib"
)

func TestRequestContext_setsRequestInfo(t *testing.T) {
	e := echo.New()
	e.Use(echomw.RequestID())
	e.Use(RequestUserID())
	e.Use(RequestContext())
	e.GET("/x", func(c echo.Context) error {
		ri, ok := contextlib.GetRequestInfo(c.Request().Context())
		require.True(t, ok)
		require.Equal(t, "/x", ri.Path)
		require.Equal(t, http.MethodGet, ri.Method)
		require.NotEmpty(t, ri.RequestID)
		uid, ok := contextlib.GetUserID(c.Request().Context())
		require.True(t, ok)
		require.Equal(t, "actor-1", uid)
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.Header.Set(contextlib.HeaderXUserID, "actor-1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}
