package collectorprofiles

import (
	"context"
	"log"
	"strings"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

var jsonMarshaler pprofile.JSONMarshaler

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
			// Copy this resource's profiles into a new Profiles and marshal to bytes.
			newPd := pprofile.NewProfiles()
			rp.CopyTo(newPd.ResourceProfiles().AppendEmpty())
			bytes, err := jsonMarshaler.MarshalProfiles(newPd)
			if err != nil {
				log.Printf("profiles: marshal error for source %q: %v", key, err)
				continue
			}
			store.AddProfileData(key, bytes)
			profilingDebugLog("[profiling] receiver: stored chunk sourceKey=%q size=%d", key, len(bytes))
		}
		return nil
	}, consumer.WithCapabilities(consumer.Capabilities{MutatesData: false}))
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
