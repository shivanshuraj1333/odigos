package flamegraph

import (
	pyrofb "github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
)

// AdaptPyroscopeFlamebearerProfile maps Grafana Pyroscope's FlamebearerProfile into Odigos's JSON shape.
// The adapter is necessary for two reasons:
//  1. Type widening: Pyroscope's Levels are [][]int32; Odigos uses [][]int64 to hold nanosecond weights
//     (e.g. 1_050_000_000 ns) without overflow.
//  2. Odigos extensions: Symbols, SymbolsHint, and Timeline are not part of Pyroscope's struct and must
//     be injected here rather than copied from the upstream type.
func AdaptPyroscopeFlamebearerProfile(
	up *pyrofb.FlamebearerProfile,
	timeline *FlamebearerTimeline,
	symbols []SymbolStats,
	symbolsHint string,
) FlamebearerProfile {
	if up == nil {
		return FlamebearerProfile{
			Version: 1,
			Flamebearer: Flamebearer{
				Names:  []string{},
				Levels: [][]int64{},
			},
			Metadata: FlamebearerMetadata{SymbolsHint: symbolsHint},
			Timeline: timeline,
			Groups:   nil,
			Heatmap:  nil,
			Symbols:  symbols,
		}
	}
	// Pyroscope's NewFlameGraph delta-encodes x-offsets (i+0 of each 4-tuple). Pass them through
	// as-is; the frontend canvas decodes delta offsets to absolute positions in flattenFlameRects.
	levels := make([][]int64, len(up.Flamebearer.Levels))
	for i, row := range up.Flamebearer.Levels {
		levels[i] = make([]int64, len(row))
		for j, v := range row {
			levels[i][j] = int64(v)
		}
	}
	var groups interface{}
	if len(up.Groups) > 0 {
		groups = up.Groups
	}
	return FlamebearerProfile{
		Version: int(up.Version),
		Flamebearer: Flamebearer{
			Names:    up.Flamebearer.Names,
			Levels:   levels,
			NumTicks: int64(up.Flamebearer.NumTicks),
			MaxSelf:  int64(up.Flamebearer.MaxSelf),
		},
		Metadata: FlamebearerMetadata{
			Format:      up.Metadata.Format,
			SpyName:     up.Metadata.SpyName,
			SampleRate:  int(up.Metadata.SampleRate),
			Units:       string(up.Metadata.Units),
			Name:        up.Metadata.Name,
			SymbolsHint: symbolsHint,
		},
		Timeline: timeline,
		Groups:   groups,
		Heatmap:  up.Heatmap,
		Symbols:  symbols,
	}
}
