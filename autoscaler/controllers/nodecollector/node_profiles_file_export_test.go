package nodecollector

import (
	"testing"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/autoscaler/controllers/nodecollector/collectorconfig"
	odigoscommon "github.com/odigos-io/odigos/common"
	"github.com/stretchr/testify/require"
)

func TestMergeNodeProfilesFileExporter_Disabled(t *testing.T) {
	nodeCG := &odigosv1.CollectorsGroup{}
	base := collectorconfig.ProfilesConfig(nodeCG)
	out := mergeNodeProfilesFileExporter(nil, base)
	require.Equal(t, base, out)
}

func TestMergeNodeProfilesFileExporter_Enabled(t *testing.T) {
	nodeCG := &odigosv1.CollectorsGroup{}
	base := collectorconfig.ProfilesConfig(nodeCG)
	enabled := true
	cfg := &odigoscommon.OdigosConfiguration{
		Profiling: &odigoscommon.ProfilingConfiguration{
			Enabled: &enabled,
			NodeFileExport: &odigoscommon.ProfilingNodeFileExport{
				Enabled: true,
				Path:    "/tmp/node-profiles.jsonl",
			},
		},
	}
	out := mergeNodeProfilesFileExporter(cfg, base)
	require.Contains(t, out.Exporters, nodeProfilesFileExporterComponentName)
	p := out.Service.Pipelines["profiles"]
	require.Contains(t, p.Exporters, nodeProfilesFileExporterComponentName)
	require.Contains(t, p.Exporters, "otlp/out-cluster-collector-profiles")
}
