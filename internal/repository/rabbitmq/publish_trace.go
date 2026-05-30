package rabbitmq

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"

	"govatars/internal/pkg/otelpkg"
)

// PublishChannel is the subset of *amqp.Channel used for traced publishes.
type PublishChannel interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// PublishWithContext records a rabbitmq.publish span, injects W3C trace context into msg.Headers, and publishes.
func PublishWithContext(ctx context.Context, ch PublishChannel, exchange, key string, pub amqp.Publishing) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	destination := key
	if exchange != "" {
		destination = exchange + "/" + key
	}

	ctx, span := otel.Tracer(otelpkg.ScopeRabbitMQ).Start(ctx, "rabbitmq.publish")
	defer span.End()
	span.SetAttributes(
		semconv.MessagingSystemKey.String("rabbitmq"),
		attribute.String("messaging.destination", destination),
		attribute.String("messaging.rabbitmq.routing_key", key),
	)
	if exchange != "" {
		span.SetAttributes(attribute.String("messaging.rabbitmq.exchange", exchange))
	}

	if pub.Headers == nil {
		pub.Headers = amqp.Table{}
	}
	otel.GetTextMapPropagator().Inject(ctx, otelpkg.HeadersCarrier(pub.Headers))

	if err := ch.PublishWithContext(ctx, exchange, key, false, false, pub); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
