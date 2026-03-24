package clustercollector

import (
	"os"
	"strings"

	odigoscommon "github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
)

const defaultGatewayProfilesFileExportPath = "/var/odigos/profiles-export/profiles.jsonl"

// resolveOtlpUiEndpoint prefers OdigosConfiguration; falls back to legacy autoscaler env for migration.
func resolveOtlpUiEndpoint(odigosCfg *odigoscommon.OdigosConfiguration) string {
	if odigosCfg != nil && odigosCfg.Profiling != nil && odigosCfg.Profiling.OtlpUiEndpoint != "" {
		return odigosCfg.Profiling.OtlpUiEndpoint
	}
	return os.Getenv("PROFILES_OTLP_ENDPOINT_UI")
}

func resolveVerificationEndpoint(odigosCfg *odigoscommon.OdigosConfiguration) string {
	if odigosCfg != nil && odigosCfg.Profiling != nil && odigosCfg.Profiling.VerificationEndpoint != "" {
		return odigosCfg.Profiling.VerificationEndpoint
	}
	return os.Getenv("PROFILE_VERIFICATION_OTLP_ENDPOINT")
}

func gatewayFileExportPath(odigosCfg *odigoscommon.OdigosConfiguration) (enabled bool, path string) {
	if odigosCfg == nil || odigosCfg.Profiling == nil || odigosCfg.Profiling.GatewayFileExport == nil {
		return false, ""
	}
	g := odigosCfg.Profiling.GatewayFileExport
	if !g.Enabled {
		return false, ""
	}
	if g.Path != "" {
		return true, g.Path
	}
	return true, defaultGatewayProfilesFileExportPath
}

func profileDebugExportEnabled() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("PROFILE_DEBUG_EXPORT"))) == "true"
}

// shouldBuildGatewayProfilesPipeline decides whether the gateway gets a profiles pipeline.
// - When profiling.enabled is true in OdigosConfiguration, require at least one sink (UI OTLP, verification, file export, or debug).
// - Legacy: PROFILES_* / PROFILE_* env without the new block still works.
func shouldBuildGatewayProfilesPipeline(odigosCfg *odigoscommon.OdigosConfiguration) bool {
	uiEp := resolveOtlpUiEndpoint(odigosCfg)
	verEp := resolveVerificationEndpoint(odigosCfg)
	fileOn, _ := gatewayFileExportPath(odigosCfg)
	debug := profileDebugExportEnabled()
	hasSink := uiEp != "" || verEp != "" || fileOn || debug

	if odigosCfg != nil && odigosCfg.ProfilingEnabled() {
		return hasSink
	}
	// Legacy installs that only set env on the autoscaler / process env.
	if os.Getenv("PROFILES_OTLP_ENDPOINT_UI") != "" || os.Getenv("PROFILE_VERIFICATION_OTLP_ENDPOINT") != "" || debug {
		return true
	}
	return false
}

// mergeProfilesExporterSettings merges Helm/env defaults with OdigosConfiguration.profiling.exporter.
func mergeProfilesExporterSettings(odigosCfg *odigoscommon.OdigosConfiguration) config.GenericMap {
	base := getProfilesExporterConfigFromEnv()
	if odigosCfg == nil || odigosCfg.Profiling == nil || odigosCfg.Profiling.Exporter == nil {
		return base
	}
	e := odigosCfg.Profiling.Exporter
	if e.Timeout != "" {
		base["timeout"] = e.Timeout
	}
	if e.RetryOnFailure != nil {
		retry := config.GenericMap{}
		if e.RetryOnFailure.Enabled != nil {
			retry["enabled"] = *e.RetryOnFailure.Enabled
		}
		if e.RetryOnFailure.InitialInterval != "" {
			retry["initial_interval"] = e.RetryOnFailure.InitialInterval
		}
		if e.RetryOnFailure.MaxInterval != "" {
			retry["max_interval"] = e.RetryOnFailure.MaxInterval
		}
		if e.RetryOnFailure.MaxElapsedTime != "" {
			retry["max_elapsed_time"] = e.RetryOnFailure.MaxElapsedTime
		}
		if len(retry) > 0 {
			base["retry_on_failure"] = retry
		}
	}
	return base
}
