package flamegraph

import "fmt"

// expandParsedChunkToSamples turns ParsedChunk (from flexible JSON OTLP parsing) into
// root-first stack samples for the flame tree. Used only when the Pyroscope OTLP→pprof
// path does not apply (see SamplesFromOTLPChunk).
func expandParsedChunkToSamples(parsed *ParsedChunk) []Sample {
	if parsed == nil || len(parsed.Samples) == 0 {
		return nil
	}
	names := resolveStackNamesWithFallback(parsed, nil)
	var out []Sample
	for _, s := range parsed.Samples {
		if len(s.LocIndices) == 0 {
			if len(s.Stack) > 0 {
				out = append(out, Sample{Stack: s.Stack, Value: s.Value})
			}
			continue
		}
		stack := make([]string, 0, len(s.LocIndices))
		for _, locIdx := range s.LocIndices {
			stack = append(stack, names[locIdx])
		}
		if len(stack) > 0 {
			out = append(out, Sample{Stack: stack, Value: s.Value})
		}
	}
	return out
}

func resolveStackNamesWithFallback(parsed *ParsedChunk, ref *ParsedChunk) map[int]string {
	out := make(map[int]string)
	for idx, name := range parsed.Names {
		if name != "" {
			out[idx] = name
		}
	}
	if ref != nil && ref != parsed {
		for idx, name := range ref.Names {
			if name != "" && out[idx] == "" {
				out[idx] = name
			}
		}
	}
	locInfos := parsed.LocationInfos
	if ref != nil && locInfos == nil {
		locInfos = ref.LocationInfos
	}
	for _, s := range parsed.Samples {
		for _, locIdx := range s.LocIndices {
			if out[locIdx] != "" {
				continue
			}
			if locInfos != nil {
				if loc, ok := locInfos[locIdx]; ok && loc.Address != 0 {
					out[locIdx] = fmt.Sprintf("0x%x", loc.Address)
					continue
				}
			}
			out[locIdx] = fmt.Sprintf("frame_%d", locIdx)
		}
	}
	return out
}
