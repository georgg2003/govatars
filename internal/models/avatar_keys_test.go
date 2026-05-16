package models_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/models"
)

func TestAvatar_AllObjectKeys(t *testing.T) {
	a := &models.Avatar{
		S3Key: "orig",
		ThumbnailS3Keys: map[string]map[string]string{
			"100x100": {"jpeg": "t1", "png": "t2"},
			"300x300": {"jpeg": "t3"},
		},
	}
	keys := a.AllObjectKeys()
	require.Len(t, keys, 4)
	require.Contains(t, keys, "orig")
	require.Contains(t, keys, "t1")
}
