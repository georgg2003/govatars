package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"govatars/internal/pkg/config"
	"govatars/internal/pkg/logging"
	"govatars/internal/pkg/metrics"
	"govatars/internal/pkg/otelpkg"
	"govatars/internal/repository/postgres"
	s3repo "govatars/internal/repository/s3"
	"govatars/internal/usecase"
	"govatars/internal/worker"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger := logging.NewWorkerLogger(logging.LevelFromString("info"))
		logger.ErrorContext(ctx, "config load failed", "err", err)
		return 1
	}

	logLevel := logging.LevelFromString(cfg.Logging.Level)
	log := logging.NewWorkerLogger(logLevel)
	var biz *metrics.Business
	if cfg.OTEL.Enabled {
		res, err := otelpkg.NewResource(ctx, cfg.OTEL.Resource)
		if err != nil {
			log.ErrorContext(ctx, "otel resource init failed", "err", err)
			return 1
		}
		var otelMetricsProvider *otelpkg.OTELMetricsProvider
		var otelTracerProvider *otelpkg.OTELTracerProvider
		if cfg.OTEL.OTELLoggerProvider.Enabled {
			otelLoggerProvider, err := otelpkg.NewOTELLoggerProvider(ctx, res, cfg.OTEL.OTELLoggerProvider)
			if err != nil {
				log.ErrorContext(ctx, "otel logger provider init failed", "err", err)
				return 1
			}
			defer func() {
				shutdownCtx := context.WithoutCancel(ctx)
				if err := otelLoggerProvider.Shutdown(shutdownCtx); err != nil {
					log.ErrorContext(shutdownCtx, "otel logger provider shutdown failed", "err", err)
				}
			}()
			log = logging.NewOTELWorkerLogger(otelLoggerProvider, logLevel)
		}
		if cfg.OTEL.OTELMetricsProvider.Enabled {
			otelMetricsProvider, err = otelpkg.NewOTELMetricsProvider(ctx, res, cfg.OTEL.OTELMetricsProvider)
			if err != nil {
				log.ErrorContext(ctx, "otel metrics provider init failed", "err", err)
				return 1
			}
			defer func() {
				shutdownCtx := context.WithoutCancel(ctx)
				if err := otelMetricsProvider.Shutdown(shutdownCtx); err != nil {
					log.ErrorContext(shutdownCtx, "otel metrics provider shutdown failed", "err", err)
				}
			}()
		}
		if cfg.OTEL.OTELTracerProvider.Enabled {
			otelTracerProvider, err = otelpkg.NewOTELTracerProvider(ctx, res, cfg.OTEL.OTELTracerProvider)
			if err != nil {
				log.ErrorContext(ctx, "otel tracer provider init failed", "err", err)
				return 1
			}
			defer func() {
				shutdownCtx := context.WithoutCancel(ctx)
				if err := otelTracerProvider.Shutdown(shutdownCtx); err != nil {
					log.ErrorContext(shutdownCtx, "otel tracer provider shutdown failed", "err", err)
				}
			}()
		}
		otelpkg.InstallGlobals(otelTracerProvider, otelMetricsProvider)
		if otelMetricsProvider != nil {
			biz, err = metrics.NewBusiness(otelMetricsProvider.MeterProvider)
		} else {
			biz, err = metrics.NewBusiness(nil)
		}
		if err != nil {
			log.ErrorContext(ctx, "business metrics init failed", "err", err)
			return 1
		}
	}

	pgPool, err := postgres.New(ctx, cfg.Postgres)
	if err != nil {
		log.ErrorContext(ctx, "postgres", "err", err)
		return 1
	}
	defer pgPool.Close()

	s3Client, err := s3repo.New(ctx, cfg.S3)
	if err != nil {
		log.ErrorContext(ctx, "s3", "err", err)
		return 1
	}

	thumbs, err := cfg.Avatars.Catalog()
	if err != nil {
		log.ErrorContext(ctx, "avatars catalog", "err", err)
		return 1
	}

	repo := postgres.NewAvatarRepository(pgPool.Pgx())
	queueJobs := usecase.NewAvatarQueueJobs(log, repo, s3Client, thumbs, biz)
	proc := worker.NewProcessor(log, queueJobs, cfg.RabbitMQ, biz)

	app, err := worker.NewApp(ctx, log, proc, cfg.RabbitMQ)
	if err != nil {
		log.ErrorContext(ctx, "worker app", "err", err)
		return 1
	}
	defer func() {
		if err := app.Close(); err != nil {
			log.WarnContext(ctx, "worker app close", "err", err)
		}
	}()

	if err := app.Run(ctx); err != nil {
		log.ErrorContext(ctx, "worker", "err", err)
		return 1
	}
	return 0
}
