//go:build integration

package worker_test

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
	tc "github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"

	"govatars/internal/models"
	"govatars/internal/pkg/config"
	"govatars/internal/repository/postgres"
	"govatars/internal/repository/rabbitmq"
	s3repo "govatars/internal/repository/s3"
	"govatars/internal/testdb"
	"govatars/internal/usecase"
	"govatars/internal/worker"
)

// 1×1 PNG used in worker integration tests.
var minPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

// WorkerAppSuite runs worker.App.Run against real Postgres + MinIO + RabbitMQ testcontainers.
type WorkerAppSuite struct {
	suite.Suite

	pgCtr *tcpostgres.PostgresContainer
	mnCtr *tcminio.MinioContainer
	mqCtr *tcrabbit.RabbitMQContainer

	pool      *postgres.Pool
	repo      *postgres.AvatarRepository
	s3Client  *s3repo.Client
	publisher *rabbitmq.Publisher
	cfg       config.RabbitMQ
	bucket    string

	appCancel context.CancelFunc
	appDone   chan struct{}
}

func (s *WorkerAppSuite) SetupSuite() {
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine")
	s.Require().NoError(err)
	s.pgCtr = pg

	mn, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-01-16T16-07-38Z")
	s.Require().NoError(err)
	s.mnCtr = mn

	mq, err := tcrabbit.Run(ctx, "rabbitmq:3.12-management-alpine")
	s.Require().NoError(err)
	s.mqCtr = mq

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	s.Require().NoError(err)
	s.Require().Eventually(func() bool {
		s.pool, err = postgres.New(ctx, config.Postgres{DSN: dsn}, false)
		return err == nil
	}, 20*time.Second, 300*time.Millisecond, "postgres container is not ready: %v", err)
	s.applyMigrations(ctx)
	s.repo = postgres.NewAvatarRepository(s.pool.Pgx())

	endpoint, err := mn.ConnectionString(ctx)
	s.Require().NoError(err)
	s.bucket = "wt-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	s.s3Client, err = s3repo.New(ctx, config.S3{
		Endpoint:  endpoint,
		AccessKey: mn.Username,
		SecretKey: mn.Password,
		Bucket:    s.bucket,
		UseSSL:    false,
		Region:    "us-east-1",
	})
	s.Require().NoError(err)

	amqpURL, err := mq.AmqpURL(ctx)
	s.Require().NoError(err)
	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	s.cfg = config.RabbitMQ{
		URL:                   amqpURL,
		Exchange:              "wt_ex_" + suffix,
		UploadRoutingKey:      "avatar.uploaded",
		DeleteRoutingKey:      "avatar.deleted",
		UploadQueue:           "wt_upload_" + suffix,
		DeleteQueue:           "wt_delete_" + suffix,
		UploadDLQQueue:        "wt_upload_dlq_" + suffix,
		UploadDLQRoutingKey:   "avatar.upload.failed",
		DeleteDLQQueue:        "wt_delete_dlq_" + suffix,
		DeleteDLQRoutingKey:   "avatar.delete.failed",
		UploadRetryDelaysMS:   []int{200},
		DeleteRetryDelaysMS:   []int{200},
		ConsumerHandleTimeout: 30 * time.Second,
	}

	s.publisher, err = rabbitmq.NewPublisher(ctx, slog.New(slog.DiscardHandler), s.cfg)
	s.Require().NoError(err)

	cat, err := (&config.App{}).Avatars.Catalog()
	if err != nil {
		// fall back to defaults
		def := &config.App{}
		def.Normalize()
		cat, err = def.Avatars.Catalog()
		s.Require().NoError(err)
	}

	jobs := usecase.NewAvatarQueueJobs(slog.New(slog.DiscardHandler), s.repo, s.s3Client, cat, nil)
	proc := worker.NewProcessor(slog.New(slog.DiscardHandler), jobs, s.cfg, nil)

	appCtx, cancel := context.WithCancel(context.Background())
	s.appCancel = cancel
	app, err := worker.NewApp(appCtx, slog.New(slog.DiscardHandler), proc, s.cfg)
	s.Require().NoError(err)

	s.appDone = make(chan struct{})
	go func() {
		defer close(s.appDone)
		if err := app.Run(appCtx); err != nil {
			s.T().Logf("worker app run: %v", err)
		}
		if err := app.Close(); err != nil {
			s.T().Logf("worker app close: %v", err)
		}
	}()

	// Give consumers a moment to subscribe so the first publish is not raced.
	time.Sleep(500 * time.Millisecond)
}

