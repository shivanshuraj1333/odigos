package podswebhook

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestInjectJavaMemoryProfiling(t *testing.T) {
	c := &corev1.Container{Name: "app"}
	got := InjectJavaMemoryProfiling(GetEnvVarNamesSet(c), c)

	if _, ok := got[jdkJavaOptionsEnvVar]; !ok {
		t.Fatalf("expected %s to be tracked as injected", jdkJavaOptionsEnvVar)
	}
	if len(c.Env) != 1 || c.Env[0].Name != jdkJavaOptionsEnvVar {
		t.Fatalf("expected one %s env var, got %+v", jdkJavaOptionsEnvVar, c.Env)
	}
	v := c.Env[0].Value
	for _, want := range []string{"StartFlightRecording", "name=odigos", "old-object-queue-size"} {
		if !strings.Contains(v, want) {
			t.Errorf("JFR flags missing %q: %s", want, v)
		}
	}
}

func TestInjectNativeMemoryProfiling(t *testing.T) {
	// glibc container: preload=true → both LD_PRELOAD and MALLOC_CONF.
	c := &corev1.Container{Name: "svc"}
	InjectNativeMemoryProfiling(GetEnvVarNamesSet(c), c, true)

	env := map[string]string{}
	for _, e := range c.Env {
		env[e.Name] = e.Value
	}
	if env[ldPreloadEnvVar] != jemallocProfSoPath {
		t.Errorf("LD_PRELOAD = %q, want %q", env[ldPreloadEnvVar], jemallocProfSoPath)
	}
	for _, want := range []string{"prof:true", "lg_prof_sample:19", "prof_prefix:/tmp/odigos-jeprof"} {
		if !strings.Contains(env[mallocConfEnvVar], want) {
			t.Errorf("MALLOC_CONF missing %q: %s", want, env[mallocConfEnvVar])
		}
	}
}

func TestInjectNativeMemoryProfiling_MuslNoPreload(t *testing.T) {
	// musl/unknown libc: preload=false → MALLOC_CONF only, NEVER LD_PRELOAD
	// (musl's loader aborts the app on an incompatible preload). This is the
	// crash-safety guarantee.
	c := &corev1.Container{Name: "svc"}
	InjectNativeMemoryProfiling(GetEnvVarNamesSet(c), c, false)

	for _, e := range c.Env {
		if e.Name == ldPreloadEnvVar {
			t.Fatalf("must NOT inject LD_PRELOAD when preload=false (musl crash risk), got %q", e.Value)
		}
	}
	var hasConf bool
	for _, e := range c.Env {
		if e.Name == mallocConfEnvVar {
			hasConf = true
		}
	}
	if !hasConf {
		t.Fatalf("expected MALLOC_CONF to still be injected (harmless, helps jemalloc-prof apps)")
	}
}

func TestInjectNativeMemoryProfiling_NoOpWhenPresent(t *testing.T) {
	// App with its own allocator/LD_PRELOAD must not be clobbered.
	c := &corev1.Container{
		Name: "svc",
		Env:  []corev1.EnvVar{{Name: ldPreloadEnvVar, Value: "/opt/mymalloc.so"}},
	}
	InjectNativeMemoryProfiling(GetEnvVarNamesSet(c), c, true)
	for _, e := range c.Env {
		if e.Name == ldPreloadEnvVar && e.Value != "/opt/mymalloc.so" {
			t.Fatalf("must not overwrite existing LD_PRELOAD, got %q", e.Value)
		}
	}
}

func TestInjectJavaMemoryProfiling_NoOpWhenPresent(t *testing.T) {
	// If the container already sets JDK_JAVA_OPTIONS, we must not clobber it.
	c := &corev1.Container{
		Name: "app",
		Env:  []corev1.EnvVar{{Name: jdkJavaOptionsEnvVar, Value: "-Xss512k"}},
	}
	InjectJavaMemoryProfiling(GetEnvVarNamesSet(c), c)
	if len(c.Env) != 1 || c.Env[0].Value != "-Xss512k" {
		t.Fatalf("must not overwrite existing %s, got %+v", jdkJavaOptionsEnvVar, c.Env)
	}
}
