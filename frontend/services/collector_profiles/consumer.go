package collectorprofiles

import (
	"context"
	"hash/fnv"
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

// maxResourceProfilesPerBatch limits how many distinct profile chunks we store per incoming batch.
// When a batch has more resource profiles than this, we group by hash(key)%maxResourceProfilesPerBatch
// and merge each group into one chunk, so we store at most this many chunks per batch.
const maxResourceProfilesPerBatch = 3

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
// When a batch has more than maxResourceProfilesPerBatch resource profiles, they are
// grouped into that many buckets (by hash of source key), merged per bucket, and stored
// under the first active source key in each bucket—so at most maxResourceProfilesPerBatch
// chunks are stored per batch.
func NewProfilesConsumer(store *ProfileStore) (xconsumer.Profiles, error) {
	return xconsumer.NewProfiles(func(ctx context.Context, pd pprofile.Profiles) error {
		rps := pd.ResourceProfiles()
		n := rps.Len()
		if n == 0 {
			return nil
		}

		if n <= maxResourceProfilesPerBatch {
			// No grouping: store one chunk per resource profile (current behavior).
			for i := 0; i < n; i++ {
				storeOne(store, pd, rps, i)
			}
			return nil
		}

		// Group resource profile indices by bucket; record first active key per bucket.
		type bucketInfo struct {
			indices    []int
			firstKey   string
			hasActive  bool
		}
		buckets := make(map[uint32]*bucketInfo)
		h := fnv.New32a()
		for i := 0; i < n; i++ {
			rp := rps.At(i)
			key, ok := SourceKeyFromResource(rp.Resource().Attributes())
			if !ok {
				profilingDebugLog("[profiling] receiver: dropped resource (no source key); have attributes: %s", attrsToDebugString(rp.Resource().Attributes()))
				continue
			}
			h.Reset()
			h.Write([]byte(key))
			b := h.Sum32() % maxResourceProfilesPerBatch
			if buckets[b] == nil {
				buckets[b] = &bucketInfo{indices: nil, firstKey: "", hasActive: false}
			}
			info := buckets[b]
			info.indices = append(info.indices, i)
			if store.IsActive(key) {
				info.hasActive = true
				if info.firstKey == "" {
					info.firstKey = key
				}
			}
		}

		for _, info := range buckets {
			if !info.hasActive || info.firstKey == "" || len(info.indices) == 0 {
				continue
			}
			merged := pprofile.NewProfiles()
			pd.Dictionary().CopyTo(merged.Dictionary())
			for _, idx := range info.indices {
				single := pprofile.NewProfiles()
				pd.Dictionary().CopyTo(single.Dictionary())
				rps.At(idx).CopyTo(single.ResourceProfiles().AppendEmpty())
				if err := single.MergeTo(merged); err != nil {
					log.Printf("profiles: merge error for bucket key %q: %v", info.firstKey, err)
					continue
				}
			}
			// Ensure the marshaled chunk has the full dictionary so the UI can resolve symbols.
			// Use the original batch dictionary so string/function/location indices in the merged
			// resource profiles correctly resolve to names (all profiles in the bucket came from pd).
			pd.Dictionary().CopyTo(merged.Dictionary())
			bytes, err := jsonMarshaler.MarshalProfiles(merged)
			if err != nil {
				log.Printf("profiles: marshal error for bucket key %q: %v", info.firstKey, err)
				continue
			}
			hasDict := len(bytes) > 0 && (strings.Contains(string(bytes), "stringTable") || strings.Contains(string(bytes), "functionTable") || strings.Contains(string(bytes), "locationTable"))
			if !hasDict {
				profilingDebugLog("[profiling] receiver: merged chunk sourceKey=%q has no dictionary", info.firstKey)
			}
			store.AddProfileData(info.firstKey, bytes)
			profilingDebugLog("[profiling] receiver: stored merged chunk sourceKey=%q size=%d (merged %d resource profiles)", info.firstKey, len(bytes), len(info.indices))
			if dumpDir != "" {
				writeRawProfileDump(info.firstKey, bytes)
			}
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
		log.Printf("profiles: marshal error for source %q: %v", key, err)
		return
	}
	hasDict := len(bytes) > 0 && (strings.Contains(string(bytes), "stringTable") || strings.Contains(string(bytes), "functionTable") || strings.Contains(string(bytes), "locationTable"))
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
