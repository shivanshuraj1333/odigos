package telemetrycache

import (
	"context"
	"sort"
	"sync"
	"time"
)

const (
	DefaultMaxSlots        = 100
	DefaultMaxTotalBytes   = 500 << 20 // 500 MiB global cap
	DefaultProfileMaxBytes = 4 << 20   // 4 MiB per profile sub-type queue
	DefaultSlotTTL         = 15 * time.Minute
	defaultCleanupInterval = 1 * time.Minute
)

// slot is one service's cache entry keyed by sourceKey.
type slot struct {
	lastUpdate time.Time

	// profiles: profileTypeKey → bounded ring ("cpu", "memory", "gpu").
	profiles map[string]*BoundedBuffer
}

func (sl *slot) profilesBytes() int {
	total := 0
	for _, b := range sl.profiles {
		total += b.Size()
	}
	return total
}

// Store is the multi-signal per-service cache.
// It supports two slot-lifecycle models:
//   - Auto-create (vm-agent): slots are created on the first AddProfile call.
//   - Explicit (k8s frontend): slots are opened via EnsureSlot and data is
//     dropped via AddProfileIfActive unless the slot exists.
type Store struct {
	mu              sync.RWMutex
	slots           map[string]*slot
	maxSlots        int
	maxTotalBytes   int
	profileMaxBytes int
	slotTTL         time.Duration
	cleanupInterval time.Duration
	evictedSlots    uint64
}

// StoreStats is the observable snapshot of the cache.
type StoreStats struct {
	Services            int    `json:"services"`
	TotalBytes          int    `json:"totalBytes"`
	MaxSlots            int    `json:"maxSlots"`
	SlotMaxBytes        int    `json:"slotMaxBytes"`
	MaxTotalBytesBudget int    `json:"maxTotalBytesBudget"`
	EvictedSlotsTotal   uint64 `json:"evictedSlotsTotal"`
}

// New builds the store with default TTL (15 min) and cleanup interval (1 min).
func New(maxSlots, maxTotalBytes, profileMaxBytes int) *Store {
	return NewWithTTL(maxSlots, maxTotalBytes, profileMaxBytes, DefaultSlotTTL, defaultCleanupInterval)
}

// NewWithTTL builds the store with configurable TTL and cleanup interval.
func NewWithTTL(maxSlots, maxTotalBytes, profileMaxBytes int, slotTTL, cleanupInterval time.Duration) *Store {
	if maxSlots <= 0 {
		maxSlots = DefaultMaxSlots
	}
	if maxTotalBytes <= 0 {
		maxTotalBytes = DefaultMaxTotalBytes
	}
	if profileMaxBytes <= 0 {
		profileMaxBytes = DefaultProfileMaxBytes
	}
	if slotTTL <= 0 {
		slotTTL = DefaultSlotTTL
	}
	if cleanupInterval <= 0 {
		cleanupInterval = defaultCleanupInterval
	}
	return &Store{
		slots:           make(map[string]*slot),
		maxSlots:        maxSlots,
		maxTotalBytes:   maxTotalBytes,
		profileMaxBytes: profileMaxBytes,
		slotTTL:         slotTTL,
		cleanupInterval: cleanupInterval,
	}
}

// ── vm-agent model: auto-create slot ─────────────────────────────────────────

// AddProfile stores one raw OTLP chunk under (sourceKey, profileTypeKey).
// Creates the slot if it does not exist (vm-agent model).
func (s *Store) AddProfile(sourceKey, profileTypeKey string, capturedAt time.Time, data []byte) {
	if sourceKey == "" || profileTypeKey == "" || len(data) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sl := s.getOrCreateLocked(sourceKey)
	sl.lastUpdate = capturedAt

	if sl.profiles == nil {
		sl.profiles = make(map[string]*BoundedBuffer)
	}
	buf, ok := sl.profiles[profileTypeKey]
	if !ok {
		buf = NewBoundedBuffer(s.profileMaxBytes)
		sl.profiles[profileTypeKey] = buf
	}
	buf.Add(capturedAt, data)

	for s.totalBytesLocked() > s.maxTotalBytes && len(s.slots) > 1 {
		if !s.evictOldestLocked(sourceKey) {
			break
		}
	}
}

// ── k8s model: explicit slot lifecycle ───────────────────────────────────────

// EnsureSlot opens or refreshes an explicit slot without writing data.
// Must be called before AddProfileIfActive will store anything.
func (s *Store) EnsureSlot(sourceKey string) {
	if sourceKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sl, ok := s.slots[sourceKey]
	if !ok {
		if len(s.slots) >= s.maxSlots {
			s.evictOldestLocked()
		}
		s.slots[sourceKey] = &slot{lastUpdate: time.Now()}
		return
	}
	sl.lastUpdate = time.Now()
}

// RemoveSlot closes the slot and frees its memory.
func (s *Store) RemoveSlot(sourceKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.slots, sourceKey)
}

// IsActive reports whether a slot exists for sourceKey.
func (s *Store) IsActive(sourceKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.slots[sourceKey]
	return ok
}

// AddProfileIfActive stores a chunk only if a slot already exists.
// Silently drops the chunk if no slot exists — caller must EnsureSlot first.
func (s *Store) AddProfileIfActive(sourceKey, profileTypeKey string, capturedAt time.Time, data []byte) {
	if sourceKey == "" || profileTypeKey == "" || len(data) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sl, ok := s.slots[sourceKey]
	if !ok {
		return
	}
	sl.lastUpdate = capturedAt

	if sl.profiles == nil {
		sl.profiles = make(map[string]*BoundedBuffer)
	}
	buf, ok := sl.profiles[profileTypeKey]
	if !ok {
		buf = NewBoundedBuffer(s.profileMaxBytes)
		sl.profiles[profileTypeKey] = buf
	}
	buf.Add(capturedAt, data)
}

