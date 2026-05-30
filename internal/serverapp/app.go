// Package serverapp wires shared dependencies for the HTTP API process.
package serverapp

import (
	"context"
	"log/slog"
	"time"

	"govatars/internal/pkg/config"
	"govatars/internal/pkg/metrics"
	"govatars/internal/repository/postgres"
	"govatars/internal/repository/rabbitmq"
	s3repo "govatars/internal/repository/s3"
	"govatars/internal/usecase"
)

// placeholderPrewarmTimeout bounds the startup placeholder upload so a slow / hung S3 cannot
// keep the API from accepting traffic indefinitely.
const placeholderPrewarmTimeout = 30 * time.Second

// App holds infrastructure opened for the API server.
type App struct {
	Logger    *slog.Logger
	Cfg       *config.App
	Postgres  *postgres.Pool
	S3        *s3repo.Client
	Publisher *rabbitmq.Publisher
	Health    *usecase.Health
	Avatar    *usecase.AvatarService
}

// New opens Postgres, S3, and RabbitMQ publisher, then builds use cases.
func New(
	ctx context.Context,
	log *slog.Logger,
	cfg *config.App,
	biz *metrics.Business,
) (*App, error) {
	pgPool, err := postgres.New(ctx, cfg.Postgres, cfg.OTEL.TracingEnabled())
	if err != nil {
		return nil, err
	}

	s3Client, err := s3repo.New(ctx, cfg.S3)
	if err != nil {
		pgPool.Close()
		return nil, err
	}

	pub, err := rabbitmq.NewPublisher(ctx, log, cfg.RabbitMQ)
	if err != nil {
		pgPool.Close()
		return nil, err
	}

	thumbs, err := cfg.Avatars.Catalog()
	if err != nil {
		if cerr := pub.Close(ctx); cerr != nil {
			log.WarnContext(ctx, "close rabbitmq publisher after catalog error", "err", cerr)
		}
		pgPool.Close()
		return nil, err
	}

	avatarRepo := postgres.NewAvatarRepository(pgPool.Pgx())
	healthUC := usecase.NewHealth(pgPool, s3Client, pub)
	avatarUC := usecase.NewAvatarService(ctx, avatarRepo, s3Client, pub, cfg, thumbs, log, biz)

	prewarmCtx, prewarmCancel := context.WithTimeout(ctx, placeholderPrewarmTimeout)
	if err := avatarUC.EnsurePlaceholderInS3(prewarmCtx); err != nil {
		log.WarnContext(prewarmCtx, "ensure placeholder in s3", "err", err)
	}
	prewarmCancel()

	return &App{
		Logger:    log,
		Cfg:       cfg,
		Postgres:  pgPool,
		S3:        s3Client,
		Publisher: pub,
		Health:    healthUC,
		Avatar:    avatarUC,
	}, nil
}

// Close releases resources opened by [New] (best-effort).
// ctx is used only for logging close failures; a context.Background is acceptable when no
// shutdown context exists, but threading the signal-cancellation ctx makes shutdown logs
// distinguishable from request logs.
func (a *App) Close(ctx context.Context) {
	if a == nil {
		return
	}
	if a.Publisher != nil {
		if err := a.Publisher.Close(ctx); err != nil {
			a.Logger.WarnContext(ctx, "publisher close", "err", err)
		}
	}
	if a.Postgres != nil {
		a.Postgres.Close()
	}
}
