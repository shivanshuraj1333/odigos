package profiles

import (
	"context"

	pyrometadata "github.com/grafana/pyroscope/pkg/og/storage/metadata"
	pyrofb "github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/odigos-io/odigos/frontend/services/profiles/flamegraph"
)

// buildPyroscopeProfileFromChunks builds a Pyroscope-shaped profile from OTLP chunks (protobuf or JSON).
// The merge window is only what is still buffered in the UI ProfileStore for this workload (rolling TTL/byte cap),
func buildPyroscopeProfileFromChunks(ctx context.Context, chunks [][]byte) flamegraph.FlamebearerProfile {
	const maxNodes = 2048
	up, tree, err := flamegraph.BuildFlamebearerViaPyroscopeSymdb(ctx, chunks, maxNodes)
	if err != nil {
		up = nil
	}
	startTimeSec := earliestProfileStartTimeUnixSec(chunks)
	numTicks := int64(0)
	if up != nil {
		numTicks = int64(up.Flamebearer.NumTicks)
	}
	timeline := pyroscopeTimeline(numTicks, startTimeSec)
	symbols := flamegraph.SymbolStatsFromFunctionNameTree(tree)
	out := flamegraph.AdaptPyroscopeFlamebearerProfile(up, timeline, symbols)
	if out.FlamebearerProfile != nil && out.FlamebearerProfile.Metadata.Format == "" {
		out.FlamebearerProfile.Metadata = pyroscopeMetadata()
	}
	return out
}

// Pyroscope web UI expects this metadata shape for single CPU-style profiles (historical JSON contract).
const (
	pyroscopeFlamebearerJSONVersion = 1
	pyroscopeMetadataFormatSingle   = "single"
	pyroscopeMetadataUnitsSamples   = "samples"
	pyroscopeMetadataProfileNameCPU = "cpu"
	// pyroscopeMetadataSampleRate matches Grafana ExportToFlamebearer for CPU (nanoseconds period hint).
	pyroscopeMetadataSampleRate = 1_000_000_000
)

func pyroscopeMetadata() pyrofb.FlamebearerMetadataV1 {
	return pyrofb.FlamebearerMetadataV1{
		Format:     pyroscopeMetadataFormatSingle,
		SpyName:    "",
		SampleRate: pyroscopeMetadataSampleRate,
		Units:      pyrometadata.Units(pyroscopeMetadataUnitsSamples),
		Name:       pyroscopeMetadataProfileNameCPU,
	}
}

// pyroscopeTimeline builds a minimal single-bar timeline when there are samples (start + duration heuristic).
func pyroscopeTimeline(numTicks int64, startTimeSec int64) *pyrofb.FlamebearerTimelineV1 {
	if numTicks == 0 {
		return nil
	}
	return &pyrofb.FlamebearerTimelineV1{
		StartTime:     startTimeSec,
		Samples:       []uint64{0, uint64(numTicks)},
		DurationDelta: 15,
		Watermarks:    nil,
	}
}
