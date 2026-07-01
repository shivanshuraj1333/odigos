package profiles

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	"github.com/odigos-io/odigos/frontend/services/common"
)

// numProfileTypes is the number of distinct profile-type buckets a source can hold:
// cpu + alloc_space + alloc_objects + inuse_space + inuse_objects. Used only to
// report the worst-case cache ceiling in MemoryStats.
const numProfileTypes = 5

// normalizeProfileType maps a request/pprof sample-type spelling to the canonical
// bucket key. CPU is emitted as "samples" (period type "cpu"); an empty request
// also means CPU. Memory types are used as-is.
func normalizeProfileType(s string) string {
	switch s {
	case "", "cpu", "samples":
		return "cpu"
	default:
		return s
	}
}

// TypeBucket holds one profile type's chunks for one source, with its own byte
// budget (BoundedBuffer) and its own request/data timestamps for independent TTL.
// CPU and each memory signal (alloc_space, alloc_objects, inuse_space,
// inuse_objects) get separate buckets, so a high-volume type can never FIFO-evict
// another type's samples out of a shared buffer.
type TypeBucket struct {
	LastRequestAt time.Time
	// LastDataAt is the UnixNano of the most recent chunk stored in this bucket
	// (0 = none). Updated atomically from the OTLP ingest path; drives data-arrival
	// retention independent of UI polling.
	LastDataAt atomic.Int64
	Buffer     *BoundedBuffer
}

// Slot is one source's set of per-profile-type buckets.
// LastRequestAt is the slot-level request time (bumped on enable and on any bucket
// read) so a freshly-enabled but not-yet-populated source survives ttlSeconds.
type Slot struct {
	LastRequestAt time.Time
	Buckets       map[string]*TypeBucket
}

// ProfileStore holds at most maxSlots source-keyed slots, each with per-profile-type
// buckets. Budget (perTypeMaxBytes) and TTL/retention are applied PER (source, type):
// each bucket has its own rolling buffer and its own request/data clocks.
// Worst-case cache ceiling ≈ maxSlots × numProfileTypes × perTypeMaxBytes.
type ProfileStore struct {
	mu                   sync.RWMutex
	slots                map[string]*Slot
	maxSlots             int
	ttlSeconds           int
	dataRetentionSeconds int
	perTypeMaxBytes      int
	cleanupInterval      time.Duration
	stopCleanup          func()
}

// slotActivityNano returns the slot's most-recent activity across all its buckets
// (later of any bucket request/data) and the slot's own last request.
func slotActivityNano(slot *Slot) int64 {
	latest := slot.LastRequestAt.UnixNano()
	for _, b := range slot.Buckets {
		act := b.LastRequestAt.UnixNano()
		if d := b.LastDataAt.Load(); d > act {
			act = d
		}
		if act > latest {
			latest = act
		}
	}
	return latest
}

func (s *ProfileStore) evictOldestSlotLocked() {
	var oldestKey string
	var oldestNano int64
	first := true
	for k, slot := range s.slots {
		act := slotActivityNano(slot)
		if first || act < oldestNano {
			oldestNano = act
			oldestKey = k
			first = false
		}
	}
	if oldestKey != "" {
		delete(s.slots, oldestKey)
	}
}

func NewProfileStore(maxSlots, ttlSeconds, dataRetentionSeconds, perTypeMaxBytes int, cleanupInterval time.Duration) *ProfileStore {
	return &ProfileStore{
		slots:                make(map[string]*Slot),
		maxSlots:             maxSlots,
		ttlSeconds:           ttlSeconds,
		dataRetentionSeconds: dataRetentionSeconds,
		perTypeMaxBytes:      perTypeMaxBytes,
		cleanupInterval:      cleanupInterval,
	}
}

// EnsureSlot opens a source slot (with no buckets yet) if absent, or refreshes its
// slot-level LastRequestAt. Per-type buckets are created lazily on first data
// (ingest) or first request (query) for that type.
func (s *ProfileStore) EnsureSlot(sourceKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if slot, ok := s.slots[sourceKey]; ok {
		slot.LastRequestAt = now
		return
	}
	if len(s.slots) >= s.maxSlots {
		s.evictOldestSlotLocked()
	}
	s.slots[sourceKey] = &Slot{LastRequestAt: now, Buckets: make(map[string]*TypeBucket)}
}

// ensureBucketLocked returns the (source, ptype) bucket, creating it if the source
// slot exists. Returns nil if the source is not active. Caller holds s.mu.
func (s *ProfileStore) ensureBucketLocked(sourceKey, ptype string) *TypeBucket {
	slot, ok := s.slots[sourceKey]
	if !ok || slot == nil {
		return nil
	}
	b, ok := slot.Buckets[ptype]
	if !ok {
		b = &TypeBucket{LastRequestAt: time.Now(), Buffer: NewBoundedBuffer(s.perTypeMaxBytes)}
		slot.Buckets[ptype] = b
	}
	return b
}

func (s *ProfileStore) RemoveSlot(sourceKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.slots, sourceKey)
}

// ClearAllSlots removes every slot (e.g. when cluster profiling is turned off).
func (s *ProfileStore) ClearAllSlots() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slots = make(map[string]*Slot)
}

