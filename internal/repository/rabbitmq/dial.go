package rabbitmq

import (
	"context"
	"errors"
	"net"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"govatars/internal/pkg/apperr"
)

var defaultConnectionTimeout = 30 * time.Second

type DialOption func(*DialOptions)

type DialOptions struct {
	ConnectionTimeout time.Duration
}

func WithConnectionTimeout(timeout time.Duration) DialOption {
	return func(opts *DialOptions) {
		opts.ConnectionTimeout = timeout
	}
}

// Dial opens a single AMQP connection (consumer/worker entrypoint). Caller must Close the connection.
func Dial(url string, opts ...DialOption) (*amqp.Connection, error) {
	return DialContext(context.Background(), url, opts...)
}

// DialContext dials RabbitMQ with a context-aware TCP connect and cancels when ctx is done.
func DialContext(ctx context.Context, url string, opts ...DialOption) (*amqp.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if url == "" {
		return nil, errors.New("rabbitmq: empty url")
	}

	options := DialOptions{
		ConnectionTimeout: defaultConnectionTimeout,
	}
	for _, opt := range opts {
		opt(&options)
	}

	type dialResult struct {
		conn *amqp.Connection
		err  error
	}
	done := make(chan dialResult, 1)
	go func() {
		conn, err := amqp.DialConfig(url, amqp.Config{
			Dial: func(network, addr string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: options.ConnectionTimeout,
				}
				conn, err := d.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}

				// Heartbeating hasn't started yet, don't stall forever on a dead server.
				// A deadline is set for TLS and AMQP handshaking. After AMQP is established,
				// the deadline is cleared in openComplete.
				if err := conn.SetDeadline(time.Now().Add(options.ConnectionTimeout)); err != nil {
					return nil, err
				}

				return conn, nil
			},
		})
		done <- dialResult{conn: conn, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		if r.err != nil {
			return nil, apperr.Wrap(r.err, "rabbitmq dial")
		}
		return r.conn, nil
	}
}
