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

	application, err := serverapp.New(ctx, logger, cfg)
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
