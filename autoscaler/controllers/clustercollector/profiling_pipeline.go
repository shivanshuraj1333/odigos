package clustercollector

import (
	"github.com/odigos-io/odigos/api/k8sconsts"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	commonconf "github.com/odigos-io/odigos/autoscaler/controllers/common"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
)

func addProfilingGatewayPipeline(c *config.Config, odigosNs string, profiling *common.ProfilingConfiguration, enabledDests *odigosv1.DestinationList) error {
	if !common.ProfilingPipelineActive(profiling) {
		return nil
	}
	// Filter + k8s_attributes run on the node collector only; OTLP to the gateway already carries
	// enriched resource attributes. The gateway profiles pipeline is receive → export.
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
	}, profiling.Exporter)

	c.Exporters[commonconf.ProfilingGatewayToUIExporter] = exp

	// Fan-out to enabled external Generic OTLP destinations that declare the PROFILES signal.
	// Their exporters are registered separately by the destination configurer; we only add them
	// to this pipeline's Exporters list when present in c.Exporters.
	exportNames := []string{commonconf.ProfilingGatewayToUIExporter}
	for _, ex := range profileOtlpExporterNames(enabledDests) {
		if _, ok := c.Exporters[ex]; ok {
			exportNames = append(exportNames, ex)
		}
	}

	c.Service.Pipelines["profiles"] = config.Pipeline{
		Receivers:  []string{"otlp"},
		Processors: nil,
		Exporters:  exportNames,
	}
	return nil
}
