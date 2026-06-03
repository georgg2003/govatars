package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/sync/errgroup"

	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
	"govatars/internal/repository/rabbitmq"
)

// App wires AMQP channels, declares topology once, and runs upload + delete consumers until ctx ends.
// On unexpected AMQP disconnect, Run returns an error so the process can exit and be restarted.
type App struct {
	log           *slog.Logger
	proc          *Processor
	cfg           config.RabbitMQ
	conn          *amqp.Connection
	handleTimeout time.Duration
	ready         atomic.Bool
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
	conn, err := rabbitmq.DialContext(ctx, cfg.URL)
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
	a.ready.Store(false)
	return a.conn.Close()
}

// Run opens two channels, declares topology, starts consumers, and blocks until ctx is cancelled
// or the AMQP session fails. healthAddr, when non-empty, serves GET /health for probes.
//
// Each *amqp.Channel is used from exactly one consumeLoop goroutine, so channel calls are not concurrent per channel.
func (a *App) Run(ctx context.Context, healthAddr string) error {
	if healthAddr != "" {
		if err := a.startHealth(ctx, healthAddr); err != nil {
			return apperr.Wrap(err, "worker health listen")
		}
	}

	connClosed := a.conn.NotifyClose(make(chan *amqp.Error, 1))
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		select {
		case <-gctx.Done():
			return nil
		case amqpErr := <-connClosed:
			if amqpErr != nil {
				return apperr.Wrap(amqpErr, "rabbitmq connection closed")
			}
			return ErrAMQPDisconnected
		}
	})

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
	a.ready.Store(true)

	g.Go(func() error {
		return a.consumeLoop(gctx, uploadMsgs, chUp, a.proc.HandleUploadDelivery, "upload", a.cfg.UploadQueue, "upload delivery")
	})
	g.Go(func() error {
		return a.consumeLoop(gctx, deleteMsgs, chDel, a.proc.HandleDeleteDelivery, "delete", a.cfg.DeleteQueue, "delete delivery")
	})

	a.log.InfoContext(ctx, "worker consuming", "upload_queue", a.cfg.UploadQueue, "delete_queue", a.cfg.DeleteQueue)

	if err := g.Wait(); err != nil {
		a.log.ErrorContext(ctx, "worker stopping after amqp failure", "err", err)
		return err
	}
	if ctx.Err() != nil {
		a.log.InfoContext(ctx, "worker stopping", "err", ctx.Err())
	}
	return nil
}

type deliveryHandler func(context.Context, amqpPublisher, amqp.Delivery) error

func (a *App) consumeLoop(
	ctx context.Context,
	deliveries <-chan amqp.Delivery,
	ch amqpPublisher,
	handler deliveryHandler,
	operation, queue, errLogKey string,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("%s consumer: %w", operation, ErrAMQPDisconnected)
			}
			spanCtx, endSpan := consumeContext(ctx, d, operation, queue)
			hctx, cancel := context.WithTimeout(spanCtx, a.handleTimeout)
			err := handler(hctx, ch, d)
			cancel()
			endSpan(err)
			if err != nil {
				a.log.ErrorContext(spanCtx, errLogKey, "err", err)
				if nackErr := d.Nack(false, false); nackErr != nil {
					a.log.WarnContext(spanCtx, "amqp nack", "err", nackErr)
				}
				continue
			}
			if ackErr := d.Ack(false); ackErr != nil {
				a.log.WarnContext(spanCtx, "amqp ack", "err", ackErr)
			}
		}
	}
}
