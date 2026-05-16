package models_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/models"
)

func TestPlaceholderOriginalKey(t *testing.T) {
	require.Equal(t, "placeholders/original.png", models.PlaceholderOriginalKey("png"))
	require.Equal(t, "placeholders/original.jpg", models.PlaceholderOriginalKey("jpg"))
	require.Equal(t, "placeholders/original.webp", models.PlaceholderOriginalKey("webp"))
}

func TestPlaceholderOriginalKey_EmptyExtReturnsEmpty(t *testing.T) {
	require.Empty(t, models.PlaceholderOriginalKey(""))
}

func TestPlaceholderThumbnailKey_KnownFormat(t *testing.T) {
	require.Equal(t, "placeholders/100x100.jpg", models.PlaceholderThumbnailKey("100x100", models.ThumbnailFormatJPEG))
	require.Equal(t, "placeholders/256x256.png", models.PlaceholderThumbnailKey("256x256", models.ThumbnailFormatPNG))
	require.Equal(t, "placeholders/512x512.webp", models.PlaceholderThumbnailKey("512x512", models.ThumbnailFormatWEBP))
}

func TestPlaceholderThumbnailKey_UnknownFormatReturnsEmpty(t *testing.T) {
	require.Empty(t, models.PlaceholderThumbnailKey("100x100", "bmp"))
	require.Empty(t, models.PlaceholderThumbnailKey("100x100", ""))
	require.Empty(t, models.PlaceholderThumbnailKey("100x100", "../etc/passwd"))
}
