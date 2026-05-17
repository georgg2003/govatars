package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"

	amqp "github.com/rabbitmq/amqp091-go"

	"govatars/internal/models"
	"govatars/internal/pkg/config"
	"govatars/internal/pkg/metrics"
)

// avatarQueueJobs is the minimal use-case surface the [Processor] invokes after unmarshaling AMQP payloads.
type avatarQueueJobs interface {
	ProcessAvatarUpload(ctx context.Context, ev *models.AvatarUploadEvent) error
	ProcessAvatarDelete(ctx context.Context, ev *models.AvatarDeleteEvent) error
}

// Processor adapts RabbitMQ deliveries to queue avatar jobs and handles retry/DLQ publishing.
type Processor struct {
	log    *slog.Logger
	jobs   avatarQueueJobs
	rabbit config.RabbitMQ
	biz    *metrics.Business
}

// NewProcessor wires a pre-built avatar queue use case with RabbitMQ retry settings.
func NewProcessor(log *slog.Logger, jobs avatarQueueJobs, rabbit config.RabbitMQ, biz *metrics.Business) *Processor {
	return &Processor{log: log, jobs: jobs, rabbit: rabbit, biz: biz}
}

// HandleUploadDelivery unmarshals the body, runs upload processing, republishes or DLQ on failure.
func (p *Processor) HandleUploadDelivery(ctx context.Context, ch amqpPublisher, d amqp.Delivery) error {
	var ev models.AvatarUploadEvent
	if err := json.Unmarshal(d.Body, &ev); err != nil {
		p.log.WarnContext(ctx, "upload: bad json", "err", err)
		return nil
	}

	err := p.jobs.ProcessAvatarUpload(ctx, &ev)
	if err == nil {
		return nil
	}
	p.log.ErrorContext(ctx, "upload: process failed", "err", err)
	return p.republishOrDLQ(ctx, ch, d, true)
}

// HandleDeleteDelivery unmarshals the body, runs delete cleanup, republishes or DLQ on failure.
func (p *Processor) HandleDeleteDelivery(ctx context.Context, ch amqpPublisher, d amqp.Delivery) error {
	var ev models.AvatarDeleteEvent
	if err := json.Unmarshal(d.Body, &ev); err != nil {
		p.log.WarnContext(ctx, "delete: bad json", "err", err)
		return nil
	}

	if err := p.jobs.ProcessAvatarDelete(ctx, &ev); err != nil {
		p.log.ErrorContext(ctx, "delete: process failed", "err", err)
		return p.republishOrDLQ(ctx, ch, d, false)
	}
	return nil
}

func (p *Processor) republishOrDLQ(ctx context.Context, ch amqpPublisher, d amqp.Delivery, upload bool) error {
	delays := p.rabbit.DeleteRetryDelaysMS
	base := p.rabbit.DeleteQueue
	dlqRK := p.rabbit.DeleteDLQRoutingKey
	if upload {
		delays = p.rabbit.UploadRetryDelaysMS
		base = p.rabbit.UploadQueue
		dlqRK = p.rabbit.UploadDLQRoutingKey
	}

	op := "delete"
	if upload {
		op = "upload"
	}

	rc := retryCountFromHeaders(d.Headers)
	if len(delays) == 0 || rc >= len(delays) {
		p.biz.RecordDLQ(ctx, op)
		return ch.PublishWithContext(ctx, p.rabbit.Exchange, dlqRK, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         d.Body,
			MessageId:    d.MessageId,
			Headers:      d.Headers,
		})
	}

	qname := retryQueueName(base, rc)
	n := int64(rc) + 1
	if n < 0 || n > math.MaxInt32 {
		return errors.New("worker: retry count overflow")
	}
	next := int32(n)
	h := make(amqp.Table, len(d.Headers)+1)
	maps.Copy(h, d.Headers)
	h["x-retry-count"] = next

	p.biz.RecordQueueRetry(ctx, op)
	return ch.PublishWithContext(ctx, "", qname, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         d.Body,
		MessageId:    d.MessageId,
		Headers:      h,
	})
}

func retryQueueName(baseQueue string, retryIndex int) string {
	return fmt.Sprintf("%s.retry.%d", baseQueue, retryIndex)
}
