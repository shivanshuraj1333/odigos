package services

import (
	"context"
	"time"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/frontend/services/profiles"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProfilingStoreLimits holds resolved limits for the in-memory profile store.
type ProfilingStoreLimits struct {
	MaxSlots       int
	SlotTTLSeconds int
	SlotMaxBytes   int
}

// ProfilingRuntimeConfig is the UI process decision for OTLP profiling ingest and store sizing.
type ProfilingRuntimeConfig struct {
	ReceiverOn      bool
	StoreLimits     ProfilingStoreLimits
	CleanupInterval time.Duration // ProfileStore TTL sweep period
}

// ResolveProfilingFromEffectiveConfig loads effective (or Helm fallback) config to decide whether
// profiling OTLP ingest is enabled and applies profiling.ui store limits when present.
func ResolveProfilingFromEffectiveConfig(ctx context.Context, c client.Client) (ProfilingRuntimeConfig, error) {
	maxSlots, ttlSec, slotMaxBytes, cleanup := profiles.StoreLimitsFromEnv()
	out := ProfilingRuntimeConfig{
		StoreLimits: ProfilingStoreLimits{
			MaxSlots:       maxSlots,
			SlotTTLSeconds: ttlSec,
			SlotMaxBytes:   slotMaxBytes,
		},
		CleanupInterval: cleanup,
	}

	cfg, err := GetEffectiveConfig(ctx, c)
	if err != nil {
		return out, err
	}

	var useCfg *common.OdigosConfiguration
	if cfg != nil {
		if cfg.Profiling != nil && cfg.Profiling.Enabled != nil && !*cfg.Profiling.Enabled {
			return out, nil
		}
		if cfg.ProfilingEnabled() {
			useCfg = cfg
		}
	}
	if useCfg == nil {
		helmCfg, helmErr := GetHelmDeploymentConfig(ctx, c)
		if helmErr != nil {
			return out, helmErr
		}
		if helmCfg != nil && helmCfg.ProfilingEnabled() {
			useCfg = helmCfg
		}
	}

	applyProfilingUiOverrides(useCfg, &out.StoreLimits.MaxSlots, &out.StoreLimits.SlotTTLSeconds, &out.StoreLimits.SlotMaxBytes)
	out.ReceiverOn = useCfg != nil
	return out, nil
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
