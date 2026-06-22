package profiles

import (
	"context"
	"time"

	"github.com/odigos-io/odigos/common/telemetrycache"
	fecommon "github.com/odigos-io/odigos/frontend/services/common"
)

const (
	defaultMaxSlots     = 50
	defaultTTLSeconds   = 600 // 10 min
	defaultSlotMaxBytes = 64 << 20 // 64 MiB per profile sub-type
)

// ProfileStore holds at most maxSlots source-keyed slots with a TTL.
// Slots must be explicitly opened via EnsureSlot before data is stored
// (on-demand model; data arriving for unknown keys is silently dropped).
// Internally backed by telemetrycache.Store from common.
type ProfileStore struct {
	store      *telemetrycache.Store
	ttlSeconds int
	cancelClean context.CancelFunc
}

func NewProfileStore(maxSlots, ttlSeconds, slotMaxBytes int, cleanupInterval time.Duration) *ProfileStore {
	if maxSlots <= 0 {
		maxSlots = defaultMaxSlots
	}
	if ttlSeconds <= 0 {
		ttlSeconds = defaultTTLSeconds
	}
	if slotMaxBytes <= 0 {
		slotMaxBytes = defaultSlotMaxBytes
	}
	if cleanupInterval <= 0 {
		cleanupInterval = time.Minute
	}
	return &ProfileStore{
		store: telemetrycache.NewWithTTL(
			maxSlots,
			0, // no global byte cap — per-type caps handle sizing
			slotMaxBytes,
			time.Duration(ttlSeconds)*time.Second,
			cleanupInterval,
		),
		ttlSeconds: ttlSeconds,
	}
}

func (s *ProfileStore) EnsureSlot(sourceKey string)      { s.store.EnsureSlot(sourceKey) }
func (s *ProfileStore) RemoveSlot(sourceKey string)      { s.store.RemoveSlot(sourceKey) }
func (s *ProfileStore) IsActive(sourceKey string) bool   { return s.store.IsActive(sourceKey) }

// AddProfileData stores one OTLP chunk under (sourceKey, profileTypeKey).
// Silently dropped if no slot exists for sourceKey.
func (s *ProfileStore) AddProfileData(sourceKey, profileTypeKey string, chunk []byte) {
	s.store.AddProfileIfActive(sourceKey, profileTypeKey, time.Now(), chunk)
}

// GetProfileData returns all profile chunks for a source, all types merged.
func (s *ProfileStore) GetProfileData(sourceKey string) [][]byte {
	s.store.EnsureSlot(sourceKey) // refresh TTL on read (mirrors old LastRequestAt touch)
	return s.store.GetProfileData(sourceKey)
}

// ListProfileTypes returns profile sub-types with buffered data for sourceKey.
func (s *ProfileStore) ListProfileTypes(sourceKey string) []string {
	return s.store.ListProfileTypes(sourceKey)
}

// ClearSlotBuffer empties profile data without removing the slot.
func (s *ProfileStore) ClearSlotBuffer(sourceKey string) bool {
	return s.store.ClearSlotBuffer(sourceKey)
}

// ClearAllSlots removes every slot (e.g. when cluster profiling is turned off).
func (s *ProfileStore) ClearAllSlots() { s.store.ClearAll() }

// MaxSlots returns the maximum number of simultaneous profiling slots.
func (s *ProfileStore) MaxSlots() int { return s.store.MaxSlots() }

// ActiveSlots returns all slot keys and the subset with buffered data.
func (s *ProfileStore) ActiveSlots() (activeKeys []string, keysWithData []string) {
	return s.store.ActiveSlots()
}

// MemoryStats returns cache occupancy and configured limits for the UI.
func (s *ProfileStore) MemoryStats() fecommon.ProfileMemoryStats {
	st := s.store.Stats()
	return fecommon.ProfileMemoryStats{
		TotalBytes:          st.TotalBytes,
		MaxSlots:            st.MaxSlots,
		SlotMaxBytes:        st.SlotMaxBytes,
		SlotTTLSeconds:      s.ttlSeconds,
		MaxTotalBytesBudget: st.MaxSlots * st.SlotMaxBytes,
	}
}

// RunCleanup spawns a background goroutine that sweeps idle slots until StopCleanup is called.
func (s *ProfileStore) RunCleanup(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancelClean = cancel
	go s.store.RunCleanup(ctx)
}

// StopCleanup terminates the background TTL sweep goroutine.
func (s *ProfileStore) StopCleanup() {
	if s.cancelClean != nil {
		s.cancelClean()
	}
}
