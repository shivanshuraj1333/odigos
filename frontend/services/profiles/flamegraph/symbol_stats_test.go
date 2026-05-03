package flamegraph

import (
	"testing"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func TestSymbolStatsFromFunctionNameTree_SumsLeafSelfAndPathTotals(t *testing.T) {
	t.Parallel()
	tree := new(phlaremodel.FunctionNameTree)
	// InsertStack order is root-first (matches Odigos Sample.Stack / Pyroscope FunctionNameTree).
	tree.InsertStack(4,
		phlaremodel.FunctionName("root"),
		phlaremodel.FunctionName("mid"),
		phlaremodel.FunctionName("leaf"),
	)
	tree.InsertStack(3,
		phlaremodel.FunctionName("root"),
		phlaremodel.FunctionName("mid"),
		phlaremodel.FunctionName("sibling"),
	)
	stats := SymbolStatsFromFunctionNameTree(tree)
	byName := make(map[string]SymbolStats, len(stats))
	for _, s := range stats {
		byName[s.Name] = s
	}
	if leaf := byName["leaf"]; leaf.Self != 4 || leaf.Total != 4 {
		t.Fatalf("leaf: got self=%d total=%d want self=4 total=4", leaf.Self, leaf.Total)
	}
	if sibling := byName["sibling"]; sibling.Self != 3 || sibling.Total != 3 {
		t.Fatalf("sibling: got self=%d total=%d want self=3 total=3", sibling.Self, sibling.Total)
	}
	mid := byName["mid"]
	if mid.Self != 0 || mid.Total != 7 {
		t.Fatalf("mid (merged callsites): got self=%d total=%d want self=0 total=7", mid.Self, mid.Total)
	}
	root := byName["root"]
	if root.Self != 0 || root.Total != 7 {
		t.Fatalf("root: got self=%d total=%d want self=0 total=7", root.Self, root.Total)
	}
}
