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
func NewProfilesConsumer(store *ProfileStore) (xconsumer.Profiles, error) {
	return xconsumer.NewProfiles(func(ctx context.Context, pd pprofile.Profiles) error {
		rps := pd.ResourceProfiles()
		for i := 0; i < rps.Len(); i++ {
			rp := rps.At(i)
			attrs := rp.Resource().Attributes()
			key, ok := SourceKeyFromResource(attrs)
			if !ok {
				profilingDebugLog("[profiling] receiver: dropped resource (no source key); have attributes: %s", attrsToDebugString(attrs))
				continue
			}
			if !store.IsActive(key) {
				profilingDebugLog("[profiling] receiver: dropped sourceKey=%q (not active/viewing)", key)
				continue
			}
			// Copy this resource's profiles (already unmarshaled from gRPC) and marshal to JSON.
			// We dump this JSON so you can run ParseOTLPChunk(dumpBytes) locally to iterate on
			// parser and Pyroscope-format flamegraph logic without hitting the cluster.
			newPd := pprofile.NewProfiles()
			rp.CopyTo(newPd.ResourceProfiles().AppendEmpty())
			bytes, err := jsonMarshaler.MarshalProfiles(newPd)
			if err != nil {
				log.Printf("profiles: marshal error for source %q: %v", key, err)
				continue
			}
			store.AddProfileData(key, bytes)
			profilingDebugLog("[profiling] receiver: stored chunk sourceKey=%q size=%d", key, len(bytes))
			if dumpDir != "" {
				writeRawProfileDump(key, bytes)
			}
		}
		return nil
	}, consumer.WithCapabilities(consumer.Capabilities{MutatesData: false}))
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
