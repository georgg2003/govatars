package otelpkg

import (
	"context"
	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

func NewResource(ctx context.Context, cfg config.OTELResource) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, apperr.Wrap(err, "failed to create resource")
	}
	return res, nil
}

type OTELLoggerProvider struct {
	loggerProvider  *sdklog.LoggerProvider
	shutdownTimeout time.Duration
}

func (p *OTELLoggerProvider) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, p.shutdownTimeout)
	defer cancel()
	if err := p.loggerProvider.Shutdown(ctx); err != nil {
		return apperr.Wrap(err, "failed to shutdown logger provider")
	}
	return nil
}

func (p *OTELLoggerProvider) NewSlogHandler() slog.Handler {
	return otelslog.NewHandler("govatars", otelslog.WithLoggerProvider(p.loggerProvider))
}

func NewOTELLoggerProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg config.OTELLoggerProvider,
) (*OTELLoggerProvider, error) {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Endpoint),
		otlploggrpc.WithTimeout(cfg.Timeout),
	}
	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}
	otlpLogExporter, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, apperr.Wrap(err, "failed to create OTLP log exporter")
	}
	shutdownTimeout := 5 * time.Second
	if cfg.ShutdownTimeout > 0 {
		shutdownTimeout = cfg.ShutdownTimeout
	}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(otlpLogExporter,
			sdklog.WithExportTimeout(cfg.BatchTimeout),
			sdklog.WithExportMaxBatchSize(cfg.BatchSize),
		)),
	)
	return &OTELLoggerProvider{loggerProvider: provider, shutdownTimeout: shutdownTimeout}, nil
}

type OTELMetricsProvider struct {
	provider        *sdkmetrics.MeterProvider
	shutdownTimeout time.Duration
}

func (p *OTELMetricsProvider) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, p.shutdownTimeout)
	defer cancel()
	if err := p.provider.Shutdown(ctx); err != nil {
		return apperr.Wrap(err, "failed to shutdown metrics provider")
	}
	return nil
}

func NewOTELMetricsProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg config.OTELMetricsProvider,
) (*OTELMetricsProvider, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		otlpmetricgrpc.WithTimeout(cfg.Timeout),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	otlpMetricExporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, apperr.Wrap(err, "failed to create OTLP metric exporter")
	}
	shutdownTimeout := 5 * time.Second
	if cfg.ShutdownTimeout > 0 {
		shutdownTimeout = cfg.ShutdownTimeout
	}
	provider := sdkmetrics.NewMeterProvider(
		sdkmetrics.WithResource(res),
		sdkmetrics.WithReader(
			sdkmetrics.NewPeriodicReader(
				otlpMetricExporter,
				sdkmetrics.WithInterval(cfg.Interval),
				sdkmetrics.WithTimeout(cfg.Timeout),
			),
		),
	)
	return &OTELMetricsProvider{provider: provider, shutdownTimeout: shutdownTimeout}, nil
}
