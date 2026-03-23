package collectorprofiles

import (
	"os"
	"strconv"
	"time"
)

const (
	envSlotTTLSeconds         = "PROFILES_SLOT_TTL_SECONDS"
	envMaxSlots               = "PROFILES_MAX_SLOTS" // max services with profiling enabled at once (default 10)
	envSlotMaxBytes           = "PROFILES_SLOT_MAX_BYTES"
	envStoreMaxTotalBytes     = "PROFILES_STORE_MAX_TOTAL_BYTES"
	envCleanupIntervalSeconds = "PROFILES_CLEANUP_INTERVAL_SECONDS"
	envProfilesEnabled        = "ENABLE_PROFILES_RECEIVER"
)

// ReceiverEnabled reports whether the OTLP profiles consumer should be registered on the UI OTLP gRPC server.
// When false, profile data is not accepted (metrics still use :4317).
func ReceiverEnabled() bool {
	v := os.Getenv(envProfilesEnabled)
	if v == "" {
		return true
	}
	enabled, _ := strconv.ParseBool(v)
	return enabled
}

// StoreConfigFromEnv reads profiling store settings from environment variables.
// Unset or invalid values use package defaults (defaultMaxSlots, defaultSlotTTLSeconds, etc.).
// maxTotalBytes defaults to maxSlots*slotMaxBytes when unset or invalid (hard cap on aggregate buffered profile bytes).
func StoreConfigFromEnv() (maxSlots, ttlSeconds, slotMaxBytes, maxTotalBytes int, cleanupInterval time.Duration) {
	maxSlots = intFromEnv(envMaxSlots, defaultMaxSlots)
	ttlSeconds = intFromEnv(envSlotTTLSeconds, defaultSlotTTLSeconds)
	slotMaxBytes = intFromEnv(envSlotMaxBytes, defaultSlotMaxBytes)
	sec := intFromEnv(envCleanupIntervalSeconds, int(defaultCleanupInt/time.Second))
	cleanupInterval = time.Duration(sec) * time.Second
	maxTotalBytes = intFromEnv(envStoreMaxTotalBytes, 0)
	if maxTotalBytes <= 0 && maxSlots > 0 && slotMaxBytes > 0 {
		maxTotalBytes = maxSlots * slotMaxBytes
	}
	return maxSlots, ttlSeconds, slotMaxBytes, maxTotalBytes, cleanupInterval
}

func intFromEnv(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
