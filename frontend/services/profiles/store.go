package profiles

import (
	"github.com/odigos-io/odigos/common/profilecache"
)

// ProfileStore is the frontend's profile buffer. The buffering/eviction/TTL logic
// lives in the shared common/profilecache package (also used by the vm-agent);
// construct it directly with profilecache.NewStore.
type ProfileStore = profilecache.Store
