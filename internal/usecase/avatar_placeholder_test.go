package usecase

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"govatars/internal/models"
	"govatars/internal/pkg/config"
	"govatars/internal/repomocks"
)

type AvatarPlaceholderSuite struct {
	suite.Suite

	ctrl             *gomock.Controller
	store            *repomocks.MockObjectStorage
	svc              *AvatarService
	placeholderBytes []byte
}

func TestAvatarPlaceholderSuite(t *testing.T) {
	suite.Run(t, new(AvatarPlaceholderSuite))
}

func (s *AvatarPlaceholderSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = repomocks.NewMockObjectStorage(s.ctrl)
	s.svc, s.placeholderBytes = s.newPlaceholderTestService()
}

// makePlaceholderPNG returns a small valid PNG used as the in-memory placeholder for these tests.
func (s *AvatarPlaceholderSuite) makePlaceholderPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: 0xa0, G: 0xa0, B: 0xa0, A: 0xff})
		}
	}
	var buf bytes.Buffer
	s.Require().NoError(png.Encode(&buf, img))
	return buf.Bytes()
}

func (s *AvatarPlaceholderSuite) newPlaceholderTestService() (*AvatarService, []byte) {
	cfg := &config.App{HTTP: config.HTTP{PublicBaseURL: "http://localhost:8080"}}
	cfg.Normalize()
	cat, err := cfg.Avatars.Catalog()
	s.Require().NoError(err)
	svc := NewAvatarService(context.Background(), nil, s.store, nil, cfg, cat, slog.New(slog.DiscardHandler), nil)
	bs := s.makePlaceholderPNG()
	decoded, err := imaging.Decode(bytes.NewReader(bs))
	s.Require().NoError(err)
	svc.ph.raw = bs
	svc.ph.img = decoded
	svc.ph.contentType = "image/png"
	svc.ph.originalKey = models.PlaceholderOriginalKey("png")
	return svc, bs
}

func (s *AvatarPlaceholderSuite) expectedPlaceholderPutCount() int {
	expectedThumbCount := 0
	for _, label := range s.svc.thumbLabels {
		if s.svc.thumbSides[label] > 0 {
			expectedThumbCount += len(models.ThumbnailFormats)
		}
	}
	return 1 + expectedThumbCount
}

func (s *AvatarPlaceholderSuite) TestBuildPlaceholderPayload_S3Hit() {
	expectedKey := models.PlaceholderThumbnailKey("100x100", models.ThumbnailFormatJPEG)
	body := []byte("stored-thumbnail-bytes")
	s.store.EXPECT().StatObject(gomock.Any(), expectedKey).Return(int64(len(body)), "etag123", nil)
	s.store.EXPECT().GetObject(gomock.Any(), expectedKey).Return(io.NopCloser(bytes.NewReader(body)), nil)

	pl, err := s.svc.buildPlaceholderPayload(context.Background(), "100x100", "")
	s.Require().NoError(err)
	s.Require().NotNil(pl)
	s.Equal("image/jpeg", pl.ContentType)
	s.Equal(int64(len(body)), pl.ContentLength)
	s.Equal(`"etag123"`, pl.ETag)
	got, err := io.ReadAll(pl.Reader)
	s.Require().NoError(err)
	s.Equal(body, got)
	s.Require().NoError(pl.Reader.Close())
}

func (s *AvatarPlaceholderSuite) TestBuildPlaceholderPayload_S3Hit_Original() {
	expectedKey := models.PlaceholderOriginalKey("png")
	body := []byte("stored-original")
	s.store.EXPECT().StatObject(gomock.Any(), expectedKey).Return(int64(len(body)), "etagO", nil)
	s.store.EXPECT().GetObject(gomock.Any(), expectedKey).Return(io.NopCloser(bytes.NewReader(body)), nil)

	pl, err := s.svc.buildPlaceholderPayload(context.Background(), "", "")
	s.Require().NoError(err)
	s.Equal("image/png", pl.ContentType)
	s.Require().NoError(pl.Reader.Close())
}

func (s *AvatarPlaceholderSuite) TestBuildPlaceholderPayload_FallbackOnS3Miss() {
	expectedKey := models.PlaceholderThumbnailKey("100x100", models.ThumbnailFormatJPEG)
	s.store.EXPECT().StatObject(gomock.Any(), expectedKey).Return(int64(0), "", ErrObjectNotFound)

	pl, err := s.svc.buildPlaceholderPayload(context.Background(), "100x100", "")
	s.Require().NoError(err)
	s.Require().NotNil(pl)
	s.Equal("image/png", pl.ContentType)
	s.Positive(pl.ContentLength)
	s.Require().NoError(pl.Reader.Close())
}

func (s *AvatarPlaceholderSuite) TestBuildPlaceholderPayload_FallbackOnS3RealError() {
	expectedKey := models.PlaceholderThumbnailKey("100x100", models.ThumbnailFormatJPEG)
	s.store.EXPECT().StatObject(gomock.Any(), expectedKey).Return(int64(0), "", errors.New("dial tcp: i/o timeout"))

	pl, err := s.svc.buildPlaceholderPayload(context.Background(), "100x100", "")
	s.Require().NoError(err)
	s.Require().NotNil(pl, "real S3 errors must still fall back to in-memory render")
	s.Require().NoError(pl.Reader.Close())
}

func (s *AvatarPlaceholderSuite) TestEnsurePlaceholderInS3_Idempotent() {
	expectedPuts := s.expectedPlaceholderPutCount()

	s.store.EXPECT().StatObject(gomock.Any(), gomock.Any()).Return(int64(0), "", ErrObjectNotFound).Times(expectedPuts)
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string, _ io.Reader, _ int64, _ string) error {
			s.Require().NotEmpty(key)
			return nil
		}).Times(expectedPuts)

	s.Require().NoError(s.svc.EnsurePlaceholderInS3(context.Background()))

	s.NotEmpty(s.placeholderBytes)

	s.store.EXPECT().StatObject(gomock.Any(), gomock.Any()).Return(int64(1), "etag", nil).Times(expectedPuts)

	s.Require().NoError(s.svc.EnsurePlaceholderInS3(context.Background()))
}

func (s *AvatarPlaceholderSuite) TestEnsurePlaceholderInS3_ContinuesAfterPerVariantError() {
	expectedPuts := s.expectedPlaceholderPutCount()

	s.store.EXPECT().StatObject(gomock.Any(), gomock.Any()).Return(int64(0), "", ErrObjectNotFound).Times(expectedPuts)

	first := true
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
			if first {
				first = false
				return errors.New("transient")
			}
			return nil
		}).Times(expectedPuts)

	err := s.svc.EnsurePlaceholderInS3(context.Background())
	s.Require().Error(err, "errors.Join must surface the failure")
	s.Contains(err.Error(), "transient")
}

func (s *AvatarPlaceholderSuite) TestEnsurePlaceholderInS3_ZeroSizeTriggersReupload() {
	expectedPuts := s.expectedPlaceholderPutCount()

	s.store.EXPECT().StatObject(gomock.Any(), gomock.Any()).Return(int64(0), "", nil).Times(expectedPuts)
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).Times(expectedPuts)

	s.Require().NoError(s.svc.EnsurePlaceholderInS3(context.Background()))
}

func (s *AvatarPlaceholderSuite) TestEnsurePlaceholderInS3_NoPlaceholderIsNoop() {
	s.svc.ph.raw = nil
	s.svc.ph.img = nil

	s.Require().NoError(s.svc.EnsurePlaceholderInS3(context.Background()))
}
