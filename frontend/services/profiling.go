package services

import (
	"context"
	"time"

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

// ResolveProfilingFromEffectiveConfig reads effective config to decide whether profiling OTLP
// ingest is enabled. Store limits (max slots, slot TTL, slot max bytes) come exclusively from
// the UI pod's environment variables (PROFILES_MAX_SLOTS, PROFILES_SLOT_TTL_SECONDS,
// PROFILES_SLOT_MAX_BYTES) — no Helm or ConfigMap plumbing needed for cache tuning.
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

	// [srs:review] [start]
	// TODO: remove once the minimum supported scheduler version includes profiling support.
	// effective-config is written by the scheduler; older versions don't know about the
	// profiling field and silently drop it on marshal. Fall back to odigos-configuration
	// (the raw helm-managed config) so profiling works during rolling upgrades.
	if cfg != nil && cfg.Profiling == nil {
		helmCfg, helmErr := GetHelmDeploymentConfig(ctx, c)
		if helmErr == nil && helmCfg != nil && helmCfg.Profiling != nil {
			cfg.Profiling = helmCfg.Profiling
		}
	}
	// [srs:review] [end]

	if cfg != nil {
		if cfg.Profiling != nil && cfg.Profiling.Enabled != nil && !*cfg.Profiling.Enabled {
			return out, nil
		}
		if cfg.ProfilingEnabled() {
			out.ReceiverOn = true
		}
	}
	return out, nil
}
