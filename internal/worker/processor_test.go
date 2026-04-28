package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"govatars/internal/models"
	"govatars/internal/pkg/config"
	"govatars/internal/repomocks"
	"govatars/internal/usecase"
)

type ProcessorSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	repo  *repomocks.MockAvatarRepository
	store *repomocks.MockObjectStorage
	cfg   config.RabbitMQ
	proc  *Processor
	ch    *MockamqpPublisher
}

func (s *ProcessorSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = repomocks.NewMockAvatarRepository(s.ctrl)
	s.store = repomocks.NewMockObjectStorage(s.ctrl)
	s.cfg = testRabbit()
	jobs := usecase.NewAvatarQueueJobs(slog.New(slog.DiscardHandler), s.repo, s.store, testThumbCatalog())
	s.proc = NewProcessor(slog.New(slog.DiscardHandler), jobs, s.cfg)
	s.ch = NewMockamqpPublisher(s.ctrl)
}

func (s *ProcessorSuite) TestHandleUploadDelivery_BadJSON_NoError() {
	err := s.proc.HandleUploadDelivery(context.Background(), s.ch, amqp.Delivery{Body: []byte("{")})
	s.Require().NoError(err)
}

func (s *ProcessorSuite) TestHandleUploadDelivery_ProcessSuccess() {
	id := uuid.New()
	key := "originals/u/" + id.String() + ".png"
	a := &models.Avatar{
		ID: id, UserID: "u", S3Key: key, MimeType: "image/png",
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByIDIncludingDeleted(gomock.Any(), id).Return(a, nil)
	s.store.EXPECT().GetObject(gomock.Any(), key).Return(io.NopCloser(bytes.NewReader(minPNG)), nil)
	cat := testThumbCatalog()
	s.store.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(len(cat.Labels) * len(models.ThumbnailFormats))
	s.repo.EXPECT().UpdateProcessingResult(gomock.Any(), id, 1, 1, gomock.Any(), models.ProcessingStatusCompleted).Return(nil)

	body, err := json.Marshal(models.AvatarUploadEvent{AvatarID: id.String(), UserID: "u", S3Key: key})
	s.Require().NoError(err)
	s.Require().NoError(s.proc.HandleUploadDelivery(context.Background(), s.ch, amqp.Delivery{Body: body}))
}

func (s *ProcessorSuite) TestHandleUploadDelivery_RepublishOnFailure() {
	id := uuid.New()
	key := "originals/u/" + id.String() + ".png"
	a := &models.Avatar{
		ID: id, UserID: "u", S3Key: key, MimeType: "image/png",
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByIDIncludingDeleted(gomock.Any(), id).Return(a, nil)
	s.store.EXPECT().GetObject(gomock.Any(), key).Return(nil, errors.New("s3"))
	s.repo.EXPECT().SetProcessingStatus(gomock.Any(), id, models.ProcessingStatusFailed).Return(nil)

	body, err := json.Marshal(models.AvatarUploadEvent{AvatarID: id.String(), UserID: "u", S3Key: key})
	s.Require().NoError(err)
	s.ch.EXPECT().PublishWithContext(gomock.Any(), "", "upload.q.retry.0", false, false, gomock.Any()).
		Do(func(_ context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) {
			s.Empty(exchange)
			s.Equal("upload.q.retry.0", key)
			s.False(mandatory)
			s.False(immediate)
			s.EqualValues(1, msg.Headers["x-retry-count"])
		}).Return(nil)
	s.Require().NoError(s.proc.HandleUploadDelivery(context.Background(), s.ch, amqp.Delivery{Body: body}))
}

func (s *ProcessorSuite) TestHandleUploadDelivery_DLQAfterRetries() {
	id := uuid.New()
	key := "originals/u/" + id.String() + ".png"
	a := &models.Avatar{
		ID: id, UserID: "u", S3Key: key, MimeType: "image/png",
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.repo.EXPECT().GetByIDIncludingDeleted(gomock.Any(), id).Return(a, nil)
	s.store.EXPECT().GetObject(gomock.Any(), key).Return(nil, errors.New("s3"))
	s.repo.EXPECT().SetProcessingStatus(gomock.Any(), id, models.ProcessingStatusFailed).Return(nil)

	body, err := json.Marshal(models.AvatarUploadEvent{AvatarID: id.String(), UserID: "u", S3Key: key})
	s.Require().NoError(err)
	h := amqp.Table{"x-retry-count": int32(2)} // already past last delay index (len=2 -> indices 0,1)
	s.ch.EXPECT().PublishWithContext(gomock.Any(), s.cfg.Exchange, s.cfg.UploadDLQRoutingKey, false, false, gomock.Any()).
		Return(nil)
	s.Require().NoError(s.proc.HandleUploadDelivery(context.Background(), s.ch, amqp.Delivery{Body: body, Headers: h}))
}

func (s *ProcessorSuite) TestHandleDeleteDelivery_BadJSON() {
	err := s.proc.HandleDeleteDelivery(context.Background(), s.ch, amqp.Delivery{Body: []byte("x")})
	s.Require().NoError(err)
}

func TestProcessorSuite(t *testing.T) {
	suite.Run(t, new(ProcessorSuite))
}
