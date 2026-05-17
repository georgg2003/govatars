//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	tc "github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"

	"govatars/internal/httpserver"
	"govatars/internal/pkg/config"
	"govatars/internal/repository/postgres"
	s3repo "govatars/internal/repository/s3"
	"govatars/internal/serverapp"
	"govatars/internal/testdb"
	"govatars/internal/usecase"
	"govatars/internal/worker"
)

// 1×1 PNG used for end-to-end uploads.
var minPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

// E2ESuite spins up Postgres + MinIO + RabbitMQ testcontainers, the HTTP API server, and the worker
// process. Tests interact via real HTTP requests and assert end-to-end behavior.
type E2ESuite struct {
	suite.Suite

	pgCtr *tcpostgres.PostgresContainer
	mnCtr *tcminio.MinioContainer
	mqCtr *tcrabbit.RabbitMQContainer

	app    *serverapp.App
	cfg    *config.App
	bucket string

	httpDone chan error
	wkDone   chan struct{}
	cancel   context.CancelFunc

	baseURL string
	client  *http.Client
}

func (s *E2ESuite) SetupSuite() {
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine")
	s.Require().NoError(err)
	s.pgCtr = pg

	mn, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-01-16T16-07-38Z")
	s.Require().NoError(err)
	s.mnCtr = mn

	mq, err := tcrabbit.Run(ctx, "rabbitmq:3.12-management-alpine")
	s.Require().NoError(err)
	s.mqCtr = mq

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	s.Require().NoError(err)

	s.applyMigrations(ctx, dsn)

	endpoint, err := mn.ConnectionString(ctx)
	s.Require().NoError(err)

	amqpURL, err := mq.AmqpURL(ctx)
	s.Require().NoError(err)

	port := s.allocFreePort(ctx)
	s.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	s.bucket = "e2e-" + strings.ReplaceAll(uuid.New().String(), "-", "")

	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	s.cfg = &config.App{
		HTTP: config.HTTP{
			Address:       fmt.Sprintf("127.0.0.1:%d", port),
			PublicBaseURL: s.baseURL,
		},
		Postgres: config.Postgres{DSN: dsn},
		S3: config.S3{
			Endpoint:  endpoint,
			AccessKey: mn.Username,
			SecretKey: mn.Password,
			Bucket:    s.bucket,
			UseSSL:    false,
			Region:    "us-east-1",
		},
		RabbitMQ: config.RabbitMQ{
			URL:                   amqpURL,
			Exchange:              "e2e_ex_" + suffix,
			UploadRoutingKey:      "avatar.uploaded",
			DeleteRoutingKey:      "avatar.deleted",
			UploadQueue:           "e2e_upload_" + suffix,
			DeleteQueue:           "e2e_delete_" + suffix,
			UploadDLQQueue:        "e2e_upload_dlq_" + suffix,
			UploadDLQRoutingKey:   "avatar.upload.failed",
			DeleteDLQQueue:        "e2e_delete_dlq_" + suffix,
			DeleteDLQRoutingKey:   "avatar.delete.failed",
			UploadRetryDelaysMS:   []int{200},
			DeleteRetryDelaysMS:   []int{200},
			ConsumerHandleTimeout: 30 * time.Second,
		},
	}
	s.cfg.Normalize()

	logger := slog.New(slog.DiscardHandler)
	rootCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	app, err := serverapp.New(rootCtx, logger, s.cfg, nil, nil, nil)
	s.Require().NoError(err)
	s.app = app

	s.httpDone = make(chan error, 1)
	go func() {
		s.httpDone <- httpserver.Run(rootCtx, s.app)
	}()

	repo := postgres.NewAvatarRepository(s.app.Postgres.Pgx())
	cat, err := s.cfg.Avatars.Catalog()
	s.Require().NoError(err)
	jobs := usecase.NewAvatarQueueJobs(logger, repo, s.app.S3, cat, nil)
	proc := worker.NewProcessor(logger, jobs, s.cfg.RabbitMQ, nil)
	wkApp, err := worker.NewApp(rootCtx, logger, proc, s.cfg.RabbitMQ)
	s.Require().NoError(err)
	s.wkDone = make(chan struct{})
	go func() {
		defer close(s.wkDone)
		if err := wkApp.Run(rootCtx); err != nil {
			s.T().Logf("worker app run: %v", err)
		}
		if err := wkApp.Close(); err != nil {
			s.T().Logf("worker app close: %v", err)
		}
	}()

	s.client = &http.Client{Timeout: 15 * time.Second}
	s.waitForHealth(ctx, 20*time.Second)
}

