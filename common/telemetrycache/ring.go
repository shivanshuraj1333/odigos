package telemetrycache

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"sort"
	"time"
	"fmt"

	commonlogger "github.com/odigos-io/odigos/common/logger"
)

// ringHeaderSize is the number of bytes reserved at the start of every slot.
// Layout (little-endian):
//
//	[0:8]   capturedAtNano int64   unix nanoseconds
//	[8:12]  dataLen        uint32  byte count of payload (0 = empty/overwritten)
//	[12:16] reserved       uint32  zero
const ringHeaderSize = 16

// ownerKey identifies one (sourceKey, subType) pair in the ring index.
type ownerKey struct {
	sourceKey string
	subType   string
}

// MemoryRingStore is a fixed-size, pre-allocated ring buffer for raw byte
// chunks. The pool is allocated once; writes are O(1). A single RWMutex
// guards both the pool and the index — critical sections are memory copies
// (microsecond range), so coarser locking is acceptable for this workload.
// Sharding can be added later if profiling identifies lock contention.
type MemoryRingStore struct {
	mu       sync.RWMutex
	pool     []byte
	slotSize int
	numSlots int
	head     int // next slot to claim; protected by mu

	// forward: (sourceKey,subType) → owned slot positions
	forward map[ownerKey][]int
	// reverse: slot position → current owner (len == numSlots)
	reverse []ownerKey

	signal Signal

	// metrics — read under RLock or atomically outside any lock
	writesTotal     atomic.Uint64
	overwritesTotal atomic.Uint64
	oversizedDrops  atomic.Uint64
	gateRejections  atomic.Uint64
}

// newMemoryRingStore allocates the ring. slotSize must be >= ringHeaderSize+1.
func newMemoryRingStore(sig Signal, slotSize int, budgetBytes int64) *MemoryRingStore {
	numSlots := int(budgetBytes) / slotSize
	if numSlots < 1 {
		numSlots = 1
	}
	return &MemoryRingStore{
		signal:   sig,
		pool:     make([]byte, numSlots*slotSize),
		slotSize: slotSize,
		numSlots: numSlots,
		forward:  make(map[ownerKey][]int),
		reverse:  make([]ownerKey, numSlots),
	}
}

// Write stores data under (sourceKey, subType). Chunks larger than the slot
// payload are dropped (logged, metric incremented) — never block or panic.
func (r *MemoryRingStore) Write(sourceKey, subType string, capturedAt time.Time, data []byte) error {
	payloadMax := r.slotSize - ringHeaderSize
	if len(data) > payloadMax {
		r.oversizedDrops.Add(1)
		commonlogger.LoggerCompat().Warn("telemetrycache: chunk oversized, dropped",
			"signal", r.signal, "sourceKey", sourceKey, "subType", subType,
			"chunkBytes", len(data), "slotPayload", payloadMax)
		return fmt.Errorf("telemetrycache: chunk %d B exceeds slot payload %d B for signal %s (dropped)", len(data), payloadMax, r.signal)
	}
	if len(data) == 0 || sourceKey == "" {
		return nil
	}

	r.mu.Lock()

	slot := r.head
	r.head = (r.head + 1) % r.numSlots

	// Evict the old occupant from the index.
	oldOwner := r.reverse[slot]
	if oldOwner.sourceKey != "" {
		r.removeFromForwardLocked(oldOwner, slot)
		r.overwritesTotal.Add(1)
	}

	// Write header into pool.
	base := slot * r.slotSize
	binary.LittleEndian.PutUint64(r.pool[base:], uint64(capturedAt.UnixNano()))
	binary.LittleEndian.PutUint32(r.pool[base+8:], uint32(len(data)))
	binary.LittleEndian.PutUint32(r.pool[base+12:], 0)

	// Write payload into pool.
	copy(r.pool[base+ringHeaderSize:base+ringHeaderSize+len(data)], data)

	// Update index.
	newOwner := ownerKey{sourceKey: sourceKey, subType: subType}
	r.forward[newOwner] = append(r.forward[newOwner], slot)
	r.reverse[slot] = newOwner

	r.mu.Unlock()
	r.writesTotal.Add(1)
	return nil
}

// ReadAll returns copies of all chunks owned by (sourceKey, subType), sorted
// by CapturedAt ascending. The copies are independent of the pool — safe to
// hold after the lock is released.
func (r *MemoryRingStore) ReadAll(sourceKey, subType string) ([]Chunk, error) {
	key := ownerKey{sourceKey: sourceKey, subType: subType}

	r.mu.RLock()
	defer r.mu.RUnlock()

	positions := r.forward[key] // direct reference; valid for duration of RLock
	if len(positions) == 0 {
		return nil, nil
	}

	chunks := make([]Chunk, 0, len(positions))
	for _, pos := range positions {
		base := pos * r.slotSize
		tNano := int64(binary.LittleEndian.Uint64(r.pool[base:]))
		dataLen := int(binary.LittleEndian.Uint32(r.pool[base+8:]))
		if dataLen == 0 || dataLen > r.slotSize-ringHeaderSize {
			continue // slot overwritten between index build and this read
		}
		data := make([]byte, dataLen)
		copy(data, r.pool[base+ringHeaderSize:base+ringHeaderSize+dataLen])
		chunks = append(chunks, Chunk{
			CapturedAt: time.Unix(0, tNano),
			Data:       data,
		})
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].CapturedAt.Before(chunks[j].CapturedAt)
	})
	return chunks, nil
}

