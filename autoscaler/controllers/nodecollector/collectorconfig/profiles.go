package collectorconfig

import (
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common/config"
)

const (
	profilesPipelineName = "profiles"
	// k8sattributesProfilesProcessorName is a dedicated instance for the profiles pipeline (pod association for eBPF/OTLP profiles).
	k8sattributesProfilesProcessorName = "k8sattributes/profiles"
)

// ProfilesConfig returns the config domain for the profiles pipeline: OTLP in -> batch, memory_limiter, nodeName, resourcedetection, k8sattributes -> cluster collector.
// Profiles are received via OTLP (e.g. from an external eBPF profiler). k8sattributes enriches with pod/namespace/deployment for the current node.
// Default pod_association is tuned for profile data that contains only PID and container.id (e.g. opentelemetry-ebpf-profiler):
// container.id first, then connection, then k8s.pod.uid/k8s.pod.ip so enrichment works without overrides.
// Other processors (batch, memory_limiter, nodeName, resourceDetection) come from the common_application_telemetry domain.
func ProfilesConfig(nodeCG *odigosv1.CollectorsGroup) config.Config {
	processors := config.GenericMap{
		k8sattributesProfilesProcessorName: config.GenericMap{
			"auth_type":   "serviceAccount",
			"passthrough": false,
			"pod_association": []config.GenericMap{
				{
					"sources": []config.GenericMap{
						{"from": "resource_attribute", "name": "container.id"},
						{"from": "connection"},
						{"from": "resource_attribute", "name": "k8s.pod.uid"},
						{"from": "resource_attribute", "name": "k8s.pod.ip"},
					},
				},
			},
			"extract": config.GenericMap{
				"metadata": []string{
					"k8s.pod.name",
					"k8s.namespace.name",
					"k8s.deployment.name",
					"k8s.statefulset.name",
					"k8s.daemonset.name",
					"k8s.node.name",
				},
			},
			"filter": config.GenericMap{
				"node_from_env_var": "NODE_NAME",
			},
		},
	}

	return config.Config{
		Processors: processors,
		Service: config.Service{
			Pipelines: map[string]config.Pipeline{
				profilesPipelineName: {
					Receivers:  []string{OTLPInReceiverName},
					Processors: []string{batchProcessorName, memoryLimiterProcessorName, nodeNameProcessorName, resourceDetectionProcessorName, k8sattributesProfilesProcessorName},
					Exporters:  []string{clusterCollectorProfilesExporterName},
				},
			},
		},
	}
}
