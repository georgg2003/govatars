package httphandler_test

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	httphandler "govatars/internal/delivery/http"
	"govatars/internal/pkg/config"
)

// testHTTPServerOpts returns max upload bytes and thumbnail labels from the normalized default config.
func testHTTPServerOpts(t *testing.T) (int64, []string) {
	t.Helper()
	c := &config.App{}
	c.Normalize()
	cat, err := c.Avatars.Catalog()
	require.NoError(t, err)
	return c.Avatars.MaxUploadBytes, cat.Labels
}

// newTestEcho registers the OpenAPI handler set on a fresh Echo without middleware.
func newTestEcho(t *testing.T, s *httphandler.Server) *echo.Echo {
	t.Helper()
	e := echo.New()
	httphandler.RegisterHandlers(e, s)
	return e
}

// buildMultipartFile produces a multipart body with one "file" form part and returns body+content-type.
func buildMultipartFile(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &body, w.FormDataContentType()
}
