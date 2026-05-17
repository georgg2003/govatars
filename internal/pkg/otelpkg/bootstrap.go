package otelpkg

import (
	"context"
	"log/slog"

	"govatars/internal/pkg/config"
)

// BootstrapResult holds OTEL providers initialized from config and a combined shutdown hook.
type BootstrapResult struct {
	Logger   *slog.Logger
	Tracer   *OTELTracerProvider
	Metrics  *OTELMetricsProvider
	Shutdown func(context.Context)
}

// Bootstrap wires logger, metrics, and tracer providers when enabled in otelCfg.
// baseLogger is used for shutdown errors; newOTELLogger builds the multi-handler logger when OTLP logs are on.
func Bootstrap(
	ctx context.Context,
	otelCfg config.OTEL,
	logLevel slog.Level,
	baseLogger *slog.Logger,
	newOTELLogger func(*OTELLoggerProvider, slog.Level) *slog.Logger,
) (BootstrapResult, error) {
	res, err := NewResource(ctx, otelCfg.Resource)
	if err != nil {
		return BootstrapResult{}, err
	}

	result := BootstrapResult{Logger: baseLogger}
	var shutdowns []func(context.Context) error

	if otelCfg.OTELLoggerProvider.Enabled {
		lp, err := NewOTELLoggerProvider(ctx, res, otelCfg.OTELLoggerProvider)
		if err != nil {
			return BootstrapResult{}, err
		}
		shutdowns = append(shutdowns, lp.Shutdown)
		result.Logger = newOTELLogger(lp, logLevel)
	}

	if otelCfg.OTELMetricsProvider.Enabled {
		mp, err := NewOTELMetricsProvider(ctx, res, otelCfg.OTELMetricsProvider)
		if err != nil {
			shutdownProviders(context.WithoutCancel(ctx), baseLogger, shutdowns)
			return BootstrapResult{}, err
		}
		shutdowns = append(shutdowns, mp.Shutdown)
		result.Metrics = mp
	}

	if otelCfg.OTELTracerProvider.Enabled {
		tp, err := NewOTELTracerProvider(ctx, res, otelCfg.OTELTracerProvider)
		if err != nil {
			shutdownProviders(context.WithoutCancel(ctx), baseLogger, shutdowns)
			return BootstrapResult{}, err
		}
		shutdowns = append(shutdowns, tp.Shutdown)
		result.Tracer = tp
	}

	InstallGlobals(result.Tracer, result.Metrics)

	result.Shutdown = func(shutdownCtx context.Context) {
		shutdownProviders(context.WithoutCancel(shutdownCtx), baseLogger, shutdowns)
	}
	return result, nil
}

func shutdownProviders(ctx context.Context, log *slog.Logger, fns []func(context.Context) error) {
	for i := len(fns) - 1; i >= 0; i-- {
		if err := fns[i](ctx); err != nil {
			log.ErrorContext(ctx, "otel provider shutdown failed", "err", err)
		}
	}
}
