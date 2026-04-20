package profiles

import (
	"github.com/odigos-io/odigos/frontend/services/profiles/flamegraph"
)

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
	if minNano == 0 {
		return 0
	}
	return minNano / 1e9
}
