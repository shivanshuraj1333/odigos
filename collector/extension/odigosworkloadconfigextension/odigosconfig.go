package odigosworkloadconfigextension

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"k8s.io/client-go/dynamic/dynamicinformer"
)

// OdigosWorkloadConfig is an extension that runs a dynamic informer for InstrumentationConfigs
// and maintains a cache of workload sampling config keyed by WorkloadKey (namespace, kind, name).
type OdigosWorkloadConfig struct {
	cache           *Cache
	logger          *zap.Logger
	cancel          context.CancelFunc
	informerFactory dynamicinformer.DynamicSharedInformerFactory // set when in-cluster; nil otherwise
}

// NewOdigosConfig creates a new OdigosConfig extension.
func NewOdigosConfig(settings component.TelemetrySettings) (*OdigosWorkloadConfig, error) {
	return &OdigosWorkloadConfig{
		cache:  NewCache(),
		logger: settings.Logger,
	}, nil
}

// Start starts the dynamic informer for InstrumentationConfigs. The informer
// fills the cache with workload sampling configs keyed by WorkloadKey.
func (o *OdigosWorkloadConfig) Start(ctx context.Context, _ component.Host) error {
	ctx, o.cancel = context.WithCancel(ctx)
	return o.startInformer(ctx)
}

// Shutdown stops the informer and clears the cache.
func (o *OdigosWorkloadConfig) Shutdown(ctx context.Context) error {
	if o.cancel != nil {
		o.cancel()
	}
	return nil
}

// GetWorkloadSamplingConfig returns the sampling config for the given workload key, or (nil, false) if not found.
func (o *OdigosWorkloadConfig) GetWorkloadSamplingConfig(key WorkloadKey) (*WorkloadConfig, bool) {
	return o.cache.Get(key)
}

// GetWorkloadUrlTemplatizationRules returns URL templatization rules for the workload identified by resource attributes.
// Rules are aggregated from all containers in the workload's InstrumentationConfig (WorkloadCollectorConfig).
//
// Return semantics:
//   - (nil, false)    workload not opted in to URL templatization; caller should skip.
//   - (nil, true)     workload opted in but has no explicit template rules → caller applies heuristics only.
//   - (rules, true)   workload opted in with explicit rules → caller applies rules then heuristics for unmatched paths.
//
// A workload is considered opted in only when at least one of its containers has a non-nil
// UrlTemplatization entry in WorkloadCollectorConfig. Instrumented workloads that are not
// targeted by any URLTemplatization action will have UrlTemplatization == nil on all containers
// and must NOT be templatized.
func (o *OdigosWorkloadConfig) GetWorkloadUrlTemplatizationRules(attrs pcommon.Map) ([]string, bool) {
	key := WorkloadKeyFromResourceAttributes(attrs)
	cfg, ok := o.cache.Get(key)
	if !ok || cfg == nil {
		return nil, false
	}
	var rules []string
	seen := make(map[string]struct{})
	optedIn := false
	for _, cc := range cfg.WorkloadCollectorConfig {
		if cc.UrlTemplatization == nil {
			// This container was not targeted by any URLTemplatization action filter.
			continue
		}
		optedIn = true
		for _, r := range cc.UrlTemplatization.Rules {
			if _, exists := seen[r]; !exists {
				seen[r] = struct{}{}
				rules = append(rules, r)
			}
		}
	}
	if !optedIn {
		// No container in this workload is targeted by a URLTemplatization action.
		return nil, false
	}
	// The workload is opted in. rules may be empty if the action matched but had no explicit
	// template strings — the processor will still apply default heuristics in that case.
	return rules, true
}
