package otelpkg

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPCarrier adapts [amqp.Table] for W3C trace context inject/extract.
type AMQPCarrier amqp.Table

func (c AMQPCarrier) Get(key string) string {
	if c == nil {
		return ""
	}
	v, ok := c[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func (c AMQPCarrier) Set(key, value string) {
	if c == nil {
		return
	}
	c[key] = value
}

func (c AMQPCarrier) Keys() []string {
	if c == nil {
		return nil
	}
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// HeadersCarrier returns a carrier backed by msg headers, creating the table when nil.
func HeadersCarrier(headers amqp.Table) AMQPCarrier {
	if headers == nil {
		headers = amqp.Table{}
	}
	return AMQPCarrier(headers)
}
