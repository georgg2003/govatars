package usecase

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // MD5 here matches S3 single-part PUT ETag for cache consistency, not for security.
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	_ "github.com/skrashevich/go-webp" // register WebP decoder for imaging.Decode (uploads may be WebP)

	"govatars/internal/models"
	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
	"govatars/internal/pkg/imagevalidate"
)

var allowedImageMIME = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

const (
	mimeJPEG           = "image/jpeg"
	mimePNG            = "image/png"
	mimeWebP           = "image/webp"
	avatarSizeOriginal = "original"
)

// AvatarService coordinates avatar lifecycle (upload, read, delete).
type AvatarService struct {
	repo              AvatarRepository
	s3                ObjectStorage
	pub               EventPublisher
	rabbit            config.RabbitMQ
	publicBase        string
	ph                avatarPlaceholder
	maxUploadBytes    int64
	thumbLabels       []string
	thumbSides        map[string]int
	imageCacheControl string
	log               *slog.Logger
}

// NewAvatarService wires repositories and messaging.
//
// If [config.HTTP.PlaceholderPath] is set, the file is read and validated:
//   - unreadable file -> Warn, no placeholder configured (404 for users without avatars);
//   - unsupported / undecodable image -> Warn, no placeholder configured;
//   - on success: content-type is detected from bytes, decoded image is cached, and S3 keys are derived.
func NewAvatarService(
	repo AvatarRepository,
	store ObjectStorage,
	pub EventPublisher,
	cfg *config.App,
	thumbs config.ThumbnailCatalog,
	log *slog.Logger,
) *AvatarService {
	icc := strings.TrimSpace(cfg.Avatars.ImageCacheControl)
	if icc == "" {
		icc = "max-age=86400"
	}
	s := &AvatarService{
		repo:              repo,
		s3:                store,
		pub:               pub,
		rabbit:            cfg.RabbitMQ,
		publicBase:        strings.TrimRight(cfg.HTTP.PublicBaseURL, "/"),
		maxUploadBytes:    cfg.Avatars.MaxUploadBytes,
		thumbLabels:       append([]string(nil), thumbs.Labels...),
		thumbSides:        thumbs.Sides,
		imageCacheControl: icc,
		log:               log,
	}

	s.ph = newAvatarPlaceholder(
		log,
		cfg.HTTP.PlaceholderPath,
		thumbs.Labels,
		s.tryStoredThumbnail,
	)

	return s
}

// UploadResult is returned after a successful upload + enqueue.
type UploadResult struct {
	ID        uuid.UUID
	UserID    string
	URL       string
	Status    string
	CreatedAt time.Time
}

// Upload stores the original in S3, inserts metadata, and publishes a processing event.
func (s *AvatarService) Upload(ctx context.Context, userID string, filename string, r io.Reader) (*UploadResult, error) {
	maxBytes := s.maxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, ErrPayloadTooLarge
	}
	if len(body) == 0 {
		return nil, ErrInvalidImage
	}

	head := body
	if len(head) > 512 {
		head = head[:512]
	}
	ct := http.DetectContentType(body)
	if _, ok := allowedImageMIME[ct]; !ok {
		return nil, ErrInvalidImage
	}
	if !imagevalidate.MatchesDeclaredMIME(ct, head) {
		return nil, ErrInvalidImage
	}

	ext := extForMIME(ct)
	if ext == "" {
		return nil, ErrInvalidImage
	}

	id := uuid.New()
	safeUser := sanitizePathSegment(userID)
	key := originalAvatarObjectKey(safeUser, id, ext)

	if err := s.s3.PutObject(ctx, key, bytes.NewReader(body), int64(len(body)), ct); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	a := &models.Avatar{
		ID:               id,
		UserID:           userID,
		FileName:         filename,
		MimeType:         ct,
		SizeBytes:        int64(len(body)),
		S3Key:            key,
		UploadStatus:     models.UploadStatusReady,
		ProcessingStatus: models.ProcessingStatusPending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.repo.Insert(ctx, a); err != nil {
		if rmErr := s.s3.RemoveObject(ctx, key); rmErr != nil {
			s.log.WarnContext(ctx, "upload rollback: remove original from s3 after insert failure", "key", key, "err", rmErr)
		}
		return nil, err
	}

	ev := models.AvatarUploadEvent{AvatarID: id.String(), UserID: userID, S3Key: key}
	if err := s.pub.PublishJSON(ctx, s.rabbit.UploadRoutingKey, id.String(), ev); err != nil {
		if delErr := s.repo.DeleteHard(ctx, id); delErr != nil {
			s.log.WarnContext(ctx, "upload rollback: delete row after publish failure", "avatar_id", id, "err", delErr)
		}
		if rmErr := s.s3.RemoveObject(ctx, key); rmErr != nil {
			s.log.WarnContext(ctx, "upload rollback: remove original from s3 after publish failure", "key", key, "err", rmErr)
		}
		return nil, err
	}

	return &UploadResult{
		ID:        id,
		UserID:    userID,
		URL:       s.avatarURL(id),
		Status:    "processing",
		CreatedAt: now,
	}, nil
}

