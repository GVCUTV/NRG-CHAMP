// v1
// kafkacb.go
package circuitbreaker

import "context"

// Producer is a minimal Kafka-like producer interface your implementation should satisfy.
type Producer interface {
	Send(ctx context.Context, topic string, key, value []byte) error
}

// CBProducer wraps a Producer with a circuit breaker, exposing the same API.
type CBProducer struct {
	inner Producer
	brk   *Breaker
}

// NewCBProducer builds a circuit-breaker-protected producer.
func NewCBProducer(name string, cfg Config, inner Producer, probe func(ctx context.Context) error) *CBProducer {
	return &CBProducer{inner: inner, brk: New(name, cfg, probe)}
}

// Send publishes a message via the inner producer guarded by the breaker.
func (p *CBProducer) Send(ctx context.Context, topic string, key, value []byte) error {
	return p.brk.Execute(ctx, func(ctx context.Context) error { return p.inner.Send(ctx, topic, key, value) })
}
