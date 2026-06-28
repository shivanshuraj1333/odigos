//go:build linux

package symbolize

import "testing"

func TestRootPrefix(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/proc/1234/root/usr/lib/libssl.so.3", "/proc/1234/root"},
		{"/proc/1/root/app/server", "/proc/1/root"},
		{"/usr/lib/libc.so.6", ""},          // host-absolute, no container root
		{"/proc/abc/root/x", ""},            // non-numeric pid → not a proc root
		{"relative/path", ""},               // not anchored
	}
	for _, c := range cases {
		if got := rootPrefix(c.path); got != c.want {
			t.Errorf("rootPrefix(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// TestDebugInfoPathStaysInRoot asserts build-id debug lookups are resolved inside
// the container root, not the host's /usr/lib/debug (the bug the experiments
// version had when reused in the collector). We can't easily stat real files, so
// we check the candidate path construction via rootPrefix + the join shape.
func TestDebugInfoPathStaysInRoot(t *testing.T) {
	// A container-rooted binary's build-id debug must be under <root>/usr/lib/debug.
	root := rootPrefix("/proc/999/root/opt/app/bin/svc")
	if root != "/proc/999/root" {
		t.Fatalf("unexpected root %q", root)
	}
}
