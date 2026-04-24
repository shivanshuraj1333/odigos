package profiles

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestBoundedBuffer_AddEvictsWholeOldestChunks(t *testing.T) {
	b := NewBoundedBuffer(9)
	require.True(t, b.Add([]byte("12345")))
	require.True(t, b.Add([]byte("67890")))
	require.Equal(t, 5, b.Size())
	require.Equal(t, [][]byte{[]byte("67890")}, b.Snapshot())
	// Evictions from rolling overflow are not "drops" — both chunks fit individually.
	require.Equal(t, uint64(0), b.DroppedOversizedChunks())
}

func TestBoundedBuffer_AddDropsOversizeChunkWhenCapped(t *testing.T) {
	b := NewBoundedBuffer(5)
	require.False(t, b.Add([]byte("1234567890")))
	require.Equal(t, 0, b.Size())
	require.Nil(t, b.Snapshot())
	require.Equal(t, uint64(1), b.DroppedOversizedChunks())
	// Further oversized Adds keep incrementing.
	require.False(t, b.Add([]byte("abcdefg")))
	require.Equal(t, uint64(2), b.DroppedOversizedChunks())
}

func TestBoundedBuffer_MaxBytesZeroTrimsImmediately(t *testing.T) {
	b := NewBoundedBuffer(0)
	require.False(t, b.Add([]byte("12")))
	require.Equal(t, 0, b.Size())
	require.Nil(t, b.Snapshot())
	require.Equal(t, uint64(1), b.DroppedOversizedChunks())
}

func TestBoundedBuffer_Clear(t *testing.T) {
	b := NewBoundedBuffer(100)
	b.Add([]byte("abc"))
	b.Add([]byte("de"))
	require.Equal(t, 5, b.Size())
	b.Clear()
	require.Equal(t, 0, b.Size())
	require.Nil(t, b.Snapshot())
	b.Add([]byte("x"))
	require.Equal(t, 1, b.Size())
}

func TestBoundedBuffer_SnapshotShallowCopiesSliceHeaders(t *testing.T) {
	b := NewBoundedBuffer(100)
	payload := []byte("hello")
	b.Add(payload)
	s1 := b.Snapshot()
	s2 := b.Snapshot()
	require.Len(t, s1, 1)
	require.Len(t, s2, 1)
	require.Equal(t, "hello", string(s1[0]))
	require.Equal(t, "hello", string(s2[0]))
	require.Equal(t, unsafe.SliceData(s1[0]), unsafe.SliceData(s2[0]))
}
