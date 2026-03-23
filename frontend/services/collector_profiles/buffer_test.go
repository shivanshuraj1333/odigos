package collectorprofiles

import "testing"

func TestChunkLikelyHasOTLPProfileDictionary(t *testing.T) {
	if chunkLikelyHasOTLPProfileDictionary([]byte(`{"dictionary":{"stringTable":["a"]}}`)) != true {
		t.Fatal("expected dictionary with stringTable")
	}
	if chunkLikelyHasOTLPProfileDictionary([]byte(`{"resourceProfiles":[]}`)) != false {
		t.Fatal("expected no dictionary markers")
	}
}

func TestBoundedBufferEvictionPrefersChunksWithoutDictionary(t *testing.T) {
	without := []byte(`{"resourceProfiles":[]}`)
	with := []byte(`{"dictionary":{"stringTable":["x"]},"resourceProfiles":[]}`)
	// After [without, without, with], two trim steps must drop both shorts and leave only `with`.
	max := len(with) + len(without) - 1
	b := NewBoundedBuffer(max)
	b.Add(without)
	b.Add(without)
	b.Add(with)
	if b.Size() > max {
		t.Fatalf("size %d > max %d", b.Size(), max)
	}
	chunks := b.Snapshot()
	if len(chunks) != 1 || !chunkLikelyHasOTLPProfileDictionary(chunks[0]) {
		t.Fatalf("expected single dictionary chunk, got %d chunks", len(chunks))
	}
}
