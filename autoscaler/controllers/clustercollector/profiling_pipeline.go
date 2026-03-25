package clustercollector

import (
	"fmt"

	commonconf "github.com/odigos-io/odigos/autoscaler/controllers/common"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
)

const (
	k8sAttributesProfilesGatewayProcessor = "k8sattributes/profiles-gateway"
	otlpProfilesToUIExporterName          = "otlp/profiles-to-ui"
)

func addProfilingGatewayPipeline(c *config.Config, odigosNs string, profiling *common.ProfilingConfiguration, uiOtlpPort int) error {
	if profiling == nil || profiling.Enabled == nil || !*profiling.Enabled {
		return nil
	}
	if c.Processors == nil {
		c.Processors = config.GenericMap{}
	}
	if c.Exporters == nil {
		c.Exporters = config.GenericMap{}
	}
	if c.Service.Pipelines == nil {
		c.Service.Pipelines = map[string]config.Pipeline{}
	}

	endpoint := fmt.Sprintf("ui.%s:%d", odigosNs, uiOtlpPort)

	exp := commonconf.MergeProfilingOtlpExporter(config.GenericMap{
		"endpoint":    endpoint,
		"tls":         config.GenericMap{"insecure": true},
		"compression": "none",
	}, profiling.Exporter)

	c.Processors[k8sAttributesProfilesGatewayProcessor] = commonconf.K8sAttributesProfilesProcessorConfig()
	c.Exporters[otlpProfilesToUIExporterName] = exp

	c.Service.Pipelines["profiles"] = config.Pipeline{
		Receivers:  []string{"otlp"},
		Processors: []string{k8sAttributesProfilesGatewayProcessor},
		Exporters:  []string{otlpProfilesToUIExporterName},
	}
	return nil
}
