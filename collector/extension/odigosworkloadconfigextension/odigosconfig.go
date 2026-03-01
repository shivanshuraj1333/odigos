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
//   - (nil, false)    workload not in cache → not opted in to URL templatization; caller should skip.
//   - (nil, true)     workload in cache but has no explicit template rules → opted in; caller applies heuristics only.
//   - (rules, true)   workload in cache with explicit rules → caller applies rules then heuristics for unmatched paths.
func (o *OdigosWorkloadConfig) GetWorkloadUrlTemplatizationRules(attrs pcommon.Map) ([]string, bool) {
	key := WorkloadKeyFromResourceAttributes(attrs)
	cfg, ok := o.cache.Get(key)
	if !ok || cfg == nil || len(cfg.WorkloadCollectorConfig) == 0 {
		// Workload not found in cache: it has no InstrumentationConfig → not opted in.
		return nil, false
	}
	var rules []string
	seen := make(map[string]struct{})
	for _, cc := range cfg.WorkloadCollectorConfig {
		if cc.UrlTemplatization == nil || len(cc.UrlTemplatization.Rules) == 0 {
			continue
		}
		for _, r := range cc.UrlTemplatization.Rules {
			if _, exists := seen[r]; !exists {
				seen[r] = struct{}{}
				rules = append(rules, r)
			}
		}
	}
	// Return true even when rules is empty: the workload IS opted in (InstrumentationConfig exists
	// and has a URL templatization entry), it just has no explicit template strings.
	// The processor will still apply default heuristics (UUID/number/date detection).
	return rules, true
}
