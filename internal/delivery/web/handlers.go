package web

import (
	"context"
	"io"

	"govatars/internal/usecase"
)

// Uploader is the minimal avatar API for HTML upload (implemented by *usecase.AvatarService).
//
//go:generate go tool mockgen -destination=../../mocks/web_uploader_mocks.go -package=mocks . Uploader
type Uploader interface {
	Upload(ctx context.Context, userID string, filename string, r io.Reader) (*usecase.UploadResult, error)
}
