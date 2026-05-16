package rabbitmq

import (
	"fmt"
	"math"

	amqp "github.com/rabbitmq/amqp091-go"

	"govatars/internal/pkg/apperr"
	"govatars/internal/pkg/config"
)

// DeclareTopology declares exchange, main queues, retry TTL queues (DLX back to main RK), and DLQs.
func DeclareTopology(ch *amqp.Channel, cfg config.RabbitMQ) error {
	if err := ch.ExchangeDeclare(cfg.Exchange, "direct", true, false, false, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq exchange")
	}

	if _, err := ch.QueueDeclare(cfg.UploadQueue, true, false, false, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq upload queue")
	}
	if err := ch.QueueBind(cfg.UploadQueue, cfg.UploadRoutingKey, cfg.Exchange, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq upload bind")
	}

	if _, err := ch.QueueDeclare(cfg.DeleteQueue, true, false, false, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq delete queue")
	}
	if err := ch.QueueBind(cfg.DeleteQueue, cfg.DeleteRoutingKey, cfg.Exchange, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq delete bind")
	}

	if err := declareRetryQueues(ch, cfg, cfg.UploadQueue, cfg.UploadRoutingKey, cfg.UploadRetryDelaysMS); err != nil {
		return err
	}
	if err := declareRetryQueues(ch, cfg, cfg.DeleteQueue, cfg.DeleteRoutingKey, cfg.DeleteRetryDelaysMS); err != nil {
		return err
	}

	if _, err := ch.QueueDeclare(cfg.UploadDLQQueue, true, false, false, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq upload dlq")
	}
	if err := ch.QueueBind(cfg.UploadDLQQueue, cfg.UploadDLQRoutingKey, cfg.Exchange, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq upload dlq bind")
	}

	if _, err := ch.QueueDeclare(cfg.DeleteDLQQueue, true, false, false, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq delete dlq")
	}
	if err := ch.QueueBind(cfg.DeleteDLQQueue, cfg.DeleteDLQRoutingKey, cfg.Exchange, false, nil); err != nil {
		return apperr.Wrap(err, "rabbitmq delete dlq bind")
	}

	return nil
}

func declareRetryQueues(ch *amqp.Channel, cfg config.RabbitMQ, baseQueue, returnRoutingKey string, delaysMS []int) error {
	for i, ttl := range delaysMS {
		if ttl <= 0 {
			continue
		}
		if ttl > math.MaxInt32 {
			return fmt.Errorf("rabbitmq retry queue %s.retry.%d: ttl exceeds int32 max", baseQueue, i)
		}
		name := fmt.Sprintf("%s.retry.%d", baseQueue, i)
		_, err := ch.QueueDeclare(name, true, false, false, false, amqp.Table{
			"x-dead-letter-exchange":    cfg.Exchange,
			"x-dead-letter-routing-key": returnRoutingKey,
			"x-message-ttl":             int32(ttl),
		})
		if err != nil {
			return apperr.Wrap(err, "rabbitmq retry queue "+name)
		}
	}
	return nil
}
