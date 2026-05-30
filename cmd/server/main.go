package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"govatars/internal/httpserver"
	"govatars/internal/pkg/config"
	"govatars/internal/pkg/logging"
	"govatars/internal/pkg/metrics"
	"govatars/internal/pkg/otelpkg"
	"govatars/internal/serverapp"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger := logging.NewServerLogger(logging.LevelFromString("info"))
		logger.ErrorContext(ctx, "config load failed", "err", err)
		return 1
	}
	logger := logging.NewServerLogger(logging.LevelFromString(cfg.Logging.Level))
	logLevel := logging.LevelFromString(cfg.Logging.Level)

	var biz *metrics.Business
	if cfg.OTEL.Enabled {
		otel, err := otelpkg.Bootstrap(ctx, cfg.OTEL, logLevel, logger, logging.NewOTELServerLogger)
		if err != nil {
			logger.ErrorContext(ctx, "otel bootstrap failed", "err", err)
			return 1
		}
		logger = otel.Logger
		defer otel.Shutdown(ctx)
		if otel.Metrics != nil {
			biz, err = metrics.NewBusiness(otel.Metrics.MeterProvider)
			if err != nil {
				logger.ErrorContext(ctx, "business metrics init failed", "err", err)
				return 1
			}
		}
	}
	if biz == nil {
		biz, err = metrics.NewBusiness(nil)
		if err != nil {
			logger.ErrorContext(ctx, "business metrics init failed", "err", err)
			return 1
		}
	}

	application, err := serverapp.New(ctx, logger, cfg, biz)
	if err != nil {
		logger.ErrorContext(ctx, "app init failed", "err", err)
		return 1
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		application.Close(closeCtx)
	}()

	if err := httpserver.Run(ctx, application); err != nil {
		logger.ErrorContext(ctx, "http server exited", "err", err)
		return 1
	}
	return 0
}
