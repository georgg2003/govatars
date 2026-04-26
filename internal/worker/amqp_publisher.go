package worker

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

// amqpPublisher is the subset of *amqp.Channel needed for retries/DLQ (testable without a broker).
//
//go:generate go tool mockgen -source=amqp_publisher.go -destination=mock_amqp_publisher_test.go -package=worker amqpPublisher
type amqpPublisher interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}
