package s3_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/config"
	"govatars/internal/repository/s3"
)

func TestNew_EmptyEndpoint(t *testing.T) {
	_, err := s3.New(context.Background(), config.S3{Endpoint: ""})
	require.Error(t, err)
}
