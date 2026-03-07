package collector

import (
	"go.opentelemetry.io/collector/pdata/pcommon"

	commonapi "github.com/odigos-io/odigos/common/api"
)

// OdigosConfigExtension is the interface that must be implemented by an extension that wants to provide Odigos configuration.
// Every platform (k8s, vm) can implement this interface to provide it's own processor extension to fetch the config from where it is stored.
type OdigosConfigExtension interface {

	// givin a specific resource, return a source collector config if exists.
	GetFromResource(res pcommon.Resource) (*commonapi.ContainerCollectorConfig, bool)
}

// ConfigCacheCallback is notified when the extension's workload config cache changes.
// Defined here so both the extension and any processor that consumes config updates use the same
// interface type (required for the processor's type assertion ext.(ConfigCacheNotifier) to succeed).
// Any processor that needs to react to per-workload config changes implements this interface.
//
// Key semantics:
//   - OnSet is called with the full container-level key: "namespace/kind/name/containerName"
//   - OnDeleteWorkloadPrefix is called with the workload-level prefix: "namespace/kind/name/"
//     (trailing slash). The receiver must delete all cache entries whose key starts with this prefix.
type ConfigCacheCallback interface {
	// OnSet is called when a container-level config entry is added or updated.
	// key is the full container key: "namespace/kind/name/containerName".
	OnSet(key string, cfg *commonapi.ContainerCollectorConfig)
	// OnDeleteWorkloadPrefix is called when all containers for a workload are invalidated.
	// keyPrefix is "namespace/kind/name/" — the receiver must evict all entries with this prefix.
	OnDeleteWorkloadPrefix(keyPrefix string)
}

// ConfigCacheNotifier is implemented by the extension so processors can register for config cache
// updates. Defined here so both sides share the same interface type.
type ConfigCacheNotifier interface {
	RegisterConfigCacheCallback(cb ConfigCacheCallback)
}