// Drop removes all stored data for sourceKey (all sub-types).
func (r *MemoryRingStore) Drop(sourceKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, positions := range r.forward {
		if key.sourceKey != sourceKey {
			continue
		}
		for _, pos := range positions {
			r.reverse[pos] = ownerKey{}
			// Zero dataLen so stale reads return nothing.
			binary.LittleEndian.PutUint32(r.pool[pos*r.slotSize+8:], 0)
		}
		delete(r.forward, key)
	}
}

// SubTypes returns the sub-types with at least one stored chunk for sourceKey.
func (r *MemoryRingStore) SubTypes(sourceKey string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for key, positions := range r.forward {
		if key.sourceKey == sourceKey && len(positions) > 0 {
			out = append(out, key.subType)
		}
	}
	sort.Strings(out)
	return out
}

// Sources returns all source keys with at least one stored chunk.
func (r *MemoryRingStore) Sources() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]struct{}, len(r.forward))
	for key, positions := range r.forward {
		if len(positions) > 0 {
			seen[key.sourceKey] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// BytesUsed returns the sum of actual payload bytes across all live slots.
func (r *MemoryRingStore) BytesUsed() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var total int64
	for _, positions := range r.forward {
		for _, pos := range positions {
			total += int64(binary.LittleEndian.Uint32(r.pool[pos*r.slotSize+8:]))
		}
	}
	return total
}

// Capacity returns the total usable payload capacity across all slots.
func (r *MemoryRingStore) Capacity() int64 {
	return int64(r.numSlots) * int64(r.slotSize-ringHeaderSize)
}

// ClearAll zeroes the index and marks all slots as empty.
// The arena pool allocation is preserved — no GC work.
func (r *MemoryRingStore) ClearAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forward = make(map[ownerKey][]int)
	r.reverse = make([]ownerKey, r.numSlots)
	// Zero out dataLen for every slot so stale reads return nothing.
	for i := 0; i < r.numSlots; i++ {
		binary.LittleEndian.PutUint32(r.pool[i*r.slotSize+8:], 0)
	}
	r.head = 0
}

// Stats returns an observable snapshot of the ring's current state.
func (r *MemoryRingStore) Stats() RingStats {
	return RingStats{
		Signal:          r.signal,
		BytesUsed:       r.BytesUsed(),
		Capacity:        r.Capacity(),
		SlotSize:        int64(r.slotSize),
		NumSlots:        int64(r.numSlots),
		WritesTotal:     r.writesTotal.Load(),
		OverwritesTotal: r.overwritesTotal.Load(),
		OversizedDrops:  r.oversizedDrops.Load(),
		GateRejections:  r.gateRejections.Load(),
	}
}

// removeFromForwardLocked removes slot from forward[key]. Must be called under mu.
func (r *MemoryRingStore) removeFromForwardLocked(key ownerKey, slot int) {
	positions := r.forward[key]
	for i, p := range positions {
		if p == slot {
			last := len(positions) - 1
			positions[i] = positions[last]
			positions[last] = 0
			r.forward[key] = positions[:last]
			if last == 0 {
				delete(r.forward, key)
			}
			return
		}
	}
}

// ── SignalRing ────────────────────────────────────────────────────────────────

// SignalRing combines a MemoryRingStore with an optional admission gate.
type SignalRing struct {
	l1   *MemoryRingStore
	gate *admissionGate // nil when GateEnabled == false
}

func newSignalRing(sig Signal, cfg SignalConfig) *SignalRing {
	slotSize := int(cfg.SlotSizeBytes)
	sr := &SignalRing{
		l1: newMemoryRingStore(sig, slotSize, cfg.L1BudgetBytes),
	}
	if cfg.GateEnabled {
		sr.gate = &admissionGate{highWaterMark: cfg.GateHighWaterMark}
	}
	return sr
}

func (sr *SignalRing) write(sourceKey, subType string, capturedAt time.Time, data []byte) error {
	if sr.gate != nil && !sr.gate.allow(sr.l1) {
		sr.l1.gateRejections.Add(1)
		return nil // graceful drop under burst — not an error
	}
	return sr.l1.Write(sourceKey, subType, capturedAt, data)
}

// ── admissionGate ─────────────────────────────────────────────────────────────

// admissionGate drops writes when the ring is above the high watermark. This
// prevents bursty traffic from thrashing the ring (hot path stays fast; only
// the gate check — one division — is added to each write).
type admissionGate struct {
	highWaterMark float64
}

func (g *admissionGate) allow(r *MemoryRingStore) bool {
	cap := r.Capacity()
	if cap <= 0 {
		return true
	}
	used := r.BytesUsed()
	return float64(used)/float64(cap) < g.highWaterMark
}
