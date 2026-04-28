package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"govatars/internal/models"
	"govatars/internal/repomocks"
	"govatars/internal/usecase"
)

type AvatarServiceSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	repo  *repomocks.MockAvatarRepository
	store *repomocks.MockObjectStorage
	pub   *repomocks.MockEventPublisher
	svc   *usecase.AvatarService
}

func (s *AvatarServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = repomocks.NewMockAvatarRepository(s.ctrl)
	s.store = repomocks.NewMockObjectStorage(s.ctrl)
	s.pub = repomocks.NewMockEventPublisher(s.ctrl)
	s.svc = usecase.NewAvatarService(s.repo, s.store, s.pub, testCfg(), testCatalog(), testDiscardLog())
}

func (s *AvatarServiceSuite) TestUpload_InvalidBody() {
	_, err := s.svc.Upload(context.Background(), "u1", "x.png", strings.NewReader("not-a-real-image"))
	s.Require().ErrorIs(err, usecase.ErrInvalidImage)
}

func (s *AvatarServiceSuite) TestUpload_Empty() {
	_, err := s.svc.Upload(context.Background(), "u1", "x.png", strings.NewReader(""))
	s.Require().ErrorIs(err, usecase.ErrInvalidImage)
}

func (s *AvatarServiceSuite) TestUpload_Success() {
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "image/png").Return(nil)
	s.repo.EXPECT().Insert(gomock.Any(), gomock.AssignableToTypeOf(&models.Avatar{})).Return(nil)
	s.pub.EXPECT().PublishJSON(gomock.Any(), "avatar.upload", gomock.Any(), gomock.Any()).Return(nil)

	res, err := s.svc.Upload(context.Background(), "user-1", "a.png", bytes.NewReader(minPNG))
	s.Require().NoError(err)
	s.Equal("user-1", res.UserID)
	s.Equal("processing", res.Status)
	s.Contains(res.URL, "/api/v1/avatars/")
}

func (s *AvatarServiceSuite) TestUpload_PutFails() {
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "image/png").Return(errors.New("s3 down"))

	_, err := s.svc.Upload(context.Background(), "u", "a.png", bytes.NewReader(minPNG))
	s.Require().Error(err)
}

func (s *AvatarServiceSuite) TestUpload_InsertFails_Rollback() {
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "image/png").Return(nil)
	s.repo.EXPECT().Insert(gomock.Any(), gomock.AssignableToTypeOf(&models.Avatar{})).Return(errors.New("db"))
	s.store.EXPECT().RemoveObject(gomock.Any(), gomock.Any()).Return(nil)

	_, err := s.svc.Upload(context.Background(), "u", "a.png", bytes.NewReader(minPNG))
	s.Require().Error(err)
}

func (s *AvatarServiceSuite) TestUpload_PublishFails_Rollback() {
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "image/png").Return(nil)
	s.repo.EXPECT().Insert(gomock.Any(), gomock.AssignableToTypeOf(&models.Avatar{})).Return(nil)
	s.pub.EXPECT().PublishJSON(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("mq"))
	s.repo.EXPECT().DeleteHard(gomock.Any(), gomock.Any()).Return(nil)
	s.store.EXPECT().RemoveObject(gomock.Any(), gomock.Any()).Return(nil)

	_, err := s.svc.Upload(context.Background(), "u", "a.png", bytes.NewReader(minPNG))
	s.Require().Error(err)
}

func (s *AvatarServiceSuite) TestGetImage_NotFound() {
	id := uuid.New()
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(nil, usecase.ErrNotFound)

	_, err := s.svc.GetImage(context.Background(), id, "", "")
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *AvatarServiceSuite) TestGetImage_OK() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, UserID: "u", MimeType: "image/png", S3Key: "originals/u/x.png",
		ProcessingStatus: models.ProcessingStatusCompleted,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(a, nil)
	// buildImagePayload: no-transcode original — Stat+Get against S3 key without buffering.
	s.store.EXPECT().StatObject(gomock.Any(), a.S3Key).Return(int64(len(minPNG)), "abc123", nil)
	s.store.EXPECT().GetObject(gomock.Any(), a.S3Key).Return(io.NopCloser(bytes.NewReader(minPNG)), nil)

	pl, err := s.svc.GetImage(context.Background(), id, "", "")
	s.Require().NoError(err)
	s.Equal("image/png", pl.ContentType)
	s.Equal(`"abc123"`, pl.ETag)
	s.Equal(int64(len(minPNG)), pl.ContentLength)
	s.Equal("max-age=86400", pl.CacheControl)
	b, rerr := io.ReadAll(pl.Reader)
	s.Require().NoError(rerr)
	s.NotEmpty(b)
	s.Require().NoError(pl.Reader.Close())
}

func (s *AvatarServiceSuite) TestGetImage_NoTranscode_FallsBackOnS3Error() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, UserID: "u", MimeType: "image/png", S3Key: "originals/u/x.png",
		ProcessingStatus: models.ProcessingStatusCompleted,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(a, nil)
	// Stat fails -> buildImagePayload falls through to buildFromOriginal (ReadAll path).
	s.store.EXPECT().StatObject(gomock.Any(), a.S3Key).Return(int64(0), "", errors.New("net glitch"))
	s.store.EXPECT().GetObject(gomock.Any(), a.S3Key).Return(io.NopCloser(bytes.NewReader(minPNG)), nil)

	pl, err := s.svc.GetImage(context.Background(), id, "", "")
	s.Require().NoError(err)
	s.Equal("image/png", pl.ContentType)
	s.Require().NoError(pl.Reader.Close())
}

