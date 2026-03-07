package odigosurltemplateprocessor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.uber.org/zap"

	"github.com/odigos-io/odigos/collector/processor/odigosurltemplateprocessor/internal/metadata"
	commonapi "github.com/odigos-io/odigos/common/api"
	"github.com/odigos-io/odigos/common/collector"
)

//go:generate mdatagen metadata.yaml

var consumerCapabilities = consumer.Capabilities{MutatesData: true}

// workloadRulesProvider is a local interface that the processor uses to obtain
// per-workload collector config from the workload config extension.
// It is satisfied by *odigosconfigk8sextension.OdigosWorkloadConfig without
// importing that package directly.
// GetFromResource returns the full ContainerCollectorConfig so the processor can
// extract domain-specific fields (e.g. UrlTemplatization) itself.
// GetWorkloadCacheKey returns a stable cache key for the processor's own parsed-rules cache.
type workloadRulesProvider interface {
	GetWorkloadCacheKey(attrs pcommon.Map) (string, error)
	GetFromResource(res pcommon.Resource) (*commonapi.ContainerCollectorConfig, bool)
}

// NewFactory creates a new ProcessorFactory with default configuration
func NewFactory() processor.Factory {
	return processor.NewFactory(
		metadata.Type,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, metadata.TracesStability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	oCfg := cfg.(*Config)
	proc, err := newUrlTemplateProcessor(set, oCfg)
	if err != nil {
		return nil, err
	}

	inner, err := processorhelper.NewTraces(ctx, set, cfg, nextConsumer, proc.processTraces, processorhelper.WithCapabilities(consumerCapabilities))
	if err != nil {
		return nil, err
	}

	if oCfg.WorkloadConfigExtensionID == "" {
		return inner, nil
	}

	return &extensionStartWrapper{
		inner:  inner,
		proc:   proc,
		cfg:    oCfg,
		logger: set.Logger,
	}, nil
}

// extensionStartWrapper wraps a processor.Traces to inject the workload rules extension at Start() time.
// It locates the extension by component type, waits for its cache to sync, and then injects it as
// the processor's workloadRulesProvider.
type extensionStartWrapper struct {
	inner  processor.Traces
	proc   *urlTemplateProcessor
	cfg    *Config
	logger *zap.Logger
}

func (w *extensionStartWrapper) Start(ctx context.Context, host component.Host) error {
	extTypeStr := w.cfg.WorkloadConfigExtensionID
	extType, err := component.NewType(extTypeStr)
	if err != nil {
		w.logger.Error("invalid workload config extension type", zap.String("type", extTypeStr), zap.Error(err))
	} else {
		extensions := host.GetExtensions()
		directID := component.NewID(extType)
		var foundExt component.Component
		// Try direct lookup first (config key is often the type string as ID)
		if ext, ok := extensions[directID]; ok {
			foundExt = ext
			w.tryRegisterWithExtension(ext, directID.String())
		} else {
			for id, ext := range extensions {
				if id.Type() == extType {
					foundExt = ext
					w.tryRegisterWithExtension(ext, id.String())
					break
				}
			}
		}
		if foundExt != nil {
			type cacheSyncer interface {
				WaitForCacheSync(context.Context) bool
			}
			if syncer, ok := foundExt.(cacheSyncer); ok {
				if !syncer.WaitForCacheSync(ctx) {
					w.logger.Warn("workload config extension cache sync did not complete; some spans may be missed on startup")
				}
			}
		}
		if w.proc.provider == nil {
			w.logger.Warn("workload config extension not found; processor will apply heuristics to all spans",
				zap.String("type", extTypeStr))
		}
	}
	return w.inner.Start(ctx, host)
}

func (w *extensionStartWrapper) tryRegisterWithExtension(ext component.Component, extensionID string) {
	oe, ok := ext.(collector.OdigosConfigExtension)
	if !ok {
		w.logger.Warn("extension does not implement OdigosConfigExtension; processor will apply heuristics to all spans",
			zap.String("extension_id", extensionID), zap.String("extGoType", fmt.Sprintf("%T", ext)))
		return
	}
	w.proc.provider = oe
	oe.RegisterConfigCacheCallback(w.proc)
}

func (w *extensionStartWrapper) Shutdown(ctx context.Context) error {
	return w.inner.Shutdown(ctx)
}

func (w *extensionStartWrapper) Capabilities() consumer.Capabilities {
	return w.inner.Capabilities()
}

func (w *extensionStartWrapper) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	return w.inner.ConsumeTraces(ctx, td)
}
