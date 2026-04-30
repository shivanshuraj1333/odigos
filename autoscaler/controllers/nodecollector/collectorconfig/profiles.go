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
		"endpoint":    endpoint,
		"tls":         config.GenericMap{"insecure": true},
		"compression": "none",
	}, profiling.Exporter)

	return config.Config{
		Receivers: config.GenericMap{
			commonconf.ProfilingReceiver: config.GenericMap{},
		},
		Processors: config.GenericMap{
			commonconf.ProfilingNodeFilterProcessor:        commonconf.ProfilingFilterProcessorConfig(),
			commonconf.ProfilingNodeK8sAttributesProcessor: commonconf.K8sAttributesProfilesProcessorConfig(),
		},
		Exporters: config.GenericMap{
			commonconf.ProfilingNodeToGatewayExporter: exp,
		},
		Service: config.Service{
			Pipelines: map[string]config.Pipeline{
				"profiles": {
					Receivers:  []string{commonconf.ProfilingReceiver},
					// Enrich first, then drop: profiles may arrive with k8s.pod.ip (no container.id yet).
					// k8sattributes can resolve container.id from pod metadata; filter then removes non-k8s processes.
					Processors: []string{commonconf.ProfilingNodeK8sAttributesProcessor, commonconf.ProfilingNodeFilterProcessor},
					Exporters:  []string{commonconf.ProfilingNodeToGatewayExporter},
				},
			},
		},
	}
}
