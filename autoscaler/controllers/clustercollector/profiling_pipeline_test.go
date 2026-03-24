package clustercollector

import (
	"testing"

	odigoscommon "github.com/odigos-io/odigos/common"
	"github.com/stretchr/testify/require"
)

func TestShouldBuildGatewayProfilesPipeline_DisabledByDefault(t *testing.T) {
	t.Parallel()
	cfg := &odigoscommon.OdigosConfiguration{}
	require.False(t, shouldBuildGatewayProfilesPipeline(cfg))
}

func TestShouldBuildGatewayProfilesPipeline_EnabledWithUI(t *testing.T) {
	t.Parallel()
	en := true
	cfg := &odigoscommon.OdigosConfiguration{
		Profiling: &odigoscommon.ProfilingConfiguration{
			Enabled:        &en,
			OtlpUiEndpoint: "dns:///ui.ns:4318",
		},
	}
	require.True(t, shouldBuildGatewayProfilesPipeline(cfg))
}

func TestShouldBuildGatewayProfilesPipeline_FileExportOnly(t *testing.T) {
	t.Parallel()
	en := true
	cfg := &odigoscommon.OdigosConfiguration{
		Profiling: &odigoscommon.ProfilingConfiguration{
			Enabled: &en,
			GatewayFileExport: &odigoscommon.ProfilingGatewayFileExport{
				Enabled: true,
				Path:    "/tmp/p.jsonl",
			},
		},
	}
	require.True(t, shouldBuildGatewayProfilesPipeline(cfg))
}

func TestShouldBuildGatewayProfilesPipeline_LegacyEnv(t *testing.T) {
	t.Setenv("PROFILES_OTLP_ENDPOINT_UI", "dns:///ui.x:4318")
	require.True(t, shouldBuildGatewayProfilesPipeline(&odigoscommon.OdigosConfiguration{}))
}

func TestGatewayFileExportPath_Default(t *testing.T) {
	t.Parallel()
	en := true
	cfg := &odigoscommon.OdigosConfiguration{
		Profiling: &odigoscommon.ProfilingConfiguration{
			Enabled: &en,
			GatewayFileExport: &odigoscommon.ProfilingGatewayFileExport{
				Enabled: true,
			},
		},
	}
	ok, p := gatewayFileExportPath(cfg)
	require.True(t, ok)
	require.Equal(t, defaultGatewayProfilesFileExportPath, p)
}
