package services

import (
	"context"
	"time"

	"github.com/odigos-io/odigos/common"
	odigosconsts "github.com/odigos-io/odigos/common/consts"
	collectorprofiles "github.com/odigos-io/odigos/frontend/services/collector_profiles"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveProfilingFromEffectiveConfig loads profiling receiver enablement, OTLP gRPC port, and store limits.
func ResolveProfilingFromEffectiveConfig(ctx context.Context, c client.Client) (receiverOn bool, otlpGrpcPort int, maxSlots, ttlSec, slotMaxBytes int, cleanupInterval time.Duration, err error) {
	maxSlots, ttlSec, slotMaxBytes, cleanup := collectorprofiles.StoreConfigFromEnv()
	otlpGrpcPort = odigosconsts.OTLPPort

	cfg, err := GetEffectiveConfig(ctx, c)
	if err != nil {
		return false, otlpGrpcPort, maxSlots, ttlSec, slotMaxBytes, cleanup, err
	}
	if cfg == nil {
		return false, otlpGrpcPort, maxSlots, ttlSec, slotMaxBytes, cleanup, nil
	}
	otlpGrpcPort = odigosconsts.OTLPPort

	if cfg.ProfilingEnabled() {
		applyProfilingUiOverrides(cfg, &maxSlots, &ttlSec, &slotMaxBytes)
		return true, otlpGrpcPort, maxSlots, ttlSec, slotMaxBytes, cleanup, nil
	}

	return false, otlpGrpcPort, maxSlots, ttlSec, slotMaxBytes, cleanup, nil
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
