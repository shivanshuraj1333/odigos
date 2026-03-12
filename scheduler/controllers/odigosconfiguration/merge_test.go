package odigosconfiguration

import (
	"testing"

	"github.com/odigos-io/odigos/common"
)

func boolPtr(b bool) *bool   { return &b }
func strPtr(s string) *string { return &s }
func f64Ptr(f float64) *float64 { return &f }

// TestMergeConfigs_BaseOnly verifies that calling mergeConfigs with a nil additional
// config leaves the base untouched.
func TestMergeConfigs_NilAdditional(t *testing.T) {
	base := &common.OdigosConfiguration{
		ConfigVersion: 1,
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:    common.LogLevelDebug,
			Autoscaler: common.LogLevelWarn,
		},
	}
	mergeConfigs(base, nil)

	if base.ComponentLogLevels.Default != common.LogLevelDebug {
		t.Errorf("Default changed: got %q, want %q", base.ComponentLogLevels.Default, common.LogLevelDebug)
	}
	if base.ComponentLogLevels.Autoscaler != common.LogLevelWarn {
		t.Errorf("Autoscaler changed: got %q, want %q", base.ComponentLogLevels.Autoscaler, common.LogLevelWarn)
	}
}

// TestMergeConfigs_AdditionalDefault verifies that additional.Default overwrites base.Default.
func TestMergeConfigs_AdditionalDefaultOverwritesBase(t *testing.T) {
	base := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default: common.LogLevelInfo,
		},
	}
	additional := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default: common.LogLevelDebug,
		},
	}
	mergeConfigs(base, additional)

	if base.ComponentLogLevels.Default != common.LogLevelDebug {
		t.Errorf("Default: got %q, want %q", base.ComponentLogLevels.Default, common.LogLevelDebug)
	}
}

// TestMergeConfigs_AdditionalPerComponent verifies that a per-component override in
// additional only touches that component; other fields are left unchanged.
func TestMergeConfigs_AdditionalPerComponentOnlyTouchesThatField(t *testing.T) {
	base := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:      common.LogLevelInfo,
			Autoscaler:   common.LogLevelWarn,
			Scheduler:    common.LogLevelError,
			Instrumentor: common.LogLevelDebug,
		},
	}
	additional := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Autoscaler: common.LogLevelDebug,
		},
	}
	mergeConfigs(base, additional)

	if base.ComponentLogLevels.Autoscaler != common.LogLevelDebug {
		t.Errorf("Autoscaler: got %q, want %q", base.ComponentLogLevels.Autoscaler, common.LogLevelDebug)
	}
	// Unchanged fields
	if base.ComponentLogLevels.Default != common.LogLevelInfo {
		t.Errorf("Default changed unexpectedly: got %q", base.ComponentLogLevels.Default)
	}
	if base.ComponentLogLevels.Scheduler != common.LogLevelError {
		t.Errorf("Scheduler changed unexpectedly: got %q", base.ComponentLogLevels.Scheduler)
	}
	if base.ComponentLogLevels.Instrumentor != common.LogLevelDebug {
		t.Errorf("Instrumentor changed unexpectedly: got %q", base.ComponentLogLevels.Instrumentor)
	}
}

// TestMergeConfigs_NilComponentLogLevels verifies that when additional.ComponentLogLevels
// is nil, base.ComponentLogLevels is preserved entirely.
func TestMergeConfigs_NilAdditionalComponentLogLevels(t *testing.T) {
	base := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:    common.LogLevelWarn,
			Autoscaler: common.LogLevelDebug,
		},
	}
	additional := &common.OdigosConfiguration{
		// ComponentLogLevels intentionally nil
	}
	mergeConfigs(base, additional)

	if base.ComponentLogLevels == nil {
		t.Fatal("base.ComponentLogLevels was unexpectedly set to nil")
	}
	if base.ComponentLogLevels.Default != common.LogLevelWarn {
		t.Errorf("Default changed: got %q", base.ComponentLogLevels.Default)
	}
	if base.ComponentLogLevels.Autoscaler != common.LogLevelDebug {
		t.Errorf("Autoscaler changed: got %q", base.ComponentLogLevels.Autoscaler)
	}
}

