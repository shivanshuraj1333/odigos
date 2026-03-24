package services

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/odigos-io/odigos/common"
	collectorprofiles "github.com/odigos-io/odigos/frontend/services/collector_profiles"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const envEnableProfilesReceiver = "ENABLE_PROFILES_RECEIVER"

// ResolveProfilingFromEffectiveConfig loads the OTLP profiles receiver enablement and store limits.
// Primary source: effective-config ConfigMap (profiling.enabled, profiling.ui.*).
// Fallbacks: ENABLE_PROFILES_RECEIVER and PROFILES_* env when effective config is missing or during migration.
func ResolveProfilingFromEffectiveConfig(ctx context.Context, c client.Client) (receiverOn bool, maxSlots, ttlSec, slotMaxBytes int, cleanupInterval time.Duration, err error) {
	maxSlots, ttlSec, slotMaxBytes, cleanup := collectorprofiles.StoreConfigFromEnv()

	cfg, err := GetEffectiveConfig(ctx, c)
	if err != nil {
		return envProfilesReceiverOn(), maxSlots, ttlSec, slotMaxBytes, cleanup, err
	}
	if cfg == nil {
		return envProfilesReceiverOn(), maxSlots, ttlSec, slotMaxBytes, cleanup, nil
	}

	if cfg.ProfilingEnabled() {
		applyProfilingUiOverrides(cfg, &maxSlots, &ttlSec, &slotMaxBytes)
		return true, maxSlots, ttlSec, slotMaxBytes, cleanup, nil
	}

	if envProfilesReceiverOn() {
		return true, maxSlots, ttlSec, slotMaxBytes, cleanup, nil
	}
	return false, maxSlots, ttlSec, slotMaxBytes, cleanup, nil
}

func envProfilesReceiverOn() bool {
	v := os.Getenv(envEnableProfilesReceiver)
	if v == "" {
		return false
	}
	on, err := strconv.ParseBool(v)
	return err == nil && on
}

func applyProfilingUiOverrides(cfg *common.OdigosConfiguration, maxSlots, ttlSec, slotMaxBytes *int) {
	if cfg == nil || cfg.Profiling == nil || cfg.Profiling.Ui == nil {
		return
	}
	u := cfg.Profiling.Ui
	if u.MaxSlots > 0 {
		*maxSlots = u.MaxSlots
	}
	if u.SlotTTLSeconds > 0 {
		*ttlSec = u.SlotTTLSeconds
	}
	if u.SlotMaxBytes > 0 {
		*slotMaxBytes = u.SlotMaxBytes
	}
}
