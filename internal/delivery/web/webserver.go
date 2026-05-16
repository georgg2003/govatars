package web

import (
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"

	"govatars/internal/pkg/logging"
	"govatars/internal/usecase"
)

// WebServer serves browser routes (HTML upload/gallery under staticDir).
type WebServer struct {
	avatars   Uploader
	staticDir string
	log       *slog.Logger
}

// Option configures [WebServer].
type Option func(*WebServer)

// WithLogger sets the structured logger. The default is [logging.DiscardLogger].
func WithLogger(log *slog.Logger) Option {
	return func(w *WebServer) {
		if log != nil {
			w.log = log
		}
	}
}

// New builds a [WebServer]. Pass [WithLogger] to use a real logger; otherwise logs are discarded.
func New(avatars Uploader, staticDir string, opts ...Option) *WebServer {
	w := &WebServer{
		avatars:   avatars,
		staticDir: staticDir,
		log:       logging.DiscardLogger(),
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Register attaches routes to e (upload page, POST upload, gallery page, static files).
func (w *WebServer) Register(e *echo.Echo) {
	dir := w.staticDir
	e.GET("/web/upload", func(c echo.Context) error {
		return c.File(filepath.Join(dir, "upload.html"))
	})
	e.POST("/web/upload", w.postUpload())
	e.GET("/web/gallery/:user_id", func(c echo.Context) error {
		return c.File(filepath.Join(dir, "gallery.html"))
	})
	e.Static("/web", dir)
}

func (w *WebServer) postUpload() echo.HandlerFunc {
	return func(c echo.Context) error {
		userID := c.FormValue("user_id")
		if userID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "user_id is required")
		}
		file, err := c.FormFile("file")
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "file is required")
		}
		fh, err := file.Open()
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		defer func() {
			if err := fh.Close(); err != nil {
				w.log.WarnContext(c.Request().Context(), "multipart file close", "err", err)
			}
		}()

		if _, err := w.avatars.Upload(c.Request().Context(), userID, file.Filename, fh); err != nil {
			return w.mapUploadErr(c, err)
		}
		return c.Redirect(http.StatusSeeOther, "/web/gallery/"+userID)
	}
}

func (w *WebServer) mapUploadErr(c echo.Context, err error) *echo.HTTPError {
	switch {
	case errors.Is(err, usecase.ErrPayloadTooLarge):
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file too large")
	case errors.Is(err, usecase.ErrInvalidImage):
		return echo.NewHTTPError(http.StatusBadRequest, "invalid image")
	default:
		w.log.ErrorContext(c.Request().Context(), "web upload: internal error (response sanitized)", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
