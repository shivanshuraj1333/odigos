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

// GetWorkloadUrlTemplatizationRules implements the workloadConfigProvider interface used by
// odigosurltemplateprocessor. It returns the URL templatization rules for the workload
// identified by the resource attributes (k8s.namespace.name, k8s.deployment.name, etc.).
//
// Returns (rules, true) when the workload has URL templatization configured in at least
// one container's WorkloadCollectorConfig. The rules slice may be empty — meaning the
// workload is opted in to URL templatization but relies on default heuristics only.
//
// Returns (nil, false) when the workload is not configured for URL templatization.
func (o *OdigosWorkloadConfig) GetWorkloadUrlTemplatizationRules(attrs pcommon.Map) ([]string, bool) {
	key := WorkloadKeyFromResourceAttributes(attrs)
	cfg, ok := o.cache.Get(key)
	if !ok {
		return nil, false
	}

	var rules []string
	hasUrlTemplatization := false
	for _, container := range cfg.WorkloadCollectorConfig {
		if container.UrlTemplatization != nil {
			hasUrlTemplatization = true
			rules = append(rules, container.UrlTemplatization.Rules...)
		}
	}

	if !hasUrlTemplatization {
		// Workload is in the cache (has an InstrumentationConfig) but no container
		// has URL templatization configured — do not apply templatization.
		return nil, false
	}
	return rules, true
}

// Cache returns the underlying cache for advanced use (e.g. iteration).
// Do not modify the cache directly; use GetWorkloadSamplingConfig for reads.
