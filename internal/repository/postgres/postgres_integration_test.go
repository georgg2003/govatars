//go:build integration

package postgres_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	tc "github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"govatars/internal/models"
	"govatars/internal/pkg/config"
	"govatars/internal/repository/postgres"
	"govatars/internal/testdb"
	"govatars/internal/usecase"
)

// PostgresRepoSuite runs once per testcontainer; each test gets unique uuids/users so they don't collide.
type PostgresRepoSuite struct {
	suite.Suite

	ctr  *tcpostgres.PostgresContainer
	pool *postgres.Pool
	repo *postgres.AvatarRepository
}

func (s *PostgresRepoSuite) SetupSuite() {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine")
	s.Require().NoError(err)
	s.ctr = ctr

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	s.Require().NoError(err)

	var pool *postgres.Pool
	s.Require().Eventually(func() bool {
		pool, err = postgres.New(ctx, config.Postgres{DSN: dsn}, false)
		return err == nil
	}, 20*time.Second, 300*time.Millisecond, "postgres container is not ready: %v", err)
	s.pool = pool

	_, thisFile, _, ok := runtime.Caller(0)
	s.Require().True(ok)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	migrationsDir := filepath.Join(repoRoot, "migrations")
	s.Require().NoError(testdb.RunMigrations(s.pool.Pgx(), migrationsDir))

	s.repo = postgres.NewAvatarRepository(s.pool.Pgx())
}

func (s *PostgresRepoSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.ctr != nil {
		if err := tc.TerminateContainer(s.ctr); err != nil {
			s.T().Logf("terminate postgres container: %v", err)
		}
	}
}

// insertAvatar creates a fresh avatar row for the test and returns its handle.
func (s *PostgresRepoSuite) insertAvatar(userID string) *models.Avatar {
	id := uuid.New()
	now := time.Now().UTC()
	a := &models.Avatar{
		ID:               id,
		UserID:           userID,
		FileName:         "a.png",
		MimeType:         "image/png",
		SizeBytes:        42,
		S3Key:            "originals/" + userID + "/" + id.String() + ".png",
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.Require().NoError(s.repo.Insert(context.Background(), a))
	return a
}

func (s *PostgresRepoSuite) TestInsertGetList() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-list")

	got, err := s.repo.GetByID(ctx, a.ID)
	s.Require().NoError(err)
	s.Equal(a.UserID, got.UserID)
	s.Equal(a.S3Key, got.S3Key)

	latest, err := s.repo.GetLatestByUser(ctx, a.UserID)
	s.Require().NoError(err)
	s.Equal(a.ID, latest.ID)

	list, err := s.repo.ListByUser(ctx, a.UserID)
	s.Require().NoError(err)
	s.Require().Len(list, 1)
	s.Equal(a.ID, list[0].ID)
}

func (s *PostgresRepoSuite) TestGetByID_NotFound_ReturnsErrNotFound() {
	_, err := s.repo.GetByID(context.Background(), uuid.New())
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestGetLatestByUser_NotFound_ReturnsErrNotFound() {
	_, err := s.repo.GetLatestByUser(context.Background(), "nobody-"+uuid.NewString())
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestDeleteHard_RemovesRow() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-hard-delete")

	s.Require().NoError(s.repo.DeleteHard(ctx, a.ID))

	_, err := s.repo.GetByID(ctx, a.ID)
	s.Require().ErrorIs(err, usecase.ErrNotFound)
	_, err = s.repo.GetByIDIncludingDeleted(ctx, a.ID)
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestDeleteHard_MissingRow_ReturnsError() {
	err := s.repo.DeleteHard(context.Background(), uuid.New())
	s.Require().Error(err)
}

func (s *PostgresRepoSuite) TestSoftDelete_AndGetIncludingDeleted() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-soft-delete")

	s.Require().NoError(s.repo.SoftDelete(ctx, a.ID, a.UserID))

	_, err := s.repo.GetByID(ctx, a.ID)
	s.Require().ErrorIs(err, usecase.ErrNotFound)

	got, err := s.repo.GetByIDIncludingDeleted(ctx, a.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.DeletedAt)
}

func (s *PostgresRepoSuite) TestSoftDelete_MissingRow_ReturnsErrNotFound() {
	err := s.repo.SoftDelete(context.Background(), uuid.New(), "ghost-user")
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestRestoreSoftDeleted() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-restore")

	s.Require().NoError(s.repo.SoftDelete(ctx, a.ID, a.UserID))
	s.Require().NoError(s.repo.RestoreSoftDeleted(ctx, a.ID, a.UserID))

	got, err := s.repo.GetByID(ctx, a.ID)
	s.Require().NoError(err)
	s.Nil(got.DeletedAt)

	latest, err := s.repo.GetLatestByUser(ctx, a.UserID)
	s.Require().NoError(err)
	s.Equal(a.ID, latest.ID)
}

func (s *PostgresRepoSuite) TestRestoreSoftDeleted_MissingRow_ReturnsErrNotFound() {
	err := s.repo.RestoreSoftDeleted(context.Background(), uuid.New(), "ghost")
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestUpdateProcessingResult() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-processing")
	thumbs := map[string]map[string]string{
		"100x100": {models.ThumbnailFormatJPEG: "thumbnails/" + a.ID.String() + "/100x100.jpg"},
		"300x300": {models.ThumbnailFormatPNG: "thumbnails/" + a.ID.String() + "/300x300.png"},
	}

	s.Require().NoError(s.repo.UpdateProcessingResult(ctx, a.ID, 256, 512, thumbs, models.ProcessingStatusCompleted))

	got, err := s.repo.GetByID(ctx, a.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.Width)
	s.Require().NotNil(got.Height)
	s.Equal(256, *got.Width)
	s.Equal(512, *got.Height)
	s.Equal(models.ProcessingStatusCompleted, got.ProcessingStatus)
	s.Equal(thumbs, got.ThumbnailS3Keys)
}

func (s *PostgresRepoSuite) TestUpdateProcessingResult_DeletedRow_ReturnsErrNotFound() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-update-deleted")
	s.Require().NoError(s.repo.SoftDelete(ctx, a.ID, a.UserID))

	err := s.repo.UpdateProcessingResult(ctx, a.ID, 1, 1, nil, models.ProcessingStatusCompleted)
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestSetProcessingStatus() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-set-status")

	s.Require().NoError(s.repo.SetProcessingStatus(ctx, a.ID, models.ProcessingStatusFailed))

	got, err := s.repo.GetByID(ctx, a.ID)
	s.Require().NoError(err)
	s.Equal(models.ProcessingStatusFailed, got.ProcessingStatus)
}

func (s *PostgresRepoSuite) TestSetProcessingStatus_NotFound_ReturnsErrNotFound() {
	err := s.repo.SetProcessingStatus(context.Background(), uuid.New(), models.ProcessingStatusFailed)
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestSetProcessingStatus_DeletedRow_ReturnsErrNotFound() {
	ctx := context.Background()
	a := s.insertAvatar("integration-user-set-status-deleted")
	s.Require().NoError(s.repo.SoftDelete(ctx, a.ID, a.UserID))

	err := s.repo.SetProcessingStatus(ctx, a.ID, models.ProcessingStatusFailed)
	s.Require().ErrorIs(err, usecase.ErrNotFound)
}

func (s *PostgresRepoSuite) TestPoolHealth() {
	s.Require().NoError(s.pool.Health(context.Background()))
}

func TestPostgresRepoSuite(t *testing.T) {
	suite.Run(t, new(PostgresRepoSuite))
}
