package collectorconfig

import (
	"testing"

	"github.com/odigos-io/odigos/api/k8sconsts"
	commonconf "github.com/odigos-io/odigos/autoscaler/controllers/common"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfilingPipelineConfig_Disabled(t *testing.T) {
	got := ProfilingPipelineConfig("odigos-system", nil)
	assert.Empty(t, got.Receivers)
	assert.Empty(t, got.Processors)
	assert.Empty(t, got.Exporters)
	assert.Empty(t, got.Service.Pipelines)

	off := false
	got = ProfilingPipelineConfig("odigos-system", &common.ProfilingConfiguration{Enabled: &off})
	assert.Empty(t, got.Service.Pipelines)
}

func TestProfilingPipelineConfig_Enabled(t *testing.T) {
	on := true
	got := ProfilingPipelineConfig("odigos-system", &common.ProfilingConfiguration{Enabled: &on})
	require.Contains(t, got.Receivers, commonconf.ProfilingReceiver)
	require.Contains(t, got.Processors, commonconf.ProfilingNodeFilterProcessor)
	require.Contains(t, got.Processors, commonconf.ProfilingNodeK8sAttributesProcessor)
	require.Contains(t, got.Processors, commonconf.ProfilingNodeOdigosProfilesProcessor)
	require.Contains(t, got.Processors, commonconf.ProfilingNodeServiceNameProcessor)
	require.Contains(t, got.Exporters, commonconf.ProfilingNodeToGatewayExporter)

	pl, ok := got.Service.Pipelines["profiles"]
	require.True(t, ok)
	assert.Equal(t, []string{commonconf.ProfilingReceiver}, pl.Receivers)
	// Native symbolization is ON by default when profiling is enabled.
	require.Contains(t, got.Processors, commonconf.ProfilingNodeSymbolizeProcessor)
	assert.Equal(t, []string{
		commonconf.ProfilingNodeFilterProcessor,
		commonconf.ProfilingNodeK8sAttributesProcessor,
		commonconf.ProfilingNodeOdigosProfilesProcessor,
		commonconf.ProfilingNodeSymbolizeProcessor,
		commonconf.ProfilingNodeServiceNameProcessor,
	}, pl.Processors)
	assert.Equal(t, []string{commonconf.ProfilingNodeToGatewayExporter}, pl.Exporters)

	filterCfg, ok := got.Processors[commonconf.ProfilingNodeFilterProcessor].(config.GenericMap)
	require.True(t, ok)
	wantFilter := commonconf.ProfilingFilterProcessorConfig()
	assert.Equal(t, wantFilter, filterCfg)

	odigosProfilesCfg, ok := got.Processors[commonconf.ProfilingNodeOdigosProfilesProcessor].(config.GenericMap)
	require.True(t, ok)
	assert.Equal(t, k8sconsts.OdigosConfigK8sExtensionType, odigosProfilesCfg["odigos_config_extension"])
}

// TestProfilingPipelineConfig_Memory verifies the "memory" block is rendered onto
// the profiling receiver when Profiling.Memory is enabled, with resolved defaults.
func TestProfilingPipelineConfig_Memory(t *testing.T) {
	on := true

	// Memory off (nil) -> receiver config has no "memory" block.
	got := ProfilingPipelineConfig("odigos-system", &common.ProfilingConfiguration{Enabled: &on})
	rc, _ := got.Receivers[commonconf.ProfilingReceiver].(config.GenericMap)
	assert.NotContains(t, rc, "memory")

	// Memory on -> "memory" block with defaults (256KiB, 15s, inuse on, go+java).
	got = ProfilingPipelineConfig("odigos-system", &common.ProfilingConfiguration{
		Enabled: &on,
		Memory:  &common.ProfilingMemoryConfiguration{Enabled: &on},
	})
	rc, ok := got.Receivers[commonconf.ProfilingReceiver].(config.GenericMap)
	require.True(t, ok)
	mem, ok := rc["memory"].(config.GenericMap)
	require.True(t, ok, "memory block must be present")
	assert.Equal(t, true, mem["enabled"])
	assert.Equal(t, 262144, mem["sample_size_bytes"])
	assert.Equal(t, "15s", mem["report_interval"])
	assert.Equal(t, true, mem["inuse_tracking"])
	langs := mem["languages"].(config.GenericMap)
	assert.Equal(t, true, langs["go"])
	assert.Equal(t, true, langs["java"])
	assert.Equal(t, false, langs["native"])

	// Native memory on -> symbolize processor is forced even if not separately enabled.
	off := false
	got = ProfilingPipelineConfig("odigos-system", &common.ProfilingConfiguration{
		Enabled:       &on,
		Symbolization: &common.ProfilingSymbolizationConfiguration{Native: &off}, // user opted out of CPU-native symbolization
		Memory: &common.ProfilingMemoryConfiguration{
			Enabled:   &on,
			Languages: &common.ProfilingMemoryLanguages{Native: &on},
		},
	})
	require.Contains(t, got.Processors, commonconf.ProfilingNodeSymbolizeProcessor,
		"native memory frames require central symbolization regardless of the CPU symbolization opt-out")
}

// TestProfilingPipelineConfig_NativeSymbolizationDisabled drops the symbolize
// processor when a user explicitly opts out (profiling.symbolization.native: false).
func TestProfilingPipelineConfig_NativeSymbolizationDisabled(t *testing.T) {
	on, off := true, false
	got := ProfilingPipelineConfig("odigos-system", &common.ProfilingConfiguration{
		Enabled:       &on,
		Symbolization: &common.ProfilingSymbolizationConfiguration{Native: &off},
	})
	require.NotContains(t, got.Processors, commonconf.ProfilingNodeSymbolizeProcessor)

	pl := got.Service.Pipelines["profiles"]
	assert.Equal(t, []string{
		commonconf.ProfilingNodeFilterProcessor,
		commonconf.ProfilingNodeK8sAttributesProcessor,
		commonconf.ProfilingNodeOdigosProfilesProcessor,
		commonconf.ProfilingNodeServiceNameProcessor,
	}, pl.Processors)
}
