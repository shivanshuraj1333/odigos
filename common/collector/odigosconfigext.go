package collector

import (
	"go.opentelemetry.io/collector/pdata/pcommon"

	commonapi "github.com/odigos-io/odigos/common/api"
)

// OdigosConfigExtension is the interface that must be implemented by an extension that wants to provide Odigos configuration.
// Every platform (k8s, vm) can implement this interface to provide its own processor extension to fetch the config from where it is stored.
type OdigosConfigExtension interface {
	// GetFromResource returns the ContainerCollectorConfig for the given resource, or false if not found.
	GetFromResource(res pcommon.Resource) (*commonapi.ContainerCollectorConfig, bool)

	// GetWorkloadCacheKey returns a stable cache key derived from resource attributes.
	// Key format: "namespace/kind/name/containerName".
	GetWorkloadCacheKey(attrs pcommon.Map) (string, error)

	// RegisterConfigCacheCallback registers a callback that is invoked whenever the
	// extension's workload config cache is updated. Existing entries are replayed to the
	// callback immediately (backfill) so the caller starts with the same state as the extension.
	RegisterConfigCacheCallback(cb ConfigCacheCallback)
}

// ConfigCacheCallback is notified when the extension's workload config cache changes.
// Defined here so both the extension and any processor that consumes config updates use the same interface type.
// Any processor that needs to react to per-workload config changes implements this interface.
//
// Key semantics:
//   - OnSet and OnDeleteKey use the full container-level key: "namespace/kind/name/containerName"
//   - OnDeleteWorkloadPrefix uses the workload-level prefix: "namespace/kind/name/"
//     (trailing slash). The receiver must delete all cache entries whose key starts with this prefix.
type ConfigCacheCallback interface {
	// OnSet is called when a container-level config entry is added or updated.
	// key is the full container key: "namespace/kind/name/containerName".
	OnSet(key string, cfg *commonapi.ContainerCollectorConfig)
	// OnDeleteKey is called when a specific container-level config entry is removed.
	// key is the full container key: "namespace/kind/name/containerName".
	OnDeleteKey(key string)
	// OnDeleteWorkloadPrefix is called when all containers for a workload are removed (e.g. IC deleted).
	// keyPrefix is "namespace/kind/name/" — the receiver must evict all entries with this prefix.
	OnDeleteWorkloadPrefix(keyPrefix string)
}
