package common

// ProfileMemoryStats summarizes buffered profiling data and configured limits for the UI / GraphQL.
type ProfileMemoryStats struct {
	TotalBytes          int
	MaxSlots            int
	SlotMaxBytes        int // per-(source, profile type) byte budget
	SlotTTLSeconds      int
	MaxTotalBytesBudget int // worst-case ≈ maxSlots × profileTypes × slotMaxBytes
}

// ProfileStoreRef is the narrow API GraphQL and OTLP use from the profiling buffer.
// Buffers are partitioned per (source, profile type): CPU and each memory signal
// have independent byte budgets + TTLs, so GetProfileData selects one type.
type ProfileStoreRef interface {
	EnsureSlot(sourceKey string)
	RemoveSlot(sourceKey string)
	ClearSlotBuffer(sourceKey string) bool
	// GetProfileData returns buffered chunks for one (source, profileType).
	// profileType is normalized by the store; "" or "cpu" selects CPU.
	GetProfileData(sourceKey, profileType string) [][]byte
	MaxSlots() int
	ActiveSlots() (activeKeys []string, keysWithData []string)
	MemoryStats() ProfileMemoryStats
}