// TestMergeConfigs_NonDestructiveEmptyStrings verifies that an empty string in
// additional does NOT clear the corresponding field in base.
func TestMergeConfigs_NonDestructiveEmptyStrings(t *testing.T) {
	base := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:    common.LogLevelWarn,
			Autoscaler: common.LogLevelDebug,
			Scheduler:  common.LogLevelError,
		},
	}
	// additional has ComponentLogLevels set (non-nil) but Autoscaler is empty string
	additional := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:    common.LogLevelInfo, // this should overwrite
			Autoscaler: "",                  // empty — must NOT clear base Autoscaler
		},
	}
	mergeConfigs(base, additional)

	// Default should be updated
	if base.ComponentLogLevels.Default != common.LogLevelInfo {
		t.Errorf("Default: got %q, want %q", base.ComponentLogLevels.Default, common.LogLevelInfo)
	}
	// Autoscaler must remain untouched (empty string in additional must not overwrite)
	if base.ComponentLogLevels.Autoscaler != common.LogLevelDebug {
		t.Errorf("Autoscaler was cleared by empty string: got %q, want %q", base.ComponentLogLevels.Autoscaler, common.LogLevelDebug)
	}
	// Scheduler not in additional — must remain
	if base.ComponentLogLevels.Scheduler != common.LogLevelError {
		t.Errorf("Scheduler changed unexpectedly: got %q", base.ComponentLogLevels.Scheduler)
	}
}

// TestMergeConfigs_LayeredMerge simulates the three-layer merge used in production:
//   base=helm → merged with remote → merged with local-ui.
//   local-ui per-component value must win over all previous layers.
func TestMergeConfigs_LayeredMerge(t *testing.T) {
	// Layer 1: helm config
	helmConfig := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:    common.LogLevelInfo,
			Autoscaler: common.LogLevelWarn,
		},
	}

	// Layer 2: remote config — sets Scheduler
	remoteConfig := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Scheduler: common.LogLevelError,
		},
	}

	// Layer 3: local-ui config — overrides Autoscaler (most important, merged last)
	localUiConfig := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Autoscaler: common.LogLevelDebug,
		},
	}

	mergeConfigs(helmConfig, remoteConfig)
	mergeConfigs(helmConfig, localUiConfig)

	if helmConfig.ComponentLogLevels.Default != common.LogLevelInfo {
		t.Errorf("Default: got %q, want %q", helmConfig.ComponentLogLevels.Default, common.LogLevelInfo)
	}
	if helmConfig.ComponentLogLevels.Autoscaler != common.LogLevelDebug {
		t.Errorf("Autoscaler: got %q, want %q (local-ui should win)", helmConfig.ComponentLogLevels.Autoscaler, common.LogLevelDebug)
	}
	if helmConfig.ComponentLogLevels.Scheduler != common.LogLevelError {
		t.Errorf("Scheduler: got %q, want %q (from remote)", helmConfig.ComponentLogLevels.Scheduler, common.LogLevelError)
	}
}

