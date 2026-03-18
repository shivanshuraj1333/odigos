package collectorprofiles

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

var jsonMarshaler pprofile.JSONMarshaler

// dumpDir is set from PROFILE_DEBUG_DUMP_DIR at first use; when non-empty, raw profile chunks are written there.
var dumpDir string
var dumpSeq atomic.Uint64

const defaultDumpDir = "profile-dumps"

func init() {
	dumpDir = os.Getenv("PROFILE_DEBUG_DUMP_DIR")
	switch strings.ToLower(dumpDir) {
	case "off", "disabled", "false":
		dumpDir = ""
	case "":
		dumpDir = defaultDumpDir
	}
	if dumpDir != "" {
		if err := os.MkdirAll(dumpDir, 0755); err != nil {
			log.Printf("[profiling] profile dump mkdir %q: %v", dumpDir, err)
			dumpDir = ""
		}
	}
}

// GetProfileDumpDir returns the directory where raw profile chunks are written (empty if disabled).
func GetProfileDumpDir() string {
	return dumpDir
}

// NewProfilesConsumer returns an xconsumer.Profiles that routes incoming profile data
// to the store only for sources that are in the active set (have a slot).
// Batches are not guaranteed to be one service: the eBPF profiler / DC / gateway do not
// batch by source key. We group by source key and only merge resource profiles that share
// the same key, so each stored chunk is exactly one service. OTLP proto has dictionary at
// root (ProfilesData), not per sample; we keep it per chunk so the parser can resolve symbols.
func NewProfilesConsumer(store *ProfileStore) (xconsumer.Profiles, error) {
	return xconsumer.NewProfiles(func(ctx context.Context, pd pprofile.Profiles) error {
		rps := pd.ResourceProfiles()
		n := rps.Len()
		if n == 0 {
			return nil
		}
		log.Printf("[profiling] receiver: batch with %d resource profile(s)", n)
		profilingDebugLog("[profiling] receiver: batch with %d resource profile(s)", n)

		// Group resource profile indices by source key (same service only). No fixed n or bucket limit.
		byKey := make(map[string][]int)
		for i := 0; i < n; i++ {
			rp := rps.At(i)
			key, ok := SourceKeyFromResource(rp.Resource().Attributes())
			if !ok {
				profilingDebugLog("[profiling] receiver: dropped resource (no source key); have attributes: %s", attrsToDebugString(rp.Resource().Attributes()))
				continue
			}
			byKey[key] = append(byKey[key], i)
		}
		// Log grouping result so we can verify one chunk per key and no arbitrary cap.
		for k, idxs := range byKey {
			profilingDebugLog("[profiling] receiver: grouped key=%q rpCount=%d", k, len(idxs))
		}

		storedAny := false
		for key, indices := range byKey {
			if !store.IsActive(key) {
				profilingDebugLog("[profiling] receiver: dropped sourceKey=%q (not active/viewing)", key)
				continue
			}
			if len(indices) == 0 {
				continue
			}
			storedAny = true
			if len(indices) == 1 {
				storeOne(store, pd, rps, indices[0])
				continue
			}
			// Merge only resource profiles with the same key (same service).
			merged := pprofile.NewProfiles()
			pd.Dictionary().CopyTo(merged.Dictionary())
			for _, idx := range indices {
				single := pprofile.NewProfiles()
				pd.Dictionary().CopyTo(single.Dictionary())
				rps.At(idx).CopyTo(single.ResourceProfiles().AppendEmpty())
				if err := single.MergeTo(merged); err != nil {
					log.Printf("[profiling] merge error for key %q: %v", key, err)
					continue
				}
			}
			bytes, err := jsonMarshaler.MarshalProfiles(merged)
			if err != nil {
				log.Printf("[profiling] marshal error for key %q: %v", key, err)
				continue
			}
			hasDict := len(bytes) > 0 && (strings.Contains(string(bytes), "stringTable") || strings.Contains(string(bytes), "functionTable") || strings.Contains(string(bytes), "locationTable"))
			log.Printf("[profiling] stored merged chunk key=%q size=%d rpCount=%d dictionary=%v", key, len(bytes), len(indices), hasDict)
			if !hasDict {
				profilingDebugLog("[profiling] receiver: merged chunk sourceKey=%q has no dictionary", key)
			}
			store.AddProfileData(key, bytes)
			profilingDebugLog("[profiling] receiver: stored merged chunk sourceKey=%q size=%d (merged %d resource profiles)", key, len(bytes), len(indices))
			if dumpDir != "" {
				writeRawProfileDump(key, bytes)
			}
		}
		if !storedAny && n > 0 {
			log.Printf("[profiling] dropped all %d resource profile(s) (no active slot or key extraction failed)", n)
			profilingDebugLog("[profiling] receiver: dropped all %d resource profile(s) (no matching active slot or key extraction failed)", n)
		}
		return nil
	}, consumer.WithCapabilities(consumer.Capabilities{MutatesData: false}))
}