// ClearSlotBuffer empties the profile buffers for a slot without removing it.
// Returns false if no such slot exists.
func (s *Store) ClearSlotBuffer(sourceKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl, ok := s.slots[sourceKey]
	if !ok {
		return false
	}
	sl.profiles = nil
	sl.lastUpdate = time.Now()
	return true
}

// ── reads ─────────────────────────────────────────────────────────────────────

// GetProfileData returns all profile chunks for a source, all types merged.
// Equivalent to SnapshotServiceProfiles(sourceKey, time.Time{}).
func (s *Store) GetProfileData(sourceKey string) [][]byte {
	return s.SnapshotServiceProfiles(sourceKey, time.Time{})
}

// SnapshotProfileByType returns chunks for one specific profile sub-type.
func (s *Store) SnapshotProfileByType(sourceKey, profileTypeKey string, since time.Time) [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sl, ok := s.slots[sourceKey]
	if !ok {
		return nil
	}
	buf, ok := sl.profiles[profileTypeKey]
	if !ok {
		return nil
	}
	return buf.SnapshotSince(since)
}

// SnapshotServiceProfiles returns all profile chunks for a service, all sub-types merged.
func (s *Store) SnapshotServiceProfiles(sourceKey string, since time.Time) [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sl, ok := s.slots[sourceKey]
	if !ok {
		return nil
	}
	var out [][]byte
	for _, buf := range sl.profiles {
		out = append(out, buf.SnapshotSince(since)...)
	}
	return out
}

// SnapshotAllProfiles returns every profile chunk across all services and sub-types.
func (s *Store) SnapshotAllProfiles(since time.Time) [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out [][]byte
	for _, sl := range s.slots {
		for _, buf := range sl.profiles {
			out = append(out, buf.SnapshotSince(since)...)
		}
	}
	return out
}

// ListProfileTypes returns profile sub-types with buffered data for sourceKey.
func (s *Store) ListProfileTypes(sourceKey string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sl, ok := s.slots[sourceKey]
	if !ok || len(sl.profiles) == 0 {
		return nil
	}
	keys := make([]string, 0, len(sl.profiles))
	for k, buf := range sl.profiles {
		if buf.Size() > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// ProfileSubTypes is an alias for ListProfileTypes for backward compat.
func (s *Store) ProfileSubTypes(sourceKey string) []string { return s.ListProfileTypes(sourceKey) }

// ── observability ─────────────────────────────────────────────────────────────

// MaxSlots returns the configured maximum number of slots.
func (s *Store) MaxSlots() int { return s.maxSlots }

// Services returns source keys with any buffered data.
func (s *Store) Services() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.slots))
	for k := range s.slots {
		out = append(out, k)
	}
	return out
}

// ActiveSlots returns (all slot keys, keys with buffered profile data).
func (s *Store) ActiveSlots() (activeKeys []string, keysWithData []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, sl := range s.slots {
		activeKeys = append(activeKeys, k)
		if sl.profilesBytes() > 0 {
			keysWithData = append(keysWithData, k)
		}
	}
	sort.Strings(activeKeys)
	sort.Strings(keysWithData)
	return
}

// Stats reports cache occupancy and configured limits.
func (s *Store) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, sl := range s.slots {
		total += sl.profilesBytes()
	}
	return StoreStats{
		Services:            len(s.slots),
		TotalBytes:          total,
		MaxSlots:            s.maxSlots,
		SlotMaxBytes:        s.profileMaxBytes,
		MaxTotalBytesBudget: s.maxTotalBytes,
		EvictedSlotsTotal:   s.evictedSlots,
	}
}

// ── maintenance ───────────────────────────────────────────────────────────────

// ClearAll drops every slot.
func (s *Store) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slots = make(map[string]*slot)
}

// RunCleanup sweeps idle slots until ctx is cancelled.
func (s *Store) RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep()
		}
	}
}

// SweepNow triggers an immediate TTL sweep. Exposed for wrappers and tests.
func (s *Store) SweepNow() { s.sweep() }

// ── internals ─────────────────────────────────────────────────────────────────

func (s *Store) getOrCreateLocked(sourceKey string) *slot {
	sl, ok := s.slots[sourceKey]
	if !ok {
		if len(s.slots) >= s.maxSlots {
			s.evictOldestLocked()
		}
		sl = &slot{}
		s.slots[sourceKey] = sl
	}
	return sl
}

func (s *Store) totalBytesLocked() int {
	total := 0
	for _, sl := range s.slots {
		total += sl.profilesBytes()
	}
	return total
}

func (s *Store) evictOldestLocked(protect ...string) bool {
	protected := make(map[string]struct{}, len(protect))
	for _, k := range protect {
		protected[k] = struct{}{}
	}
	var oldestKey string
	var oldest time.Time
	for k, sl := range s.slots {
		if _, skip := protected[k]; skip {
			continue
		}
		if oldestKey == "" || sl.lastUpdate.Before(oldest) {
			oldest = sl.lastUpdate
			oldestKey = k
		}
	}
	if oldestKey == "" {
		return false
	}
	delete(s.slots, oldestKey)
	s.evictedSlots++
	return true
}

func (s *Store) sweep() {
	cutoff := time.Now().Add(-s.slotTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, sl := range s.slots {
		if sl.lastUpdate.Before(cutoff) {
			delete(s.slots, k)
		}
	}
}
