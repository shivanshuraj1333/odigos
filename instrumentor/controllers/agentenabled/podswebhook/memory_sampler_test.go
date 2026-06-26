package podswebhook

import (
	"testing"

	"github.com/odigos-io/odigos/common"

	corev1 "k8s.io/api/core/v1"
)

func envValue(c *corev1.Container, name string) (string, bool) {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

func TestInjectMemorySampler_FreshContainer(t *testing.T) {
	c := &corev1.Container{Name: "app"}
	InjectMemorySampler(c, common.PythonProgrammingLanguage, false)

	if v, ok := envValue(c, "LD_PRELOAD"); !ok || v != memprofDir+"/libmemsample.so" {
		t.Fatalf("LD_PRELOAD = %q, ok=%v", v, ok)
	}
	if v, _ := envValue(c, "ODIGOS_MEMSAMPLE_PREFIX"); v != "/tmp/odigos-memsample" {
		t.Fatalf("prefix = %q", v)
	}
	if _, ok := envValue(c, "USE_ZEND_ALLOC"); ok {
		t.Fatal("USE_ZEND_ALLOC must not be set for non-PHP")
	}
	if len(c.VolumeMounts) == 0 {
		t.Fatal("expected the agents directory to be mounted")
	}
}

func TestInjectMemorySampler_ChainsLdPreload(t *testing.T) {
	c := &corev1.Container{
		Name: "app",
		Env:  []corev1.EnvVar{{Name: "LD_PRELOAD", Value: "/var/odigos/loader/loader.so"}},
	}
	InjectMemorySampler(c, common.RubyProgrammingLanguage, false)

	v, _ := envValue(c, "LD_PRELOAD")
	want := "/var/odigos/loader/loader.so " + memprofDir + "/libmemsample.so"
	if v != want {
		t.Fatalf("LD_PRELOAD = %q, want %q", v, want)
	}
}

func TestInjectMemorySampler_PHPDisablesZendPools_AndMuslSo(t *testing.T) {
	c := &corev1.Container{Name: "app"}
	InjectMemorySampler(c, common.PhpProgrammingLanguage, true)

	if v, _ := envValue(c, "USE_ZEND_ALLOC"); v != "0" {
		t.Fatalf("USE_ZEND_ALLOC = %q, want 0", v)
	}
	if v, _ := envValue(c, "LD_PRELOAD"); v != memprofDir+"/libmemsample-musl.so" {
		t.Fatalf("musl LD_PRELOAD = %q", v)
	}
}

func TestInjectMemorySampler_RespectsUserEnv(t *testing.T) {
	c := &corev1.Container{
		Name: "app",
		Env:  []corev1.EnvVar{{Name: "ODIGOS_MEMSAMPLE_INTERVAL", Value: "1048576"}},
	}
	InjectMemorySampler(c, common.CPlusPlusProgrammingLanguage, false)
	if v, _ := envValue(c, "ODIGOS_MEMSAMPLE_INTERVAL"); v != "1048576" {
		t.Fatalf("user interval overwritten: %q", v)
	}
}

func TestMemorySamplerLanguage(t *testing.T) {
	preload := []common.ProgrammingLanguage{
		common.PythonProgrammingLanguage, common.RubyProgrammingLanguage,
		common.PhpProgrammingLanguage, common.CPlusPlusProgrammingLanguage,
		common.RustProgrammingLanguage,
	}
	for _, l := range preload {
		if !MemorySamplerLanguage(l) {
			t.Errorf("%s should need the sampler", l)
		}
	}
	// Runtime-protocol languages must NOT be preloaded.
	for _, l := range []common.ProgrammingLanguage{
		common.GoProgrammingLanguage, common.JavaProgrammingLanguage,
		common.DotNetProgrammingLanguage, common.JavascriptProgrammingLanguage,
	} {
		if MemorySamplerLanguage(l) {
			t.Errorf("%s should NOT be preloaded (runtime protocol)", l)
		}
	}
}
