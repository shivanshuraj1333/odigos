package profiles

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProfileStore_ClearSlotBufferKeepsSlot(t *testing.T) {
	s := NewProfileStore(4, 3600, 1024, 0)
	key := "ns/Deployment/app"
	s.EnsureSlot(key)
	s.AddProfileData(key, []byte("chunk-one"))
	s.AddProfileData(key, []byte("chunk-two"))
	require.True(t, s.ClearSlotBuffer(key))
	chunks := s.GetProfileData(key)
	require.Empty(t, chunks)
	require.True(t, s.IsActive(key))
}

func TestProfileStore_ClearSlotBufferUnknownKey(t *testing.T) {
	s := NewProfileStore(4, 3600, 1024, 0)
	require.False(t, s.ClearSlotBuffer("ns/Deployment/missing"))
}

// TestProfileStore_EvictsOldestWhenAtCapacity verifies EnsureSlot removes the slot with
// the smallest LastRequestAt once the store hits maxSlots.
func TestProfileStore_EvictsOldestWhenAtCapacity(t *testing.T) {
	s := NewProfileStore(2, 3600, 1024, 0)
	s.EnsureSlot("k1")
	// Sleep is enough to make LastRequestAt differ; time.Now has >= ns resolution on macOS/Linux.
	time.Sleep(2 * time.Millisecond)
	s.EnsureSlot("k2")
	require.True(t, s.IsActive("k1"))
	require.True(t, s.IsActive("k2"))

	// At capacity; inserting k3 should evict the oldest (k1).
	time.Sleep(2 * time.Millisecond)
	s.EnsureSlot("k3")
	require.False(t, s.IsActive("k1"), "oldest slot should have been evicted")
	require.True(t, s.IsActive("k2"))
	require.True(t, s.IsActive("k3"))
}

// TestProfileStore_CleanupExpiredDropsStaleSlots drives cleanupExpired directly so we don't
// depend on the background ticker. Covers the core TTL invariant.
func TestProfileStore_CleanupExpiredDropsStaleSlots(t *testing.T) {
	s := NewProfileStore(8, 1 /*ttlSeconds*/, 1024, 0)
	s.EnsureSlot("fresh")
	s.EnsureSlot("stale")

	// Back-date the "stale" slot past the TTL window. We reach through the mutex because
	// we are in the same package; mirrors how cleanupExpired compares timestamps.
	s.mu.Lock()
	s.slots["stale"].LastRequestAt = time.Now().Add(-10 * time.Second)
	s.mu.Unlock()

	s.cleanupExpired()

	require.True(t, s.IsActive("fresh"))
	require.False(t, s.IsActive("stale"), "slot older than ttlSeconds should be cleaned up")
}

// TestProfileStore_RunCleanupTicksAndStops verifies the cleanup goroutine actually fires and
// that StopCleanup terminates it. Uses a short cleanupInterval and polls for eviction so we
// don't race on ticker scheduling.
func TestProfileStore_RunCleanupTicksAndStops(t *testing.T) {
	s := NewProfileStore(8, 1 /*ttlSeconds*/, 1024, 10*time.Millisecond)
	s.EnsureSlot("stale")
	s.mu.Lock()
	s.slots["stale"].LastRequestAt = time.Now().Add(-10 * time.Second)
	s.mu.Unlock()

	s.RunCleanup(context.Background())
	defer s.StopCleanup()

	require.Eventually(t, func() bool { return !s.IsActive("stale") }, time.Second, 5*time.Millisecond,
		"cleanup goroutine should evict slot older than ttlSeconds")
}

// TestProfileStore_ConcurrentAddAndSnapshot exercises the cross-goroutine happy path:
// many writers calling AddProfileData while readers call GetProfileData/ActiveSlots. The
// assertion is absence of data races (run with `go test -race`); the test also checks that
// totals stay coherent.
func TestProfileStore_ConcurrentAddAndSnapshot(t *testing.T) {
	const (
		writers        = 8
		readers        = 4
		addsPerWriter  = 200
		readsPerReader = 200
		chunkLen       = 32
		slotBytes      = 64 * 1024 // far bigger than total writes so nothing gets trimmed
	)
	s := NewProfileStore(16, 3600, slotBytes, 0)
	keys := []string{"ns/Deployment/a", "ns/Deployment/b", "ns/Deployment/c"}
	for _, k := range keys {
		s.EnsureSlot(k)
	}

	var wg sync.WaitGroup
	wg.Add(writers + readers)
	chunk := make([]byte, chunkLen)
	for i := 0; i < writers; i++ {
		go func(i int) {
			defer wg.Done()
			k := keys[i%len(keys)]
			for j := 0; j < addsPerWriter; j++ {
				s.AddProfileData(k, chunk)
			}
		}(i)
	}
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerReader; j++ {
				for _, k := range keys {
					_ = s.GetProfileData(k)
				}
				_, _ = s.ActiveSlots()
				_ = s.MemoryStats()
			}
		}()
	}
	wg.Wait()

	// Totals across the three slots should equal writers * addsPerWriter * chunkLen,
	// because no trim was triggered at these sizes.
	stats := s.MemoryStats()
	require.Equal(t, writers*addsPerWriter*chunkLen, stats.TotalBytes)
}

// TestProfileStore_MemoryStatsReportsConfigAndTotal sanity-checks the numbers GraphQL relies on.
func TestProfileStore_MemoryStatsReportsConfigAndTotal(t *testing.T) {
	s := NewProfileStore(4, 42, 1024, 0)
	s.EnsureSlot("k1")
	s.AddProfileData("k1", []byte("12345"))
	stats := s.MemoryStats()
	require.Equal(t, 5, stats.TotalBytes)
	require.Equal(t, 4, stats.MaxSlots)
	require.Equal(t, 1024, stats.SlotMaxBytes)
	require.Equal(t, 42, stats.SlotTTLSeconds)
	require.Equal(t, 4*1024, stats.MaxTotalBytesBudget)
}
