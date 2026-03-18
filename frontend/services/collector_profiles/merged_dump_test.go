package collectorprofiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestMergedDumpPyroscopeFormat verifies that the merged accounting dump contains all
// data necessary to produce a full Pyroscope-compatible response via the same code path as GET profiling.
func TestMergedDumpPyroscopeFormat(t *testing.T) {
	data, err := os.ReadFile("testdata/accounting-merged.json")
	if err != nil {
		t.Skipf("testdata/accounting-merged.json not found: %v", err)
		return
	}
	// Single merged chunk (same format the handler gets when store returns one or many chunks).
	chunks := [][]byte{data}
	profile := BuildPyroscopeProfileFromChunks(chunks)

	// Pyroscope response shape: version, flamebearer (names, levels, numTicks, maxSelf), metadata, timeline
	if profile.Version != 1 {
		t.Errorf("version: got %d, want 1", profile.Version)
	}
	fb := profile.Flamebearer
	if len(fb.Names) == 0 {
		t.Error("flamebearer.names must be non-empty (at least \"total\")")
	}
	if fb.Names[0] != "total" {
		t.Errorf("flamebearer.names[0]: got %q, want \"total\"", fb.Names[0])
	}
	if fb.NumTicks == 0 {
		t.Error("flamebearer.numTicks must be > 0 when dump has samples")
	}
	if len(fb.Levels) == 0 {
		t.Error("flamebearer.levels must be non-empty")
	}
	// 4-tuple per node: xOffset, total, self, nameIndex
	for i, row := range fb.Levels {
		if len(row)%4 != 0 {
			t.Errorf("levels[%d] length %d not multiple of 4", i, len(row))
		}
	}
	if profile.Metadata.Format != "single" {
		t.Errorf("metadata.format: got %q, want \"single\"", profile.Metadata.Format)
	}
	if profile.Metadata.Units != "samples" {
		t.Errorf("metadata.units: got %q, want \"samples\"", profile.Metadata.Units)
	}
	if profile.Metadata.Name != "cpu" {
		t.Errorf("metadata.name: got %q, want \"cpu\"", profile.Metadata.Name)
	}
	if profile.Timeline == nil {
		t.Error("timeline must be set when numTicks > 0")
	} else {
		if len(profile.Timeline.Samples) < 2 {
			t.Errorf("timeline.samples: got len %d, want at least 2", len(profile.Timeline.Samples))
		}
		// Merged dump has timeUnixNano -> startTime should be set (Pyroscope shape)
		if profile.Timeline.StartTime <= 0 {
			t.Errorf("timeline.startTime: got %d, want > 0 (from dump)", profile.Timeline.StartTime)
		}
	}
	if profile.Metadata.SpyName != "" {
		t.Errorf("metadata.spyName: got %q, want \"\" (Pyroscope)", profile.Metadata.SpyName)
	}
	if profile.Symbols != nil {
		t.Error("symbols should be nil for strict Pyroscope response shape")
	}
	t.Logf("numTicks=%d names=%d levels=%d startTime=%d (Pyroscope-like)", fb.NumTicks, len(fb.Names), len(fb.Levels), profile.Timeline.StartTime)

	// Marshal to JSON and assert the serialized shape matches Pyroscope (same keys as reference).
	gotJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	// Pyroscope response keys (no "symbols").
	wantKeys := map[string]bool{"version": true, "flamebearer": true, "metadata": true, "timeline": true, "groups": true, "heatmap": true}
	for k := range got {
		if !wantKeys[k] {
			t.Errorf("response has unexpected key %q (Pyroscope shape has only version, flamebearer, metadata, timeline, groups, heatmap)", k)
		}
	}
	for k := range wantKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("response missing key %q", k)
		}
	}
	fbMap, _ := got["flamebearer"].(map[string]interface{})
	for _, key := range []string{"names", "levels", "numTicks", "maxSelf"} {
		if _, ok := fbMap[key]; !ok {
			t.Errorf("flamebearer missing key %q", key)
		}
	}
	metaMap, _ := got["metadata"].(map[string]interface{})
	for _, key := range []string{"format", "spyName", "sampleRate", "units", "name"} {
		if _, ok := metaMap[key]; !ok {
			t.Errorf("metadata missing key %q", key)
		}
	}
	timelineMap, _ := got["timeline"].(map[string]interface{})
	for _, key := range []string{"startTime", "samples", "durationDelta", "watermarks"} {
		if _, ok := timelineMap[key]; !ok {
			t.Errorf("timeline missing key %q", key)
		}
	}
	t.Logf("JSON shape matches Pyroscope (keys and nested structure)")
}

// TestDcDumpRunsOnRealDumps runs BuildPyroscopeProfileFromChunks on all JSON files in dc-dump/
// (when present). Run from frontend: go test -v -run TestDcDump ./services/collector_profiles/
// Dumps live at repo dc-dump/ (use ../dc-dump when running from frontend).
func TestDcDumpRunsOnRealDumps(t *testing.T) {
	var dumpDir string
	for _, d := range []string{"../../../dc-dump", "../dc-dump", "dc-dump"} {
		if _, err := os.Stat(d); err == nil {
			dumpDir = d
			break
		}
	}
	if dumpDir == "" {
		t.Skipf("dc-dump/ not found (try from frontend with dumps at ../dc-dump)")
		return
	}
	entries, err := os.ReadDir(dumpDir)
	if err != nil {
		t.Fatalf("read dc-dump: %v", err)
	}
	var chunks [][]byte
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dumpDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Logf("skip %s: %v", path, err)
			continue
		}
		chunks = append(chunks, data)
	}
	if len(chunks) == 0 {
		t.Skip("no .json files in dc-dump/")
		return
	}
	profile := BuildPyroscopeProfileFromChunks(chunks)
	fb := profile.Flamebearer
	t.Logf("dc-dump: %d chunks -> numTicks=%d names=%d levels=%d", len(chunks), fb.NumTicks, len(fb.Names), len(fb.Levels))
	show := 25
	if len(fb.Names) < show {
		show = len(fb.Names)
	}
	for i := 0; i < show; i++ {
		t.Logf("  names[%d]=%q", i, fb.Names[i])
	}
	if len(fb.Names) > show {
		t.Logf("  ... and %d more names", len(fb.Names)-show)
	}
	// Sanity: same shape as merged test
	if profile.Version != 1 || fb.Names[0] != "total" {
		t.Errorf("unexpected shape: version=%d names[0]=%q", profile.Version, fb.Names[0])
	}
}
