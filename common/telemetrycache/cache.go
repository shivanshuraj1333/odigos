// Package telemetrycache is a fixed-size in-memory ring-buffer cache for raw
// OTLP proto chunks. Each signal (profiles, traces) gets its own independently
// sized ring. Memory is pre-allocated once at startup and never grows — there
// are no GC-visible allocations on the hot write path.
//
// Defaults are derived from opentelemetry-collector v0.148 constants:
//   - gRPC max_recv_msg_size = 4 MiB (hard upper bound per chunk)
//   - batch_processor send_batch_size = 8192 spans / 200ms / ~50 services → ~164 spans/source
//     at ~300 B/span ≈ 49 KB → trace slot = 16 KB covers median without multi-slot spanning
//   - eBPF profiler emits 20–100 KB/chunk → profile slot = 128 KB fits all without chaining
//   - exporter queue_size = 1000 → drain channel depth = 1000
//   - memory_limiter spike = 20 % of limit → gate high watermark = 0.85 (15 % headroom)
package telemetrycache

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Signal identifies a telemetry signal type stored in the cache.
type Signal string

const (
	SignalProfiles Signal = "profiles"
	SignalTraces   Signal = "traces"
)

// Chunk is one stored blob with its capture timestamp.
type Chunk struct {
	CapturedAt time.Time
	Data       []byte
}

// RingStats is the observable state snapshot of a MemoryRingStore.
type RingStats struct {
	Signal          Signal `json:"signal"`
	BytesUsed       int64  `json:"bytesUsed"`
	Capacity        int64  `json:"capacity"`
	SlotSize        int64  `json:"slotSize"`
	NumSlots        int64  `json:"numSlots"`
	WritesTotal     uint64 `json:"writesTotal"`
	OverwritesTotal uint64 `json:"overwritesTotal"`
	OversizedDrops  uint64 `json:"oversizedDrops"`
	GateRejections  uint64 `json:"gateRejections"`
}

// SignalConfig configures one signal's ring store.
type SignalConfig struct {
	// SlotSizeBytes is the fixed size of each ring slot (header + payload).
	// Chunks exceeding (SlotSizeBytes - ringHeaderSize) are logged and dropped.
	// Minimum 256 B, maximum 1 MiB.
	SlotSizeBytes int64

	// L1BudgetBytes is the total pre-allocated arena size for the in-memory ring.
	// numSlots = L1BudgetBytes / SlotSizeBytes (floor). Minimum 16 slots.
	L1BudgetBytes int64

	// GateEnabled activates the admission gate for this signal.
	// When the ring's used capacity exceeds GateHighWaterMark, writes from
	// non-priority sources are dropped to protect the ring from burst flooding.
	// Apply only to bursty signals (traces); leave false for fixed-rate signals (profiles).
	GateEnabled bool

	// GateHighWaterMark is the fractional fill level (0–1) above which the gate
	// starts rejecting writes. Only used when GateEnabled is true.
	GateHighWaterMark float64
}

// Config is the top-level TelemetryCache configuration.
type Config struct {
	Signals map[Signal]SignalConfig
}

// DefaultConfig returns production defaults derived from OTel collector v0.148 values.
// Total in-memory footprint: profiles 192 MB + traces 128 MB + overhead ≈ 320 MB.
func DefaultConfig() Config {
	return Config{
		Signals: map[Signal]SignalConfig{
			SignalProfiles: {
				SlotSizeBytes: 128 << 10, // 128 KB — fits eBPF chunks (20–100 KB) in one slot
				L1BudgetBytes: 192 << 20, // 192 MB → 1536 slots; at 1 chunk/s ≈ 25 min history
				GateEnabled:   false,     // eBPF fixed rate, not traffic-proportional
			},
			SignalTraces: {
				SlotSizeBytes:     16 << 10, // 16 KB — covers median ResourceSpans batch (~50 spans)
				L1BudgetBytes:     128 << 20, // 128 MB → 8192 slots; at 200ms interval ≈ 27 min/source
				GateEnabled:       true,
				GateHighWaterMark: 0.85,
			},
		},
	}
}

// TelemetryCache is the top-level in-memory cache.
// Each registered signal gets its own independent ring store.
// All methods are safe for concurrent use.
type TelemetryCache struct {
	rings map[Signal]*SignalRing
}

// New allocates and validates all rings from cfg.
// Returns a non-nil error for any invalid SignalConfig — fail fast at startup.
func New(cfg Config) (*TelemetryCache, error) {
	if len(cfg.Signals) == 0 {
		return nil, errors.New("telemetrycache: config has no signals")
	}
	tc := &TelemetryCache{rings: make(map[Signal]*SignalRing, len(cfg.Signals))}
	for sig, scfg := range cfg.Signals {
		if err := validateSignalConfig(sig, scfg); err != nil {
			return nil, err
		}
		tc.rings[sig] = newSignalRing(sig, scfg)
	}
	return tc, nil
}

// NewDefault allocates a TelemetryCache with DefaultConfig. Panics on invalid
// config (static config; should never fail in production).
func NewDefault() *TelemetryCache {
	tc, err := New(DefaultConfig())
	if err != nil {
		panic(fmt.Sprintf("telemetrycache NewDefault: %v", err))
	}
	return tc
}

