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
	MaxSlots             int
	SlotTTLSeconds       int
	DataRetentionSeconds int
	SlotMaxBytes         int
}

// ProfilingRuntimeConfig is the UI process decision for OTLP profiling ingest and store sizing.
type ProfilingRuntimeConfig struct {
	ReceiverOn      bool
	StoreLimits     ProfilingStoreLimits
	CleanupInterval time.Duration // ProfileStore TTL sweep period
}

func ResolveProfilingFromEffectiveConfig(ctx context.Context, c client.Client) (ProfilingRuntimeConfig, error) {
	maxSlots, ttlSec, dataRetentionSec, slotMaxBytes, cleanup := profiles.StoreLimitsFromEnv()
	out := ProfilingRuntimeConfig{
		StoreLimits: ProfilingStoreLimits{
			MaxSlots:             maxSlots,
			SlotTTLSeconds:       ttlSec,
			DataRetentionSeconds: dataRetentionSec,
			SlotMaxBytes:         slotMaxBytes,
		},
		CleanupInterval: cleanup,
	}

	cfg, err := GetEffectiveConfig(ctx, c)
	if err != nil {
		return out, err
	}

	// Effective-config (profiling.ui) overrides the env/defaults, so the Helm
	// knobs actually drive the store. Without this the UI store was stuck on the
	// built-in defaults (env vars are not set on the pod) and the configured
	// slotTTLSeconds was inert — which is why populated profiles aged out fast.
	if cfg != nil && cfg.Profiling != nil && cfg.Profiling.UI != nil {
		ui := cfg.Profiling.UI
		if ui.MaxSlots > 0 {
			out.StoreLimits.MaxSlots = ui.MaxSlots
		}
		if ui.SlotTTLSeconds > 0 {
			out.StoreLimits.SlotTTLSeconds = ui.SlotTTLSeconds
		}
		if ui.DataRetentionSeconds > 0 {
			out.StoreLimits.DataRetentionSeconds = ui.DataRetentionSeconds
		}
		if ui.SlotMaxBytes > 0 {
			out.StoreLimits.SlotMaxBytes = ui.SlotMaxBytes
		}
	}

	if ProfilingEnabledFromOdigosConfig(cfg) {
		out.ReceiverOn = true
	}
	return out, nil
}

// ProfilingEnabledFromOdigosConfig reports whether the UI should accept OTLP profiles for this config snapshot.
func ProfilingEnabledFromOdigosConfig(cfg *common.OdigosConfiguration) bool {
	return cfg != nil && cfg.ProfilingEnabled()
}
