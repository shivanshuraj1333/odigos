package profiles

import (
	"time"

	"github.com/odigos-io/odigos/common/profilecache"
)

// ProfileStore is the frontend's profile buffer. The buffering/eviction/TTL logic
// lives in the shared common/profilecache package (also used by the vm-agent).
type ProfileStore = profilecache.Store

func NewProfileStore(maxSlots, ttlSeconds, slotMaxBytes int, cleanupInterval time.Duration) *ProfileStore {
	return profilecache.NewStore(maxSlots, ttlSeconds, slotMaxBytes, cleanupInterval)
}
