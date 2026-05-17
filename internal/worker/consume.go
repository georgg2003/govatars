package worker

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"

	"govatars/internal/pkg/otelpkg"
)

func consumeContext(ctx context.Context, d amqp.Delivery, operation, queue string) (context.Context, func(error)) {
	carrier := otelpkg.HeadersCarrier(d.Headers)
	parent := otel.GetTextMapPropagator().Extract(ctx, carrier)
	spanCtx, span := otel.Tracer(otelpkg.ScopeWorker).Start(parent, "rabbitmq.process")
	span.SetAttributes(
		semconv.MessagingSystemKey.String("rabbitmq"),
		attribute.String("messaging.destination", queue),
		attribute.String("messaging.operation", operation),
	)
	if d.MessageId != "" {
		span.SetAttributes(attribute.String("messaging.message_id", d.MessageId))
	}
	return spanCtx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}
