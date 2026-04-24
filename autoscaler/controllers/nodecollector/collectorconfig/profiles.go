package collectorconfig

import (
	"github.com/odigos-io/odigos/api/k8sconsts"
	commonconf "github.com/odigos-io/odigos/autoscaler/controllers/common"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
	odigosconsts "github.com/odigos-io/odigos/common/consts"
)

// ProfilingPipelineConfig builds the node collector profiles domain when profiling is enabled.
func ProfilingPipelineConfig(odigosNamespace string, profiling *common.ProfilingConfiguration) config.Config {
	if !common.ProfilingPipelineActive(profiling) {
		return config.Config{}
	}

	endpoint := k8sconsts.OtlpGrpcDNSEndpoint(k8sconsts.OdigosClusterCollectorServiceName, odigosNamespace, odigosconsts.OTLPPort)
	exp := commonconf.MergeProfilingOtlpExporter(config.GenericMap{
		"endpoint":      endpoint,
		"tls":           config.GenericMap{"insecure": true},
		"compression":   "none",
		"balancer_name": "round_robin",
	}, profiling.Exporter, profiling.PipelineStability)

	return config.Config{
		Receivers: config.GenericMap{
			commonconf.ProfilingReceiver: config.GenericMap{},
		},
		Processors: config.GenericMap{
			commonconf.ProfilingNodeK8sAttributesProcessor:        commonconf.K8sAttributesProfilesProcessorConfig(),
			commonconf.ProfilingNodeServiceNameTransformProcessor: commonconf.ProfilingServiceNameTransformProcessorConfig(),
			commonconf.ProfilingNodeFilterProcessor:               commonconf.ProfilingFilterProcessorConfig(),
		},
		Exporters: config.GenericMap{
			commonconf.ProfilingNodeToGatewayExporter: exp,
		},
		Service: config.Service{
			Pipelines: map[string]config.Pipeline{
				"profiles": {
					Receivers: []string{commonconf.ProfilingReceiver},
					// memory_limiter is merged from common_application_telemetry.
					// k8s_attributes → transform(service.name) → filter: enrich K8s metadata first, then set
					// service.name for Pyroscope/destinations when the agent leaves it unset, then drop
					// host noise. The ebpf receiver may not set container.id on every profile
					Processors: []string{
						commonconf.ProfilingNodeK8sAttributesProcessor,
						commonconf.ProfilingNodeServiceNameTransformProcessor,
						commonconf.ProfilingNodeFilterProcessor,
						memoryLimiterProcessorName,
					},
					Exporters: []string{commonconf.ProfilingNodeToGatewayExporter},
				},
			},
		},
	}
}
