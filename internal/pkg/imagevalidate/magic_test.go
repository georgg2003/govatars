package imagevalidate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/imagevalidate"
)

func TestMatchesDeclaredMIME_JPEG(t *testing.T) {
	head := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	require.True(t, imagevalidate.MatchesDeclaredMIME("image/jpeg", head))
	require.False(t, imagevalidate.MatchesDeclaredMIME("image/png", head))
}

func TestMatchesDeclaredMIME_PNG(t *testing.T) {
	head := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}
	require.True(t, imagevalidate.MatchesDeclaredMIME("image/png", head))
}

func TestMatchesDeclaredMIME_WebP(t *testing.T) {
	head := []byte("RIFF\x00\x00\x00\x00WEBP")
	require.True(t, imagevalidate.MatchesDeclaredMIME("image/webp", head))
}