func (s *WorkerAppSuite) TearDownSuite() {
	if s.appCancel != nil {
		s.appCancel()
	}
	if s.appDone != nil {
		select {
		case <-s.appDone:
		case <-time.After(15 * time.Second):
			s.T().Log("worker.App.Run did not exit in time")
		}
	}
	if s.publisher != nil {
		if err := s.publisher.Close(); err != nil {
			s.T().Logf("publisher close: %v", err)
		}
	}
	if s.pool != nil {
		s.pool.Close()
	}
	if s.mqCtr != nil {
		if err := tc.TerminateContainer(s.mqCtr); err != nil {
			s.T().Logf("terminate rabbitmq container: %v", err)
		}
	}
	if s.mnCtr != nil {
		if err := tc.TerminateContainer(s.mnCtr); err != nil {
			s.T().Logf("terminate minio container: %v", err)
		}
	}
	if s.pgCtr != nil {
		if err := tc.TerminateContainer(s.pgCtr); err != nil {
			s.T().Logf("terminate postgres container: %v", err)
		}
	}
}

func (s *WorkerAppSuite) applyMigrations(ctx context.Context) {
	_, thisFile, _, ok := runtime.Caller(0)
	s.Require().True(ok)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	migrationsDir := filepath.Join(repoRoot, "migrations")
	s.Require().NoError(testdb.RunMigrations(s.pool.Pgx(), migrationsDir))
}

func (s *WorkerAppSuite) waitForStatus(id uuid.UUID, want models.ProcessingStatus, timeout time.Duration) *models.Avatar {
	var got *models.Avatar
	s.Require().Eventually(func() bool {
		a, err := s.repo.GetByIDIncludingDeleted(context.Background(), id)
		if err != nil {
			return false
		}
		if a.ProcessingStatus == want {
			got = a
			return true
		}
		return false
	}, timeout, 200*time.Millisecond, "avatar %s never reached status %q", id, want)
	return got
}

// TestAppRun_ProcessesUpload publishes a real upload event and checks the worker writes thumbnails
// and updates DB status to completed.
func (s *WorkerAppSuite) TestAppRun_ProcessesUpload() {
	ctx := context.Background()
	id := uuid.New()
	user := "wuser-" + id.String()[:8]
	key := "originals/" + user + "/" + id.String() + ".png"

	now := time.Now().UTC()
	a := &models.Avatar{
		ID: id, UserID: user, FileName: "a.png", MimeType: "image/png",
		SizeBytes: int64(len(minPNG)), S3Key: key,
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	s.Require().NoError(s.repo.Insert(ctx, a))
	s.Require().NoError(s.s3Client.PutObject(ctx, key, bytes.NewReader(minPNG), int64(len(minPNG)), "image/png"))

	s.Require().NoError(s.publisher.PublishJSON(ctx, s.cfg.UploadRoutingKey, id.String(), models.AvatarUploadEvent{
		AvatarID: id.String(), UserID: user, S3Key: key,
	}))

	got := s.waitForStatus(id, models.ProcessingStatusCompleted, 30*time.Second)
	s.NotEmpty(got.ThumbnailS3Keys, "thumbnail keys must be persisted")
	for label, byFmt := range got.ThumbnailS3Keys {
		for _, thumbKey := range byFmt {
			_, _, statErr := s.s3Client.StatObject(ctx, thumbKey)
			s.Require().NoErrorf(statErr, "thumbnail %q must exist for label %q", thumbKey, label)
		}
	}
}

// TestAppRun_DLQAfterRetries publishes an event whose original is missing in S3; processing fails,
// retries exhaust, and the message lands in the upload DLQ. Final DB status becomes failed.
func (s *WorkerAppSuite) TestAppRun_DLQAfterRetries() {
	ctx := context.Background()

	conn, err := amqp.Dial(s.cfg.URL)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		if cerr := conn.Close(); cerr != nil {
			s.T().Logf("amqp conn close: %v", cerr)
		}
	})
	ch, err := conn.Channel()
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		if cerr := ch.Close(); cerr != nil {
			s.T().Logf("amqp channel close: %v", cerr)
		}
	})
	dlqMsgs, err := ch.Consume(s.cfg.UploadDLQQueue, "wt-dlq-"+uuid.NewString()[:6], true, false, false, false, nil)
	s.Require().NoError(err)

	id := uuid.New()
	user := "wuser-dlq-" + id.String()[:8]
	missingKey := "originals/" + user + "/missing-" + id.String() + ".png"

	now := time.Now().UTC()
	a := &models.Avatar{
		ID: id, UserID: user, FileName: "x.png", MimeType: "image/png",
		SizeBytes: 0, S3Key: missingKey,
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	s.Require().NoError(s.repo.Insert(ctx, a))

	s.Require().NoError(s.publisher.PublishJSON(ctx, s.cfg.UploadRoutingKey, id.String(), models.AvatarUploadEvent{
		AvatarID: id.String(), UserID: user, S3Key: missingKey,
	}))

	select {
	case d := <-dlqMsgs:
		s.Equal(id.String(), d.MessageId)
	case <-time.After(30 * time.Second):
		s.FailNow("timed out waiting for DLQ message")
	}

	got := s.waitForStatus(id, models.ProcessingStatusFailed, 15*time.Second)
	s.Equal(models.ProcessingStatusFailed, got.ProcessingStatus)
}

func TestWorkerAppSuite(t *testing.T) {
	suite.Run(t, new(WorkerAppSuite))
}
