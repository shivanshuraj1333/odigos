package flamegraph

// ChunkTransformRoute describes how SamplesFromOTLPChunk produced stack samples.
type ChunkTransformRoute string

const (
	// RoutePyroscopeOTLP uses Grafana Pyroscope's otlp.ConvertOtelToGoogle (preferred; matches server ingest).
	RoutePyroscopeOTLP ChunkTransformRoute = "pyroscope-otlp"
	// RouteJSONFallback uses ParseOTLPChunk when proto JSON does not match or Pyroscope conversion yields nothing.
	RouteJSONFallback ChunkTransformRoute = "json-fallback"
	// RouteError means neither path produced usable samples.
	RouteError ChunkTransformRoute = "error"
)

// ChunkTransformStats is per-chunk diagnostics for logging and debug JSON.
type ChunkTransformStats struct {
	Route               ChunkTransformRoute
	ByteLen             int
	PyroscopeFailReason string
	JSONFallbackErr     error
	SampleCount         int
}

// SamplesFromOTLPChunk is the single entry point for turning one stored OTLP JSON blob into stack samples.
// 1) Prefer Pyroscope's OTLP→pprof conversion (same library as Grafana Pyroscope ingest).
// 2) Fall back to flexible JSON parsing (ParseOTLPChunk) for shapes protojson rejects or sparse dictionaries.
func SamplesFromOTLPChunk(chunk []byte) ([]Sample, ChunkTransformStats) {
	st := ChunkTransformStats{ByteLen: len(chunk)}
	if len(chunk) == 0 {
		st.Route = RouteError
		bpFlamef("chunk→samples: empty chunk")
		return nil, st
	}

	samples, ok, reason := tryPyroscopeOTLP(chunk)
	if ok && len(samples) > 0 {
		st.Route = RoutePyroscopeOTLP
		st.SampleCount = len(samples)
		bpFlamef("chunk→samples: route=%s bytes=%d samples=%d (ConvertOtelToGoogle)", RoutePyroscopeOTLP, len(chunk), len(samples))
		return samples, st
	}
	if reason != "" {
		st.PyroscopeFailReason = reason
		bpFlamef("chunk→samples: pyroscope path not used: %s", reason)
	}

	parsed, err := ParseOTLPChunk(chunk)
	if err != nil {
		st.Route = RouteError
		st.JSONFallbackErr = err
		bpFlamef("chunk→samples: route=%s json-fallback parse error: %v", RouteError, err)
		return nil, st
	}
	out := expandParsedChunkToSamples(parsed)
	st.Route = RouteJSONFallback
	st.SampleCount = len(out)
	bpFlamef("chunk→samples: route=%s bytes=%d expanded_samples=%d raw_parsed_samples=%d names_in_table=%d",
		RouteJSONFallback, len(chunk), len(out), len(parsed.Samples), len(parsed.Names))
	return out, st
}
