package nodecollector

import (
	odigoscommon "github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
)

const (
	defaultNodeProfilesFileExportPath     = "/var/odigos/node-profiles-export/profiles.jsonl"
	nodeProfilesFileExporterComponentName = "file/node-profiles"
	profilesPipelineNameForFileExport     = "profiles"
)

// nodeFileExportPath returns whether node (data-collection) profile file export is enabled and the output path.
func nodeFileExportPath(odigosCfg *odigoscommon.OdigosConfiguration) (enabled bool, path string) {
	if odigosCfg == nil || odigosCfg.Profiling == nil || odigosCfg.Profiling.NodeFileExport == nil {
		return false, ""
	}
	n := odigosCfg.Profiling.NodeFileExport
	if !n.Enabled {
		return false, ""
	}
	if n.Path != "" {
		return true, n.Path
	}
	return true, defaultNodeProfilesFileExportPath
}

// mergeNodeProfilesFileExporter appends the file exporter to the profiles pipeline and defines the exporter when enabled in OdigosConfiguration.
func mergeNodeProfilesFileExporter(odigosCfg *odigoscommon.OdigosConfiguration, base config.Config) config.Config {
	on, path := nodeFileExportPath(odigosCfg)
	if !on {
		return base
	}
	if base.Exporters == nil {
		base.Exporters = config.GenericMap{}
	}
	base.Exporters[nodeProfilesFileExporterComponentName] = config.GenericMap{
		"path": path,
	}
	if base.Service.Pipelines == nil {
		return base
	}
	p, ok := base.Service.Pipelines[profilesPipelineNameForFileExport]
	if !ok {
		return base
	}
	for _, e := range p.Exporters {
		if e == nodeProfilesFileExporterComponentName {
			return base
		}
	}
	p.Exporters = append(p.Exporters, nodeProfilesFileExporterComponentName)
	base.Service.Pipelines[profilesPipelineNameForFileExport] = p
	return base
}