// TestMergeConfigs_AllComponentFields verifies all 8 per-component fields are merged correctly.
func TestMergeConfigs_AllComponentFields(t *testing.T) {
	base := &common.OdigosConfiguration{}
	additional := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:      common.LogLevelInfo,
			Autoscaler:   common.LogLevelDebug,
			Scheduler:    common.LogLevelWarn,
			Instrumentor: common.LogLevelError,
			Odiglet:      common.LogLevelDebug,
			Deviceplugin: common.LogLevelWarn,
			UI:           common.LogLevelError,
			Collector:    common.LogLevelDebug,
		},
	}
	mergeConfigs(base, additional)

	if base.ComponentLogLevels == nil {
		t.Fatal("base.ComponentLogLevels is nil after merge")
	}
	checks := map[string]struct {
		got  common.OdigosLogLevel
		want common.OdigosLogLevel
	}{
		"Default":      {base.ComponentLogLevels.Default, common.LogLevelInfo},
		"Autoscaler":   {base.ComponentLogLevels.Autoscaler, common.LogLevelDebug},
		"Scheduler":    {base.ComponentLogLevels.Scheduler, common.LogLevelWarn},
		"Instrumentor": {base.ComponentLogLevels.Instrumentor, common.LogLevelError},
		"Odiglet":      {base.ComponentLogLevels.Odiglet, common.LogLevelDebug},
		"Deviceplugin": {base.ComponentLogLevels.Deviceplugin, common.LogLevelWarn},
		"UI":           {base.ComponentLogLevels.UI, common.LogLevelError},
		"Collector":    {base.ComponentLogLevels.Collector, common.LogLevelDebug},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", field, c.got, c.want)
		}
	}
}

// TestMergeConfigs_BaseNilComponentLogLevels verifies that when base.ComponentLogLevels
// is nil and additional has a value, base gets the new ComponentLogLevels struct.
func TestMergeConfigs_BaseNilComponentLogLevelsCreated(t *testing.T) {
	base := &common.OdigosConfiguration{}
	additional := &common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default: common.LogLevelDebug,
		},
	}
	mergeConfigs(base, additional)

	if base.ComponentLogLevels == nil {
		t.Fatal("base.ComponentLogLevels should have been created")
	}
	if base.ComponentLogLevels.Default != common.LogLevelDebug {
		t.Errorf("Default: got %q, want %q", base.ComponentLogLevels.Default, common.LogLevelDebug)
	}
}

// TestMergeConfigs_Rollout verifies that Rollout fields are merged properly.
func TestMergeConfigs_RolloutMerge(t *testing.T) {
	disabled := boolPtr(true)
	base := &common.OdigosConfiguration{
		Rollout: &common.RolloutConfiguration{
			AutomaticRolloutDisabled: boolPtr(false),
			MaxConcurrentRollouts:    3,
		},
	}
	additional := &common.OdigosConfiguration{
		Rollout: &common.RolloutConfiguration{
			AutomaticRolloutDisabled: disabled,
		},
	}
	mergeConfigs(base, additional)

	if base.Rollout == nil {
		t.Fatal("base.Rollout is nil after merge")
	}
	if base.Rollout.AutomaticRolloutDisabled == nil || *base.Rollout.AutomaticRolloutDisabled != true {
		t.Errorf("AutomaticRolloutDisabled: got %v, want true", base.Rollout.AutomaticRolloutDisabled)
	}
	// MaxConcurrentRollouts is not merged (additional is 0/unset) — base value is unchanged
	if base.Rollout.MaxConcurrentRollouts != 3 {
		t.Errorf("MaxConcurrentRollouts changed unexpectedly: got %d", base.Rollout.MaxConcurrentRollouts)
	}
}

// TestMergeConfigs_RolloutNilAdditional verifies that a nil additional.Rollout
// leaves base.Rollout untouched.
func TestMergeConfigs_RolloutNilAdditional(t *testing.T) {
	base := &common.OdigosConfiguration{
		Rollout: &common.RolloutConfiguration{
			AutomaticRolloutDisabled: boolPtr(true),
		},
	}
	additional := &common.OdigosConfiguration{}
	mergeConfigs(base, additional)

	if base.Rollout == nil {
		t.Fatal("base.Rollout became nil")
	}
	if base.Rollout.AutomaticRolloutDisabled == nil || *base.Rollout.AutomaticRolloutDisabled != true {
		t.Error("AutomaticRolloutDisabled changed unexpectedly")
	}
}

