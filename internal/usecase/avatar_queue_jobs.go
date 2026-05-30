package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	_ "github.com/skrashevich/go-webp" // register WebP decoder (pure Go; aligns with API server)
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"govatars/internal/models"
	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
	"govatars/internal/pkg/metrics"
	"govatars/internal/pkg/otelpkg"
)

// AvatarQueueJobs performs avatar work triggered from async queues (thumbnails, delete cleanup).
// It has no AMQP/RabbitMQ dependencies; the worker adapts deliveries and retries.
type AvatarQueueJobs struct {
	log    *slog.Logger
	repo   AvatarRepository
	s3     ObjectStorage
	thumbs config.ThumbnailCatalog
	biz    *metrics.Business
}

// NewAvatarQueueJobs builds queue-driven avatar processing (used by the worker).
func NewAvatarQueueJobs(log *slog.Logger, repo AvatarRepository, s3 ObjectStorage, thumbs config.ThumbnailCatalog, biz *metrics.Business) *AvatarQueueJobs {
	return &AvatarQueueJobs{log: log, repo: repo, s3: s3, thumbs: thumbs, biz: biz}
}

// ProcessAvatarUpload generates thumbnails and updates DB for one upload event.
func (j *AvatarQueueJobs) ProcessAvatarUpload(ctx context.Context, ev *models.AvatarUploadEvent) (err error) {
	start := time.Now()
	ctx, span := otel.Tracer(otelpkg.ScopeUsecase).Start(ctx, "avatar.process_upload")
	defer func() {
		j.biz.RecordThumbnailJob(ctx, "upload", err == nil, time.Since(start))
		span.End()
	}()
	span.SetAttributes(attribute.String("avatar_id", ev.AvatarID), attribute.String("user_id", ev.UserID))

	id, err := uuid.Parse(ev.AvatarID)
	if err != nil {
		return err
	}

	var uploadedKeys []string
	trackFailure := false
	defer func() {
		if err == nil {
			return
		}
		ctxb := context.WithoutCancel(ctx)
		for _, k := range uploadedKeys {
			if rmErr := j.s3.RemoveObject(ctxb, k); rmErr != nil {
				j.log.WarnContext(ctxb, "upload rollback remove", "key", k, "err", rmErr)
			}
		}
		if trackFailure {
			if stErr := j.repo.SetProcessingStatus(ctxb, id, models.ProcessingStatusFailed); stErr != nil {
				j.log.WarnContext(ctxb, "upload set processing failed", "err", stErr)
			}
		}
	}()

	a, err := j.repo.GetByIDIncludingDeleted(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if a.DeletedAt != nil {
		return nil
	}
	if a.ProcessingStatus == models.ProcessingStatusCompleted {
		return nil
	}
	trackFailure = true

	obj, err := j.s3.GetObject(ctx, ev.S3Key)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := obj.Close(); cerr != nil {
			j.log.WarnContext(ctx, "s3 getobject body close", "err", cerr)
		}
	}()

	data, err := io.ReadAll(obj)
	if err != nil {
		return err
	}

	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	thumbs := make(map[string]map[string]string, len(j.thumbs.Labels))

	for _, label := range j.thumbs.Labels {
		side := j.thumbs.Sides[label]
		out := imaging.Fill(img, side, side, imaging.Center, imaging.Lanczos)
		byFormat := make(map[string]string, len(models.ThumbnailFormats))
		for _, format := range models.ThumbnailFormats {
			data, contentType, ext, encErr := encodeThumbnailImageByFormat(out, format)
			if encErr != nil {
				return encErr
			}
			key := fmt.Sprintf("thumbnails/%s/%s.%s", id.String(), label, ext)
			if err := j.s3.PutObject(ctx, key, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
				return apperr.Wrap(err, "queue upload thumb")
			}
			uploadedKeys = append(uploadedKeys, key)
			byFormat[format] = key
		}
		thumbs[label] = byFormat
	}

	if err := j.repo.UpdateProcessingResult(ctx, id, w, h, thumbs, models.ProcessingStatusCompleted); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// ProcessAvatarDelete removes S3 objects listed on a delete event; joins partial failures.
func (j *AvatarQueueJobs) ProcessAvatarDelete(ctx context.Context, ev *models.AvatarDeleteEvent) (err error) {
	start := time.Now()
	ctx, span := otel.Tracer(otelpkg.ScopeUsecase).Start(ctx, "avatar.process_delete")
	defer func() {
		j.biz.RecordThumbnailJob(ctx, "delete", err == nil, time.Since(start))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	span.SetAttributes(attribute.String("avatar_id", ev.AvatarID))

	var errs []error
	for _, key := range ev.S3Keys {
		if key == "" {
			continue
		}
		if rmErr := j.s3.RemoveObject(ctx, key); rmErr != nil {
			j.log.WarnContext(ctx, "delete object", "key", key, "err", rmErr)
			errs = append(errs, rmErr)
		}
	}
	err = errors.Join(errs...)
	return err
}
