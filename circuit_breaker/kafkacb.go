// v0
// kafkacb.go
package circuitbreaker

import "context"

// Producer is a minimal Kafka-like producer interface.
type Producer interface {
	Send(ctx context.Context, topic string, key, value []byte) error
}

type CBProducer struct {
	inner Producer
	brk   *Breaker
}

func NewCBProducer(name string, cfg Config, inner Producer, probe func(ctx context.Context) error) *CBProducer {
	return &CBProducer{inner: inner, brk: New(name, cfg, probe)}
}

func (p *CBProducer) Send(ctx context.Context, topic string, key, value []byte) error {
	return p.brk.Execute(ctx, func(ctx context.Context) error {
		return p.inner.Send(ctx, topic, key, value)
	})
}
