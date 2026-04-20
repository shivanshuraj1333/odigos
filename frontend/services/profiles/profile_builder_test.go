package profiles

import (
	"context"
	"encoding/json"
	"testing"

	pprofileotlp "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	otelCommon "go.opentelemetry.io/proto/otlp/common/v1"
	otelProfile "go.opentelemetry.io/proto/otlp/profiles/v1development"
	otelResource "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odigos-io/odigos/frontend/services/profiles/flamegraph"
)

// minimalOTLPProfilesExportWire builds a tiny valid ExportProfilesServiceRequest (protobuf wire)
// with one stack frame "main" and a single sample counter, for end-to-end merge tests.
func minimalOTLPProfilesExportWire(t *testing.T, sampleValue int64) []byte {
	t.Helper()
	dict := &otelProfile.ProfilesDictionary{
		StringTable: []string{"", "cpu", "nanoseconds", "process_cpu", "main"},
		MappingTable: []*otelProfile.Mapping{
			{},
		},
		FunctionTable: []*otelProfile.Function{
			{},
			{NameStrindex: 4},
		},
		LocationTable: []*otelProfile.Location{
			{},
			{Lines: []*otelProfile.Line{{FunctionIndex: 1, Line: 1}}},
		},
		StackTable: []*otelProfile.Stack{
			{},
			{LocationIndices: []int32{1}},
		},
	}
	prof := &otelProfile.Profile{
		ProfileId:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SampleType:   &otelProfile.ValueType{TypeStrindex: 1, UnitStrindex: 2},
		PeriodType:   &otelProfile.ValueType{TypeStrindex: 3, UnitStrindex: 2},
		Period:       1_000_000_000,
		TimeUnixNano: 1_700_000_000_000_000_000,
		DurationNano: 1_000_000_000,
		Samples:      []*otelProfile.Sample{{StackIndex: 1, Values: []int64{sampleValue}}},
	}
	res := &otelResource.Resource{Attributes: []*otelCommon.KeyValue{{
		Key: "service.name",
		Value: &otelCommon.AnyValue{
			Value: &otelCommon.AnyValue_StringValue{StringValue: "e2e-merge"},
		},
	}}}
	req := &pprofileotlp.ExportProfilesServiceRequest{
		Dictionary: dict,
		ResourceProfiles: []*otelProfile.ResourceProfiles{{
			Resource: res,
			ScopeProfiles: []*otelProfile.ScopeProfiles{{
				Profiles: []*otelProfile.Profile{prof},
			}},
		}},
	}
	b, err := proto.Marshal(req)
	require.NoError(t, err)
	return b
}

// Smoke: merged profile JSON matches Pyroscope-style metadata (Grafana ExportToFlamebearer for CPU)
// and flamebearer level tuple width (4 ints per node for single format).
func TestBuildPyroscopeProfileFromChunks_JSONShape(t *testing.T) {
	prof := BuildPyroscopeProfileFromChunks(nil)
	b, err := json.Marshal(prof)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	meta, ok := m["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "single", meta["format"])
	assert.Equal(t, "samples", meta["units"])
	assert.Equal(t, float64(1_000_000_000), meta["sampleRate"]) // json numbers
	assert.Equal(t, "cpu", meta["name"])

	fb, ok := m["flamebearer"].(map[string]any)
	require.True(t, ok)
	levels, ok := fb["levels"].([]any)
	require.True(t, ok)
	for _, row := range levels {
		r, ok := row.([]any)
		require.True(t, ok)
		assert.Equal(t, 0, len(r)%4, "each level row must be a multiple of 4 (single-format tuples)")
	}
}

func TestBuildPyroscopeProfileFromChunks_EndToEnd_OTLPMergeAcrossChunks(t *testing.T) {
	a := minimalOTLPProfilesExportWire(t, 5)
	b := minimalOTLPProfilesExportWire(t, 7)
	prof := BuildPyroscopeProfileFromChunks([][]byte{a, b})
	raw, err := json.Marshal(prof)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	fb, ok := m["flamebearer"].(map[string]any)
	require.True(t, ok)
	// Root total should reflect merged sample weights (5+7) on a single-frame stack.
	numTicks, ok := fb["numTicks"].(float64)
	require.True(t, ok, "numTicks missing: %s", string(raw))
	assert.Equal(t, float64(12), numTicks)
}

func TestBuildPyroscopeProfileFromChunks_InvalidWireDoesNotPanic(t *testing.T) {
	prof := BuildPyroscopeProfileFromChunks([][]byte{[]byte("not-valid-protobuf"), nil, {}})
	_, err := json.Marshal(prof)
	require.NoError(t, err)
}

// TestE2E_SymdbPath_UIMatchesRawFlamebearer_MinimalChunks ensures BuildPyroscopeProfileFromChunks (UI)
// matches flamegraph.BuildFlamebearerViaPyroscopeSymdb on the same OTLP wire chunks (symdb path parity).
func TestE2E_SymdbPath_UIMatchesRawFlamebearer_MinimalChunks(t *testing.T) {
	a := minimalOTLPProfilesExportWire(t, 5)
	b := minimalOTLPProfilesExportWire(t, 7)
	chunks := [][]byte{a, b}
	const maxNodes = 2048
	up, _, err := flamegraph.BuildFlamebearerViaPyroscopeSymdb(context.Background(), chunks, maxNodes)
	require.NoError(t, err)
	require.NotNil(t, up)

	prof := BuildPyroscopeProfileFromChunks(chunks)
	require.Equal(t, int64(up.Flamebearer.NumTicks), prof.Flamebearer.NumTicks, "numTicks must match raw ExportToFlamebearer")
	require.Equal(t, int64(up.Flamebearer.MaxSelf), prof.Flamebearer.MaxSelf)
	require.Len(t, prof.Flamebearer.Names, len(up.Flamebearer.Names))
	require.Len(t, prof.Flamebearer.Levels, len(up.Flamebearer.Levels))
	for i, row := range up.Flamebearer.Levels {
		require.Len(t, prof.Flamebearer.Levels[i], len(row))
		for j, v := range row {
			require.Equal(t, int64(v), prof.Flamebearer.Levels[i][j], "level %d col %d", i, j)
		}
	}
}
