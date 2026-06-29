package profiles

import (
	"strconv"
	"time"

	"github.com/odigos-io/odigos/k8sutils/pkg/env"
)

const (
	// Optional env overrides for the UI in-memory profile store.
	envSlotTTLSeconds         = "PROFILES_SLOT_TTL_SECONDS"
	envDataRetentionSeconds   = "PROFILES_SLOT_DATA_RETENTION_SECONDS"
	envMaxSlots               = "PROFILES_MAX_SLOTS"
	envSlotMaxBytes           = "PROFILES_SLOT_MAX_BYTES"
	envCleanupIntervalSeconds = "PROFILES_CLEANUP_INTERVAL_SECONDS"

	// Default settings for in-memory profile store.
	DefaultProfilingMaxSlots = 24
	DefaultProfilingSlotMaxBytes = 8 * 1024 * 1024 // 8 MiB
	// DefaultProfilingSlotTTLSeconds is how long an EMPTY slot (a tab open on a
	// source that produced nothing) is kept after the last request.
	DefaultProfilingSlotTTLSeconds = 120 // seconds
	// DefaultProfilingDataRetentionSeconds is the minimum time a slot that has
	// RECEIVED data is kept past the last chunk, even with no polling — so a
	// populated profile stays visible. Default 10 minutes.
	DefaultProfilingDataRetentionSeconds   = 600 // seconds
	DefaultProfilingCleanupIntervalSeconds = 15  // ProfileStore TTL sweep ticker period (pod-local only)
)

// StoreLimitsFromEnv returns profile store tuning from the UI pod's environment variables,
func StoreLimitsFromEnv() (maxSlots, ttlSeconds, dataRetentionSeconds, slotMaxBytes int, cleanupInterval time.Duration) {
	maxSlots = intFromEnvOrDefault(envMaxSlots, DefaultProfilingMaxSlots)
	ttlSeconds = intFromEnvOrDefault(envSlotTTLSeconds, DefaultProfilingSlotTTLSeconds)
	dataRetentionSeconds = intFromEnvOrDefault(envDataRetentionSeconds, DefaultProfilingDataRetentionSeconds)
	slotMaxBytes = intFromEnvOrDefault(envSlotMaxBytes, DefaultProfilingSlotMaxBytes)
	cleanupInterval = time.Duration(intFromEnvOrDefault(envCleanupIntervalSeconds, DefaultProfilingCleanupIntervalSeconds)) * time.Second
	return
}

func intFromEnvOrDefault(key string, def int) int {
	if v, err := strconv.Atoi(env.GetEnvVarOrDefault(key, strconv.Itoa(def))); err == nil && v > 0 {
		return v
	}
	return def
}
