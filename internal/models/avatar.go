package models

import (
	"time"

	"github.com/google/uuid"
)

// UploadStatus is persisted in avatars.upload_status.
type UploadStatus string

// ProcessingStatus is persisted in avatars.processing_status.
type ProcessingStatus string

const (
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusReady     UploadStatus = "ready"

	ProcessingStatusPending    ProcessingStatus = "pending"
	ProcessingStatusProcessing ProcessingStatus = "processing"
	ProcessingStatusCompleted  ProcessingStatus = "completed"
	ProcessingStatusFailed     ProcessingStatus = "failed"
)

// Avatar is metadata for a stored image and its derivatives.
type Avatar struct {
	ID               uuid.UUID
	UserID           string
	FileName         string
	MimeType         string
	SizeBytes        int64
	Width            *int
	Height           *int
	S3Key            string
	ThumbnailS3Keys  map[string]map[string]string
	UploadStatus     UploadStatus
	ProcessingStatus ProcessingStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}
