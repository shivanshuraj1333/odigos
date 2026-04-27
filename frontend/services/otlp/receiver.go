package otlp

import (
	"context"
	"fmt"
	"time"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/pcommon"
	recv "go.opentelemetry.io/collector/receiver"
	otlprecvfactory "go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/receiver/xreceiver"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

// uiOTLPHost is a minimal component.Host for the UI-embedded OTLP receiver.
type uiOTLPHost struct{}

func (uiOTLPHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{}
}

type Receiver struct {
	Factory  recv.Factory
	Cfg      *otlprecvfactory.Config
	Host     component.Host
	Settings recv.Settings
	Port     int

	metricsReceiver  recv.Metrics
	profilesReceiver xreceiver.Profiles
}

func NewReceiver(port int) (*Receiver, error) {
	f := otlprecvfactory.NewFactory()

	// Derive the default OTLP gRPC port config
	cfg, ok := f.CreateDefaultConfig().(*otlprecvfactory.Config)

	if !ok {
		return nil, fmt.Errorf("otlp: default config is not *otlpreceiver.Config")
	}

	grpcCfg := configgrpc.NewDefaultServerConfig()
	grpcCfg.NetAddr = confignet.AddrConfig{
		Endpoint:  fmt.Sprintf("0.0.0.0:%d", port),
		Transport: confignet.TransportTypeTCP,
	}

	// we only open gRPC port on 4317 and no http port
	cfg.GRPC = configoptional.Some(grpcCfg)
	cfg.HTTP = configoptional.None[otlprecvfactory.HTTPConfig]()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("otlp: validate receiver config: %w", err)
	}

	zapLogger := commonlogger.Logger()
	if zapLogger == nil {
		zapLogger = zap.NewNop()
	}

	return &Receiver{
		Factory: f,
		Cfg:     cfg,
		Host:    uiOTLPHost{},
		Settings: recv.Settings{
			ID: component.NewIDWithName(f.Type(), "odigos-ui"),
			TelemetrySettings: component.TelemetrySettings{
				Logger:         zapLogger.Named("otlp-receiver"),
				TracerProvider: nooptrace.NewTracerProvider(),
				MeterProvider:  noopmetric.NewMeterProvider(),
				Resource:       pcommon.NewResource(),
			},
			BuildInfo: component.NewDefaultBuildInfo(),
		},
		Port: port,
	}, nil
}

// Setup registers every consumer, then starts each.
func (r *Receiver) Setup(ctx context.Context, consumers ...Consumer) error {
	for _, c := range consumers {
		if c == nil {
			continue
		}
		if err := c.Register(ctx, r); err != nil {
			return fmt.Errorf("otlp: register: %w", err)
		}
	}
	for _, c := range consumers {
		if c == nil {
			continue
		}
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("otlp: start: %w", err)
		}
	}
	return nil
}

func (r *Receiver) WaitAndShutdown(ctx context.Context, consumers ...Consumer) error {
	commonlogger.LoggerCompat().With("subsystem", "ui-otlp", "component", "receiver").Info("OTLP gRPC running",
		"endpoint", fmt.Sprintf("0.0.0.0:%d", r.Port),
		"metrics", r.metricsReceiver != nil,
		"profiles", r.profilesReceiver != nil,
	)
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for i := len(consumers) - 1; i >= 0; i-- {
		c := consumers[i]
		if c == nil {
			continue
		}
		_ = c.Shutdown(shutCtx)
	}
	return nil
}
