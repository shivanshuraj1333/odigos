package profiles

import (
	"testing"
	"time"
)

// TestPerTypeBucketsIsolateCPUFromMemory is the regression guard for the shared-buffer
// bug: a high-volume CPU stream must not FIFO-evict the sparser memory samples,
// because CPU and each memory signal now live in independent per-(source,type) buckets.
func TestPerTypeBucketsIsolateCPUFromMemory(t *testing.T) {
	// small per-type budget so the CPU flood far exceeds it
	s := NewProfileStore(4 /*maxSlots*/, 300, 1800, 1000 /*perTypeMaxBytes*/, time.Minute)
	s.EnsureSlot("s1")

	// one memory chunk carrying the two space signals
	mem := []byte("MEMCHUNK-alloc-inuse")
	s.AddProfileDataTyped("s1", []string{"alloc_space", "inuse_space"}, mem)

	// flood CPU far beyond the 1000-byte per-type budget
	cpu := []byte("cpu-chunk-0123456789-0123456789-0123456789")
	for i := 0; i < 500; i++ {
		s.AddProfileDataTyped("s1", []string{"cpu"}, cpu)
	}

	if got := s.GetProfileData("s1", "cpu"); len(got) == 0 {
		t.Fatal("cpu bucket unexpectedly empty")
	}
	// memory must be untouched by the CPU flood
	memChunks := s.GetProfileData("s1", "alloc_space")
	if len(memChunks) != 1 || string(memChunks[0]) != string(mem) {
		t.Fatalf("memory was evicted by CPU flood: got %d chunks (want 1, intact)", len(memChunks))
	}
	if got := s.GetProfileData("s1", "inuse_space"); len(got) != 1 {
		t.Fatalf("inuse_space bucket lost its chunk: got %d (want 1)", len(got))
	}
}

// TestNormalizeAndEmptyMeansCPU verifies the "", "cpu", "samples" spellings all
// resolve to the CPU bucket.
func TestNormalizeAndEmptyMeansCPU(t *testing.T) {
	s := NewProfileStore(4, 300, 1800, 1<<20, time.Minute)
	s.EnsureSlot("s1")
	s.AddProfileDataTyped("s1", []string{"cpu"}, []byte("c"))
	for _, req := range []string{"", "cpu", "samples"} {
		if got := s.GetProfileData("s1", req); len(got) != 1 {
			t.Fatalf("request %q did not hit the cpu bucket: got %d", req, len(got))
		}
	}
	// a memory request must not see the cpu chunk
	if got := s.GetProfileData("s1", "alloc_space"); len(got) != 0 {
		t.Fatalf("alloc_space leaked cpu data: got %d", len(got))
	}
}
