package usecase

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/disintegration/imaging"

	"govatars/internal/models"
	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/imagevalidate"
)

// avatarPlaceholder holds the configured default image (bytes + decoded bitmap), its MIME type,
// S3 keys for the original and pre-sized variants. It is not [models.Avatar] (persisted user upload).

type storedThumbnailFetcher func(ctx context.Context, key, contentType string) (*ImagePayload, error)
type avatarPlaceholder struct {
	raw         []byte
	img         image.Image
	contentType string
	originalKey string
	keys        map[string]map[string]string
	fetch       storedThumbnailFetcher
}

func (p *avatarPlaceholder) configured() bool {
	return len(p.raw) > 0 && p.img != nil
}

// newAvatarPlaceholder reads path when non-empty, validates image bytes, and builds the S3 key map
// for all thumbnail labels (keys exist even when no file loads, so routing stays consistent).
func newAvatarPlaceholder(
	ctx context.Context,
	log *slog.Logger,
	path string,
	thumbLabels []string,
	fetch storedThumbnailFetcher,
) avatarPlaceholder {
	keys := buildPlaceholderKeyMap(thumbLabels)
	empty := func() avatarPlaceholder {
		return avatarPlaceholder{keys: keys, fetch: fetch}
	}
	if path == "" {
		return empty()
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is operator-controlled; staticDir is documented to host only public assets
	if err != nil {
		log.WarnContext(ctx, "placeholder read failed", "path", path, "err", err)
		return empty()
	}
	if len(b) == 0 {
		log.WarnContext(ctx, "placeholder file is empty", "path", path)
		return empty()
	}
	head := b
	if len(head) > 512 {
		head = head[:512]
	}
	ct := http.DetectContentType(b)
	if _, ok := allowedImageMIME[ct]; !ok || !imagevalidate.MatchesDeclaredMIME(ct, head) {
		log.WarnContext(ctx, "placeholder content-type unsupported", "path", path, "detected_ct", ct)
		return empty()
	}
	img, err := imaging.Decode(bytes.NewReader(b))
	if err != nil {
		log.WarnContext(ctx, "placeholder decode failed", "path", path, "ct", ct, "err", err)
		return empty()
	}
	return avatarPlaceholder{
		raw:         b,
		img:         img,
		contentType: ct,
		originalKey: models.PlaceholderOriginalKey(strings.TrimPrefix(extForMIME(ct), ".")),
		keys:        keys,
		fetch:       fetch,
	}
}

func buildPlaceholderKeyMap(labels []string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(labels))
	for _, label := range labels {
		byFormat := make(map[string]string, len(models.ThumbnailFormats))
		for _, format := range models.ThumbnailFormats {
			byFormat[format] = models.PlaceholderThumbnailKey(label, format)
		}
		out[label] = byFormat
	}
	return out
}

func (p *avatarPlaceholder) ensureInS3(ctx context.Context, store ObjectStorage, log *slog.Logger, thumbLabels []string, thumbSides map[string]int) error {
	if !p.configured() {
		log.InfoContext(ctx, "ensure placeholder in s3 skipped (no placeholder configured)")
		return nil
	}

	var errs []error
	if err := p.ensureOriginal(ctx, store, log); err != nil {
		errs = append(errs, err)
	}
	for _, label := range thumbLabels {
		errs = append(errs, p.ensureThumbsForSize(ctx, store, log, thumbSides, p.img, label)...)
	}
	return errors.Join(errs...)
}

func (p *avatarPlaceholder) ensureOriginal(ctx context.Context, store ObjectStorage, log *slog.Logger) error {
	key := p.originalKey
	if key == "" {
		return nil
	}
	if !p.shouldUpload(ctx, store, log, key) {
		return nil
	}
	data := p.raw
	if err := store.PutObject(ctx, key, bytes.NewReader(data), int64(len(data)), p.contentType); err != nil {
		return apperr.Wrap(err, "ensure placeholder: put original")
	}
	log.InfoContext(ctx, "placeholder uploaded to s3", "key", key, "content_type", p.contentType)
	return nil
}

