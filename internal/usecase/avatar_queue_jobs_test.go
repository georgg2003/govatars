package usecase_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"govatars/internal/models"
	"govatars/internal/repomocks"
	"govatars/internal/usecase"
)

type AvatarQueueJobsSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	repo  *repomocks.MockAvatarRepository
	store *repomocks.MockObjectStorage
	jobs  *usecase.AvatarQueueJobs
}

func (s *AvatarQueueJobsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = repomocks.NewMockAvatarRepository(s.ctrl)
	s.store = repomocks.NewMockObjectStorage(s.ctrl)
	s.jobs = usecase.NewAvatarQueueJobs(testDiscardLog(), s.repo, s.store, testCatalog(), nil)
}

func (s *AvatarQueueJobsSuite) TestProcessAvatarUpload_Success() {
	id := uuid.New()
	key := "originals/u/" + id.String() + ".png"
	a := &models.Avatar{
		ID: id, UserID: "u", S3Key: key, MimeType: "image/png",
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByIDIncludingDeleted(gomock.Any(), id).Return(a, nil)
	s.store.EXPECT().GetObject(gomock.Any(), key).Return(io.NopCloser(bytes.NewReader(minPNG)), nil)
	cat := testCatalog()
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(len(cat.Labels) * len(models.ThumbnailFormats))
	s.repo.EXPECT().UpdateProcessingResult(gomock.Any(), id, 1, 1, gomock.Any(), models.ProcessingStatusCompleted).Return(nil)

	err := s.jobs.ProcessAvatarUpload(context.Background(), &models.AvatarUploadEvent{
		AvatarID: id.String(), UserID: "u", S3Key: key,
	})
	s.Require().NoError(err)
}

func (s *AvatarQueueJobsSuite) TestProcessAvatarUpload_NotFound() {
	id := uuid.New()
	s.repo.EXPECT().GetByIDIncludingDeleted(gomock.Any(), id).Return(nil, usecase.ErrNotFound)

	err := s.jobs.ProcessAvatarUpload(context.Background(), &models.AvatarUploadEvent{
		AvatarID: id.String(), S3Key: "k",
	})
	s.Require().NoError(err)
}

func (s *AvatarQueueJobsSuite) TestProcessAvatarUpload_AlreadyCompleted() {
	id := uuid.New()
	a := &models.Avatar{
		ID: id, S3Key: "k", ProcessingStatus: models.ProcessingStatusCompleted,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByIDIncludingDeleted(gomock.Any(), id).Return(a, nil)

	err := s.jobs.ProcessAvatarUpload(context.Background(), &models.AvatarUploadEvent{
		AvatarID: id.String(), S3Key: "k",
	})
	s.Require().NoError(err)
}

func (s *AvatarQueueJobsSuite) TestProcessAvatarDelete() {
	s.store.EXPECT().RemoveObject(gomock.Any(), "a").Return(nil)
	s.store.EXPECT().RemoveObject(gomock.Any(), "b").Return(nil)

	err := s.jobs.ProcessAvatarDelete(context.Background(), &models.AvatarDeleteEvent{S3Keys: []string{"a", "b"}})
	s.Require().NoError(err)
}

func TestAvatarQueueJobsSuite(t *testing.T) {
	suite.Run(t, new(AvatarQueueJobsSuite))
}
