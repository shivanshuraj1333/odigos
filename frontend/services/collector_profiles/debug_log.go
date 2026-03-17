package collectorprofiles

import (
	"log"
	"os"
)

func profilingDebugLog(format string, args ...interface{}) {
	if os.Getenv("PROFILING_DEBUG") == "" {
		return
	}
	log.Printf(format, args...)
}