func (s *E2ESuite) TearDownSuite() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.httpDone != nil {
		select {
		case <-s.httpDone:
		case <-time.After(20 * time.Second):
			s.T().Log("httpserver.Run did not exit in time")
		}
	}
	if s.wkDone != nil {
		select {
		case <-s.wkDone:
		case <-time.After(20 * time.Second):
			s.T().Log("worker.App.Run did not exit in time")
		}
	}
	if s.app != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.app.Close(closeCtx)
		cancel()
	}
	if s.mqCtr != nil {
		if err := tc.TerminateContainer(s.mqCtr); err != nil {
			s.T().Logf("terminate rabbitmq container: %v", err)
		}
	}
	if s.mnCtr != nil {
		if err := tc.TerminateContainer(s.mnCtr); err != nil {
			s.T().Logf("terminate minio container: %v", err)
		}
	}
	if s.pgCtr != nil {
		if err := tc.TerminateContainer(s.pgCtr); err != nil {
			s.T().Logf("terminate postgres container: %v", err)
		}
	}
}

func (s *E2ESuite) applyMigrations(ctx context.Context, dsn string) {
	var pool *postgres.Pool
	var err error
	s.Require().Eventually(func() bool {
		pool, err = postgres.New(ctx, config.Postgres{DSN: dsn})
		return err == nil
	}, 20*time.Second, 300*time.Millisecond, "postgres container is not ready: %v", err)
	defer pool.Close()

	_, thisFile, _, ok := runtime.Caller(0)
	s.Require().True(ok)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	migrationsDir := filepath.Join(repoRoot, "migrations")
	s.Require().NoError(testdb.RunMigrations(pool.Pgx(), migrationsDir))
}

func (s *E2ESuite) allocFreePort(ctx context.Context) int {
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	addr, ok := l.Addr().(*net.TCPAddr)
	s.Require().True(ok, "listener address must be *net.TCPAddr")
	port := addr.Port
	s.Require().NoError(l.Close())
	return port
}

func (s *E2ESuite) waitForHealth(ctx context.Context, timeout time.Duration) {
	s.Require().Eventually(func() bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/health", nil)
		if err != nil {
			return false
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return false
		}
		if cerr := resp.Body.Close(); cerr != nil {
			s.T().Logf("close health body: %v", cerr)
		}
		return resp.StatusCode == http.StatusOK
	}, timeout, 200*time.Millisecond, "http server did not become healthy")
}

func (s *E2ESuite) closeBody(rc io.Closer) {
	if err := rc.Close(); err != nil {
		s.T().Logf("close body: %v", err)
	}
}

// do executes a request and returns the raw response. Callers must close the body.
// gosec G704 false-positive: the URL is composed from a test-controlled baseURL.
func (s *E2ESuite) do(req *http.Request) *http.Response {
	resp, err := s.client.Do(req) //nolint:gosec // SSRF: test-only baseURL.
	s.Require().NoError(err)
	return resp
}

func (s *E2ESuite) newRequest(method, path string) *http.Request {
	req, err := http.NewRequestWithContext(context.Background(), method, s.baseURL+path, nil)
	s.Require().NoError(err)
	return req
}

func (s *E2ESuite) uploadAvatar(userID string) string {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", "a.png")
	s.Require().NoError(err)
	_, err = fw.Write(minPNG)
	s.Require().NoError(err)
	s.Require().NoError(w.Close())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.baseURL+"/api/v1/avatars", &body)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-User-Id", userID)

	resp := s.do(req)
	defer s.closeBody(resp.Body)
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var out struct {
		ID     string `json:"id"`
		UserID string `json:"user_id"`
		Status string `json:"status"`
	}
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&out))
	s.Equal(userID, out.UserID)
	s.Equal("processing", out.Status)
	return out.ID
}

