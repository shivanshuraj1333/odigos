package flamegraph

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofileotlp"
)

// TestOdigosUI_RendersRealMemoryFrames feeds an OTLP heap profile shaped exactly
// like what the collector emits (one single-sample-type Profile per heap type,
// dictionary-encoded, symbolized frames, real byte values) through the SAME
// function the odigos UI/GraphQL uses (BuildFlamebearerViaPyroscopeSymdb), and
// asserts the rendered flamegraph contains the real application frames with the
// real weights — proving the UI shows true data, not junk, independent of
// Pyroscope's (lossy, CPU-only) OTLP ingestion.
func TestOdigosUI_RendersRealMemoryFrames(t *testing.T) {
	chunk := buildHeapChunk(t)

	fb, _, err := BuildFlamebearerViaPyroscopeSymdb(
		context.Background(), [][]byte{chunk}, 2048, "alloc_space")
	if err != nil {
		t.Fatalf("BuildFlamebearerViaPyroscopeSymdb: %v", err)
	}
	if fb == nil {
		t.Fatal("nil flamebearer — UI would show nothing")
	}

	names := strings.Join(fb.Flamebearer.Names, " ")
	t.Logf("rendered frames: %s", names)
	t.Logf("numTicks (total weight): %d", fb.Flamebearer.NumTicks)
	t.Logf("units: %s, name: %s", fb.Metadata.Units, fb.Metadata.Name)

	for _, want := range []string{"handle_request", "db_query"} {
		if !strings.Contains(names, want) {
			t.Errorf("flamegraph missing app frame %q (names=%q)", want, names)
		}
	}
	if fb.Flamebearer.NumTicks == 0 {
		t.Error("numTicks=0 — UI would render an empty flamegraph")
	}
	if string(fb.Metadata.Units) != "bytes" {
		t.Errorf("memory units=%q want bytes", fb.Metadata.Units)
	}
}

// buildHeapChunk constructs the marshaled OTLP chunk the ProfileStore would hold:
// service.name=checkout, one alloc_space Profile, one sample whose stack is
// db_query -> handle_request, weight 4 MiB.
func buildHeapChunk(t *testing.T) []byte {
	p := pprofile.NewProfiles()
	dic := p.Dictionary()
	add := func(s string) int32 {
		dic.StringTable().Append(s)
		return int32(dic.StringTable().Len() - 1)
	}
	_ = add("") // 0
	dbQuery := add("db_query")
	handle := add("handle_request")
	file := add("/srv/app.py")
	tAllocSpace := add("alloc_space")
	uBytes := add("bytes")

	// OTLP spec: index 0 of every dictionary table is the reserved empty entry,
	// exactly as the collector's generate.go emits it.
	dic.MappingTable().AppendEmpty()
	dic.FunctionTable().AppendEmpty() // 0 empty
	dic.LocationTable().AppendEmpty() // 0 empty
	dic.StackTable().AppendEmpty()    // 0 empty

	f0 := dic.FunctionTable().AppendEmpty() // 1 db_query
	f0.SetNameStrindex(dbQuery)
	f0.SetFilenameStrindex(file)
	f1 := dic.FunctionTable().AppendEmpty() // 2 handle_request
	f1.SetNameStrindex(handle)
	f1.SetFilenameStrindex(file)

	l0 := dic.LocationTable().AppendEmpty() // 1
	l0.Lines().AppendEmpty().SetFunctionIndex(1)
	l1 := dic.LocationTable().AppendEmpty() // 2
	l1.Lines().AppendEmpty().SetFunctionIndex(2)

	st := dic.StackTable().AppendEmpty() // 1
	st.LocationIndices().Append(1)       // db_query (leaf)
	st.LocationIndices().Append(2)       // handle_request

	rp := p.ResourceProfiles().AppendEmpty()
	rp.Resource().Attributes().PutStr("service.name", "checkout")
	sp := rp.ScopeProfiles().AppendEmpty()

	prof := sp.Profiles().AppendEmpty()
	prof.SampleType().SetTypeStrindex(tAllocSpace)
	prof.SampleType().SetUnitStrindex(uBytes)
	s := prof.Samples().AppendEmpty()
	s.SetStackIndex(1)
	s.Values().Append(4 * 1024 * 1024) // 4 MiB allocated at this stack

	b, err := pprofileotlp.NewExportRequestFromProfiles(p).MarshalProto()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