func (s *AvatarService) avatarURL(id uuid.UUID) string {
	return fmt.Sprintf("%s/api/v1/avatars/%s", s.publicBase, id.String())
}

// ThumbnailURL builds a public API URL for a derived size and optional output format (jpeg, png, webp).
// Empty format omits the query parameter (server default applies).
func (s *AvatarService) ThumbnailURL(id uuid.UUID, size, format string) string {
	q := url.Values{}
	q.Set("size", size)
	if strings.TrimSpace(format) != "" {
		q.Set("format", strings.TrimSpace(format))
	}
	return fmt.Sprintf("%s/api/v1/avatars/%s?%s", s.publicBase, id.String(), q.Encode())
}

// ImagePayload is streamed to the client for GET avatar.
type ImagePayload struct {
	Reader        io.ReadCloser
	ContentType   string
	ContentLength int64
	ETag          string
	CacheControl  string
}

// GetImage returns bytes for an avatar id (optionally a thumbnail size and output format).
func (s *AvatarService) GetImage(ctx context.Context, id uuid.UUID, size, format string) (*ImagePayload, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.buildImagePayload(ctx, a, size, format)
}

// GetLatestImageForUser returns the latest avatar for a user, or the configured placeholder if none exist.
func (s *AvatarService) GetLatestImageForUser(ctx context.Context, userID string, size, format string) (*ImagePayload, error) {
	a, err := s.repo.GetLatestByUser(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.buildPlaceholderPayload(ctx, size, format)
		}
		return nil, err
	}
	return s.buildImagePayload(ctx, a, size, format)
}

func (s *AvatarService) buildImagePayload(ctx context.Context, a *models.Avatar, size, format string) (*ImagePayload, error) {
	wantOriginal := size == "" || size == avatarSizeOriginal
	if !wantOriginal && a.ProcessingStatus == models.ProcessingStatusCompleted && a.ThumbnailS3Keys != nil {
		requestedFormat := normalizeThumbnailFormat(format)
		if byFormat, ok := a.ThumbnailS3Keys[size]; ok && len(byFormat) > 0 {
			if k, ok := byFormat[requestedFormat]; ok && k != "" {
				contentType := models.ThumbnailContentTypeByFormat[requestedFormat]
				if pl, err := s.tryStoredThumbnail(ctx, k, contentType); err == nil && pl != nil {
					return pl, nil
				}
			}
		}
	}
	// No transcode: stream the stored original (Stat+Get) without buffering the body in memory.
	if wantOriginal && strings.TrimSpace(format) == "" {
		if pl, err := s.tryStoredThumbnail(ctx, a.S3Key, a.MimeType); err == nil && pl != nil {
			return pl, nil
		}
	}
	return s.buildFromOriginal(ctx, a, size, format)
}

// tryStoredThumbnail serves pre-generated S3 object for requested size+format.
// Returns ([usecase.ErrObjectNotFound], nil) on miss and a wrapped error on real failures so callers can
// log only when something is actually broken.
func (s *AvatarService) tryStoredThumbnail(ctx context.Context, objectKey, contentType string) (*ImagePayload, error) {
	n, etag, err := s.s3.StatObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	rc, err := s.s3.GetObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	etagOut := etag
	if etagOut != "" && !strings.HasPrefix(etagOut, `"`) {
		etagOut = fmt.Sprintf(`"%s"`, etagOut)
	}
	return &ImagePayload{
		Reader:        rc,
		ContentType:   contentType,
		ContentLength: n,
		ETag:          etagOut,
		CacheControl:  s.imageCacheControl,
	}, nil
}

// EnsurePlaceholderInS3 uploads the placeholder original and every (size, format) thumbnail
// variant to S3 if missing.
//
// Idempotency: existing objects are detected via StatObject and skipped. Zero-byte stored objects
// (e.g. left over from a crashed previous run) are re-uploaded so the system self-heals.
//
// Multi-replica safety: two replicas may both hit a Stat-miss and PUT the same content; this is
// safe because the bytes are deterministic for a given placeholder file (last-writer-wins).
//
// Error handling: errors per variant are collected via [errors.Join] and returned as one — a
// transient failure on one (size, format) does NOT prevent the rest from being uploaded.
// When no placeholder is configured (file missing or undecodable at startup), this is a no-op.
func (s *AvatarService) EnsurePlaceholderInS3(ctx context.Context) error {
	return s.ph.ensureInS3(ctx, s.s3, s.log, s.thumbLabels, s.thumbSides)
}

func (s *AvatarService) buildPlaceholderPayload(ctx context.Context, size, format string) (*ImagePayload, error) {
	return s.ph.buildPayload(ctx, s.log, s.thumbSides, s.imageCacheControl, size, format)
}

func normalizeThumbnailFormat(format string) string {
	switch format {
	case models.ThumbnailFormatPNG:
		return models.ThumbnailFormatPNG
	case models.ThumbnailFormatWEBP:
		return models.ThumbnailFormatWEBP
	case models.ThumbnailFormatJPEG, "":
		return models.ThumbnailFormatJPEG
	default:
		// OpenAPI validation should reject unknown values; keep jpeg fallback for safety.
		return models.ThumbnailFormatJPEG
	}
}