func (p *avatarPlaceholder) ensureThumbsForSize(ctx context.Context, store ObjectStorage, log *slog.Logger, thumbSides map[string]int, img image.Image, label string) []error {
	side := thumbSides[label]
	if side <= 0 {
		return nil
	}
	resized := imaging.Fill(img, side, side, imaging.Center, imaging.Lanczos)

	var errs []error
	for _, format := range models.ThumbnailFormats {
		key := p.keys[label][format]
		if key == "" {
			continue
		}
		if !p.shouldUpload(ctx, store, log, key) {
			continue
		}
		data, contentType, _, encErr := encodeThumbnailImageByFormat(resized, format)
		if encErr != nil {
			errs = append(errs, apperr.Wrapf(encErr, "ensure placeholder: encode %s/%s", label, format))
			continue
		}
		if err := store.PutObject(ctx, key, bytes.NewReader(data), int64(len(data)), contentType); err != nil {
			errs = append(errs, apperr.Wrapf(err, "ensure placeholder: put %s/%s", label, format))
			continue
		}
		log.InfoContext(ctx, "placeholder uploaded to s3", "key", key, "content_type", contentType)
	}
	return errs
}

func (p *avatarPlaceholder) shouldUpload(ctx context.Context, store ObjectStorage, log *slog.Logger, key string) bool {
	size, _, err := store.StatObject(ctx, key)
	if err == nil {
		if size > 0 {
			return false
		}
		log.WarnContext(ctx, "placeholder object stored with zero size; re-uploading", "key", key)
		return true
	}
	if errors.Is(err, ErrObjectNotFound) {
		return true
	}
	log.WarnContext(ctx, "placeholder stat failed; skipping put to avoid overwriting", "key", key, "err", err)
	return false
}

func (p *avatarPlaceholder) buildPayload(
	ctx context.Context,
	log *slog.Logger,
	thumbSides map[string]int,
	imageCacheControl string,
	size, format string,
) (*ImagePayload, error) {
	if !p.configured() {
		return nil, ErrNotFound
	}
	if pl := p.tryStoredFromS3(ctx, log, size, format); pl != nil {
		return pl, nil
	}
	data := append([]byte(nil), p.raw...)
	data, ct, err := renderAvatarVariant(data, p.contentType, size, format, thumbSides)
	if err != nil {
		return nil, err
	}
	return &ImagePayload{
		Reader:        io.NopCloser(bytes.NewReader(data)),
		ContentType:   ct,
		ContentLength: int64(len(data)),
		ETag:          contentETag(data),
		CacheControl:  imageCacheControl,
	}, nil
}

// tryStoredFromS3 returns a payload streamed from S3 if the prewarmed object exists.
// Returns nil to signal "fall back to in-memory render".
//   - object missing -> silent (every request would otherwise spam logs);
//   - real S3 error  -> Warn, then fall back so the user still sees something;
//   - format/size not pre-keyed -> silent.
func (p *avatarPlaceholder) tryStoredFromS3(ctx context.Context, log *slog.Logger, size, format string) *ImagePayload {
	wantOriginal := size == "" || size == avatarSizeOriginal
	if wantOriginal {
		if p.originalKey == "" {
			return nil
		}
		return p.fetchStored(ctx, log, p.originalKey, p.contentType)
	}

	byFormat, ok := p.keys[size]
	if !ok || len(byFormat) == 0 {
		return nil
	}
	requestedFormat := normalizeThumbnailFormat(format)
	key := byFormat[requestedFormat]
	if key == "" {
		return nil
	}
	contentType := models.ThumbnailContentTypeByFormat[requestedFormat]
	return p.fetchStored(ctx, log, key, contentType)
}

func (p *avatarPlaceholder) fetchStored(ctx context.Context, log *slog.Logger, key, contentType string) *ImagePayload {
	pl, err := p.fetch(ctx, key, contentType)
	if err == nil {
		return pl
	}
	if !errors.Is(err, ErrObjectNotFound) {
		log.WarnContext(ctx, "stored placeholder lookup failed; falling back to in-memory render",
			"key", key, "err", err)
	}
	return nil
}
