package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/circuitbreaker"
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
	cb   *circuitbreaker.CircuitBreaker
}

// NewPublisher connects, declares topology, and returns a publisher. Caller must Close().
func NewPublisher(ctx context.Context, log *slog.Logger, cfg config.RabbitMQ) (*Publisher, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, errors.New("rabbitmq: empty url")
	}
	p := &Publisher{
		cfg: cfg,
		log: log,
		cb: circuitbreaker.New(circuitbreaker.Config{
			Threshold: cfg.CircuitBreaker.Threshold,
			Cooldown:  cfg.CircuitBreaker.Cooldown,
		}),
	}
	if err := p.connect(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Publisher) connect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.cb.Execute(func() error {
		conn, err := DialContext(ctx, p.cfg.URL)
		if err != nil {
			return err
		}
		ch, err := conn.Channel()
		if err != nil {
			if cerr := conn.Close(); cerr != nil {
				p.log.WarnContext(ctx, "rabbitmq close conn after channel open failure", "err", cerr)
			}
			return apperr.Wrap(err, "rabbitmq channel")
		}
		if err := ctx.Err(); err != nil {
			p.closeConnCh(ctx, conn, ch)
			return err
		}
		if err := DeclareTopology(ch, p.cfg); err != nil {
			p.closeConnCh(ctx, conn, ch)
			return err
		}
		p.conn = conn
		p.ch = ch
		return nil
	})
}

func (p *Publisher) closeConnCh(ctx context.Context, conn *amqp.Connection, ch *amqp.Channel) {
	if ch != nil {
		if cerr := ch.Close(); cerr != nil {
			p.log.WarnContext(ctx, "rabbitmq close channel", "err", cerr)
		}
	}
	if conn != nil {
		if cerr := conn.Close(); cerr != nil {
			p.log.WarnContext(ctx, "rabbitmq close conn", "err", cerr)
		}
	}
}

func (p *Publisher) closeLocked(ctx context.Context) {
	if p.ch != nil {
		if err := p.ch.Close(); err != nil {
			p.log.WarnContext(ctx, "rabbitmq channel close", "err", err)
		}
		p.ch = nil
	}
	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			p.log.WarnContext(ctx, "rabbitmq conn close", "err", err)
		}
		p.conn = nil
	}
}

// reconnectIfNeeded re-establishes the AMQP session when the broker restarted or TCP dropped.
func (p *Publisher) reconnectIfNeeded(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.conn != nil && !p.conn.IsClosed() && p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	p.closeLocked(ctx)
	return p.connect(ctx)
}

// Close releases the channel and connection.
func (p *Publisher) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked(ctx)
	return nil
}

// Health checks the publisher's AMQP connection and that the upload queue exists (passive declare).
func (p *Publisher) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.reconnectIfNeeded(ctx); err != nil {
		return err
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
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if err := p.reconnectIfNeeded(ctx); err != nil {
		return err
	}
	return PublishWithContext(ctx, p.ch, p.cfg.Exchange, routingKey, pub)
}

var _ usecase.EventPublisher = (*Publisher)(nil)
