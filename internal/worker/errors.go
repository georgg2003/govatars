package worker

import "errors"

// ErrAMQPDisconnected is returned when the RabbitMQ connection or consumer stops unexpectedly.
var ErrAMQPDisconnected = errors.New("worker: amqp disconnected")
