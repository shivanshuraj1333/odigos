package clustercollector

import (
	"github.com/odigos-io/odigos/api/k8sconsts"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	commonconf "github.com/odigos-io/odigos/autoscaler/controllers/common"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
)

func addProfilingGatewayPipeline(c *config.Config, odigosNs string, profiling *common.ProfilingConfiguration, gateway *odigosv1.CollectorsGroup, enabledDests *odigosv1.DestinationList) error {
	if !common.ProfilingPipelineActive(profiling) {
		return nil
	}
	var rs odigosv1.CollectorsGroupResourcesSettings
	if gateway != nil {
		rs = gateway.Spec.ResourcesSettings
	}
	// k8s_attributes, service.name transform, and filter run on the node collector only,
	// OTLP to the gateway already carries enriched resource attributes.
	// The gateway profiles pipeline is receive → limit → export.
	if c.Exporters == nil {
		c.Exporters = config.GenericMap{}
	}
	if c.Service.Pipelines == nil {
		c.Service.Pipelines = map[string]config.Pipeline{}
	}

	endpoint := k8sconsts.UiOtlpGrpcEndpoint(odigosNs)
	if profiling != nil && profiling.UiOtlpEndpoint != "" {
		endpoint = profiling.UiOtlpEndpoint
	}

	exp := commonconf.MergeProfilingOtlpExporter(config.GenericMap{
		"endpoint":    endpoint,
		"tls":         config.GenericMap{"insecure": true},
		"compression": "none",
	}, profiling.Exporter, profiling.PipelineStability)

	c.Exporters[commonconf.ProfilingGatewayToUIExporter] = exp

	exportNames := []string{commonconf.ProfilingGatewayToUIExporter}
	for _, ex := range profileOtlpExporterNames(enabledDests) {
		if _, ok := c.Exporters[ex]; ok {
			exportNames = append(exportNames, ex)
		}
	}

	// advance mode: add memory_limiter to the gateway profiles pipeline.
	// basic mode: receive → export only, no extra processor overhead on the gateway.
	var gatewayProcessors []string
	if profiling.PipelineStability == common.ProfilingPipelineStabilityAdvance {
		if c.Processors == nil {
			c.Processors = config.GenericMap{}
		}
		if _, ok := c.Processors["memory_limiter"]; !ok {
			c.Processors["memory_limiter"] = commonconf.GetMemoryLimiterConfig(rs)
		}
		gatewayProcessors = []string{"memory_limiter"}
	}

	c.Service.Pipelines["profiles"] = config.Pipeline{
		Receivers:  []string{"otlp"},
		Processors: gatewayProcessors,
		Exporters:  exportNames,
	}
	return nil
}
