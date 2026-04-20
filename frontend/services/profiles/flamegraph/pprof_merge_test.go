package flamegraph

import (
	"testing"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalMergeableProfile returns a tiny valid Google pprof profile with one location "main" and one sample.
func minimalMergeableProfile(value int64) *googleProfile.Profile {
	// String table indices: 0="", 1-4 period/sample type strings, 5 function name.
	return &googleProfile.Profile{
		PeriodType:        &googleProfile.ValueType{Type: 1, Unit: 2},
		SampleType:        []*googleProfile.ValueType{{Type: 3, Unit: 4}},
		DefaultSampleType: 0,
		StringTable:       []string{"", "space", "ns", "cpu", "ns", "main"},
		Sample: []*googleProfile.Sample{{
			LocationId: []uint64{1},
			Value:      []int64{value},
		}},
		Location: []*googleProfile.Location{{
			Id:   1,
			Line: []*googleProfile.Line{{FunctionId: 1}},
		}},
		Function: []*googleProfile.Function{{
			Id:   1,
			Name: 5,
		}},
	}
}

func TestMergeGoogleProfilesGrouped_SumsIdenticalStacks(t *testing.T) {
	p1 := minimalMergeableProfile(5)
	p2 := minimalMergeableProfile(7)
	merged, extra := mergeGoogleProfilesGrouped([]*googleProfile.Profile{p1, p2})
	require.Empty(t, extra)
	require.Len(t, merged, 1)
	var mp *googleProfile.Profile
	for _, v := range merged {
		mp = v
		break
	}
	require.NotNil(t, mp)
	samples := googleProfileToSamples(mp)
	require.Len(t, samples, 1)
	assert.Equal(t, int64(12), samples[0].Value)
	assert.Equal(t, []string{"main"}, samples[0].Stack)
}

func TestMergedSamplesFromChunks_Empty(t *testing.T) {
	s, pt := MergedSamplesFromChunks(nil)
	assert.Nil(t, s)
	require.NotNil(t, pt)
	assert.Equal(t, "cpu", pt.SampleType)
}

func TestDominantMergedGoogleProfileFromChunks_Empty(t *testing.T) {
	gp, pt := DominantMergedGoogleProfileFromChunks(nil)
	assert.Nil(t, gp)
	require.NotNil(t, pt)
	assert.Equal(t, "cpu", pt.SampleType)
}

func TestMergedGoogleProfileForPyroscopeSymdb_Empty(t *testing.T) {
	gp, pt := MergedGoogleProfileForPyroscopeSymdb(nil)
	assert.Nil(t, gp)
	require.NotNil(t, pt)
	assert.Equal(t, "cpu", pt.SampleType)
}