func (s *AvatarServiceSuite) TestGetLatestImageForUser_NoAvatar_NoPlaceholder() {
	s.repo.EXPECT().GetLatestByUser(gomock.Any(), "nobody").Return(nil, usecase.ErrNotFound)

	_, err := s.svc.GetLatestImageForUser(context.Background(), "nobody", "", "")
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *AvatarServiceSuite) TestDeleteByID_Forbidden() {
	id := uuid.New()
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(&models.Avatar{ID: id, UserID: "owner"}, nil)

	err := s.svc.DeleteByID(context.Background(), "other", id)
	s.Require().ErrorIs(err, usecase.ErrForbidden)
}

func (s *AvatarServiceSuite) TestDeleteByID_OK() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, UserID: "me", S3Key: "k",
		ProcessingStatus: models.ProcessingStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(a, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), id, "me").Return(nil)
	s.pub.EXPECT().PublishJSON(gomock.Any(), "avatar.delete", id.String(), gomock.Any()).Return(nil)

	s.Require().NoError(s.svc.DeleteByID(context.Background(), "me", id))
}

func (s *AvatarServiceSuite) TestDeleteByID_PublishFails_RollbackOK() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, UserID: "me", S3Key: "k",
		ProcessingStatus: models.ProcessingStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	pubErr := errors.New("mq down")
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(a, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), id, "me").Return(nil)
	s.pub.EXPECT().PublishJSON(gomock.Any(), "avatar.delete", id.String(), gomock.Any()).Return(pubErr)
	s.repo.EXPECT().RestoreSoftDeleted(gomock.Any(), id, "me").Return(nil)

	err := s.svc.DeleteByID(context.Background(), "me", id)
	s.Require().ErrorIs(err, pubErr)
}

func (s *AvatarServiceSuite) TestDeleteByID_PublishFails_RollbackFails() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, UserID: "me", S3Key: "k",
		ProcessingStatus: models.ProcessingStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	pubErr := errors.New("mq down")
	rollbackErr := errors.New("rollback failed")
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(a, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), id, "me").Return(nil)
	s.pub.EXPECT().PublishJSON(gomock.Any(), "avatar.delete", id.String(), gomock.Any()).Return(pubErr)
	s.repo.EXPECT().RestoreSoftDeleted(gomock.Any(), id, "me").Return(rollbackErr)

	err := s.svc.DeleteByID(context.Background(), "me", id)
	s.Require().Error(err)
	s.Require().ErrorIs(err, pubErr)
	s.Require().ErrorIs(err, rollbackErr)
}

func (s *AvatarServiceSuite) TestDeleteLatestForUser_PublishFails_RollbackOK() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, UserID: "me", S3Key: "k",
		ProcessingStatus: models.ProcessingStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	pubErr := errors.New("mq down")
	s.repo.EXPECT().GetLatestByUser(gomock.Any(), "me").Return(a, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), id, "me").Return(nil)
	s.pub.EXPECT().PublishJSON(gomock.Any(), "avatar.delete", id.String(), gomock.Any()).Return(pubErr)
	s.repo.EXPECT().RestoreSoftDeleted(gomock.Any(), id, "me").Return(nil)

	err := s.svc.DeleteLatestForUser(context.Background(), "me", "me")
	s.Require().ErrorIs(err, pubErr)
}

func (s *AvatarServiceSuite) TestByID_NotFound() {
	id := uuid.New()
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(nil, usecase.ErrNotFound)

	_, err := s.svc.ByID(context.Background(), id)
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *AvatarServiceSuite) TestListByUser() {
	s.repo.EXPECT().ListByUser(gomock.Any(), "u").Return([]models.Avatar{}, nil)

	list, err := s.svc.ListByUser(context.Background(), "u")
	s.Require().NoError(err)
	s.Empty(list)
}

func (s *AvatarServiceSuite) TestThumbnailURL() {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	u := s.svc.ThumbnailURL(id, "100x100", "")
	s.Contains(u, "100x100")
	s.Contains(u, id.String())
}

func (s *AvatarServiceSuite) TestGetImage_UsesThumbnailKey() {
	id := uuid.New()
	thumbKey := "thumbnails/" + id.String() + "/100x100.jpg"
	a := &models.Avatar{
		ID: id, UserID: "u", MimeType: "image/png", S3Key: "originals/u/x.png",
		ThumbnailS3Keys: map[string]map[string]string{"100x100": {models.ThumbnailFormatJPEG: thumbKey}},
		ProcessingStatus: models.ProcessingStatusCompleted,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(a, nil)
	s.store.EXPECT().StatObject(gomock.Any(), thumbKey).Return(int64(len(minPNG)), "etagval", nil)
	s.store.EXPECT().GetObject(gomock.Any(), thumbKey).Return(io.NopCloser(bytes.NewReader(minPNG)), nil)

	pl, err := s.svc.GetImage(context.Background(), id, "100x100", "")
	s.Require().NoError(err)
	s.Equal("image/jpeg", pl.ContentType)
	s.Require().NoError(pl.Reader.Close())
}

func (s *AvatarServiceSuite) TestGetImage_FormatJPEG() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, MimeType: "image/png", S3Key: "k",
		ProcessingStatus: models.ProcessingStatusCompleted,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByID(gomock.Any(), id).Return(a, nil)
	s.store.EXPECT().GetObject(gomock.Any(), "k").Return(io.NopCloser(bytes.NewReader(minPNG)), nil)

	pl, err := s.svc.GetImage(context.Background(), id, "", "jpeg")
	s.Require().NoError(err)
	s.Equal("image/jpeg", pl.ContentType)
	s.Require().NoError(pl.Reader.Close())
}

func TestAvatarServiceSuite(t *testing.T) {
	suite.Run(t, new(AvatarServiceSuite))
}
