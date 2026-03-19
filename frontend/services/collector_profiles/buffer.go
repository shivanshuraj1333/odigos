package collectorprofiles

import (
	"sync"
)

const (
	defaultSlotMaxBytes = 20 * 1024 * 1024 // 20 MB per slot
	maxSnapshotChunks   = 50               // cap GET snapshot to avoid large allocs and merge cost
)

// BoundedBuffer keeps a size-bounded list of profile data chunks (raw bytes).
// Oldest chunks are dropped when total size exceeds maxBytes.
type BoundedBuffer struct {
	mu       sync.Mutex
	chunks   [][]byte
	totalBytes int
	maxBytes  int
}

// NewBoundedBuffer creates a buffer with the given max size in bytes.
// If maxBytes <= 0, defaultSlotMaxBytes (20 MB) is used.
func NewBoundedBuffer(maxBytes int) *BoundedBuffer {
	if maxBytes <= 0 {
		maxBytes = defaultSlotMaxBytes
	}
	return &BoundedBuffer{maxBytes: maxBytes}
}

// Add appends a chunk and trims from the front until total size is at most maxBytes.
func (b *BoundedBuffer) Add(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chunks = append(b.chunks, chunk)
	b.totalBytes += len(chunk)
	for len(b.chunks) > 0 && b.totalBytes > b.maxBytes {
		old := b.chunks[0]
		b.chunks = b.chunks[1:]
		b.totalBytes -= len(old)
	}
}

// Snapshot returns a copy of at most the last maxSnapshotChunks chunks (for read-only use by API).
// Capping avoids large allocations and merge cost on GET when the buffer is full.
func (b *BoundedBuffer) Snapshot() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.chunks) == 0 {
		return nil
	}
	n := len(b.chunks)
	if n > maxSnapshotChunks {
		n = maxSnapshotChunks
	}
	start := len(b.chunks) - n
	out := make([][]byte, n)
	for i := range out {
		chunk := b.chunks[start+i]
		out[i] = make([]byte, len(chunk))
		copy(out[i], chunk)
	}
	return out
}

// Size returns current total bytes held.
func (b *BoundedBuffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.totalBytes
}