// renderAvatarVariant decodes once, optionally fits to a configured thumbnail side (worker-style), then encodes.
// Order matches the worker: resize from source bitmap, then encode to the target format.
// When format is empty and no resize is applied, returns the original bytes unchanged.
func renderAvatarVariant(data []byte, originalMIME string, size, format string, sides map[string]int) ([]byte, string, error) {
	format = strings.TrimSpace(format)
	size = strings.TrimSpace(size)

	side, haveSide := 0, false
	if size != "" && size != avatarSizeOriginal && sides != nil {
		var ok bool
		side, ok = sides[size]
		haveSide = ok
	}
	needResize := size != "" && size != avatarSizeOriginal && haveSide

	if format == "" && !needResize {
		return data, originalMIME, nil
	}

	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		if format != "" {
			return nil, "", err
		}
		return data, originalMIME, nil
	}

	if needResize {
		img = imaging.Fill(img, side, side, imaging.Center, imaging.Lanczos)
	}

	if format != "" {
		f := normalizeThumbnailFormat(format)
		b, ct, _, encErr := encodeThumbnailImageByFormat(img, f)
		return b, ct, encErr
	}

	var buf bytes.Buffer
	switch originalMIME {
	case mimePNG:
		if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), mimePNG, nil
	case mimeWebP:
		b, ct, _, encErr := encodeThumbnailImageByFormat(img, models.ThumbnailFormatWEBP)
		return b, ct, encErr
	default:
		if err := imaging.Encode(&buf, img, imaging.JPEG); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), mimeJPEG, nil
	}
}

func (s *AvatarService) buildFromOriginal(ctx context.Context, a *models.Avatar, size, format string) (*ImagePayload, error) {
	obj, err := s.s3.GetObject(ctx, a.S3Key)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := obj.Close(); cerr != nil {
			s.log.WarnContext(ctx, "s3 getobject body close", "err", cerr)
		}
	}()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, err
	}

	data, contentType, err := renderAvatarVariant(data, a.MimeType, size, format, s.thumbSides)
	if err != nil {
		return nil, err
	}

	return &ImagePayload{
		Reader:        io.NopCloser(bytes.NewReader(data)),
		ContentType:   contentType,
		ContentLength: int64(len(data)),
		ETag:          contentETag(data),
		CacheControl:  s.imageCacheControl,
	}, nil
}

// ByID returns metadata for a non-deleted avatar.
func (s *AvatarService) ByID(ctx context.Context, id uuid.UUID) (*models.Avatar, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ListByUser lists avatars for a user.
func (s *AvatarService) ListByUser(ctx context.Context, userID string) ([]models.Avatar, error) {
	return s.repo.ListByUser(ctx, userID)
}

// DeleteByID soft-deletes an avatar if the actor owns it and publishes storage cleanup.
func (s *AvatarService) DeleteByID(ctx context.Context, actorUserID string, id uuid.UUID) error {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a.UserID != actorUserID {
		return ErrForbidden
	}
	return s.deleteAvatarWithPublish(ctx, a)
}

// DeleteLatestForUser removes the latest avatar for the user (actor must match path user id).
func (s *AvatarService) DeleteLatestForUser(ctx context.Context, actorUserID, pathUserID string) error {
	if actorUserID != pathUserID {
		return ErrForbidden
	}
	a, err := s.repo.GetLatestByUser(ctx, pathUserID)
	if err != nil {
		return err
	}
	return s.deleteAvatarWithPublish(ctx, a)
}

func (s *AvatarService) deleteAvatarWithPublish(ctx context.Context, a *models.Avatar) error {
	keys := a.AllObjectKeys()
	if err := s.repo.SoftDelete(ctx, a.ID, a.UserID); err != nil {
		return err
	}
	ev := models.AvatarDeleteEvent{AvatarID: a.ID.String(), S3Keys: keys}
	if err := s.pub.PublishJSON(ctx, s.rabbit.DeleteRoutingKey, a.ID.String(), ev); err != nil {
		if rollbackErr := s.repo.RestoreSoftDeleted(ctx, a.ID, a.UserID); rollbackErr != nil {
			return errors.Join(err, apperr.Wrap(rollbackErr, "restore soft-delete"))
		}
		return err
	}
	return nil
}

// contentETag returns a quoted MD5-hex ETag for in-memory bytes. We deliberately use the same
// scheme as S3 (single-part PUT ETag = MD5 hex) so a fallback render and a stored object produce
// the same ETag for identical content; otherwise an If-None-Match round-trip would always miss.
func contentETag(data []byte) string {
	sum := md5.Sum(data) //nolint:gosec // MD5 used to mirror S3 ETag, not as a security primitive.
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func extForMIME(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func sanitizePathSegment(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "/", "_")
}

func originalAvatarObjectKey(sanitizedUserID string, id uuid.UUID, ext string) string {
	return fmt.Sprintf("originals/%s/%s%s", sanitizedUserID, id.String(), ext)
}
