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
	var otelMetricsProvider *otelpkg.OTELMetricsProvider
	var otelTracerProvider *otelpkg.OTELTracerProvider
	var biz *metrics.Business
	if cfg.OTEL.Enabled {
		res, err := otelpkg.NewResource(ctx, cfg.OTEL.Resource)
		if err != nil {
			logger.ErrorContext(ctx, "otel resource init failed", "err", err)
			return 1
		}
		logLevel := logging.LevelFromString(cfg.Logging.Level)
		if cfg.OTEL.OTELLoggerProvider.Enabled {
			otelLoggerProvider, err := otelpkg.NewOTELLoggerProvider(ctx, res, cfg.OTEL.OTELLoggerProvider)
			if err != nil {
				logger.ErrorContext(ctx, "otel logger provider init failed", "err", err)
				return 1
			}
			defer func() {
				shutdownCtx := context.WithoutCancel(ctx)
				if err := otelLoggerProvider.Shutdown(shutdownCtx); err != nil {
					logger.ErrorContext(shutdownCtx, "otel logger provider shutdown failed", "err", err)
				}
			}()
			logger = logging.NewOTELServerLogger(otelLoggerProvider, logLevel)
		}
		if cfg.OTEL.OTELMetricsProvider.Enabled {
			var err error
			otelMetricsProvider, err = otelpkg.NewOTELMetricsProvider(ctx, res, cfg.OTEL.OTELMetricsProvider)
			if err != nil {
				logger.ErrorContext(ctx, "otel metrics provider init failed", "err", err)
				return 1
			}
			defer func() {
				shutdownCtx := context.WithoutCancel(ctx)
				if err := otelMetricsProvider.Shutdown(shutdownCtx); err != nil {
					logger.ErrorContext(shutdownCtx, "otel metrics provider shutdown failed", "err", err)
				}
			}()
		}
		if cfg.OTEL.OTELTracerProvider.Enabled {
			var err error
			otelTracerProvider, err = otelpkg.NewOTELTracerProvider(ctx, res, cfg.OTEL.OTELTracerProvider)
			if err != nil {
				logger.ErrorContext(ctx, "otel tracer provider init failed", "err", err)
				return 1
			}
			defer func() {
				shutdownCtx := context.WithoutCancel(ctx)
				if err := otelTracerProvider.Shutdown(shutdownCtx); err != nil {
					logger.ErrorContext(shutdownCtx, "otel tracer provider shutdown failed", "err", err)
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
			logger.ErrorContext(ctx, "business metrics init failed", "err", err)
			return 1
		}
	}

	application, err := serverapp.New(
		ctx,
		logger,
		cfg,
		otelMetricsProvider,
		otelTracerProvider,
		biz,
	)
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