func (s *E2ESuite) waitMetadataReady(id string, timeout time.Duration) {
	s.Require().Eventually(func() bool {
		req := s.newRequest(http.MethodGet, "/api/v1/avatars/"+id+"/metadata")
		resp, err := s.client.Do(req)
		if err != nil {
			return false
		}
		defer s.closeBody(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		var meta struct {
			Thumbnails []struct {
				Size string `json:"size"`
			} `json:"thumbnails"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
			return false
		}
		return len(meta.Thumbnails) > 0
	}, timeout, 300*time.Millisecond, "avatar %s did not finish processing", id)
}

func (s *E2ESuite) TestUploadFlow_GeneratesThumbnailsAndServesImages() {
	user := "e2e-up-" + uuid.NewString()[:8]
	id := s.uploadAvatar(user)

	s.waitMetadataReady(id, 60*time.Second)

	// GET original.
	req := s.newRequest(http.MethodGet, "/api/v1/avatars/"+id)
	resp := s.do(req) //nolint:bodyclose // closed below.
	s.Require().Equal(http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	s.closeBody(resp.Body)
	s.Require().NoError(err)
	s.NotEmpty(body)
	s.Equal("image/png", resp.Header.Get("Content-Type"))

	// GET thumbnail JPEG.
	req = s.newRequest(http.MethodGet, "/api/v1/avatars/"+id+"?size=100x100&format=jpeg")
	resp = s.do(req) //nolint:bodyclose // closed below.
	s.Require().Equal(http.StatusOK, resp.StatusCode)
	thumb, err := io.ReadAll(resp.Body)
	s.closeBody(resp.Body)
	s.Require().NoError(err)
	s.NotEmpty(thumb)
	s.Equal("image/jpeg", resp.Header.Get("Content-Type"))

	// List user avatars; expect status=ready.
	req = s.newRequest(http.MethodGet, "/api/v1/users/"+user+"/avatars")
	resp = s.do(req) //nolint:bodyclose // closed below.
	s.Require().Equal(http.StatusOK, resp.StatusCode)
	var list []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&list))
	s.closeBody(resp.Body)
	s.Require().Len(list, 1)
	s.Equal(id, list[0].ID)
	s.Equal("ready", list[0].Status)
}

func (s *E2ESuite) TestDeleteFlow_RemovesObjectsFromS3() {
	user := "e2e-del-" + uuid.NewString()[:8]
	id := s.uploadAvatar(user)
	s.waitMetadataReady(id, 60*time.Second)

	// Capture thumbnail keys via repository for post-delete verification.
	repo := postgres.NewAvatarRepository(s.app.Postgres.Pgx())
	uid, err := uuid.Parse(id)
	s.Require().NoError(err)
	a, err := repo.GetByID(context.Background(), uid)
	s.Require().NoError(err)
	keys := append([]string{a.S3Key}, flattenThumbnailKeys(a.ThumbnailS3Keys)...)
	s.Require().NotEmpty(keys)

	// DELETE.
	req := s.newRequest(http.MethodDelete, "/api/v1/avatars/"+id)
	req.Header.Set("X-User-Id", user)
	resp := s.do(req) //nolint:bodyclose // closed below.
	s.Require().Equal(http.StatusNoContent, resp.StatusCode)
	s.closeBody(resp.Body)

	// API now reports 404 for the avatar.
	req = s.newRequest(http.MethodGet, "/api/v1/avatars/"+id+"/metadata")
	resp = s.do(req) //nolint:bodyclose // closed below.
	s.Require().Equal(http.StatusNotFound, resp.StatusCode)
	s.closeBody(resp.Body)

	// Worker eventually removes S3 originals + thumbnails.
	client, err := s3repo.New(context.Background(), s.cfg.S3)
	s.Require().NoError(err)
	s.Require().Eventually(func() bool {
		for _, k := range keys {
			if _, _, statErr := client.StatObject(context.Background(), k); statErr == nil {
				return false
			}
		}
		return true
	}, 30*time.Second, 500*time.Millisecond, "S3 objects were not removed after delete")
}

func flattenThumbnailKeys(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m)*2)
	for _, byFmt := range m {
		for _, k := range byFmt {
			out = append(out, k)
		}
	}
	return out
}

func TestE2ESuite(t *testing.T) {
	suite.Run(t, new(E2ESuite))
}
