package rabbit

import amqp "github.com/rabbitmq/amqp091-go"

type amqpHeaderCarrier struct {
	headers amqp.Table
}

func (c *amqpHeaderCarrier) Get(key string) string {
	if val, ok := c.headers[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func (c *amqpHeaderCarrier) Set(key, val string) {
	c.headers[key] = val
}

func (c *amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}
