package profiles

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	"github.com/odigos-io/odigos/frontend/services/common"
)

type Slot struct {
	LastRequestAt time.Time
	// LastDataAt is the UnixNano of the most recent profile chunk that landed in
	// this slot (0 = none yet). Updated atomically from the OTLP ingest path so it
	// needs no write lock. It drives data-arrival retention: a slot holding data is
	// kept for at least dataRetentionSeconds after that data arrived, independent of
	// whether anyone is polling — so a populated flamegraph does not vanish the
	// moment the UI tab stops refreshing.
	LastDataAt atomic.Int64
	Buffer     *BoundedBuffer
}

// ProfileStore holds at most maxSlots source-keyed slots.
// Eviction: when full, the slot whose most-recent activity (request OR data) is
// oldest is removed.
// Retention: a slot is removed by the background sweep only when it has had no
// request in ttlSeconds AND no data in dataRetentionSeconds. Empty slots (a tab
// open on a source that never produced) age out on ttlSeconds; slots that ever
// received data live for at least dataRetentionSeconds past the last chunk.
type ProfileStore struct {
	mu                   sync.RWMutex
	slots                map[string]*Slot
	maxSlots             int
	ttlSeconds           int
	dataRetentionSeconds int
	slotMaxBytes         int
	cleanupInterval      time.Duration
	// StopCleanup invokes it to end the TTL goroutine.
	stopCleanup func()
}

// slotActivityNano returns the slot's most-recent activity time (the later of its
// last request and last data arrival) in UnixNano.
func slotActivityNano(slot *Slot) int64 {
	act := slot.LastRequestAt.UnixNano()
	if d := slot.LastDataAt.Load(); d > act {
		act = d
	}
	return act
}

// evictOldestSlotLocked removes the slot with the oldest activity (request or data).
func (s *ProfileStore) evictOldestSlotLocked() {
	var oldestKey string
	var oldestNano int64
	for k, slot := range s.slots {
		act := slotActivityNano(slot)
		if oldestKey == "" || act < oldestNano {
			oldestNano = act
			oldestKey = k
		}
	}
	if oldestKey != "" {
		delete(s.slots, oldestKey)
	}
}

func NewProfileStore(maxSlots, ttlSeconds, dataRetentionSeconds, slotMaxBytes int, cleanupInterval time.Duration) *ProfileStore {
	return &ProfileStore{
		slots:                make(map[string]*Slot),
		maxSlots:             maxSlots,
		ttlSeconds:           ttlSeconds,
		dataRetentionSeconds: dataRetentionSeconds,
		slotMaxBytes:         slotMaxBytes,
		cleanupInterval:      cleanupInterval,
	}
}

// EnsureSlot opens a slot for sourceKey if one does not already exist,
// or refreshes its LastRequestAt if it does.
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

	s.slots[sourceKey] = &Slot{
		LastRequestAt: now,
		Buffer:        NewBoundedBuffer(s.slotMaxBytes),
	}
}

func (s *ProfileStore) RemoveSlot(sourceKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.slots, sourceKey)
}

// ClearAllSlots removes every slot (e.g. when cluster profiling is turned off via effective-config).
func (s *ProfileStore) ClearAllSlots() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slots = make(map[string]*Slot)
}

// ClearSlotBuffer removes all buffered profile chunks for sourceKey but keeps the slot
func (s *ProfileStore) ClearSlotBuffer(sourceKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.slots[sourceKey]
	if !ok || slot == nil {
		return false
	}
	slot.LastRequestAt = time.Now()
	if slot.Buffer != nil {
		slot.Buffer.Clear()
	}
	return true
}

func (s *ProfileStore) MaxSlots() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxSlots
}

// MemoryStats returns total bytes buffered across slots and the configured limits for debugging purposes
func (s *ProfileStore) MemoryStats() common.ProfileMemoryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var totalBytes int
	for _, slot := range s.slots {
		if slot.Buffer != nil {
			totalBytes += slot.Buffer.Size()
		}
	}
	return common.ProfileMemoryStats{
		TotalBytes:          totalBytes,
		MaxSlots:            s.maxSlots,
		SlotMaxBytes:        s.slotMaxBytes,
		SlotTTLSeconds:      s.ttlSeconds,
		MaxTotalBytesBudget: s.maxSlots * s.slotMaxBytes,
	}
}

// AddProfileData appends serialized profile data to the slot for sourceKey if it exists.
func (s *ProfileStore) AddProfileData(sourceKey string, chunk []byte) {
	s.mu.RLock()
	slot, ok := s.slots[sourceKey]
	var buf *BoundedBuffer
	if ok && slot != nil {
		buf = slot.Buffer
	}
	s.mu.RUnlock()
	if buf == nil {
		return
	}
	if !buf.Add(chunk) {
		commonlogger.LoggerCompat().With("subsystem", "backend-profiling").Warn(
			"profile_chunk_dropped_oversized", "sourceKey", sourceKey,
		)
		return
	}
	// Stamp the data-arrival time so retention keeps this slot alive for at least
	// dataRetentionSeconds past now, regardless of UI polling. Atomic store is safe
	// while holding only the read lock (slot pointer stays valid even if the map
	// entry is later deleted).
	if slot != nil {
		slot.LastDataAt.Store(time.Now().UnixNano())
	}
}

// GetProfileData returns a shallow snapshot of buffered chunks for the given source key (see BoundedBuffer.Snapshot).
func (s *ProfileStore) GetProfileData(sourceKey string) [][]byte {
	s.mu.Lock()
	slot, ok := s.slots[sourceKey]
	if ok {
		slot.LastRequestAt = time.Now()
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return slot.Buffer.Snapshot()
}

func (s *ProfileStore) IsActive(sourceKey string) bool {
	s.mu.RLock()
	_, ok := s.slots[sourceKey]
	s.mu.RUnlock()
	return ok
}

// ActiveSlots returns source keys for all open slots and the subset that have buffered data.
func (s *ProfileStore) ActiveSlots() (activeKeys []string, keysWithData []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, slot := range s.slots {
		activeKeys = append(activeKeys, k)
		if slot.Buffer != nil && slot.Buffer.Size() > 0 {
			keysWithData = append(keysWithData, k)
		}
	}
	return activeKeys, keysWithData
}

// RunCleanup is used for ttlSeconds based background goroutine for store slots cleanup.
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

func (s *ProfileStore) cleanupExpired() {
	now := time.Now()
	reqCutoff := now.Add(-time.Duration(s.ttlSeconds) * time.Second)
	dataCutoffNano := now.Add(-time.Duration(s.dataRetentionSeconds) * time.Second).UnixNano()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, slot := range s.slots {
		// Keep while a request is recent (an open tab keeps the source warm)...
		if !slot.LastRequestAt.Before(reqCutoff) {
			continue
		}
		// ...or while it holds data that arrived within the retention window, so a
		// populated profile survives at least dataRetentionSeconds with no polling.
		if d := slot.LastDataAt.Load(); d != 0 && d >= dataCutoffNano {
			continue
		}
		delete(s.slots, k)
	}
}

// StopCleanup stops the TTL cleanup goroutine
func (s *ProfileStore) StopCleanup() {
	if s.stopCleanup != nil {
		s.stopCleanup()
	}
}
