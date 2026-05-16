//go:build integration

package s3_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	tc "github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"govatars/internal/pkg/config"
	"govatars/internal/repository/s3"
)

// MinioSuite shares one MinIO testcontainer for all object-storage tests.
type MinioSuite struct {
	suite.Suite

	ctr *tcminio.MinioContainer
	cli *s3.Client
}

func (s *MinioSuite) SetupSuite() {
	ctx := context.Background()

	ctr, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-01-16T16-07-38Z")
	s.Require().NoError(err)
	s.ctr = ctr

	endpoint, err := ctr.ConnectionString(ctx)
	s.Require().NoError(err)

	bucket := "it-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	cli, err := s3.New(ctx, config.S3{
		Endpoint:  endpoint,
		AccessKey: ctr.Username,
		SecretKey: ctr.Password,
		Bucket:    bucket,
		UseSSL:    false,
		Region:    "us-east-1",
	})
	s.Require().NoError(err)
	s.cli = cli
}

func (s *MinioSuite) TearDownSuite() {
	if s.ctr != nil {
		if err := tc.TerminateContainer(s.ctr); err != nil {
			s.T().Logf("terminate minio container: %v", err)
		}
	}
}

func (s *MinioSuite) TestHealth() {
	s.Require().NoError(s.cli.Health(context.Background()))
}

func (s *MinioSuite) TestPutGetStatRemove() {
	ctx := context.Background()
	key := "objects/" + uuid.NewString() + ".txt"
	payload := []byte("hello-minio")
	s.Require().NoError(s.cli.PutObject(ctx, key, bytes.NewReader(payload), int64(len(payload)), "text/plain"))

	rc, err := s.cli.GetObject(ctx, key)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		if cerr := rc.Close(); cerr != nil {
			s.T().Logf("minio getobject close: %v", cerr)
		}
	})
	got, err := io.ReadAll(rc)
	s.Require().NoError(err)
	s.Equal(payload, got)

	size, etag, err := s.cli.StatObject(ctx, key)
	s.Require().NoError(err)
	s.Equal(int64(len(payload)), size)
	s.NotEmpty(etag)

	s.Require().NoError(s.cli.RemoveObject(ctx, key))
	s.Require().Eventually(func() bool {
		_, _, statErr := s.cli.StatObject(ctx, key)
		return statErr != nil
	}, 3*time.Second, 100*time.Millisecond, "object should be gone after RemoveObject")
}

func TestMinioSuite(t *testing.T) {
	suite.Run(t, new(MinioSuite))
}
