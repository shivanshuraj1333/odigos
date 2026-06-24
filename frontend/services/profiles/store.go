package profiles

import (
	"context"
	"time"

	"github.com/odigos-io/odigos/common/telemetrycache"
	fecommon "github.com/odigos-io/odigos/frontend/services/common"
)

// ProfileStore is a thin wrapper over TelemetryCache scoped to the profiles
// signal.  The explicit-gate model (k8s frontend): slots are opened via
// EnsureSlot; chunks arriving for unknown sourceKeys are silently dropped.
type ProfileStore struct {
	tc       *telemetrycache.TelemetryCache
	active   map[string]struct{} // guarded by activeMu
	activeMu chan struct{}        // single-element channel used as a mutex
}

func NewProfileStore() *ProfileStore {
	s := &ProfileStore{
		tc:       telemetrycache.NewDefault(),
		active:   make(map[string]struct{}),
		activeMu: make(chan struct{}, 1),
	}
	s.activeMu <- struct{}{}
	return s
}

func (s *ProfileStore) lockActive()   { <-s.activeMu }
func (s *ProfileStore) unlockActive() { s.activeMu <- struct{}{} }

// EnsureSlot marks sourceKey as active so AddProfileData will store chunks.
func (s *ProfileStore) EnsureSlot(sourceKey string) {
	s.lockActive()
	s.active[sourceKey] = struct{}{}
	s.unlockActive()
}

// RemoveSlot marks sourceKey as inactive and drops its cached data.
func (s *ProfileStore) RemoveSlot(sourceKey string) {
	s.lockActive()
	delete(s.active, sourceKey)
	s.unlockActive()
	s.tc.Drop(sourceKey)
}

// IsActive reports whether sourceKey has an open slot.
func (s *ProfileStore) IsActive(sourceKey string) bool {
	s.lockActive()
	_, ok := s.active[sourceKey]
	s.unlockActive()
	return ok
}

// AddProfileData stores one OTLP chunk. Silently dropped if no slot exists.
func (s *ProfileStore) AddProfileData(sourceKey, profileTypeKey string, chunk []byte) {
	if !s.IsActive(sourceKey) {
		return
	}
	_ = s.tc.Write(telemetrycache.SignalProfiles, sourceKey, profileTypeKey, time.Now(), chunk)
}

// GetProfileData returns all profile chunks for a source, all types merged.
func (s *ProfileStore) GetProfileData(sourceKey string) [][]byte {
	chunks, _ := s.tc.ReadAllSubTypes(telemetrycache.SignalProfiles, sourceKey)
	return chunksToBytes(chunks)
}

// ListProfileTypes returns profile sub-types with data for sourceKey.
func (s *ProfileStore) ListProfileTypes(sourceKey string) []string {
	return s.tc.SubTypes(telemetrycache.SignalProfiles, sourceKey)
}

// ClearSlotBuffer empties profile data without removing the slot.
func (s *ProfileStore) ClearSlotBuffer(sourceKey string) bool {
	if !s.IsActive(sourceKey) {
		return false
	}
	s.tc.ClearSignal(telemetrycache.SignalProfiles)
	return true
}

// ClearAllSlots removes every slot.
func (s *ProfileStore) ClearAllSlots() {
	s.lockActive()
	s.active = make(map[string]struct{})
	s.unlockActive()
	s.tc.ClearAll()
}

// MaxSlots returns the number of ring slots for the profiles signal.
func (s *ProfileStore) MaxSlots() int {
	st, ok := s.tc.RingStatsFor(telemetrycache.SignalProfiles)
	if !ok {
		return 0
	}
	return int(st.NumSlots)
}

// ActiveSlots returns (all slot keys, keys with buffered data).
func (s *ProfileStore) ActiveSlots() (activeKeys []string, keysWithData []string) {
	s.lockActive()
	for k := range s.active {
		activeKeys = append(activeKeys, k)
	}
	s.unlockActive()
	keysWithData = s.tc.Sources(telemetrycache.SignalProfiles)
	return
}

// MemoryStats returns cache occupancy for the UI.
func (s *ProfileStore) MemoryStats() fecommon.ProfileMemoryStats {
	st, ok := s.tc.RingStatsFor(telemetrycache.SignalProfiles)
	if !ok {
		return fecommon.ProfileMemoryStats{}
	}
	return fecommon.ProfileMemoryStats{
		TotalBytes:          int(st.BytesUsed),
		MaxSlots:            int(st.NumSlots),
		SlotMaxBytes:        int(st.SlotSize),
		SlotTTLSeconds:      0, // ring has no TTL
		MaxTotalBytesBudget: int(st.Capacity),
	}
}

// RunCleanup is a no-op — the ring has no TTL sweeping.
func (s *ProfileStore) RunCleanup(_ context.Context) {}

// StopCleanup is a no-op.
func (s *ProfileStore) StopCleanup() {}

// chunksToBytes extracts the raw bytes from a Chunk slice.
func chunksToBytes(chunks []telemetrycache.Chunk) [][]byte {
	if len(chunks) == 0 {
		return nil
	}
	out := make([][]byte, len(chunks))
	for i, c := range chunks {
		out[i] = c.Data
	}
	return out
}
