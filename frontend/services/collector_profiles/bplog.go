package collectorprofiles

import "log"

const backendProfilingPrefix = "[backend-profiling]"

// bpInfof logs a pipeline trace line (always on; use for deploy / E2E visibility).
func bpInfof(format string, args ...interface{}) {
	log.Printf(backendProfilingPrefix+" "+format, args...)
}
