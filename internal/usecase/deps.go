package usecase

//go:generate go tool mockgen -destination=../repomocks/repo_gen.go -package=repomocks . AvatarRepository,ObjectStorage,EventPublisher

import (
	"context"
	"io"

	"github.com/google/uuid"

	"govatars/internal/models"
)

// AvatarRepository persists and queries avatar metadata (e.g. postgres implementation).
// Implementations return [ErrNotFound] when a row is missing or a conditional update matches no row.
type AvatarRepository interface {
	Insert(ctx context.Context, a *models.Avatar) error
	DeleteHard(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Avatar, error)
	GetByIDIncludingDeleted(ctx context.Context, id uuid.UUID) (*models.Avatar, error)
	GetLatestByUser(ctx context.Context, userID string) (*models.Avatar, error)
	ListByUser(ctx context.Context, userID string) ([]models.Avatar, error)
	SoftDelete(ctx context.Context, id uuid.UUID, userID string) error
	RestoreSoftDeleted(ctx context.Context, id uuid.UUID, userID string) error
	SetProcessingStatus(ctx context.Context, id uuid.UUID, status models.ProcessingStatus) error
	UpdateProcessingResult(ctx context.Context, id uuid.UUID, width, height int, thumbs map[string]map[string]string, status models.ProcessingStatus) error
}

// ObjectStorage reads and writes binary objects (e.g. S3/MinIO client).
type ObjectStorage interface {
	PutObject(ctx context.Context, objectKey string, r io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
	StatObject(ctx context.Context, objectKey string) (size int64, etag string, err error)
	RemoveObject(ctx context.Context, objectKey string) error
}

// EventPublisher sends domain events to the message broker (e.g. RabbitMQ publisher).
type EventPublisher interface {
	PublishJSON(ctx context.Context, routingKey string, messageID string, body any) error
}
