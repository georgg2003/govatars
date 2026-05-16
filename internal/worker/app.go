package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
	"govatars/internal/repository/rabbitmq"
)

// App wires AMQP channels, declares topology once, and runs upload + delete consumers until ctx ends.
type App struct {
	log           *slog.Logger
	proc          *Processor
	cfg           config.RabbitMQ
	conn          *amqp.Connection
	handleTimeout time.Duration
}

// NewApp builds the runnable worker and opens the RabbitMQ connection.
func NewApp(ctx context.Context, log *slog.Logger, proc *Processor, cfg config.RabbitMQ) (*App, error) {
	if proc == nil {
		return nil, errors.New("worker.NewApp: processor is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, errors.New("worker.NewApp: empty rabbitmq url")
	}
	conn, err := rabbitmq.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}
	ht := cfg.ConsumerHandleTimeout
	if ht <= 0 {
		ht = 3 * time.Minute
	}
	return &App{log: log, proc: proc, cfg: cfg, conn: conn, handleTimeout: ht}, nil
}

// Close closes the AMQP connection opened by [NewApp].
func (a *App) Close() error {
	if a == nil || a.conn == nil {
		return nil
	}
	return a.conn.Close()
}

// Run opens two channels, declares topology on the first, starts consumers, and blocks until ctx is done.
//
// Each *amqp.Channel is used from exactly one consumeLoop goroutine, so channel calls are not concurrent per channel.
func (a *App) Run(ctx context.Context) error {
	chUp, err := a.conn.Channel()
	if err != nil {
		return apperr.Wrap(err, "rabbitmq upload channel")
	}
	defer func() {
		if err := chUp.Close(); err != nil {
			a.log.WarnContext(ctx, "rabbitmq upload channel close", "err", err)
		}
	}()
	if err := chUp.Qos(1, 0, false); err != nil {
		return apperr.Wrap(err, "upload qos")
	}
	if err := rabbitmq.DeclareTopology(chUp, a.cfg); err != nil {
		return apperr.Wrap(err, "rabbitmq topology")
	}

	chDel, err := a.conn.Channel()
	if err != nil {
		return apperr.Wrap(err, "rabbitmq delete channel")
	}
	defer func() {
		if err := chDel.Close(); err != nil {
			a.log.WarnContext(ctx, "rabbitmq delete channel close", "err", err)
		}
	}()
	if err := chDel.Qos(1, 0, false); err != nil {
		return apperr.Wrap(err, "delete qos")
	}

	uploadMsgs, err := chUp.Consume(a.cfg.UploadQueue, a.cfg.UploadConsumerTag, false, false, false, false, nil)
	if err != nil {
		return apperr.Wrap(err, "consume upload")
	}
	deleteMsgs, err := chDel.Consume(a.cfg.DeleteQueue, a.cfg.DeleteConsumerTag, false, false, false, false, nil)
	if err != nil {
		return apperr.Wrap(err, "consume delete")
	}

	go a.consumeLoop(ctx, uploadMsgs, chUp, a.proc.HandleUploadDelivery, "upload delivery")
	go a.consumeLoop(ctx, deleteMsgs, chDel, a.proc.HandleDeleteDelivery, "delete delivery")

	a.log.InfoContext(ctx, "worker consuming", "upload_queue", a.cfg.UploadQueue, "delete_queue", a.cfg.DeleteQueue)
	<-ctx.Done()
	a.log.InfoContext(ctx, "worker stopping")
	return nil
}

type deliveryHandler func(context.Context, amqpPublisher, amqp.Delivery) error

func (a *App) consumeLoop(
	ctx context.Context,
	deliveries <-chan amqp.Delivery,
	ch amqpPublisher,
	handler deliveryHandler,
	errLogKey string,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			hctx, cancel := context.WithTimeout(ctx, a.handleTimeout)
			err := handler(hctx, ch, d)
			cancel()
			if err != nil {
				a.log.ErrorContext(ctx, errLogKey, "err", err)
				if nackErr := d.Nack(false, false); nackErr != nil {
					a.log.WarnContext(ctx, "amqp nack", "err", nackErr)
				}
				continue
			}
			if ackErr := d.Ack(false); ackErr != nil {
				a.log.WarnContext(ctx, "amqp ack", "err", ackErr)
			}
		}
	}
}
