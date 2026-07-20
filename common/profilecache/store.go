// Package profilecache is the shared in-memory cache of raw OTLP CPU-profile
// chunks.
// Each source gets one byte-bounded buffer; sourceKey is an opaque string chosen
// by the caller (namespace/kind/name in k8s, service.name on a VM).
package profilecache

import (
	"context"
	"sync"
	"time"
)

// Slot is one source's byte-bounded profile buffer. lastRequestAt drives both LRU
// eviction and the TTL sweep.
type Slot struct {
	lastRequestAt time.Time
	buffer        *BoundedBuffer
}

// Store keeps at most maxSlots source-keyed slots. A slot is evicted when the count
// hits maxSlots (least-recently-used), when it is idle past ttlSeconds, or, when a
// global cap is configured, when the total buffered bytes exceed it.
type Store struct {
	mu              sync.RWMutex
	slots           map[string]*Slot
	maxSlots        int
	ttlSeconds      int
	slotMaxBytes    int
	maxTotalBytes   int
	cleanupInterval time.Duration
	stopCleanup     func()
	evictedSlots    uint64
}

// MemoryStats summarizes buffered data and configured limits for the UI / TUI.
type MemoryStats struct {
	TotalBytes          int
	MaxSlots            int
	SlotMaxBytes        int
	SlotTTLSeconds      int
	MaxTotalBytesBudget int
}

// StoreRef is the read/lifecycle API the frontend GraphQL layer depends on.
type StoreRef interface {
	EnsureSlot(sourceKey string)
	RemoveSlot(sourceKey string)
	ClearSlotBuffer(sourceKey string) bool
	GetProfileData(sourceKey string) [][]byte
	MaxSlots() int
	ActiveSlots() (activeKeys []string, keysWithData []string)
	MemoryStats() MemoryStats
}

// Default cache limits, applied by NewStore for any non-positive argument.
const (
	DefaultMaxSlots        = 100
	DefaultSlotMaxBytes    = 5 << 20   // 5 MiB per source
	DefaultMaxTotalBytes   = 500 << 20 // 500 MiB across all sources
	DefaultSlotTTLSeconds  = 15 * 60   // 15 minutes
	DefaultCleanupInterval = time.Minute
)

// Option configures optional Store behavior.
type Option func(*Store)

// WithGlobalByteCap bounds the total bytes buffered across all slots; whole
// least-recently-used slots are evicted to stay within it. 0 (default) disables it.
func WithGlobalByteCap(maxTotalBytes int) Option {
	return func(s *Store) { s.maxTotalBytes = maxTotalBytes }
}

func NewStore(maxSlots, ttlSeconds, slotMaxBytes int, cleanupInterval time.Duration, opts ...Option) *Store {
	if maxSlots <= 0 {
		maxSlots = DefaultMaxSlots
	}
	if ttlSeconds <= 0 {
		ttlSeconds = DefaultSlotTTLSeconds
	}
	if slotMaxBytes <= 0 {
		slotMaxBytes = DefaultSlotMaxBytes
	}
	if cleanupInterval <= 0 {
		cleanupInterval = DefaultCleanupInterval
	}
	s := &Store{
		slots:           make(map[string]*Slot),
		maxSlots:        maxSlots,
		ttlSeconds:      ttlSeconds,
		slotMaxBytes:    slotMaxBytes,
		cleanupInterval: cleanupInterval,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Store) evictOldestSlotLocked(protect string) bool {
	var oldestKey string
	var oldest time.Time
	for k, slot := range s.slots {
		if k == protect {
			continue
		}
		if oldestKey == "" || slot.lastRequestAt.Before(oldest) {
			oldest = slot.lastRequestAt
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

func (s *Store) totalBytesLocked() int {
	total := 0
	for _, slot := range s.slots {
		if slot.buffer != nil {
			total += slot.buffer.Size()
		}
	}
	return total
}

// EnsureSlot opens a slot for sourceKey, or refreshes its request time if present.
func (s *Store) EnsureSlot(sourceKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot, ok := s.slots[sourceKey]; ok {
		slot.lastRequestAt = time.Now()
		return
	}
	if len(s.slots) >= s.maxSlots {
		s.evictOldestSlotLocked("")
	}
	s.slots[sourceKey] = &Slot{lastRequestAt: time.Now(), buffer: NewBoundedBuffer(s.slotMaxBytes)}
}

func (s *Store) RemoveSlot(sourceKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.slots, sourceKey)
}

func (s *Store) ClearAllSlots() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slots = make(map[string]*Slot)
}

// ClearSlotBuffer empties a source's buffer but keeps the slot.
func (s *Store) ClearSlotBuffer(sourceKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.slots[sourceKey]
	if !ok || slot == nil {
		return false
	}
	slot.lastRequestAt = time.Now()
	if slot.buffer != nil {
		slot.buffer.Clear()
	}
	return true
}

func (s *Store) MaxSlots() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxSlots
}

// Reconfigure changes cache limits at runtime; any argument <= 0 leaves that
// limit unchanged. Evicts/trims to fit immediately; ttlSeconds applies next sweep.
func (s *Store) Reconfigure(maxSlots, slotMaxBytes, ttlSeconds, maxTotalBytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxSlots > 0 {
		s.maxSlots = maxSlots
		for len(s.slots) > s.maxSlots {
			if !s.evictOldestSlotLocked("") {
				break
			}
		}
	}
	if slotMaxBytes > 0 && slotMaxBytes != s.slotMaxBytes {
		s.slotMaxBytes = slotMaxBytes
		for _, slot := range s.slots {
			if slot.buffer != nil {
				slot.buffer.Resize(slotMaxBytes)
			}
		}
	}
	if ttlSeconds > 0 {
		s.ttlSeconds = ttlSeconds
	}
	if maxTotalBytes > 0 {
		s.maxTotalBytes = maxTotalBytes
	}
	for s.maxTotalBytes > 0 && s.totalBytesLocked() > s.maxTotalBytes && len(s.slots) > 1 {
		if !s.evictOldestSlotLocked("") {
			break
		}
	}
}

func (s *Store) MemoryStats() MemoryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	budget := s.maxTotalBytes
	if budget == 0 {
		budget = s.maxSlots * s.slotMaxBytes
	}
	return MemoryStats{
		TotalBytes:          s.totalBytesLocked(),
		MaxSlots:            s.maxSlots,
		SlotMaxBytes:        s.slotMaxBytes,
		SlotTTLSeconds:      s.ttlSeconds,
		MaxTotalBytesBudget: budget,
	}
}

