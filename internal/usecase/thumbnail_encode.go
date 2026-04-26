package usecase

import (
	"bytes"
	"fmt"
	"image"

	"github.com/disintegration/imaging"
	gowebp "github.com/skrashevich/go-webp"

	"govatars/internal/models"
)

// encodeThumbnailImageByFormat encodes a raster image to JPEG, PNG, or WebP (same as worker thumbnails).
func encodeThumbnailImageByFormat(img image.Image, format string) (data []byte, contentType, fileExt string, err error) {
	var buf bytes.Buffer
	switch format {
	case models.ThumbnailFormatJPEG:
		if err := imaging.Encode(&buf, img, imaging.JPEG); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), models.ThumbnailContentTypeByFormat[format], "jpg", nil
	case models.ThumbnailFormatPNG:
		if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), models.ThumbnailContentTypeByFormat[format], "png", nil
	case models.ThumbnailFormatWEBP:
		if err := gowebp.Encode(&buf, img, &gowebp.Options{Lossy: true, Quality: 85}); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), models.ThumbnailContentTypeByFormat[format], "webp", nil
	default:
		return nil, "", "", fmt.Errorf("unsupported thumbnail format %q", format)
	}
}