// TestMergeConfigs_SamplingTailSampling verifies tail-sampling fields are merged.
func TestMergeConfigs_SamplingTailSamplingMerge(t *testing.T) {
	dur := strPtr("30s")
	base := &common.OdigosConfiguration{
		Sampling: &common.SamplingConfiguration{
			TailSampling: &common.TailSamplingConfiguration{
				Disabled:                     boolPtr(false),
				TraceAggregationWaitDuration: strPtr("10s"),
			},
		},
	}
	additional := &common.OdigosConfiguration{
		Sampling: &common.SamplingConfiguration{
			TailSampling: &common.TailSamplingConfiguration{
				Disabled:                     boolPtr(true),
				TraceAggregationWaitDuration: dur,
			},
		},
	}
	mergeConfigs(base, additional)

	if base.Sampling == nil || base.Sampling.TailSampling == nil {
		t.Fatal("base.Sampling.TailSampling is nil after merge")
	}
	if base.Sampling.TailSampling.Disabled == nil || *base.Sampling.TailSampling.Disabled != true {
		t.Errorf("Disabled: expected true after merge")
	}
	if base.Sampling.TailSampling.TraceAggregationWaitDuration == nil || *base.Sampling.TailSampling.TraceAggregationWaitDuration != "30s" {
		t.Errorf("TraceAggregationWaitDuration: got %v, want 30s", base.Sampling.TailSampling.TraceAggregationWaitDuration)
	}
}

// TestMergeConfigs_SamplingK8sHealthProbes verifies K8sHealthProbesSampling fields are merged.
func TestMergeConfigs_SamplingK8sHealthProbesMerge(t *testing.T) {
	pct := f64Ptr(5.0)
	base := &common.OdigosConfiguration{
		Sampling: &common.SamplingConfiguration{
			K8sHealthProbesSampling: &common.K8sHealthProbesSamplingConfiguration{
				Enabled:         boolPtr(false),
				KeepPercentage:  f64Ptr(0.0),
			},
		},
	}
	additional := &common.OdigosConfiguration{
		Sampling: &common.SamplingConfiguration{
			K8sHealthProbesSampling: &common.K8sHealthProbesSamplingConfiguration{
				Enabled:        boolPtr(true),
				KeepPercentage: pct,
			},
		},
	}
	mergeConfigs(base, additional)

	if base.Sampling == nil || base.Sampling.K8sHealthProbesSampling == nil {
		t.Fatal("base.Sampling.K8sHealthProbesSampling is nil after merge")
	}
	if base.Sampling.K8sHealthProbesSampling.Enabled == nil || *base.Sampling.K8sHealthProbesSampling.Enabled != true {
		t.Errorf("Enabled: expected true after merge")
	}
	if base.Sampling.K8sHealthProbesSampling.KeepPercentage == nil || *base.Sampling.K8sHealthProbesSampling.KeepPercentage != 5.0 {
		t.Errorf("KeepPercentage: got %v, want 5.0", base.Sampling.K8sHealthProbesSampling.KeepPercentage)
	}
}

// TestMergeConfigs_SamplingNilAdditional verifies that a nil additional.Sampling
// leaves base.Sampling untouched.
func TestMergeConfigs_SamplingNilAdditional(t *testing.T) {
	base := &common.OdigosConfiguration{
		Sampling: &common.SamplingConfiguration{
			TailSampling: &common.TailSamplingConfiguration{
				Disabled: boolPtr(true),
			},
		},
	}
	additional := &common.OdigosConfiguration{}
	mergeConfigs(base, additional)

	if base.Sampling == nil || base.Sampling.TailSampling == nil {
		t.Fatal("base.Sampling was unexpectedly cleared")
	}
	if base.Sampling.TailSampling.Disabled == nil || *base.Sampling.TailSampling.Disabled != true {
		t.Error("TailSampling.Disabled changed unexpectedly")
	}
}
