package flamegraph

import "sort"

// Flamebearer is the JSON shape sent to the frontend (Pyroscope-compatible).
// Each level is a row; each node is 4 ints: xOffset (delta), total, self, nameIndex.
type Flamebearer struct {
	Names    []string  `json:"names"`
	Levels   [][]int64 `json:"levels"`
	NumTicks int64     `json:"numTicks"`
	MaxSelf  int64     `json:"maxSelf"`
}

// FlamebearerProfile is the full response (Pyroscope-compatible: version, flamebearer, metadata, timeline; plus symbols for Odigos).
type FlamebearerProfile struct {
	Version      int                  `json:"version"`
	Flamebearer  Flamebearer         `json:"flamebearer"`
	Metadata     FlamebearerMetadata  `json:"metadata"`
	Timeline     *FlamebearerTimeline `json:"timeline,omitempty"`
	Groups       interface{}          `json:"groups"` // null for single profile (Pyroscope shape)
	Heatmap      interface{}          `json:"heatmap"`
	Symbols      []SymbolStats        `json:"symbols,omitempty"`
}

// FlamebearerMetadata describes the profile (Pyroscope metadata shape).
type FlamebearerMetadata struct {
	Format     string `json:"format"`     // "single"
	SpyName    string `json:"spyName"`    // e.g. "ebpf" or ""
	SampleRate int    `json:"sampleRate"` // e.g. 1000000000 (Hz) or 0
	Units      string `json:"units"`      // e.g. "samples"
	Name       string `json:"name"`      // e.g. "cpu"
}

// FlamebearerTimeline is optional timeline data (Pyroscope shape); when nil omitted from JSON.
type FlamebearerTimeline struct {
	StartTime      int64   `json:"startTime"`
	Samples        []int64 `json:"samples"`
	DurationDelta  int     `json:"durationDelta"`
	Watermarks     *[]int  `json:"watermarks,omitempty"`
}

const (
	defaultMaxNodes = 1024
	otherName       = "other"
)

// TreeToFlamebearer converts a Tree to Flamebearer (Pyroscope format).
// maxNodes limits the number of nodes; smaller nodes are folded into "other".
func TreeToFlamebearer(t *Tree, maxNodes int64) Flamebearer {
	if maxNodes <= 0 {
		maxNodes = defaultMaxNodes
	}
	var total, maxSelf int64
	for _, n := range t.root {
		total += n.total
	}
	names := []string{}
	nameIdx := map[string]int{}
	var levels [][]int64
	minVal := t.minValue(maxNodes)

	type item struct {
		xOffset int64
		level   int
		n       *node
	}
	stack := []item{{0, 0, &node{children: t.root, total: total}}}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := cur.n
		if n.self > maxSelf {
			maxSelf = n.self
		}
		name := n.name
		if name == "" && cur.level == 0 {
			name = "total"
		}
		idx, ok := nameIdx[name]
		if !ok {
			idx = len(names)
			nameIdx[name] = idx
			names = append(names, name)
		}
		for cur.level >= len(levels) {
			levels = append(levels, []int64{})
		}
		row := levels[cur.level]
		// Append: xOffset (will delta-encode later), total, self, nameIndex
		row = append(row, cur.xOffset, n.total, n.self, int64(idx))
		levels[cur.level] = row

		// Compute xOffset per child left-to-right; push in reverse so pop order = left-to-right (Pyroscope-compatible).
		xStart := cur.xOffset
		var otherTotal int64
		offsets := make([]int64, len(n.children))
		for i := 0; i < len(n.children); i++ {
			c := n.children[i]
			if c.total >= minVal && c.name != otherName {
				offsets[i] = xStart
				xStart += c.total
			} else {
				otherTotal += c.total
			}
		}
		for i := len(n.children) - 1; i >= 0; i-- {
			c := n.children[i]
			if c.total >= minVal && c.name != otherName {
				stack = append(stack, item{xOffset: offsets[i], level: cur.level + 1, n: c})
			}
		}
		if otherTotal > 0 {
			stack = append(stack, item{xOffset: xStart, level: cur.level + 1, n: &node{name: otherName, self: otherTotal, total: otherTotal}})
		}
	}

	// Delta-encode x offsets (first of each 4-tuple)
	for _, row := range levels {
		var prev int64
		for i := 0; i < len(row); i += 4 {
			row[i] -= prev
			prev += row[i] + row[i+1]
		}
	}

	return Flamebearer{
		Names:    names,
		Levels:   levels,
		NumTicks: total,
		MaxSelf:  maxSelf,
	}
}

// minValue returns the minimum node total to include (nodes below are folded into "other").
func (t *Tree) minValue(maxNodes int64) int64 {
	if maxNodes < 1 {
		return 0
	}
	count := t.nodeCount()
	if count <= maxNodes {
		return 0
	}
	type pair struct {
		total int64
		n     *node
	}
	var nodes []pair
	var visit func(*node)
	visit = func(n *node) {
		if n == nil {
			return
		}
		nodes = append(nodes, pair{n.total, n})
		for _, c := range n.children {
			visit(c)
		}
	}
	for _, r := range t.root {
		visit(r)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].total < nodes[j].total })
	if len(nodes) <= int(maxNodes) {
		return 0
	}
	cut := len(nodes) - int(maxNodes)
	return nodes[cut].total
}

func (t *Tree) nodeCount() int64 {
	var c int64
	var visit func(*node)
	visit = func(n *node) {
		if n == nil {
			return
		}
		c++
		for _, ch := range n.children {
			visit(ch)
		}
	}
	for _, r := range t.root {
		visit(r)
	}
	return c
}
