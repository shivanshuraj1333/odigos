package common

import "github.com/odigos-io/odigos/common/profilecache"

// The profiling buffer and its contract live in the shared common/profilecache
// package (used by both the frontend and the vm-agent). These aliases keep the
// existing common.ProfileMemoryStats / common.ProfileStoreRef spellings.
type (
	ProfileMemoryStats = profilecache.MemoryStats
	ProfileStoreRef    = profilecache.StoreRef
)
