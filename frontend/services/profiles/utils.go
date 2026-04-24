package profiles

import (
	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/frontend/services/common"
	"github.com/odigos-io/odigos/frontend/services/profiles/flamegraph"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// SourceKeyFromResource extracts namespace, kind and name from OTLP resource attributes
// using the same resolution rules as collector traffic metrics (ResourceAttributesToSourceID).
func SourceKeyFromResource(attrs pcommon.Map) (string, bool) {
	sID, err := common.ResourceAttributesToSourceID(attrs)
	if err != nil || sID.Name == "" {
		return "", false
	}
	return sID.Namespace + "/" + string(sID.Kind) + "/" + sID.Name, true
}

// NormalizeWorkloadKind maps API/UI strings to canonical WorkloadKind values for source keys.
// GraphQL and resource attributes may use mixed casing; keys must match k8sconsts.
// Delegates to workload.WorkloadKindFromString; unknown values pass through unchanged.
func NormalizeWorkloadKind(kindStr string) k8sconsts.WorkloadKind {
	if k := workload.WorkloadKindFromString(kindStr); k != "" {
		return k
	}
	return k8sconsts.WorkloadKind(kindStr)
}

// SourceKeyFromSourceID returns a stable string key for the given SourceID.
// Format: "namespace/kind/name" so it matches keys derived from profile resource attributes.
func SourceKeyFromSourceID(id common.SourceID) string {
	return id.Namespace + "/" + string(id.Kind) + "/" + id.Name
}

// allNamesArePlaceholders reports whether every non-root frame name is synthetic (no resolved symbols).
// If there are no real frames (e.g. only "total"), returns false — that is an empty graph, not a symbols issue.
func allNamesArePlaceholders(names []string) bool {
	nonTrivial := 0
	for _, n := range names {
		if n == "" || n == "total" || n == "other" {
			continue
		}
		nonTrivial++
		if !isSyntheticFrameName(n) {
			return false
		}
	}
	return nonTrivial > 0
}

func isSyntheticFrameName(n string) bool {
	if len(n) > 6 && n[:6] == "frame_" {
		return true
	}
	if len(n) > 2 && n[:2] == "0x" {
		return true
	}
	return false
}

// earliestProfileStartTimeUnixSec scans OTLP profile chunks (binary protobuf or JSON) for the smallest
// profile TimeUnixNano to populate Flamebearer timeline start (seconds). Required for correct time axis
// when merging chunks. Binary chunks use the same wire as pdata ProtoMarshaler (ExportProfilesServiceRequest).
func earliestProfileStartTimeUnixSec(chunks [][]byte) int64 {
	var minNano int64
	for _, chunk := range chunks {
		req, err := flamegraph.ParseExportProfilesServiceRequest(chunk)
		if err != nil {
			continue
		}
		for _, rp := range req.ResourceProfiles {
			for _, sp := range rp.ScopeProfiles {
				for _, prof := range sp.Profiles {
					nano := int64(prof.TimeUnixNano)
					if nano > 0 && (minNano == 0 || nano < minNano) {
						minNano = nano
					}
				}
			}
		}
	}
	return minNano / 1e9
}
