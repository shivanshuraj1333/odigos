package odigosurltemplateprocessor

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"

	"github.com/odigos-io/odigos/collector/extension/odigosworkloadconfigextension"
	"github.com/odigos-io/odigos/collector/processor/odigosurltemplateprocessor/internal/metadata"
)

//go:generate mdatagen metadata.yaml

var consumerCapabilities = consumer.Capabilities{MutatesData: true}

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

// extensionStartWrapper runs at Start to inject the workload config extension into the processor.
type extensionStartWrapper struct {
	inner *urlTemplateProcessor
	extID string
}

func (w *extensionStartWrapper) Start(ctx context.Context, host component.Host) error {
	if w.extID == "" {
		return nil
	}
	ext, ok := host.GetExtensions()[component.MustNewID(w.extID)]
	if !ok || ext == nil {
		w.inner.warnExtensionNotFound(w.extID)
		return nil
	}
	wc, ok := ext.(*odigosworkloadconfigextension.OdigosWorkloadConfig)
	if !ok {
		w.inner.warnExtensionWrongType(w.extID)
		return nil
	}
	w.inner.workloadRulesProvider = wc
	return nil
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	oCfg := cfg.(*Config)
	tmp, err := newUrlTemplateProcessor(set, oCfg)
	if err != nil {
		return nil, err
	}

	opts := []processorhelper.Option{processorhelper.WithCapabilities(consumerCapabilities)}
	if oCfg.WorkloadConfigExtensionID != "" {
		wrapper := &extensionStartWrapper{inner: tmp, extID: oCfg.WorkloadConfigExtensionID}
		opts = append(opts, processorhelper.WithStart(wrapper.Start))
	}
	return processorhelper.NewTraces(ctx, set, cfg, nextConsumer, tmp.processTraces, opts...)
}
