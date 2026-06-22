// Package telemetrycache provides the BoundedBuffer primitive used by both the
// odigos frontend (k8s) and vm-agent (VM) to store raw OTLP proto chunks
// in-memory with a byte-budget ceiling and oldest-first eviction.
//
// Each side builds its own store on top of this shared buffer:
//   - odigos frontend: on-demand slots (EnsureSlot/RemoveSlot) keyed by
//     namespace/kind/name, used for the Odigos UI flame graph.
//   - vm-agent: auto-evicting LRU store keyed by service.name, used for
//     local diagnostic profiling (odictl hot-functions / flame view).
package telemetrycache

import (
	"sync"
	"time"
)

const (
	// DefaultMaxBytes is the per-buffer byte budget used when the caller passes <= 0.
	DefaultMaxBytes = 64 << 20 // 64 MiB

	// compactThreshold triggers a slice copy when the live segment drops below
	// this fraction of the underlying array cap, preventing memory leaks on
	// long-running buffers with steady eviction.
	compactThreshold = 2
)

// chunk is one OTLP ExportXServiceRequest blob. capturedAt records wall-clock
// time at Add() so callers can filter with SnapshotSince.
type chunk struct {
	capturedAt time.Time
	bytes      []byte
}

// BoundedBuffer is a byte-budgeted ring of OTLP proto chunks.
// When totalBytes exceeds maxBytes, whole oldest chunks are dropped.
// All methods are safe for concurrent use.
type BoundedBuffer struct {
	mu         sync.RWMutex
	chunks     []chunk
	totalBytes int
	maxBytes   int

	addedTotal   uint64
	evictedTotal uint64
}

// BufferStats is the observable snapshot of a BoundedBuffer.
type BufferStats struct {
	LiveChunks   int    `json:"liveChunks"`
	LiveBytes    int    `json:"liveBytes"`
	MaxBytes     int    `json:"maxBytes"`
	AddedTotal   uint64 `json:"addedTotal"`
	EvictedTotal uint64 `json:"evictedTotal"`
}

// NewBoundedBuffer returns a buffer that drops oldest whole chunks when
// totalBytes exceeds maxBytes. maxBytes <= 0 uses DefaultMaxBytes.
func NewBoundedBuffer(maxBytes int) *BoundedBuffer {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &BoundedBuffer{maxBytes: maxBytes}
}

// Add stores one chunk captured at capturedAt and evicts oldest entries until
// totalBytes <= maxBytes.
func (b *BoundedBuffer) Add(capturedAt time.Time, data []byte) {
	if len(data) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.chunks = append(b.chunks, chunk{capturedAt: capturedAt, bytes: data})
	b.totalBytes += len(data)
	b.addedTotal++

	dropped := 0
	for dropped < len(b.chunks) && b.totalBytes > b.maxBytes {
		b.totalBytes -= len(b.chunks[dropped].bytes)
		b.chunks[dropped] = chunk{}
		dropped++
	}
	if dropped == 0 {
		return
	}
	b.evictedTotal += uint64(dropped)

	live := b.chunks[dropped:]
	if cap(b.chunks) >= compactThreshold*len(live) && cap(b.chunks) > 16 {
		fresh := make([]chunk, len(live))
		copy(fresh, live)
		b.chunks = fresh
	} else {
		b.chunks = live
	}
}

// SnapshotSince returns a shallow copy of chunks captured at or after since.
// Pass time.Time{} (zero) to get all chunks.
func (b *BoundedBuffer) SnapshotSince(since time.Time) [][]byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.chunks) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(b.chunks))
	for _, c := range b.chunks {
		if since.IsZero() || !c.capturedAt.Before(since) {
			out = append(out, c.bytes)
		}
	}
	return out
}

// Snapshot returns all buffered chunks (equivalent to SnapshotSince(time.Time{})).
func (b *BoundedBuffer) Snapshot() [][]byte {
	return b.SnapshotSince(time.Time{})
}

// Size returns the current total byte count.
func (b *BoundedBuffer) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.totalBytes
}

// Clear removes all buffered chunks and resets byte accounting.
func (b *BoundedBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chunks = nil
	b.totalBytes = 0
}

// Stats returns the current observable state of the buffer.
func (b *BoundedBuffer) Stats() BufferStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return BufferStats{
		LiveChunks:   len(b.chunks),
		LiveBytes:    b.totalBytes,
		MaxBytes:     b.maxBytes,
		AddedTotal:   b.addedTotal,
		EvictedTotal: b.evictedTotal,
	}
}
