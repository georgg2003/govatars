package web

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"govatars/internal/mocks"
	"govatars/internal/usecase"
)

type WebHandlersSuite struct {
	suite.Suite

	echo *echo.Echo
	ctrl *gomock.Controller
	mock *mocks.MockUploader
}

func (s *WebHandlersSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mock = mocks.NewMockUploader(s.ctrl)
	s.echo = echo.New()
	New(s.mock, "web/static").Register(s.echo)
}

func (s *WebHandlersSuite) postUpload(body *bytes.Buffer, ct string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/web/upload", body)
	req.Header.Set(echo.HeaderContentType, ct)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	return rec
}

func (s *WebHandlersSuite) buildBody(withUserID bool) (*bytes.Buffer, string) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if withUserID {
		s.Require().NoError(w.WriteField("user_id", "u42"))
	}
	fw, err := w.CreateFormFile("file", "a.png")
	s.Require().NoError(err)
	_, err = fw.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().NoError(w.Close())
	return &body, w.FormDataContentType()
}

func (s *WebHandlersSuite) TestPostUpload_Redirect() {
	s.mock.EXPECT().Upload(gomock.Any(), "u42", "a.png", gomock.Any()).
		Return(&usecase.UploadResult{}, nil)

	body, ct := s.buildBody(true)
	rec := s.postUpload(body, ct)
	s.Equal(http.StatusSeeOther, rec.Code)
	s.Equal("/web/gallery/u42", rec.Header().Get(echo.HeaderLocation))
}

func (s *WebHandlersSuite) TestPostUpload_MissingUser() {
	body, ct := s.buildBody(false)
	rec := s.postUpload(body, ct)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *WebHandlersSuite) TestPostUpload_InternalErrorIsSanitized() {
	s.mock.EXPECT().Upload(gomock.Any(), "u42", "a.png", gomock.Any()).
		Return(nil, errors.New("s3 access denied"))

	body, ct := s.buildBody(true)
	rec := s.postUpload(body, ct)

	s.Equal(http.StatusInternalServerError, rec.Code)
	s.Contains(rec.Body.String(), "internal server error")
	s.NotContains(rec.Body.String(), "s3 access denied")
}

func TestWebHandlersSuite(t *testing.T) {
	suite.Run(t, new(WebHandlersSuite))
}
