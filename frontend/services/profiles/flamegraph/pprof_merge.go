package flamegraph

import (
	"sort"
	"strconv"
	"strings"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
	pprofileotlp "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	otelProfile "go.opentelemetry.io/proto/otlp/profiles/v1development"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/pkg/ingester/otlp"
)

// googleProfilesFromParsedRequest runs the same OTLP→Google conversion as ingest for every profile
// in an already-decoded ExportProfilesServiceRequest. The request dictionary must be non-nil.
func googleProfilesFromParsedRequest(req *pprofileotlp.ExportProfilesServiceRequest) []*googleProfile.Profile {
	if req == nil {
		return nil
	}
	if req.Dictionary == nil {
		req.Dictionary = &otelProfile.ProfilesDictionary{}
	}
	var out []*googleProfile.Profile
	for _, rp := range req.ResourceProfiles {
		if rp == nil || rp.ScopeProfiles == nil {
			continue
		}
		for _, sp := range rp.ScopeProfiles {
			if sp == nil || sp.Profiles == nil {
				continue
			}
			for _, p := range sp.Profiles {
				if p == nil {
					continue
				}
				normalizeSampleValuesForPyroscope(p)
				ensureServiceNameInSamples(p, req.Dictionary, rp.Resource)
				converted, err := otlp.ConvertOtelToGoogle(p, req.Dictionary)
				if err != nil {
					continue
				}
				for _, cp := range converted {
					cp := cp
					if gp := extractGoogleProfile(&cp); gp != nil && len(gp.Sample) > 0 {
						out = append(out, gp)
					}
				}
			}
		}
	}
	return out
}

// profileCompatibilityKey groups Google profiles that Pyroscope's pprof.ProfileMerge can combine
// (same period type and sample-type vector). We key on resolved string-table values, not raw
// string indices, so OTLP chunks with different dictionary layouts still land in one merge bucket.
func profileCompatibilityKey(p *googleProfile.Profile) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("period:")
	if p.PeriodType != nil {
		b.WriteString(stringFromPprofStringTable(p.StringTable, p.PeriodType.Type))
		b.WriteByte('/')
		b.WriteString(stringFromPprofStringTable(p.StringTable, p.PeriodType.Unit))
	}
	b.WriteString("|sampletypes:")
	for _, st := range p.SampleType {
		if st == nil {
			b.WriteString("<nil>;")
			continue
		}
		b.WriteString(stringFromPprofStringTable(p.StringTable, st.Type))
		b.WriteByte('/')
		b.WriteString(stringFromPprofStringTable(p.StringTable, st.Unit))
		b.WriteByte(';')
	}
	b.WriteString("|dst:")
	b.WriteString(strconv.FormatInt(int64(p.DefaultSampleType), 10))
	return b.String()
}

// mergeGoogleProfilesGrouped merges compatible Google profiles per bucket using pprof.ProfileMerge.
// If a bucket fails to merge (unexpected incompatibility), samples are expanded without cross-profile merge.
func mergeGoogleProfilesGrouped(profiles []*googleProfile.Profile) (merged map[string]*googleProfile.Profile, extraSamples []Sample) {
	buckets := make(map[string][]*googleProfile.Profile)
	for _, p := range profiles {
		if p == nil {
			continue
		}
		k := profileCompatibilityKey(p)
		buckets[k] = append(buckets[k], p)
	}
	out := make(map[string]*googleProfile.Profile, len(buckets))
	for bkey, list := range buckets {
		if len(list) == 0 {
			continue
		}
		var merger pprof.ProfileMerge
		mergeOK := true
		for _, p := range list {
			pc := proto.Clone(p).(*googleProfile.Profile)
			if err := merger.Merge(pc, true); err != nil {
				mergeOK = false
				break
			}
		}
		if mergeOK {
			mp := merger.Profile()
			if mp != nil && len(mp.Sample) > 0 {
				out[bkey] = mp
			}
			continue
		}
		for _, p := range list {
			extraSamples = append(extraSamples, googleProfileToSamples(p)...)
		}
	}
	return out, extraSamples
}

func sortedKeys(m map[string]*googleProfile.Profile) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func profileTotalWeight(p *googleProfile.Profile) int64 {
	if p == nil {
		return 0
	}
	var w int64
	for _, s := range p.Sample {
		if s != nil && len(s.Value) > 0 {
			w += s.Value[0]
		}
	}
	return w
}

// MergedSamplesFromChunks aggregates all OTLP chunks using Grafana Pyroscope's pprof.ProfileMerge
// (same merge primitive as pkg/querier selectMergePprofProfile), then expands merged profiles to
// stack samples for FunctionNameTree / ExportToFlamebearer.
func MergedSamplesFromChunks(chunks [][]byte) ([]Sample, *typesv1.ProfileType) {
	all := collectGoogleProfilesFromChunks(chunks)
	merged, extra := mergeGoogleProfilesGrouped(all)
	if len(merged) == 0 && len(extra) == 0 {
		return nil, DefaultProfileType()
	}
	var rep *googleProfile.Profile
	var maxW int64
	for _, mp := range merged {
		if w := profileTotalWeight(mp); w > maxW {
			maxW = w
			rep = mp
		}
	}
	pt := DefaultProfileType()
	if rep != nil {
		pt = profileTypeFromGoogleProfile(rep)
	}
	var samples []Sample
	for _, k := range sortedKeys(merged) {
		mp := merged[k]
		samples = append(samples, googleProfileToSamples(mp)...)
	}
	samples = append(samples, extra...)
	return samples, pt
}

// DominantMergedGoogleProfileFromChunks runs the same OTLP→Google merge as MergedSamplesFromChunks but
// returns only the merged profile with the largest total sample weight (one compatibility bucket). This
// is the profile shape Pyroscope symdb.WriteProfileSymbols expects. Buckets that failed merge are not
// included (same limitation as the Explore path when only unmergeable fragments exist). The returned
// profile is a protobuf clone safe for in-place mutation (e.g. pkg/pprof Normalize).
func DominantMergedGoogleProfileFromChunks(chunks [][]byte) (*googleProfile.Profile, *typesv1.ProfileType) {
	all := collectGoogleProfilesFromChunks(chunks)
	merged, _ := mergeGoogleProfilesGrouped(all)
	if len(merged) == 0 {
		return nil, DefaultProfileType()
	}
	var rep *googleProfile.Profile
	var maxW int64
	for _, mp := range merged {
		if w := profileTotalWeight(mp); w > maxW {
			maxW = w
			rep = mp
		}
	}
	pt := DefaultProfileType()
	if rep != nil {
		pt = profileTypeFromGoogleProfile(rep)
		return proto.Clone(rep).(*googleProfile.Profile), pt
	}
	return nil, pt
}
