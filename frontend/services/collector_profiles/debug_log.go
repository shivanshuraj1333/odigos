package collectorprofiles

import (
	"log"
	"os"
	"strings"
)

// profilingLogEnabled is true when PROFILE_DEBUG_LOG / PROFILING_DEBUG is enabled.
func profilingLogEnabled() bool {
	v := strings.TrimSpace(os.Getenv("PROFILE_DEBUG_LOG"))
	if v == "" {
		v = strings.TrimSpace(os.Getenv("PROFILING_DEBUG"))
	}
	v = strings.ToLower(v)
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

func profilingDebugLog(format string, args ...interface{}) {
	if !profilingLogEnabled() {
		return
	}
	log.Printf("[backend-profiling] debug "+format, args...)
}
