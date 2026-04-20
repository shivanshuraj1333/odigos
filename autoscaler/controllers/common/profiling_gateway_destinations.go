package common

import (
	"slices"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
)

// ProfileOtlpExporterNames returns gateway OTLP exporter IDs (e.g. otlp/generic-<destination name>)
// for enabled destinations that declare the PROFILES signal on a type that registers the shared
// otlp/generic-* exporter (currently Generic OTLP / "otlp").
func ProfileOtlpExporterNames(destinations *odigosv1.DestinationList) []string {
	if destinations == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for i := range destinations.Items {
		d := &destinations.Items[i]
		if d.Spec.Disabled != nil && *d.Spec.Disabled {
			continue
		}
		if !slices.Contains(d.Spec.Signals, common.ProfilesObservabilitySignal) {
			continue
		}
		if d.Spec.Type != common.GenericOTLPDestinationType {
			continue
		}
		name := "otlp/generic-" + d.Name
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
