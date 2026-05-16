package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/contextlib"
)

func TestRequestUserID_setsContext(t *testing.T) {
	e := echo.New()
	e.Use(RequestUserID())
	e.GET("/", func(c echo.Context) error {
		uid, ok := contextlib.GetUserID(c.Request().Context())
		require.True(t, ok)
		require.Equal(t, "u42", uid)
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.Header.Set(contextlib.HeaderXUserID, "u42")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestRequestUserID_emptyHeader(t *testing.T) {
	e := echo.New()
	e.Use(RequestUserID())
	e.GET("/", func(c echo.Context) error {
		_, ok := contextlib.GetUserID(c.Request().Context())
		require.False(t, ok)
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}
