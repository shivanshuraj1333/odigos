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
// Includes all needed receivers, processors, and exporters so the domain is self-contained
// even when nodeCG is nil (no traditional sources instrumented).
func ProfilesConfig(nodeCG *odigosv1.CollectorsGroup, odigosNamespace string) config.Config {
	return config.Config{
		Receivers: config.GenericMap{
			ebpfProfilerReceiverName: config.GenericMap{},
		},
		Processors: config.GenericMap{
			ebpfServiceNameProcessorName: config.GenericMap{
				"attributes": []config.GenericMap{{
					"key":    "service.name",
					"value":  "ebpf-profiler",
					"action": "upsert",
				}},
			},
			nodeNameProcessorName: staticProcessors[nodeNameProcessorName],
			resourceDetectionProcessorName: config.GenericMap{
				"detectors": []string{"ec2", "eks", "azure", "aks"},
				"timeout":   "2s",
			},
		},
		Exporters: config.GenericMap{
			clusterCollectorProfilesExporterName: buildBaseExporterConfig(odigosNamespace, "none"),
		},
		Service: config.Service{
			Pipelines: map[string]config.Pipeline{
				profilesPipelineName: {
					Receivers:  []string{ebpfProfilerReceiverName},
					Processors: []string{ebpfServiceNameProcessorName, nodeNameProcessorName, resourceDetectionProcessorName},
					Exporters:  []string{clusterCollectorProfilesExporterName},
				},
			},
		},
	}
}
