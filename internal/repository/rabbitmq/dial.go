package rabbitmq

import (
	"errors"

	amqp "github.com/rabbitmq/amqp091-go"

	"govatars/internal/pkg/apperr"
)

// Dial opens a single AMQP connection (consumer/worker entrypoint). Caller must Close the connection.
func Dial(url string) (*amqp.Connection, error) {
	if url == "" {
		return nil, errors.New("rabbitmq: empty url")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, apperr.Wrap(err, "rabbitmq dial")
	}
	return conn, nil
}