// AddProfileData appends a chunk to an existing slot; it is a no-op if the slot was
// not opened (the caller gates which sources are stored).
func (s *Store) AddProfileData(sourceKey string, chunk []byte) {
	s.mu.RLock()
	slot, ok := s.slots[sourceKey]
	s.mu.RUnlock()
	if !ok || slot == nil || slot.buffer == nil {
		return
	}
	slot.buffer.Add(chunk)
}

// IngestAt opens the slot on demand and appends a chunk stamped at capturedAt,
// advancing the slot's clock to it so idle-slot TTL is driven by data arrival — for
// callers with no separate request signal (the vm-agent). A configured global cap
// evicts other least-recently-used slots to stay within budget.
func (s *Store) IngestAt(sourceKey string, capturedAt time.Time, chunk []byte) {
	if sourceKey == "" || len(chunk) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.slots[sourceKey]
	if !ok {
		if len(s.slots) >= s.maxSlots {
			s.evictOldestSlotLocked("")
		}
		slot = &Slot{buffer: NewBoundedBuffer(s.slotMaxBytes)}
		s.slots[sourceKey] = slot
	}
	if capturedAt.After(slot.lastRequestAt) {
		slot.lastRequestAt = capturedAt
	}
	slot.buffer.AddAt(capturedAt, chunk)
	for s.maxTotalBytes > 0 && s.totalBytesLocked() > s.maxTotalBytes && len(s.slots) > 1 {
		if !s.evictOldestSlotLocked(sourceKey) {
			break
		}
	}
}

// GetProfileData returns buffered chunks for a source, refreshing its request time.
func (s *Store) GetProfileData(sourceKey string) [][]byte {
	return s.snapshot(sourceKey, time.Time{}, true)
}

// SnapshotSince returns a source's chunks captured at or after since, without
// touching the request clock.
func (s *Store) SnapshotSince(sourceKey string, since time.Time) [][]byte {
	return s.snapshot(sourceKey, since, false)
}

func (s *Store) snapshot(sourceKey string, since time.Time, bumpRequest bool) [][]byte {
	s.mu.Lock()
	slot, ok := s.slots[sourceKey]
	if ok && slot != nil && bumpRequest {
		slot.lastRequestAt = time.Now()
	}
	s.mu.Unlock()
	if !ok || slot == nil || slot.buffer == nil {
		return nil
	}
	return slot.buffer.SnapshotSince(since)
}

// SnapshotAllSince returns every source's chunks captured at or after since.
func (s *Store) SnapshotAllSince(since time.Time) [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out [][]byte
	for _, slot := range s.slots {
		if slot.buffer != nil {
			out = append(out, slot.buffer.SnapshotSince(since)...)
		}
	}
	return out
}

func (s *Store) IsActive(sourceKey string) bool {
	s.mu.RLock()
	_, ok := s.slots[sourceKey]
	s.mu.RUnlock()
	return ok
}

// ActiveSlots returns all open source keys and the subset that hold buffered data.
func (s *Store) ActiveSlots() (activeKeys []string, keysWithData []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, slot := range s.slots {
		activeKeys = append(activeKeys, k)
		if slot.buffer != nil && slot.buffer.Size() > 0 {
			keysWithData = append(keysWithData, k)
		}
	}
	return activeKeys, keysWithData
}

func (s *Store) EvictedSlots() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.evictedSlots
}

// RunCleanup starts the background TTL sweep until ctx is canceled.
func (s *Store) RunCleanup(ctx context.Context) {
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
				s.SweepNow()
			}
		}
	}()
}

// SweepNow removes slots with no request in the last ttlSeconds.
func (s *Store) SweepNow() {
	cutoff := time.Now().Add(-time.Duration(s.ttlSeconds) * time.Second)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, slot := range s.slots {
		if slot.lastRequestAt.Before(cutoff) {
			delete(s.slots, k)
		}
	}
}

func (s *Store) StopCleanup() {
	if s.stopCleanup != nil {
		s.stopCleanup()
	}
}
