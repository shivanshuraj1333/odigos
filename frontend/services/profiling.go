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
