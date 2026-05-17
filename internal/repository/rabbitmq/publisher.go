package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
	"govatars/internal/usecase"
)

// Publisher sends JSON messages to a direct exchange.
//
// A single [amqp.Channel] is not goroutine-safe; all channel operations are serialized with mu.
type Publisher struct {
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
	cfg  config.RabbitMQ
	log  *slog.Logger
}

// NewPublisher connects, declares topology, and returns a publisher. Caller must Close().
func NewPublisher(ctx context.Context, log *slog.Logger, cfg config.RabbitMQ) (*Publisher, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, errors.New("rabbitmq: empty url")
	}
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, apperr.Wrap(err, "rabbitmq dial")
	}
	ch, err := conn.Channel()
	if err != nil {
		if cerr := conn.Close(); cerr != nil {
			log.WarnContext(ctx, "rabbitmq close conn after channel open failure", "err", cerr)
		}
		return nil, apperr.Wrap(err, "rabbitmq channel")
	}
	if err := DeclareTopology(ch, cfg); err != nil {
		if cerr := ch.Close(); cerr != nil {
			log.WarnContext(ctx, "rabbitmq close channel after topology failure", "err", cerr)
		}
		if cerr := conn.Close(); cerr != nil {
			log.WarnContext(ctx, "rabbitmq close conn after topology failure", "err", cerr)
		}
		return nil, err
	}
	return &Publisher{conn: conn, ch: ch, cfg: cfg, log: log}, nil
}

// Close releases the channel and connection.
func (p *Publisher) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil {
		if err := p.ch.Close(); err != nil {
			p.log.WarnContext(ctx, "rabbitmq channel close", "err", err)
		}
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Health checks the publisher's AMQP connection and that the upload queue exists (passive declare).
func (p *Publisher) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil || p.conn.IsClosed() {
		return errors.New("rabbitmq: connection closed")
	}
	if p.ch == nil || p.ch.IsClosed() {
		return errors.New("rabbitmq: channel closed")
	}
	if p.cfg.UploadQueue == "" {
		return errors.New("rabbitmq: empty upload queue name")
	}
	_, err := p.ch.QueueDeclarePassive(p.cfg.UploadQueue, true, false, false, false, nil)
	if err != nil {
		return apperr.Wrap(err, "rabbitmq health")
	}
	return nil
}

// PublishJSON sends a persistent JSON message with optional AMQP message id (idempotency hint).
func (p *Publisher) PublishJSON(ctx context.Context, routingKey string, messageID string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         b,
	}
	if messageID != "" {
		pub.MessageId = messageID
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return PublishWithContext(ctx, p.ch, p.cfg.Exchange, routingKey, pub)
}

var _ usecase.EventPublisher = (*Publisher)(nil)
