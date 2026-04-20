package flamegraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: layout must match Grafana Pyroscope pkg/model.NewFlameGraph — children start at
// parent.x + parent.self so bars fill the parent width (no spurious gaps when self > 0).
func TestTreeToFlamebearer_ChildrenOffsetByParentSelf(t *testing.T) {
	tr := NewTree()
	tr.InsertStack(3, "A")      // leaf-only time at A
	tr.InsertStack(7, "A", "B") // rest under A→B
	fb := TreeToFlamebearer(tr, 0)
	require.Equal(t, int64(10), fb.NumTicks)

	var bLevelIdx, bNameIdx int = -1, -1
	for i, n := range fb.Names {
		if n == "B" {
			bNameIdx = i
			break
		}
	}
	require.NotEqual(t, -1, bNameIdx)

	for li, row := range fb.Levels {
		for j := 0; j < len(row); j += 4 {
			if int(row[j+3]) == bNameIdx {
				bLevelIdx = li
				require.Equal(t, int64(3), row[j], "child B should start after A.self=3")
				require.Equal(t, int64(7), row[j+1])
				require.Equal(t, int64(7), row[j+2])
			}
		}
	}
	require.NotEqual(t, -1, bLevelIdx)
}

func TestTree_AggregateSymbolStats(t *testing.T) {
	tr := NewTree()
	tr.InsertStack(3, "A")
	tr.InsertStack(7, "A", "B")
	stats := tr.AggregateSymbolStats()
	require.Len(t, stats, 2)
	// Sorted by self descending.
	assert.Equal(t, "B", stats[0].Name)
	assert.Equal(t, int64(7), stats[0].Self)
	assert.Equal(t, "A", stats[1].Name)
	assert.Equal(t, int64(3), stats[1].Self)
}
