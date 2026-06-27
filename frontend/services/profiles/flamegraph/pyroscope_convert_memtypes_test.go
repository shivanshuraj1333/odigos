package flamegraph

import "testing"

func TestNormalizeProfileType_AllFourHeapTypesPassThrough(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"cpu", SampleTypeCPU},
		{"", SampleTypeCPU},                      // empty -> cpu (never regress)
		{"garbage", SampleTypeCPU},               // unknown -> cpu
		{"alloc_space", SampleTypeAllocSpace},    // the four heap types pass through
		{"alloc_objects", SampleTypeAllocObjects},
		{"inuse_space", SampleTypeInuseSpace},
		{"inuse_objects", SampleTypeInuseObjects},
	} {
		if got := NormalizeProfileType(tc.in); got != tc.want {
			t.Errorf("NormalizeProfileType(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsMemoryType(t *testing.T) {
	mem := []string{"alloc_space", "alloc_objects", "inuse_space", "inuse_objects"}
	for _, m := range mem {
		if !IsMemoryType(m) {
			t.Errorf("IsMemoryType(%q)=false want true", m)
		}
	}
	for _, c := range []string{"cpu", "", "samples"} {
		if IsMemoryType(c) {
			t.Errorf("IsMemoryType(%q)=true want false", c)
		}
	}
}

func TestIsObjectCountType(t *testing.T) {
	for _, o := range []string{"alloc_objects", "inuse_objects"} {
		if !IsObjectCountType(o) {
			t.Errorf("IsObjectCountType(%q)=false want true", o)
		}
	}
	for _, s := range []string{"alloc_space", "inuse_space", "cpu"} {
		if IsObjectCountType(s) {
			t.Errorf("IsObjectCountType(%q)=true want false (space/cpu are not object counts)", s)
		}
	}
}
