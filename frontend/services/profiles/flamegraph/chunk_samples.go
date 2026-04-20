package flamegraph

// Sample is one profile stack aggregate: frame names root-first and total value (e.g. sample count).
type Sample struct {
	Stack []string
	Value int64
}

// SamplesFromOTLPChunk turns one stored OTLP chunk (binary protobuf from pdata.ProtoMarshaler, or JSON)
// into stack samples: OTLP → Google pprof (Pyroscope ingest path), then pprof.ProfileMerge across profiles
// in the chunk, then location→name expansion. Invalid wire format or conversion failures yield nil.
func SamplesFromOTLPChunk(chunk []byte) []Sample {
	if len(chunk) == 0 {
		return nil
	}
	samples, ok, _ := tryPyroscopeOTLP(chunk)
	if !ok || len(samples) == 0 {
		return nil
	}
	return samples
}
