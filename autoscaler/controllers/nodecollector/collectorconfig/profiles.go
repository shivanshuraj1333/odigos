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
		// Defaults sized for profiles fan-out: a node emits ~5 profile types per
		// workload per tick, so a slow/contended downstream can exceed the OTLP
		// exporter's 5s default timeout, retry-storm, and build a backlog that
		// drags profile timestamps minutes behind live query windows (renders
		// empty). A 30s timeout plus a bounded sending queue absorb bursts and
		// keep delivery fresh. User-supplied profiling.exporter overrides win.
		"timeout":          "30s",
		"sending_queue":    config.GenericMap{"enabled": true, "num_consumers": 4, "queue_size": 512},
		"retry_on_failure": config.GenericMap{"enabled": true, "initial_interval": "2s", "max_interval": "10s", "max_elapsed_time": "120s"},
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
		// sample_size_bytes must be one of 128/256/512 KiB; the receiver rejects 0.
		// Default to 256KiB when unset so the collector never gets an invalid config.
		sampleSize := p.Memory.SampleSizeBytes
		if sampleSize == 0 {
			sampleSize = 262144
		}
		nativeOn := boolOrDefault(p.Memory.Native, false)
		injectOn := boolOrDefault(p.Memory.Inject, false)
		mem := config.GenericMap{
			"enabled": true,
			"inject":  injectOn,
			// sample_size_bytes must be 128/256/512 KiB; report_interval must be >= 1s.
			// The receiver validates (does not default) both, so always emit valid values.
			"sample_size_bytes": sampleSize,
			"report_interval":   "15s",
			"languages": config.GenericMap{
				"go":     boolOrDefault(p.Memory.Go, true),
				"java":   boolOrDefault(p.Memory.Java, true),
				"native": nativeOn,
				"dotnet": boolOrDefault(p.Memory.Dotnet, false),
				"node":   boolOrDefault(p.Memory.Node, false),
			},
		}
		// When native heap profiling is on, native.mode must be non-off (the receiver
		// rejects native language with mode=off). inject => no-restart; else restart.
		if nativeOn {
			mode := "restart"
			if injectOn {
				mode = "inject"
			}
			mem["native"] = config.GenericMap{"mode": mode}
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
