package httphandler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"govatars/internal/models"
	"govatars/internal/usecase"
)

// Server implements ServerInterface.
type Server struct {
	health         *usecase.Health
	avatars        AvatarQueries
	maxUploadBytes int64
	thumbLabels    []string
	log            *slog.Logger
}

var _ ServerInterface = (*Server)(nil)

// NewServer returns an HTTP API handler. Optional settings via [ServerOption]; logger defaults to a discard logger.
func NewServer(health *usecase.Health, avatars AvatarQueries, maxUploadBytes int64, thumbLabels []string, opts ...ServerOption) *Server {
	s := &Server{
		health:         health,
		avatars:        avatars,
		maxUploadBytes: maxUploadBytes,
		thumbLabels:    append([]string(nil), thumbLabels...),
		log:            defaultServerLogger(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Server) GetHealth(ctx echo.Context) error {
	st := s.health.Status(ctx.Request().Context())
	return ctx.JSON(http.StatusOK, st)
}

func (s *Server) UploadAvatar(ctx echo.Context, params UploadAvatarParams) error {
	file, err := ctx.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	fh, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	defer func() {
		if err := fh.Close(); err != nil {
			s.log.WarnContext(ctx.Request().Context(), "multipart file close", "err", err)
		}
	}()

	res, err := s.avatars.Upload(ctx.Request().Context(), params.XUserID, file.Filename, fh)
	if err != nil {
		return s.mapAvatarErr(ctx, err)
	}

	return ctx.JSON(http.StatusCreated, AvatarUploadResponse{
		Id:        res.ID,
		UserId:    res.UserID,
		Url:       res.URL,
		Status:    res.Status,
		CreatedAt: res.CreatedAt,
	})
}

func (s *Server) DeleteAvatarById(ctx echo.Context, avatarId AvatarId, params DeleteAvatarByIdParams) error {
	id := avatarId
	err := s.avatars.DeleteByID(ctx.Request().Context(), params.XUserID, id)
	if err != nil {
		return s.mapAvatarErr(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) GetAvatarImage(ctx echo.Context, avatarId AvatarId, params GetAvatarImageParams) error {
	id := avatarId
	size := ptrStringEnum(params.Size)
	format := ptrStringEnum(params.Format)

	pl, err := s.avatars.GetImage(ctx.Request().Context(), id, size, format)
	if err != nil {
		return s.mapAvatarErr(ctx, err)
	}
	return s.streamImagePayload(ctx, pl, "avatar image reader close")
}

func (s *Server) GetAvatarMetadata(ctx echo.Context, avatarId AvatarId) error {
	id := avatarId
	a, err := s.avatars.ByID(ctx.Request().Context(), id)
	if err != nil {
		return s.mapAvatarErr(ctx, err)
	}

	dimW, dimH := 0, 0
	if a.Width != nil {
		dimW = *a.Width
	}
	if a.Height != nil {
		dimH = *a.Height
	}

	thumbs := make([]ThumbnailInfo, 0, len(s.thumbLabels)*len(models.ThumbnailFormats))
	for _, label := range s.thumbLabels {
		if a.ThumbnailS3Keys == nil {
			continue
		}
		byFormat, ok := a.ThumbnailS3Keys[label]
		if !ok || len(byFormat) == 0 {
			continue
		}
		for format, key := range byFormat {
			if key == "" {
				continue
			}
			thumbs = append(thumbs, ThumbnailInfo{
				Size:   label,
				Format: ThumbnailInfoFormat(format),
				Url:    s.avatars.ThumbnailURL(a.ID, label, format),
			})
		}
	}

	meta := AvatarMetadata{
		Id:         a.ID,
		UserId:     a.UserID,
		FileName:   a.FileName,
		MimeType:   a.MimeType,
		Size:       a.SizeBytes,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
		Thumbnails: thumbs,
	}
	meta.Dimensions.Width = dimW
	meta.Dimensions.Height = dimH

	return ctx.JSON(http.StatusOK, meta)
}

func (s *Server) DeleteUserAvatar(ctx echo.Context, userId UserIdPath, params DeleteUserAvatarParams) error {
	err := s.avatars.DeleteLatestForUser(ctx.Request().Context(), params.XUserID, userId)
	if err != nil {
		return s.mapAvatarErr(ctx, err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

func (s *Server) GetUserAvatar(ctx echo.Context, userId UserIdPath, params GetUserAvatarParams) error {
	size := ptrStringEnum(params.Size)
	format := ptrStringEnum(params.Format)

	pl, err := s.avatars.GetLatestImageForUser(ctx.Request().Context(), userId, size, format)
	if err != nil {
		return s.mapAvatarErr(ctx, err)
	}
	return s.streamImagePayload(ctx, pl, "user avatar image reader close")
}

func (s *Server) ListUserAvatars(ctx echo.Context, userId UserIdPath) error {
	list, err := s.avatars.ListByUser(ctx.Request().Context(), userId)
	if err != nil {
		return s.mapAvatarErr(ctx, err)
	}

	out := make([]AvatarListItem, 0, len(list))
	for _, a := range list {
		out = append(out, AvatarListItem{
			Id:        a.ID,
			Status:    displayStatus(a),
			CreatedAt: a.CreatedAt,
		})
	}
	return ctx.JSON(http.StatusOK, out)
}

func displayStatus(a models.Avatar) string {
	if a.ProcessingStatus == models.ProcessingStatusCompleted {
		return "ready"
	}
	return "processing"
}

func ptrStringEnum[S ~string](p *S) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func (s *Server) streamImagePayload(ctx echo.Context, pl *usecase.ImagePayload, readerCloseLog string) error {
	defer func() {
		if err := pl.Reader.Close(); err != nil {
			s.log.WarnContext(ctx.Request().Context(), readerCloseLog, "err", err)
		}
	}()
	h := ctx.Response().Header()
	h.Set("ETag", pl.ETag)
	h.Set("Cache-Control", pl.CacheControl)
	if pl.ContentLength >= 0 {
		h.Set(echo.HeaderContentLength, strconv.FormatInt(pl.ContentLength, 10))
	}
	return ctx.Stream(http.StatusOK, pl.ContentType, pl.Reader)
}

func (s *Server) mapAvatarErr(ctx echo.Context, err error) *echo.HTTPError {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, NotFound{Error: "Avatar not found"})
	case errors.Is(err, usecase.ErrForbidden):
		details := "You can only delete your own avatars"
		return echo.NewHTTPError(http.StatusForbidden, Forbidden{Error: "Forbidden", Details: &details})
	case errors.Is(err, usecase.ErrPayloadTooLarge):
		maxBytes := s.maxUploadBytes
		if maxBytes <= 0 {
			maxBytes = 10 * 1024 * 1024
		}
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, PayloadTooLarge{Error: "File too large", MaxSize: maxBytes})
	case errors.Is(err, usecase.ErrInvalidImage):
		details := "Supported formats: jpeg, png, webp"
		return echo.NewHTTPError(http.StatusBadRequest, BadRequest{Error: "Invalid file format", Details: &details})
	default:
		if errors.Is(err, io.EOF) {
			return echo.NewHTTPError(http.StatusBadRequest, "empty file")
		}
		if err != nil {
			s.log.ErrorContext(ctx.Request().Context(), "avatar api internal error", "err", err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
