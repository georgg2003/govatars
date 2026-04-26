package httphandler_test

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	httphandler "govatars/internal/delivery/http"
	"govatars/internal/mocks"
	"govatars/internal/models"
	"govatars/internal/usecase"
)

type HandlersSuite struct {
	suite.Suite

	ctrl   *gomock.Controller
	mock   *mocks.MockAvatarQueries
	echo   *echo.Echo
	maxUp  int64
	labels []string
}

func (s *HandlersSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mock = mocks.NewMockAvatarQueries(s.ctrl)
	s.maxUp, s.labels = testHTTPServerOpts(s.T())
	s.echo = newTestEcho(s.T(), httphandler.NewServer(usecase.NewHealth(nil, nil, nil), s.mock, s.maxUp, s.labels))
}

func (s *HandlersSuite) TestGetHealth() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"status"`)
}

func (s *HandlersSuite) TestGetAvatarMetadata() {
	id := uuid.New()
	thumbKeys := make(map[string]map[string]string)
	for _, label := range s.labels {
		thumbKeys[label] = map[string]string{models.ThumbnailFormatJPEG: "k_" + label}
	}
	s.mock.EXPECT().ByID(gomock.Any(), id).Return(&models.Avatar{
		ID: id, UserID: "u1", FileName: "f.png", MimeType: "image/png", SizeBytes: 10,
		ProcessingStatus: models.ProcessingStatusCompleted,
		CreatedAt:        time.Now(), UpdatedAt: time.Now(),
		ThumbnailS3Keys: thumbKeys,
	}, nil)
	for _, label := range s.labels {
		l := label
		q := url.Values{}
		q.Set("format", models.ThumbnailFormatJPEG)
		q.Set("size", l)
		s.mock.EXPECT().ThumbnailURL(id, l, models.ThumbnailFormatJPEG).Return(
			"http://localhost/api/v1/avatars/" + id.String() + "?" + q.Encode())
	}

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/api/v1/avatars/"+id.String()+"/metadata", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"thumbnails"`)
}

func (s *HandlersSuite) TestUploadAvatar_MissingFile() {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	s.Require().NoError(w.Close())

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/api/v1/avatars", body)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *HandlersSuite) TestUploadAvatar_InvalidImage() {
	s.mock.EXPECT().Upload(gomock.Any(), "user-1", "x.png", gomock.Any()).Return(nil, usecase.ErrInvalidImage)

	body, ct := buildMultipartFile(s.T(), "x.png", []byte("bad"))
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/api/v1/avatars", body)
	req.Header.Set(echo.HeaderContentType, ct)
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *HandlersSuite) TestListUserAvatars() {
	s.mock.EXPECT().ListByUser(gomock.Any(), "alice").Return([]models.Avatar{}, nil)

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/api/v1/users/alice/avatars", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *HandlersSuite) TestListUserAvatars_ProcessingStatus() {
	id := uuid.New()
	s.mock.EXPECT().ListByUser(gomock.Any(), "bob").Return([]models.Avatar{{
		ID:               id,
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt:        time.Now(),
	}}, nil)

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/api/v1/users/bob/avatars", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), "processing")
}

func (s *HandlersSuite) TestDeleteAvatarById_NotFound() {
	id := uuid.New()
	s.mock.EXPECT().DeleteByID(gomock.Any(), "u1", id).Return(usecase.ErrNotFound)

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodDelete, "/api/v1/avatars/"+id.String(), nil)
	req.Header.Set("X-User-Id", "u1")
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusNotFound, rec.Code)
}

func (s *HandlersSuite) TestGetUserAvatar_Stream() {
	pl := &usecase.ImagePayload{
		Reader:        io.NopCloser(bytes.NewReader([]byte("avatar-bytes"))),
		ContentType:   "image/png",
		ContentLength: 13,
		ETag:          `"etag-user"`,
		CacheControl:  "max-age=60",
	}
	s.mock.EXPECT().GetLatestImageForUser(gomock.Any(), "alice", "", "").Return(pl, nil)

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/api/v1/users/alice/avatar", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Equal("image/png", rec.Header().Get("Content-Type"))
	s.Equal(`"etag-user"`, rec.Header().Get("ETag"))
}

func (s *HandlersSuite) TestGetAvatarImage_Stream() {
	id := uuid.New()
	pl := &usecase.ImagePayload{
		Reader:        io.NopCloser(bytes.NewReader([]byte("img"))),
		ContentType:   "image/jpeg",
		ContentLength: 3,
		ETag:          `"abc"`,
		CacheControl:  "max-age=1",
	}
	s.mock.EXPECT().GetImage(gomock.Any(), id, "", "").Return(pl, nil)

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/api/v1/avatars/"+id.String(), nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Equal("image/jpeg", rec.Header().Get("Content-Type"))
}

func (s *HandlersSuite) TestGetAvatarImage_InternalErrorIsSanitized() {
	id := uuid.New()
	s.mock.EXPECT().GetImage(gomock.Any(), id, "", "").Return(nil, errors.New("postgres auth failed"))

	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/api/v1/avatars/"+id.String(), nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)

	s.Equal(http.StatusInternalServerError, rec.Code)
	s.Contains(rec.Body.String(), "internal server error")
	s.NotContains(rec.Body.String(), "postgres auth failed")
}

func TestHandlersSuite(t *testing.T) {
	suite.Run(t, new(HandlersSuite))
}
