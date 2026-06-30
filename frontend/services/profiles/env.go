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

	// Default settings for in-memory profile store. Kept in sync with the Helm
	// profiling.ui defaults (helm/odigos/values.yaml); the UI deployment wires those
	// values in as PROFILES_* env, and these are the fallback when env is unset.
	// Cache memory ceiling is MaxSlots*SlotMaxBytes — keep it under the UI mem limit.
	DefaultProfilingMaxSlots = 32
	DefaultProfilingSlotMaxBytes = 10 * 1024 * 1024 // 10 MiB
	// DefaultProfilingSlotTTLSeconds is how long an EMPTY slot (a tab open on a
	// source that produced nothing) is kept after the last request.
	DefaultProfilingSlotTTLSeconds = 300 // seconds (5 min)
	// DefaultProfilingDataRetentionSeconds is the minimum time a slot that has
	// RECEIVED data is kept past the last chunk, even with no polling — so a
	// populated profile stays visible. Default 30 minutes.
	DefaultProfilingDataRetentionSeconds   = 1800 // seconds
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
