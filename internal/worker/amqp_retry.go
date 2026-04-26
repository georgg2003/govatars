package worker

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

func retryCountFromHeaders(h amqp.Table) int {
	if h == nil {
		return 0
	}
	v, ok := h["x-retry-count"]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}
