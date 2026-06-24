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

	processors := config.GenericMap{
		commonconf.ProfilingNodeFilterProcessor:         commonconf.ProfilingFilterProcessorConfig(),
		commonconf.ProfilingNodeK8sAttributesProcessor:  commonconf.K8sAttributesProfilesProcessorConfig(),
		commonconf.ProfilingNodeOdigosProfilesProcessor: commonconf.OdigosProfilesProcessorConfig(),
		commonconf.ProfilingNodeServiceNameProcessor:    commonconf.ProfilingServiceNameTransformConfig(),
	}
	pipelineProcessors := []string{
		commonconf.ProfilingNodeFilterProcessor,
		commonconf.ProfilingNodeK8sAttributesProcessor,
		commonconf.ProfilingNodeOdigosProfilesProcessor,
	}
	// Native symbolization is opt-in (profiling.symbolization.native). When on, the
	// symbolize processor runs after the keep-filter (only retained profiles are
	// symbolized) and before service-name enrichment.
	if profiling.NativeSymbolizationEnabled() {
		processors[commonconf.ProfilingNodeSymbolizeProcessor] = commonconf.OdigosSymbolizeProcessorConfig()
		pipelineProcessors = append(pipelineProcessors, commonconf.ProfilingNodeSymbolizeProcessor)
	}
	pipelineProcessors = append(pipelineProcessors, commonconf.ProfilingNodeServiceNameProcessor)

	return config.Config{
		Receivers: config.GenericMap{
			commonconf.ProfilingReceiver: profilingReceiverConfig(profiling),
		},
		Processors: processors,
		Exporters: config.GenericMap{
			commonconf.ProfilingNodeToGatewayExporter: exp,
		},
		Service: config.Service{
			Pipelines: map[string]config.Pipeline{
				"profiles": {
					Receivers:  []string{commonconf.ProfilingReceiver},
					Processors: pipelineProcessors,
					Exporters:  []string{commonconf.ProfilingNodeToGatewayExporter},
				},
			},
		},
	}
}

// profilingReceiverConfig builds the "profiling" receiver config. CPU profiling is
// on by the receiver's own defaults; when memory profiling is enabled we add the
// memory block (mapped to the ebpf-profiler receiver's collector/config.MemoryConfig).
func profilingReceiverConfig(p *common.ProfilingConfiguration) config.GenericMap {
	cfg := config.GenericMap{}
	if p.MemoryEnabled() {
		mem := config.GenericMap{
			"enabled": true,
			"inject":  boolOrDefault(p.Memory.Inject, false),
			"languages": config.GenericMap{
				"go":     boolOrDefault(p.Memory.Go, true),
				"java":   boolOrDefault(p.Memory.Java, true),
				"native": boolOrDefault(p.Memory.Native, false),
			},
		}
		if p.Memory.SampleSizeBytes > 0 {
			mem["sample_size_bytes"] = p.Memory.SampleSizeBytes
		}
		cfg["memory"] = mem
	}
	return cfg
}

func boolOrDefault(b *bool, def bool) bool {
	if b != nil {
		return *b
	}
	return def
}
