package flamegraph

import (
	"context"
	"fmt"
	"os"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	pyrofb "github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/pprof"
	"google.golang.org/protobuf/proto"
)

const symdbFlameMaxNodesDefault int64 = 2048

func BuildFlamebearerViaPyroscopeSymdb(ctx context.Context, chunks [][]byte, maxNodes int64, sampleType string) (*pyrofb.FlamebearerProfile, *phlaremodel.FunctionNameTree, error) {
	gp, pt, extra := MergedGoogleProfileForPyroscopeSymdb(chunks, sampleType)
	if maxNodes <= 0 {
		maxNodes = symdbFlameMaxNodesDefault
	}

	// No merged pprof, but we still have per-profile stacks (intra-bucket merge failures, etc.).
	if gp == nil || len(gp.Sample) == 0 {
		if len(extra) == 0 {
			return nil, nil, nil
		}
		t := new(phlaremodel.FunctionNameTree)
		insertSamplesIntoFunctionNameTree(t, extra)
		fg := phlaremodel.NewFlameGraph(t, maxNodes)
		return phlaremodel.ExportToFlamebearer(fg, pt), t, nil
	}

	raw := pprof.RawFromProto(proto.Clone(gp).(*googleProfile.Profile))
	raw.Normalize()

	tmp, err := os.MkdirTemp("", "odigos-pyro-symdb-*")
	if err != nil {
		return nil, nil, fmt.Errorf("symdb temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	db := symdb.NewSymDB(symdb.DefaultConfig().WithDirectory(tmp))
	w := db.WriteProfileSymbols(0, raw.Profile)
	if len(w) == 0 {
		return nil, nil, fmt.Errorf("symdb WriteProfileSymbols returned no sample types")
	}
	idx := int(raw.Profile.DefaultSampleType)
	if idx < 0 || idx >= len(w) {
		idx = 0
	}

	r := symdb.NewResolver(ctx, db, symdb.WithResolverMaxNodes(maxNodes))
	defer r.Release()
	r.AddSamples(0, w[idx].Samples)
	tree, err := r.Tree()
	if err != nil {
		return nil, nil, fmt.Errorf("symdb resolver tree: %w", err)
	}
	insertSamplesIntoFunctionNameTree(tree, extra)
	fg := phlaremodel.NewFlameGraph(tree, maxNodes)
	return phlaremodel.ExportToFlamebearer(fg, pt), tree, nil
}

func insertSamplesIntoFunctionNameTree(tree *phlaremodel.FunctionNameTree, samples []Sample) {
	for _, s := range samples {
		if s.Value <= 0 || len(s.Stack) == 0 {
			continue
		}
		fn := make([]phlaremodel.FunctionName, 0, len(s.Stack))
		for _, name := range s.Stack {
			fn = append(fn, phlaremodel.FunctionName(name))
		}
		tree.InsertStack(s.Value, fn...)
	}
}

// SymbolStatsFromFunctionNameTree rebuilds Odigos top-table rows from the same FunctionNameTree Pyroscope
// used before ExportToFlamebearer, by replaying leaf stacks into the local Tree aggregator.
func SymbolStatsFromFunctionNameTree(t *phlaremodel.FunctionNameTree) []SymbolStats {
	if t == nil {
		return nil
	}
	sym := NewTree()
	t.IterateStacks(func(_ phlaremodel.FunctionName, self int64, stack []phlaremodel.FunctionName) {
		if self <= 0 {
			return
		}
		path := make([]string, 0, len(stack))
		for i := len(stack) - 1; i >= 0; i-- {
			s := string(stack[i])
			if s != "" {
				path = append(path, s)
			}
		}
		if len(path) == 0 {
			return
		}
		sym.InsertStack(self, path...)
	})
	return sym.AggregateSymbolStats()
}

// sampleTypeMatches reports whether a merged bucket's pprof sample type (got)
// satisfies the requested profile type. CPU is special: the collector emits the
// CPU origin with pprof SampleType "samples" (period type "cpu"), so an empty or
// "cpu"/"samples" request accepts either spelling. Memory types match exactly:
// alloc_space, alloc_objects, inuse_space, inuse_objects.
func sampleTypeMatches(got, requested string) bool {
	switch requested {
	case "", "cpu", "samples":
		return got == "cpu" || got == "samples"
	default:
		return got == requested
	}
}

// profileTypeForRequest is the ProfileType (name+unit) to report when no samples
// of the requested kind exist yet, so the UI labels units correctly while empty.
func profileTypeForRequest(requested string) *typesv1.ProfileType {
	switch requested {
	case "", "cpu", "samples":
		return &typesv1.ProfileType{SampleType: "cpu", SampleUnit: "samples"}
	case "alloc_space", "inuse_space":
		return &typesv1.ProfileType{SampleType: requested, SampleUnit: "bytes"}
	case "alloc_objects", "inuse_objects":
		return &typesv1.ProfileType{SampleType: requested, SampleUnit: "count"}
	default:
		return &typesv1.ProfileType{SampleType: requested}
	}
}

// MergedGoogleProfileForPyroscopeSymdb returns one Google pprof profile to feed
// symdb, restricted to the requested sampleType so each of CPU + the four memory
// signals (alloc_space, alloc_objects, inuse_space, inuse_objects) renders as its
// own flamegraph instead of being collapsed together. An empty sampleType means CPU.
func MergedGoogleProfileForPyroscopeSymdb(chunks [][]byte, sampleType string) (*googleProfile.Profile, *typesv1.ProfileType, []Sample) {
	all := collectGoogleProfilesFromChunks(chunks)
	merged, intraExtra := mergeGoogleProfilesGrouped(all)
	// Keep only buckets whose sample type matches the request. intraExtra carries
	// stacks from intra-bucket merge failures with no recoverable type, so when a
	// specific type is requested we drop it rather than leak other-signal samples.
	{
		filtered := make(map[string]*googleProfile.Profile, len(merged))
		for k, mp := range merged {
			if mp != nil && sampleTypeMatches(profileTypeFromGoogleProfile(mp).SampleType, sampleType) {
				filtered[k] = mp
			}
		}
		if len(filtered) != len(merged) {
			intraExtra = nil
		}
		merged = filtered
	}
	if len(merged) == 0 {
		return nil, profileTypeForRequest(sampleType), intraExtra
	}
	keys := sortedKeys(merged)
	if len(keys) == 1 {
		mp := merged[keys[0]]
		if mp == nil || len(mp.Sample) == 0 {
			return nil, profileTypeForRequest(sampleType), intraExtra
		}
		return proto.Clone(mp).(*googleProfile.Profile), profileTypeFromGoogleProfile(mp), intraExtra
	}
	var merger pprof.ProfileMerge
	mergeAllOK := true
	for _, k := range keys {
		pc := proto.Clone(merged[k]).(*googleProfile.Profile)
		if err := merger.Merge(pc, true); err != nil {
			mergeAllOK = false
			break
		}
	}
	if mergeAllOK {
		mp := merger.Profile()
		if mp != nil && len(mp.Sample) > 0 {
			return proto.Clone(mp).(*googleProfile.Profile), profileTypeFromGoogleProfile(mp), intraExtra
		}
	}
	var rep *googleProfile.Profile
	var maxW int64
	for _, mp := range merged {
		if mp == nil {
			continue
		}
		if w := profileTotalWeight(mp); w > maxW {
			maxW = w
			rep = mp
		}
	}
	var crossExtra []Sample
	for _, k := range keys {
		mp := merged[k]
		if mp == nil || mp == rep {
			continue
		}
		crossExtra = append(crossExtra, googleProfileToSamples(mp)...)
	}
	outExtra := append(append([]Sample(nil), intraExtra...), crossExtra...)
	if rep == nil {
		return nil, profileTypeForRequest(sampleType), outExtra
	}
	return proto.Clone(rep).(*googleProfile.Profile), profileTypeFromGoogleProfile(rep), outExtra
}

func collectGoogleProfilesFromChunks(chunks [][]byte) []*googleProfile.Profile {
	var all []*googleProfile.Profile
	for _, ch := range chunks {
		if len(ch) == 0 {
			continue
		}
		req, err := ParseExportProfilesServiceRequest(ch)
		if err != nil {
			continue
		}
		all = append(all, googleProfilesFromParsedRequest(req)...)
	}
	return all
}
