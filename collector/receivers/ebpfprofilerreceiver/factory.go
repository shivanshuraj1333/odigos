package ebpfprofilerreceiver

import (
	"go.opentelemetry.io/collector/receiver"

	"go.opentelemetry.io/ebpf-profiler/collector"
)

// NewFactory returns the OpenTelemetry eBPF profiler receiver factory.
// It runs only on Linux (node collector); on other OSes the factory returns an error from CreateProfiles.
func NewFactory() receiver.Factory {
	return collector.NewFactory()
}
