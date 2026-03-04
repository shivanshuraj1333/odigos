package odigosconfigk8sextension

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"k8s.io/client-go/dynamic/dynamicinformer"

	commonapi "github.com/odigos-io/odigos/common/api"
	"github.com/odigos-io/odigos/common/collector"
)

// UrlTemplatizationCacheCallback is notified when the extension's workload cache changes
// so the processor can keep its processorURLTemplateParsedRulesCache in sync (parse rules once per entry).
type UrlTemplatizationCacheCallback interface {
	OnSet(key string, cfg *commonapi.ContainerCollectorConfig)
	OnDeleteKey(key string)
}

// UrlTemplatizationCacheNotifier is implemented by the extension so the processor can
// register a callback for cache updates (add/update/delete) and avoid re-parsing rules per batch.
type UrlTemplatizationCacheNotifier interface {
	RegisterUrlTemplatizationCacheCallback(cb UrlTemplatizationCacheCallback)
}

// OdigosWorkloadConfig is an extension that runs a dynamic informer for InstrumentationConfigs
// and maintains a cache of workload sampling config keyed by WorkloadKey (namespace, kind, name).
type OdigosWorkloadConfig struct {
	cache           *cache
	logger          *zap.Logger
	cancel          context.CancelFunc
	informerFactory dynamicinformer.DynamicSharedInformerFactory // set when in-cluster; nil otherwise

	urlTemplatizationCB UrlTemplatizationCacheCallback
	urlTemplatizationMu sync.RWMutex
}

// OdigosConfigExtension is the interface that must be implemented by an extension that wants to provide Odigos configuration.
var _ collector.OdigosConfigExtension = (*OdigosWorkloadConfig)(nil)

// NewOdigosConfig creates a new OdigosConfig extension.
func NewOdigosConfig(settings component.TelemetrySettings) (*OdigosWorkloadConfig, error) {
	return &OdigosWorkloadConfig{
		cache:  newCache(),
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

func (o *OdigosWorkloadConfig) GetFromResource(res pcommon.Resource) (*commonapi.ContainerCollectorConfig, bool) {
	key, err := workloadKeyFromResourceAttributes(res.Attributes())
	if err != nil {
		return nil, false
	}
	return o.cache.Get(key)
}

// GetWorkloadUrlTemplatizationRules returns the URL templatization rules for the container
func (o *OdigosWorkloadConfig) GetWorkloadUrlTemplatizationRules(attrs pcommon.Map) (rules []string) {
	key, err := workloadKeyFromResourceAttributes(attrs)
	if err != nil {
		o.logger.Debug("GetWorkloadUrlTemplatizationRules: workload key from attrs failed", zap.Error(err))
		return nil
	}
	cfg, found := o.cache.Get(key)
	if !found || cfg.UrlTemplatization == nil {
		return nil
	}
	rules = cfg.UrlTemplatization.TemplatizationRules
	return rules
}

// GetWorkloadCacheKey returns the cache key for the container identified by resource attributes.
// The processor uses this to look up entries in its processorURLTemplateParsedRulesCache
// without duplicating key logic. Key format: "namespace/kind/name/containerName".
func (o *OdigosWorkloadConfig) GetWorkloadCacheKey(attrs pcommon.Map) (string, error) {
	return workloadKeyFromResourceAttributes(attrs)
}

// RegisterUrlTemplatizationCacheCallback registers a callback that is invoked when the
// extension cache is updated (add/update/delete). The processor uses it to keep its
// parsed rules cache in sync so rules are parsed once per workload entry, not per batch.
// Existing cache entries are replayed to the callback (backfill) so the processor
// starts with the same state as the extension when it registers after the informer has synced.
func (o *OdigosWorkloadConfig) RegisterUrlTemplatizationCacheCallback(cb UrlTemplatizationCacheCallback) {
	o.urlTemplatizationMu.Lock()
	o.urlTemplatizationCB = cb
	o.urlTemplatizationMu.Unlock()
	o.logger.Debug("RegisterUrlTemplatizationCacheCallback: callback registered")
	// Backfill: processor may start after informer has already synced; replay current cache state.
	o.cache.Range(func(key string, cfg *commonapi.ContainerCollectorConfig) {
		cb.OnSet(key, cfg)
	})
}

func (o *OdigosWorkloadConfig) getUrlTemplatizationCallback() UrlTemplatizationCacheCallback {
	o.urlTemplatizationMu.RLock()
	defer o.urlTemplatizationMu.RUnlock()
	return o.urlTemplatizationCB
}
