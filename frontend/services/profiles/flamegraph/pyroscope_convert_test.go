package flamegraph

import (
	"strings"
	"testing"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pprofile "go.opentelemetry.io/collector/pdata/pprofile"
	otelProfile "go.opentelemetry.io/proto/otlp/profiles/v1development"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryPyroscopeOTLP_AcceptsBinaryFromPdataProtoMarshaler(t *testing.T) {
	pd := pprofile.NewProfiles()
	pd.ResourceProfiles().AppendEmpty().ScopeProfiles().AppendEmpty().Profiles().AppendEmpty()
	pd.Dictionary().StringTable().Append("cpu")
	m := &pprofile.ProtoMarshaler{}
	b, err := m.MarshalProfiles(pd)
	require.NoError(t, err)
	_, convErr := ParseExportProfilesServiceRequest(b)
	convErrStr := ""
	if convErr != nil {
		convErrStr = convErr.Error()
	}
	require.False(t, strings.HasPrefix(convErrStr, "protojson_unmarshal:"),
		"binary pdata chunks must not be parsed only as JSON; got: %v", convErr)
}

func TestGoogleProfileToSamples_UsesFirstValueOnly(t *testing.T) {
	p := &googleProfile.Profile{
		Sample: []*googleProfile.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{3, 50_000_000},
		}},
		Location: []*googleProfile.Location{{
			Id:   1,
			Line: []*googleProfile.Line{{FunctionId: 1}},
		}},
		Function: []*googleProfile.Function{{
			Id:   1,
			Name: 1,
		}},
		StringTable: []string{"", "main"},
	}
	samples := googleProfileToSamples(p)
	require.Len(t, samples, 1)
	assert.Equal(t, int64(3), samples[0].Value)
	assert.Equal(t, []string{"main"}, samples[0].Stack)
}

func TestGoogleProfileToSamples_KeepsSampleWhenLocationMissingUsesPlaceholder(t *testing.T) {
	p := &googleProfile.Profile{
		Sample: []*googleProfile.Sample{{
			// pprof: location_id[0] is the leaf (hottest frame); last index is toward the root.
			LocationId: []uint64{999, 1},
			Value:      []int64{10},
		}},
		Location: []*googleProfile.Location{{
			Id:   1,
			Line: []*googleProfile.Line{{FunctionId: 1}},
		}},
		Function: []*googleProfile.Function{{
			Id:   1,
			Name: 1,
		}},
		StringTable: []string{"", "main"},
	}
	samples := googleProfileToSamples(p)
	require.Len(t, samples, 1)
	assert.Equal(t, int64(10), samples[0].Value)
	// Root-first: location 999 (missing) is leaf in pprof order → appears last in our stack slice.
	assert.Equal(t, []string{"main", "[missing location id=999]"}, samples[0].Stack)
}

func TestGoogleProfileToSamples_SkipsNonPositivePrimaryValue(t *testing.T) {
	p := &googleProfile.Profile{
		Sample: []*googleProfile.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{0},
		}},
		Location: []*googleProfile.Location{{
			Id:   1,
			Line: []*googleProfile.Line{{FunctionId: 1}},
		}},
		Function: []*googleProfile.Function{{
			Id:   1,
			Name: 1,
		}},
		StringTable: []string{"", "main"},
	}
	assert.Empty(t, googleProfileToSamples(p))
}

func TestGoogleProfileToSamples_UsesLaterLineWhenFirstLineHasNoName(t *testing.T) {
	// pprof: inlined / multi-line locations — first line may have no human-readable Name; later lines do.
	p := &googleProfile.Profile{
		Sample: []*googleProfile.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{5},
		}},
		Location: []*googleProfile.Location{{
			Id: 1,
			Line: []*googleProfile.Line{
				{FunctionId: 1, Line: 10},
				{FunctionId: 2, Line: 20},
			},
		}},
		Function: []*googleProfile.Function{
			{Id: 1, Name: 0},
			{Id: 2, Name: 3},
		},
		StringTable: []string{"", "", "", "RealFunc"},
	}
	samples := googleProfileToSamples(p)
	require.Len(t, samples, 1)
	assert.Equal(t, []string{"RealFunc"}, samples[0].Stack)
}

func TestGoogleProfileToSamples_UsesSystemNameWhenNameEmpty(t *testing.T) {
	p := &googleProfile.Profile{
		Sample: []*googleProfile.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{1},
		}},
		Location: []*googleProfile.Location{{
			Id:   1,
			Line: []*googleProfile.Line{{FunctionId: 1, Line: 42}},
		}},
		Function: []*googleProfile.Function{{
			Id:         1,
			Name:       0,
			SystemName: 2,
			Filename:   3,
		}},
		StringTable: []string{"", "", "_ZN4java3fooEv", "Foo.java"},
	}
	samples := googleProfileToSamples(p)
	require.Len(t, samples, 1)
	assert.Equal(t, []string{"_ZN4java3fooEv"}, samples[0].Stack)
}

func TestGoogleProfileToSamples_ExpandsMultiLineLocationCallerToCallee(t *testing.T) {
	// google/v1: last Line is caller into which earlier lines were inlined — we emit caller→…→callee
	// as separate frames (root-first stack assembly still walks locations root→leaf).
	p := &googleProfile.Profile{
		Sample: []*googleProfile.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{1},
		}},
		Location: []*googleProfile.Location{{
			Id: 1,
			Line: []*googleProfile.Line{
				{FunctionId: 1, Line: 10},
				{FunctionId: 2, Line: 20},
			},
		}},
		Function: []*googleProfile.Function{
			{Id: 1, Name: 3},
			{Id: 2, Name: 4},
		},
		StringTable: []string{"", "", "", "inner", "outer"},
	}
	samples := googleProfileToSamples(p)
	require.Len(t, samples, 1)
	assert.Equal(t, []string{"outer", "inner"}, samples[0].Stack)
}

func TestGoogleProfileToSamples_MappingPlusAddressWhenNoLines(t *testing.T) {
	p := &googleProfile.Profile{
		Sample: []*googleProfile.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{1},
		}},
		Location: []*googleProfile.Location{{
			Id:        1,
			MappingId: 9,
			Address:   0xa50133,
			Line:      nil,
		}},
		Mapping: []*googleProfile.Mapping{{
			Id:       9,
			Filename: 1,
		}},
		StringTable: []string{"", "libc.so.6"},
	}
	samples := googleProfileToSamples(p)
	require.Len(t, samples, 1)
	assert.Equal(t, []string{"libc.so.6+0xa50133"}, samples[0].Stack)
}

func TestNormalizeSampleValuesForPyroscope_KeepsFirstCounterOnly(t *testing.T) {
	prof := &otelProfile.Profile{
		Samples: []*otelProfile.Sample{
			{Values: []int64{100, 50_000_000}},
			{Values: []int64{7}},
		},
	}
	normalizeSampleValuesForPyroscope(prof)
	require.Len(t, prof.Samples, 2)
	assert.Equal(t, []int64{100}, prof.Samples[0].Values)
	assert.Equal(t, []int64{7}, prof.Samples[1].Values)
}
