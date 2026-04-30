package profiles

import (
	"context"
	"fmt"

	"github.com/odigos-io/odigos/frontend/services/profiles/flamegraph"
)

// buildPyroscopeProfileFromChunks builds a Pyroscope-shaped profile from OTLP chunks (protobuf or JSON).
// It calls flamegraph.BuildFlamebearerViaPyroscopeSymdb — the same symdb→resolver→flame path as
// Grafana Pyroscope (Normalize, symdb WriteProfileSymbols, Resolver.Tree, NewFlameGraph,
// ExportToFlamebearer). It follows Grafana Pyroscope's query-side stack on a merged Google profile:
// ingester/otlp conversion, pkg/pprof.ProfileMerge (per compatibility bucket, then cross-bucket merge when possible).
// The merge window is only what is still buffered in the UI ProfileStore for this workload (rolling TTL/byte cap),
// not the same as querying Pyroscope with an arbitrary [start,end] over durable storage.
func buildPyroscopeProfileFromChunks(ctx context.Context, chunks [][]byte) flamegraph.FlamebearerProfile {
	const maxNodes = 2048 // Pyroscope querier maxNodesDefault when SelectMergeStacktraces omits maxNodes.
	up, tree, err := flamegraph.BuildFlamebearerViaPyroscopeSymdb(ctx, chunks, maxNodes)
	symbolsHint := ""
	if err != nil {
		symbolsHint = fmt.Sprintf("Pyroscope symdb flame build failed: %v", err)
		up = nil
	}
	startTimeSec := earliestProfileStartTimeUnixSec(chunks)
	numTicks := int64(0)
	if up != nil {
		numTicks = int64(up.Flamebearer.NumTicks)
	}
	timeline := pyroscopeTimeline(numTicks, startTimeSec)
	symbols := flamegraph.SymbolStatsFromFunctionNameTree(tree)
	if up != nil && numTicks > 0 && symbolsHint == "" && allNamesArePlaceholders(up.Flamebearer.Names) {
		symbolsHint = "Symbols unavailable. Ensure the collector sends full OTLP profile dictionaries (Pyroscope-shaped path)."
	}
	out := flamegraph.AdaptPyroscopeFlamebearerProfile(up, timeline, symbols, symbolsHint)
	if up == nil {
		def := pyroscopeMetadata()
		out.Metadata.Format = def.Format
		out.Metadata.SpyName = def.SpyName
		out.Metadata.SampleRate = def.SampleRate
		out.Metadata.Units = def.Units
		out.Metadata.Name = def.Name
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

func pyroscopeMetadata() flamegraph.FlamebearerMetadata {
	return flamegraph.FlamebearerMetadata{
		Format:     pyroscopeMetadataFormatSingle,
		SpyName:    "",
		SampleRate: pyroscopeMetadataSampleRate,
		Units:      pyroscopeMetadataUnitsSamples,
		Name:       pyroscopeMetadataProfileNameCPU,
	}
}

// pyroscopeTimeline builds a minimal single-bar timeline when there are samples (start + duration heuristic).
func pyroscopeTimeline(numTicks int64, startTimeSec int64) *flamegraph.FlamebearerTimeline {
	if numTicks == 0 {
		return nil
	}
	return &flamegraph.FlamebearerTimeline{
		StartTime:     startTimeSec,
		Samples:       []int64{0, numTicks},
		DurationDelta: 15,
		Watermarks:    nil,
	}
}
