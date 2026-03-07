package collectorconfig

import (
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common/config"
)

const (
	profilesPipelineName = "profiles"
)

// ProfilesConfig returns the collector config for the profiles pipeline (eBPF profiler receiver).
// Used only by the node collector; profiles are exported to the cluster collector.
// Receivers, exporters, and processors are merged from common config domains.
func ProfilesConfig(nodeCG *odigosv1.CollectorsGroup, _ string) config.Config {
	return config.Config{
		Service: config.Service{
			Pipelines: map[string]config.Pipeline{
				profilesPipelineName: {
					Receivers:  []string{ebpfProfilerReceiverName},
					Processors: []string{batchProcessorName, memoryLimiterProcessorName, nodeNameProcessorName, resourceDetectionProcessorName},
					Exporters:  []string{clusterCollectorProfilesExporterName},
				},
			},
		},
	}
}
