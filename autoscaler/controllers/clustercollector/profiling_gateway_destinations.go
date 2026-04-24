package clustercollector

import (
	"slices"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
)

// profileOtlpExporterNames returns gateway OTLP exporter IDs (e.g. otlp_grpc/generic-<destination ID>)
// for enabled destinations that declare the PROFILES signal on a type that registers the shared
// otlp_grpc/generic-* exporter (currently Generic OTLP / "otlp"). The form must match what
// common/config/genericotlp.go registers — "otlp_grpc/generic-" + dest.GetID() — or the exporter
// lookup in addProfilingGatewayPipeline silently misses and profiles never fan out.
// Fan-out to external destinations is a gateway-only concern; node collectors always forward to the gateway.
func profileOtlpExporterNames(destinations *odigosv1.DestinationList) []string {
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
		name := "otlp_grpc/generic-" + d.GetID()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
