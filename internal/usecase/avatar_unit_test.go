package usecase

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/stretchr/testify/require"

	"govatars/internal/models"
)

func TestExtForMIME(t *testing.T) {
	require.Equal(t, ".jpg", extForMIME("image/jpeg"))
	require.Equal(t, ".png", extForMIME("image/png"))
	require.Equal(t, ".webp", extForMIME("image/webp"))
	require.Empty(t, extForMIME("image/gif"))
}

func TestSanitizePathSegment(t *testing.T) {
	require.Equal(t, "a_b", sanitizePathSegment("a/b"))
	require.Equal(t, "u1", sanitizePathSegment("  u1  "))
}

func TestRenderAvatarVariant_FormatWebP(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var enc bytes.Buffer
	require.NoError(t, png.Encode(&enc, img))
	data := enc.Bytes()
	out, ct, err := renderAvatarVariant(data, "image/png", "", "webp", nil)
	require.NoError(t, err)
	require.Equal(t, "image/webp", ct)
	require.GreaterOrEqual(t, len(out), 12)
	require.Equal(t, "RIFF", string(out[0:4]))
	require.Equal(t, "WEBP", string(out[8:12]))
}

func TestRenderAvatarVariant_ResizePreservesWebPMIME(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, img))
	decoded, err := imaging.Decode(bytes.NewReader(pngBuf.Bytes()))
	require.NoError(t, err)
	webpData, _, _, err := encodeThumbnailImageByFormat(decoded, models.ThumbnailFormatWEBP)
	require.NoError(t, err)

	sides := map[string]int{"sm": 4}
	out, ct, err := renderAvatarVariant(webpData, mimeWebP, "sm", "", sides)
	require.NoError(t, err)
	require.Equal(t, mimeWebP, ct)
	require.GreaterOrEqual(t, len(out), 12)
	require.Equal(t, "RIFF", string(out[0:4]))
}
