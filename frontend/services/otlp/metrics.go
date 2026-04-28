package otlp

import (
	"context"

	xconsumer "go.opentelemetry.io/collector/consumer"
	xreceiver "go.opentelemetry.io/collector/receiver"
)

type Metrics struct {
	otlpReceiver *Receiver
	receiver     xreceiver.Metrics
	consumer     xconsumer.Metrics
}

var _ OTLPPipeline = (*Metrics)(nil)

func NewMetricsPipeline(r *Receiver, c xconsumer.Metrics) *Metrics {
	return &Metrics{otlpReceiver: r, consumer: c}
}

func (m *Metrics) Register(ctx context.Context) error {
	metricsReceiver, err := m.otlpReceiver.Factory.CreateMetrics(ctx, m.otlpReceiver.Settings, m.otlpReceiver.Cfg, m.consumer)
	if err != nil {
		return err
	}
	m.receiver = metricsReceiver
	return nil
}

func (m *Metrics) Start(ctx context.Context) error {
	return m.receiver.Start(ctx, nil)
}

func (m *Metrics) Shutdown(ctx context.Context) error {
	return m.receiver.Shutdown(ctx)
}
