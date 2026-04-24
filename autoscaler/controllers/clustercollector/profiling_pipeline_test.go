package clustercollector

import (
	"testing"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	commonconf "github.com/odigos-io/odigos/autoscaler/controllers/common"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddProfilingGatewayPipeline_Disabled(t *testing.T) {
	var c config.Config
	err := addProfilingGatewayPipeline(&c, "odigos-system", nil, nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, c.Processors)

	off := false
	err = addProfilingGatewayPipeline(&c, "odigos-system", &common.ProfilingConfiguration{Enabled: &off}, nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, c.Processors)
}

// basic (default): no memory_limiter on the gateway profiles pipeline
func TestAddProfilingGatewayPipeline_BasicMode(t *testing.T) {
	on := true
	var c config.Config
	gw := &odigosv1.CollectorsGroup{
		Spec: odigosv1.CollectorsGroupSpec{
			ResourcesSettings: odigosv1.CollectorsGroupResourcesSettings{
				MemoryLimiterLimitMiB:      400,
				MemoryLimiterSpikeLimitMiB: 80,
			},
		},
	}
	prof := &common.ProfilingConfiguration{
		Enabled:           &on,
		PipelineStability: common.ProfilingPipelineStabilityBasic,
	}
	err := addProfilingGatewayPipeline(&c, "odigos-system", prof, gw, nil)
	require.NoError(t, err)

	pl := c.Service.Pipelines["profiles"]
	assert.Equal(t, []string{"otlp"}, pl.Receivers)
	assert.Empty(t, pl.Processors, "basic mode must have no processors on the gateway profiles pipeline")
	assert.Equal(t, []string{commonconf.ProfilingGatewayToUIExporter}, pl.Exporters)
}

// advance mode: memory_limiter is added to the gateway profiles pipeline
func TestAddProfilingGatewayPipeline_AdvanceMode(t *testing.T) {
	on := true
	var c config.Config
	gw := &odigosv1.CollectorsGroup{
		Spec: odigosv1.CollectorsGroupSpec{
			ResourcesSettings: odigosv1.CollectorsGroupResourcesSettings{
				MemoryLimiterLimitMiB:      400,
				MemoryLimiterSpikeLimitMiB: 80,
			},
		},
	}
	prof := &common.ProfilingConfiguration{
		Enabled:           &on,
		PipelineStability: common.ProfilingPipelineStabilityAdvance,
	}
	err := addProfilingGatewayPipeline(&c, "odigos-system", prof, gw, nil)
	require.NoError(t, err)

	require.NotNil(t, c.Processors)
	assert.Contains(t, c.Processors, "memory_limiter")

	pl := c.Service.Pipelines["profiles"]
	assert.Equal(t, []string{"otlp"}, pl.Receivers)
	assert.Equal(t, []string{"memory_limiter"}, pl.Processors)
	assert.Equal(t, []string{commonconf.ProfilingGatewayToUIExporter}, pl.Exporters)
}

func TestAddProfilingGatewayPipeline_GenericOtlpProfilesDestination(t *testing.T) {
	on := true
	c := config.Config{
		Exporters: config.GenericMap{},
		Service: config.Service{
			Pipelines: map[string]config.Pipeline{},
		},
	}
	c.Exporters["otlp/generic-pyroscope"] = config.GenericMap{"endpoint": "dns:///pyroscope:4040"}
	dests := &odigosv1.DestinationList{Items: []odigosv1.Destination{{
		ObjectMeta: metav1.ObjectMeta{Name: "pyroscope"},
		Spec: odigosv1.DestinationSpec{
			Type:    common.GenericOTLPDestinationType,
			Signals: []common.ObservabilitySignal{common.ProfilesObservabilitySignal},
		},
	}}}
	err := addProfilingGatewayPipeline(&c, "odigos-system", &common.ProfilingConfiguration{Enabled: &on}, nil, dests)
	require.NoError(t, err)
	pl := c.Service.Pipelines["profiles"]
	assert.Equal(t, []string{commonconf.ProfilingGatewayToUIExporter, "otlp/generic-pyroscope"}, pl.Exporters)
}

func TestAddProfilingGatewayPipeline_UiOtlpEndpointOverride(t *testing.T) {
	on := true
	var c config.Config
	gw := &odigosv1.CollectorsGroup{
		Spec: odigosv1.CollectorsGroupSpec{
			ResourcesSettings: odigosv1.CollectorsGroupResourcesSettings{
				MemoryLimiterLimitMiB:      400,
				MemoryLimiterSpikeLimitMiB: 80,
			},
		},
	}
	prof := &common.ProfilingConfiguration{
		Enabled:               &on,
		UiOtlpEndpoint: "10.0.1.50:4317",
	}
	err := addProfilingGatewayPipeline(&c, "odigos-system", prof, gw, nil)
	require.NoError(t, err)
	raw := c.Exporters[commonconf.ProfilingGatewayToUIExporter]
	require.NotNil(t, raw)
	exp, ok := raw.(config.GenericMap)
	require.True(t, ok)
	ep, ok := exp["endpoint"].(string)
	require.True(t, ok, "endpoint should be a string")
	assert.Equal(t, "10.0.1.50:4317", ep)
}
