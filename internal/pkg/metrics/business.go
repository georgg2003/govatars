package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"govatars/internal/pkg/otelpkg"
)

// Business holds domain counters and histograms exported via OTLP.
type Business struct {
	avatarUploads         metric.Int64Counter
	avatarDeletes         metric.Int64Counter
	thumbnailJobs         metric.Int64Counter
	thumbnailJobDuration  metric.Float64Histogram
	queueRetries          metric.Int64Counter
	dlqMessages           metric.Int64Counter
}

// NewBusiness registers instruments on mp. Returns a no-op Business when mp is nil.
func NewBusiness(mp *sdkmetric.MeterProvider) (*Business, error) {
	if mp == nil {
		return &Business{}, nil
	}
	meter := mp.Meter(otelpkg.ScopeBusiness)

	uploads, err := meter.Int64Counter("govatars.avatar.uploads.total",
		metric.WithDescription("Avatar uploads accepted and enqueued"))
	if err != nil {
		return nil, err
	}
	deletes, err := meter.Int64Counter("govatars.avatar.deletes.total",
		metric.WithDescription("Avatar deletes accepted and enqueued"))
	if err != nil {
		return nil, err
	}
	jobs, err := meter.Int64Counter("govatars.thumbnail.jobs.total",
		metric.WithDescription("Thumbnail queue jobs processed"))
	if err != nil {
		return nil, err
	}
	jobDur, err := meter.Float64Histogram("govatars.thumbnail.job.duration.seconds",
		metric.WithDescription("Thumbnail queue job duration"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	retries, err := meter.Int64Counter("govatars.queue.retries.total",
		metric.WithDescription("AMQP messages republished for retry"))
	if err != nil {
		return nil, err
	}
	dlq, err := meter.Int64Counter("govatars.queue.dlq.total",
		metric.WithDescription("AMQP messages sent to DLQ"))
	if err != nil {
		return nil, err
	}

	return &Business{
		avatarUploads:        uploads,
		avatarDeletes:        deletes,
		thumbnailJobs:        jobs,
		thumbnailJobDuration: jobDur,
		queueRetries:         retries,
		dlqMessages:          dlq,
	}, nil
}

func (b *Business) RecordAvatarUpload(ctx context.Context) {
	if b == nil || b.avatarUploads == nil {
		return
	}
	b.avatarUploads.Add(ctx, 1)
}

func (b *Business) RecordAvatarDelete(ctx context.Context) {
	if b == nil || b.avatarDeletes == nil {
		return
	}
	b.avatarDeletes.Add(ctx, 1)
}

func (b *Business) RecordThumbnailJob(ctx context.Context, operation string, ok bool, duration time.Duration) {
	if b == nil {
		return
	}
	status := attribute.String("status", "error")
	if ok {
		status = attribute.String("status", "ok")
	}
	op := attribute.String("operation", operation)
	if b.thumbnailJobs != nil {
		b.thumbnailJobs.Add(ctx, 1, metric.WithAttributes(op, status))
	}
	if b.thumbnailJobDuration != nil {
		b.thumbnailJobDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(op, status))
	}
}

func (b *Business) RecordQueueRetry(ctx context.Context, operation string) {
	if b == nil || b.queueRetries == nil {
		return
	}
	b.queueRetries.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(
		attribute.String("operation", operation),
	)))
}

func (b *Business) RecordDLQ(ctx context.Context, operation string) {
	if b == nil || b.dlqMessages == nil {
		return
	}
	b.dlqMessages.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(
		attribute.String("operation", operation),
	)))
}
