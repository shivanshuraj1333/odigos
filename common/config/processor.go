package config

import (
	"fmt"
)

// TraceSpanMetricsOrderSplit separates trace processors on the data-collection collector:
// hints strictly below run in the main "traces" pipeline before exporters (including the spanmetrics connector);
// hints at or above run in the optional "traces/exporting" pipeline after span metrics (see nodecollector TracesConfig).
const TraceSpanMetricsOrderSplit = 10

type CrdProcessorResults struct {
	ProcessorsConfig                Config
	TracesProcessors                []string
	TracesProcessorsPostSpanMetrics []string
	MetricsProcessors               []string
	LogsProcessors                  []string
	Errs                            map[string]error
}

func CrdProcessorToConfig(processors []ProcessorConfigurer) CrdProcessorResults {
	results := CrdProcessorResults{
		ProcessorsConfig: Config{
			Processors: GenericMap{},
		},
		TracesProcessorsPostSpanMetrics: []string{},
		MetricsProcessors:               []string{},
		LogsProcessors:                  []string{},
		Errs:                            make(map[string]error),
	}

	for _, processor := range processors {
		processorKey := fmt.Sprintf("%s/%s", processor.GetType(), processor.GetID())
		processorsConfig, err := processor.GetConfig()
		if err != nil {
			// TODO: write the error to the status of the processor
			// consider how to handle this error
			results.Errs[processor.GetID()] = fmt.Errorf("failed to convert processor %q to collector config: %w", processor.GetID(), err)
			continue
		}
		if processorKey == "" || processorsConfig == nil {
			continue
		}
		results.ProcessorsConfig.Processors[processorKey] = processorsConfig

		if isTracingEnabled(processor) {
			if processor.GetOrderHint() < TraceSpanMetricsOrderSplit {
				results.TracesProcessors = append(results.TracesProcessors, processorKey)
			} else {
				// After spanmetrics: optional second pipeline for processors that must see spans post-connector.
				results.TracesProcessorsPostSpanMetrics = append(results.TracesProcessorsPostSpanMetrics, processorKey)
			}
		}
		if isMetricsEnabled(processor) {
			results.MetricsProcessors = append(results.MetricsProcessors, processorKey)
		}
		if isLoggingEnabled(processor) {
			results.LogsProcessors = append(results.LogsProcessors, processorKey)
		}
	}
	if len(results.Errs) != 0 {
		return results
	}

	return results
}
