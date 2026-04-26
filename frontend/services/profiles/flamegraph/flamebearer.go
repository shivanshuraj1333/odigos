package flamegraph

// Flamebearer and FlamebearerProfile mirror the JSON the Pyroscope UI consumes. OTLP→samples uses
// github.com/grafana/pyroscope/pkg/ingester/otlp; merged layout + metadata use pkg/model
// (NewFlameGraph + ExportToFlamebearer), then AdaptPyroscopeFlamebearerProfile maps to these structs
// ([][]int64 levels, symbols extension). TreeToFlamebearer remains for tests and legacy comparisons.
//
// Flamebearer is the JSON shape sent to the frontend (Pyroscope-compatible).
type Flamebearer struct {
	Names    []string  `json:"names"`
	Levels   [][]int64 `json:"levels"`
	NumTicks int64     `json:"numTicks"`
	MaxSelf  int64     `json:"maxSelf"`
}

// FlamebearerProfile is the full response (Pyroscope-compatible: version, flamebearer, metadata, timeline; plus symbols for Odigos).
type FlamebearerProfile struct {
	Version     int                  `json:"version"`
	Flamebearer Flamebearer          `json:"flamebearer"`
	Metadata    FlamebearerMetadata  `json:"metadata"`
	Timeline    *FlamebearerTimeline `json:"timeline,omitempty"`
	Groups      interface{}          `json:"groups"` // null for single profile (Pyroscope shape)
	Heatmap     interface{}          `json:"heatmap"`
	Symbols     []SymbolStats        `json:"symbols,omitempty"`
}

// FlamebearerMetadata describes the profile (Pyroscope metadata shape).
type FlamebearerMetadata struct {
	Format      string `json:"format"`                // "single"
	SpyName     string `json:"spyName"`               // e.g. "ebpf" or ""
	SampleRate  int    `json:"sampleRate"`            // e.g. 1000000000 (Hz) or 0
	Units       string `json:"units"`                 // e.g. "samples"
	Name        string `json:"name"`                  // e.g. "cpu"
	SymbolsHint string `json:"symbolsHint,omitempty"` // Shown in UI when symbols are placeholders (frame_N)
}

// FlamebearerTimeline is optional timeline data (Pyroscope shape); when nil omitted from JSON.
type FlamebearerTimeline struct {
	StartTime     int64   `json:"startTime"`
	Samples       []int64 `json:"samples"`
	DurationDelta int     `json:"durationDelta"`
	Watermarks    *[]int  `json:"watermarks"` // null for single profile (Pyroscope shape)
}

// Sample is one profile stack aggregate: frame names root-first and total value (e.g. sample count).
type Sample struct {
	Stack []string
	Value int64
}

const (
	otherName = "other"
)