// ClearSlotBuffer clears every per-type bucket for a source but keeps the slot.
func (s *ProfileStore) ClearSlotBuffer(sourceKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.slots[sourceKey]
	if !ok || slot == nil {
		return false
	}
	slot.LastRequestAt = time.Now()
	for _, b := range slot.Buckets {
		b.LastRequestAt = time.Now()
		if b.Buffer != nil {
			b.Buffer.Clear()
		}
	}
	return true
}

func (s *ProfileStore) MaxSlots() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxSlots
}

// MemoryStats returns total buffered bytes across all per-type buckets and the
// configured limits. SlotMaxBytes here is the PER-(source,type) budget.
func (s *ProfileStore) MemoryStats() common.ProfileMemoryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var totalBytes int
	for _, slot := range s.slots {
		for _, b := range slot.Buckets {
			if b.Buffer != nil {
				totalBytes += b.Buffer.Size()
			}
		}
	}
	return common.ProfileMemoryStats{
		TotalBytes:          totalBytes,
		MaxSlots:            s.maxSlots,
		SlotMaxBytes:        s.perTypeMaxBytes,
		SlotTTLSeconds:      s.ttlSeconds,
		MaxTotalBytesBudget: s.maxSlots * numProfileTypes * s.perTypeMaxBytes,
	}
}

// AddProfileDataTyped routes a chunk to the bucket for each profile type it carries.
// ptypes must already be normalized. A memory chunk (which carries alloc_* and
// inuse_* together) lands in each of its type buckets; CPU lands only in "cpu".
func (s *ProfileStore) AddProfileDataTyped(sourceKey string, ptypes []string, chunk []byte) {
	if len(ptypes) == 0 || len(chunk) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.slots[sourceKey]; !ok {
		return
	}
	nowNano := time.Now().UnixNano()
	for _, pt := range ptypes {
		b := s.ensureBucketLocked(sourceKey, pt)
		if b == nil {
			continue
		}
		if !b.Buffer.Add(chunk) {
			commonlogger.LoggerCompat().With("subsystem", "backend-profiling").Warn(
				"profile_chunk_dropped_oversized", "sourceKey", sourceKey, "type", pt,
			)
			continue
		}
		b.LastDataAt.Store(nowNano)
	}
}

// GetProfileData returns a snapshot of buffered chunks for one (source, profileType),
// stamping the bucket's (and slot's) request time. profileType is normalized here,
// so callers may pass "", "cpu", "samples", or a memory type.
func (s *ProfileStore) GetProfileData(sourceKey, profileType string) [][]byte {
	pt := normalizeProfileType(profileType)
	s.mu.Lock()
	if slot, ok := s.slots[sourceKey]; ok {
		slot.LastRequestAt = time.Now()
	}
	b := s.ensureBucketLocked(sourceKey, pt)
	if b != nil {
		b.LastRequestAt = time.Now()
	}
	s.mu.Unlock()
	if b == nil {
		return nil
	}
	return b.Buffer.Snapshot()
}

func (s *ProfileStore) IsActive(sourceKey string) bool {
	s.mu.RLock()
	_, ok := s.slots[sourceKey]
	s.mu.RUnlock()
	return ok
}

// ActiveSlots returns source keys for all open slots and the subset holding data
// in at least one type bucket.
func (s *ProfileStore) ActiveSlots() (activeKeys []string, keysWithData []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, slot := range s.slots {
		activeKeys = append(activeKeys, k)
		for _, b := range slot.Buckets {
			if b.Buffer != nil && b.Buffer.Size() > 0 {
				keysWithData = append(keysWithData, k)
				break
			}
		}
	}
	return activeKeys, keysWithData
}

// RunCleanup starts the background TTL sweep goroutine.
func (s *ProfileStore) RunCleanup(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.stopCleanup = cancel
	go func() {
		ticker := time.NewTicker(s.cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanupExpired()
			}
		}
	}()
}

// cleanupExpired sweeps per (source, type): a bucket is dropped when it has had no
// request in ttlSeconds AND no data in dataRetentionSeconds. A source slot is
// removed once it has no live buckets AND no recent slot-level request (so a
// freshly-enabled empty source still survives ttlSeconds).
func (s *ProfileStore) cleanupExpired() {
	now := time.Now()
	reqCutoff := now.Add(-time.Duration(s.ttlSeconds) * time.Second)
	dataCutoffNano := now.Add(-time.Duration(s.dataRetentionSeconds) * time.Second).UnixNano()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, slot := range s.slots {
		for pt, b := range slot.Buckets {
			// Keep while a request is recent (an open tab keeps the type warm)...
			if !b.LastRequestAt.Before(reqCutoff) {
				continue
			}
			// ...or while it holds data that arrived within the retention window.
			if d := b.LastDataAt.Load(); d != 0 && d >= dataCutoffNano {
				continue
			}
			delete(slot.Buckets, pt)
		}
		if len(slot.Buckets) > 0 {
			continue
		}
		if !slot.LastRequestAt.Before(reqCutoff) {
			continue
		}
		delete(s.slots, k)
	}
}

// StopCleanup stops the TTL cleanup goroutine.
func (s *ProfileStore) StopCleanup() {
	if s.stopCleanup != nil {
		s.stopCleanup()
	}
}
