package httphandler

import (
	"context"
	"io"

	"github.com/google/uuid"

	"govatars/internal/models"
	"govatars/internal/usecase"
)

// AvatarQueries is the avatar use case surface used by the HTTP API (mockable in tests).
//
//go:generate go tool mockgen -destination=../../mocks/http_avatar_mocks.go -package=mocks . AvatarQueries
type AvatarQueries interface {
	Upload(ctx context.Context, userID string, filename string, r io.Reader) (*usecase.UploadResult, error)
	DeleteByID(ctx context.Context, actorUserID string, id uuid.UUID) error
	GetImage(ctx context.Context, id uuid.UUID, size, format string) (*usecase.ImagePayload, error)
	ByID(ctx context.Context, id uuid.UUID) (*models.Avatar, error)
	DeleteLatestForUser(ctx context.Context, actorUserID, pathUserID string) error
	GetLatestImageForUser(ctx context.Context, userID string, size, format string) (*usecase.ImagePayload, error)
	ListByUser(ctx context.Context, userID string) ([]models.Avatar, error)
	ThumbnailURL(id uuid.UUID, size, format string) string
}