func validateSignalConfig(sig Signal, cfg SignalConfig) error {
	if cfg.SlotSizeBytes < 256 {
		return fmt.Errorf("telemetrycache: signal %q: slotSizeBytes %d < 256 minimum", sig, cfg.SlotSizeBytes)
	}
	if cfg.SlotSizeBytes > 1<<20 {
		return fmt.Errorf("telemetrycache: signal %q: slotSizeBytes %d > 1 MiB maximum", sig, cfg.SlotSizeBytes)
	}
	minBudget := cfg.SlotSizeBytes * 16
	if cfg.L1BudgetBytes < minBudget {
		return fmt.Errorf("telemetrycache: signal %q: l1BudgetBytes %d < minimum %d (16 slots)", sig, cfg.L1BudgetBytes, minBudget)
	}
	if cfg.GateEnabled && (cfg.GateHighWaterMark <= 0 || cfg.GateHighWaterMark >= 1) {
		return fmt.Errorf("telemetrycache: signal %q: gateHighWaterMark %.3f must be in (0,1)", sig, cfg.GateHighWaterMark)
	}
	return nil
}

// Write stores one OTLP proto chunk under (signal, sourceKey, subType).
// Chunks larger than the ring's slot payload are logged and dropped (never block).
// Gate-rejected writes return nil (not an error — expected under burst).
func (tc *TelemetryCache) Write(sig Signal, sourceKey, subType string, capturedAt time.Time, data []byte) error {
	ring, ok := tc.rings[sig]
	if !ok {
		return fmt.Errorf("telemetrycache: unknown signal %q", sig)
	}
	return ring.write(sourceKey, subType, capturedAt, data)
}

// ReadAll returns all stored chunks for (signal, sourceKey, subType), sorted
// by CapturedAt ascending. Returns nil (not an error) when no data is present.
func (tc *TelemetryCache) ReadAll(sig Signal, sourceKey, subType string) ([]Chunk, error) {
	ring, ok := tc.rings[sig]
	if !ok {
		return nil, fmt.Errorf("telemetrycache: unknown signal %q", sig)
	}
	return ring.l1.ReadAll(sourceKey, subType)
}

// ReadAllSubTypes returns all chunks for (signal, sourceKey) across every
// sub-type, sorted by CapturedAt ascending.
func (tc *TelemetryCache) ReadAllSubTypes(sig Signal, sourceKey string) ([]Chunk, error) {
	ring, ok := tc.rings[sig]
	if !ok {
		return nil, fmt.Errorf("telemetrycache: unknown signal %q", sig)
	}
	subtypes := ring.l1.SubTypes(sourceKey)
	var all []Chunk
	for _, st := range subtypes {
		chunks, err := ring.l1.ReadAll(sourceKey, st)
		if err != nil {
			return nil, err
		}
		all = append(all, chunks...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CapturedAt.Before(all[j].CapturedAt)
	})
	return all, nil
}

// ReadAllSince returns all chunks for (signal, sourceKey) across every
// sub-type captured at or after since.
func (tc *TelemetryCache) ReadAllSince(sig Signal, sourceKey string, since time.Time) ([]Chunk, error) {
	all, err := tc.ReadAllSubTypes(sig, sourceKey)
	if err != nil {
		return nil, err
	}
	if since.IsZero() {
		return all, nil
	}
	out := all[:0]
	for _, c := range all {
		if !c.CapturedAt.Before(since) {
			out = append(out, c)
		}
	}
	return out, nil
}

// ReadAllSourcesSince returns every chunk for every source for sig captured at
// or after since.
func (tc *TelemetryCache) ReadAllSourcesSince(sig Signal, since time.Time) ([]Chunk, error) {
	sources := tc.Sources(sig)
	var all []Chunk
	for _, src := range sources {
		chunks, err := tc.ReadAllSince(sig, src, since)
		if err != nil {
			return nil, err
		}
		all = append(all, chunks...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CapturedAt.Before(all[j].CapturedAt)
	})
	return all, nil
}

// Drop removes all stored data for sourceKey across all signals.
func (tc *TelemetryCache) Drop(sourceKey string) {
	for _, ring := range tc.rings {
		ring.l1.Drop(sourceKey)
	}
}

// SubTypes returns the sub-types with any stored data for (signal, sourceKey).
func (tc *TelemetryCache) SubTypes(sig Signal, sourceKey string) []string {
	ring, ok := tc.rings[sig]
	if !ok {
		return nil
	}
	return ring.l1.SubTypes(sourceKey)
}

// Sources returns all source keys with any stored data for sig.
func (tc *TelemetryCache) Sources(sig Signal) []string {
	ring, ok := tc.rings[sig]
	if !ok {
		return nil
	}
	return ring.l1.Sources()
}

// ClearSignal resets the ring for sig (zeros the index; arena stays allocated).
func (tc *TelemetryCache) ClearSignal(sig Signal) {
	ring, ok := tc.rings[sig]
	if !ok {
		return
	}
	ring.l1.ClearAll()
}

// ClearAll resets every signal ring.
func (tc *TelemetryCache) ClearAll() {
	for _, ring := range tc.rings {
		ring.l1.ClearAll()
	}
}

// RingStatsFor returns stats for a specific signal's ring.
func (tc *TelemetryCache) RingStatsFor(sig Signal) (RingStats, bool) {
	ring, ok := tc.rings[sig]
	if !ok {
		return RingStats{}, false
	}
	return ring.l1.Stats(), true
}

// Stats returns stats for all registered signal rings, sorted by signal name.
func (tc *TelemetryCache) Stats() []RingStats {
	out := make([]RingStats, 0, len(tc.rings))
	for _, ring := range tc.rings {
		out = append(out, ring.l1.Stats())
	}
	sort.Slice(out, func(i, j int) bool {
		return string(out[i].Signal) < string(out[j].Signal)
	})
	return out
}

// MarshalJSON returns a JSON snapshot of all ring stats for health/debug endpoints.
func (tc *TelemetryCache) MarshalJSON() ([]byte, error) {
	return json.Marshal(tc.Stats())
}