// storeOne stores a single resource profile (index i in rps) as its own chunk, if the source is active.
func storeOne(store *ProfileStore, pd pprofile.Profiles, rps pprofile.ResourceProfilesSlice, i int) {
	rp := rps.At(i)
	attrs := rp.Resource().Attributes()
	key, ok := SourceKeyFromResource(attrs)
	if !ok {
		profilingDebugLog("[profiling] receiver: dropped resource (no source key); have attributes: %s", attrsToDebugString(attrs))
		return
	}
	if !store.IsActive(key) {
		profilingDebugLog("[profiling] receiver: dropped sourceKey=%q (not active/viewing)", key)
		return
	}
	newPd := pprofile.NewProfiles()
	pd.Dictionary().CopyTo(newPd.Dictionary())
	rp.CopyTo(newPd.ResourceProfiles().AppendEmpty())
	bytes, err := jsonMarshaler.MarshalProfiles(newPd)
	if err != nil {
		log.Printf("[profiling] marshal error for source %q: %v", key, err)
		return
	}
	hasDict := len(bytes) > 0 && (strings.Contains(string(bytes), "stringTable") || strings.Contains(string(bytes), "functionTable") || strings.Contains(string(bytes), "locationTable"))
	log.Printf("[profiling] stored single chunk key=%q size=%d dictionary=%v", key, len(bytes), hasDict)
	if !hasDict {
		profilingDebugLog("[profiling] receiver: chunk sourceKey=%q has no dictionary (symbols will show as frame_N); add backend symbolization or have exporter fill dictionary", key)
	}
	store.AddProfileData(key, bytes)
	profilingDebugLog("[profiling] receiver: stored chunk sourceKey=%q size=%d", key, len(bytes))
	if dumpDir != "" {
		writeRawProfileDump(key, bytes)
	}
}

// writeRawProfileDump writes profile JSON (post gRPC unmarshal, same as store) to dumpDir.
// Use the file with ParseOTLPChunk(dumpBytes) locally to iterate on parser and Pyroscope-format output.
// Filename: {sanitizedSourceKey}_{unixNano}_{seq}.json
func writeRawProfileDump(sourceKey string, rawJSON []byte) {
	sanitized := strings.ReplaceAll(sourceKey, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, " ", "_")
	seq := dumpSeq.Add(1)
	name := sanitized + "_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatUint(seq, 10) + ".json"
	path := filepath.Join(dumpDir, name)
	if err := os.WriteFile(path, rawJSON, 0644); err != nil {
		log.Printf("[profiling] dump write failed: %v", err)
		return
	}
	profilingDebugLog("[profiling] dump wrote %s (%d bytes)", path, len(rawJSON))
}

// attrsToDebugString returns a short string of attribute keys for debug logs (e.g. "k8s.namespace.name,k8s.pod.name").
func attrsToDebugString(attrs pcommon.Map) string {
	var keys []string
	attrs.Range(func(k string, _ pcommon.Value) bool {
		keys = append(keys, k)
		return len(keys) <= 15
	})
	return strings.Join(keys, ",")
}
