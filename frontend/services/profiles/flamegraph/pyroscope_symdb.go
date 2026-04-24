package flamegraph

import (
	"context"
	"fmt"
	"os"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	pyrofb "github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/pprof"
	"google.golang.org/protobuf/proto"
)

const symdbFlameMaxNodesDefault int64 = 2048

// BuildFlamebearerViaPyroscopeSymdb builds a flamebearer profile using the same stack as Grafana Pyroscope
// Explore for merged profiles: pkg/pprof Normalize → symdb WriteProfileSymbols → symdb Resolver.Tree →
// pkg/model NewFlameGraph → ExportToFlamebearer (see pkg/querier SelectMergeStacktraces).
// The merged Google profile comes from MergedGoogleProfileForPyroscopeSymdb (OTLP via ingester/otlp,
// per-bucket ProfileMerge, then cross-bucket merge when compatible, else heaviest bucket).
// On empty input it returns (nil, nil, nil). On symdb/resolver failure it returns a non-nil error.
func BuildFlamebearerViaPyroscopeSymdb(ctx context.Context, chunks [][]byte, maxNodes int64) (*pyrofb.FlamebearerProfile, *phlaremodel.FunctionNameTree, error) {
	gp, pt := MergedGoogleProfileForPyroscopeSymdb(chunks)
	if gp == nil || len(gp.Sample) == 0 {
		return nil, nil, nil
	}
	if maxNodes <= 0 {
		maxNodes = symdbFlameMaxNodesDefault
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
	fg := phlaremodel.NewFlameGraph(tree, maxNodes)
	return phlaremodel.ExportToFlamebearer(fg, pt), tree, nil
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

// MergedGoogleProfileForPyroscopeSymdb returns one Google pprof profile to feed symdb, mirroring how the
// querier ends up with a single merged profile: each compatibility bucket is already merged; if all buckets
// can be merged together with ProfileMerge, that combined profile is used; otherwise the single heaviest
// bucket (incompatible profile types in one buffer cannot be one Pyroscope flame).
func MergedGoogleProfileForPyroscopeSymdb(chunks [][]byte) (*googleProfile.Profile, *typesv1.ProfileType) {
	all := collectGoogleProfilesFromChunks(chunks)
	merged, _ := mergeGoogleProfilesGrouped(all)
	if len(merged) == 0 {
		return nil, DefaultProfileType()
	}
	keys := sortedKeys(merged)
	if len(keys) == 1 {
		mp := merged[keys[0]]
		if mp == nil || len(mp.Sample) == 0 {
			return nil, DefaultProfileType()
		}
		return proto.Clone(mp).(*googleProfile.Profile), profileTypeFromGoogleProfile(mp)
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
			return proto.Clone(mp).(*googleProfile.Profile), profileTypeFromGoogleProfile(mp)
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
	if rep == nil {
		return nil, DefaultProfileType()
	}
	return proto.Clone(rep).(*googleProfile.Profile), profileTypeFromGoogleProfile(rep)
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
