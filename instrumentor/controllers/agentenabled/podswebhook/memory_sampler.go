package podswebhook

import (
	"strings"

	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/common"
	commonconsts "github.com/odigos-io/odigos/common/consts"

	corev1 "k8s.io/api/core/v1"
)

// memprofDir is where the memory-profiling allocation interposer (libmemsample)
// is delivered inside the odigos agents bundle.
const memprofDir = k8sconsts.OdigosAgentsDirectory + "/memprof"

// MemorySamplerLanguage reports whether a runtime needs the libmemsample
// interposer preloaded for memory profiling. Go/Java/.NET/Node are profiled
// out-of-process via their own runtime protocols and need no workload change, so
// they are intentionally excluded here.
func MemorySamplerLanguage(lang common.ProgrammingLanguage) bool {
	switch lang {
	case common.PythonProgrammingLanguage,
		common.RubyProgrammingLanguage,
		common.PhpProgrammingLanguage,
		common.CPlusPlusProgrammingLanguage,
		common.RustProgrammingLanguage:
		return true
	default:
		return false
	}
}

// InjectMemorySampler wires the libmemsample allocation interposer into a
// container so the out-of-process memory profiler can consume its heap dumps:
//   - LD_PRELOAD, chained after any value already present (odigos may preload its
//     own loader; LD_PRELOAD is a space-separated list);
//   - the sampler configuration env;
//   - for PHP, USE_ZEND_ALLOC=0 so emalloc routes through malloc where the
//     interposer can see it (ZendMM pools hide allocations otherwise).
//
// The .so ships in the odigos agents bundle (mounted at OdigosAgentsDirectory).
// musl selects the musl-linked build for Alpine-based images.
func InjectMemorySampler(container *corev1.Container, lang common.ProgrammingLanguage, musl bool) {
	so := memprofDir + "/libmemsample.so"
	if musl {
		so = memprofDir + "/libmemsample-musl.so"
	}

	appendToListEnv(container, commonconsts.LdPreloadEnvVarName, so, " ")
	setEnvIfAbsent(container, "ODIGOS_MEMSAMPLE_PREFIX", "/tmp/odigos-memsample")
	setEnvIfAbsent(container, "ODIGOS_MEMSAMPLE_INTERVAL", "262144")
	setEnvIfAbsent(container, "ODIGOS_MEMSAMPLE_PERIOD_MS", "2000")
	if lang == common.PhpProgrammingLanguage {
		setEnvIfAbsent(container, "USE_ZEND_ALLOC", "0")
	}

	MountDirectory(container, k8sconsts.OdigosAgentsDirectory)
}

// appendToListEnv appends value to a separator-delimited list env var, creating
// it if absent and never duplicating. If the var uses valueFrom (not a literal
// value) it is left untouched, since a list cannot be appended to a reference.
func appendToListEnv(c *corev1.Container, name, value, sep string) {
	for i := range c.Env {
		if c.Env[i].Name != name {
			continue
		}
		if c.Env[i].ValueFrom != nil {
			return
		}
		switch {
		case c.Env[i].Value == "":
			c.Env[i].Value = value
		case !strings.Contains(c.Env[i].Value, value):
			c.Env[i].Value += sep + value
		}
		return
	}
	c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
}

// setEnvIfAbsent sets an env var only when the container does not already define
// it, so an explicit user/manifest value always wins.
func setEnvIfAbsent(c *corev1.Container, name, value string) {
	for i := range c.Env {
		if c.Env[i].Name == name {
			return
		}
	}
	c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
}
