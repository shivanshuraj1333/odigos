package profiles

import (
	"context"
	"strings"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofileotlp"
)

type OdigosProfilesConsumer struct {
	store    *ProfileStore
	gate     *IngestGate
	profiles xconsumer.Profiles
}

// NewOdigosProfilesConsumer builds a profiles consumer for the given store.
// gate controls whether incoming OTLP profiles are stored; when false, batches are dropped.
func NewOdigosProfilesConsumer(store *ProfileStore, gate *IngestGate) (*OdigosProfilesConsumer, error) {
	profilesConsumer := &OdigosProfilesConsumer{store: store, gate: gate}
	profiles, err := xconsumer.NewProfiles(
		profilesConsumer.consume,
		consumer.WithCapabilities(consumer.Capabilities{MutatesData: false}),
	)
	if err != nil {
		return nil, err
	}
	profilesConsumer.profiles = profiles
	return profilesConsumer, nil
}

func (c *OdigosProfilesConsumer) GetConsumer() xconsumer.Profiles {
	return c.profiles
}

func (c *OdigosProfilesConsumer) consume(ctx context.Context, incomingBatch pprofile.Profiles) error {
	// IngestGate is flipped by k8s watcher watching the effective-config.
	// The OTLP profiles receiver stays up; when the gate is off we drop batches here.
	if c.gate != nil && !c.gate.IsEnabled() {
		return nil
	}
	resourceProfiles := incomingBatch.ResourceProfiles()
	numResources := resourceProfiles.Len()
	if numResources == 0 {
		return nil
	}

	for idx := 0; idx < numResources; idx++ {
		attrs := resourceProfiles.At(idx).Resource().Attributes()
		sourceKey, ok := SourceKeyFromResource(attrs)
		if !ok {
			continue
		}
		if !c.store.IsActive(sourceKey) {
			continue
		}
		appendResourceProfileChunk(c.store, sourceKey, incomingBatch, resourceProfiles, idx)
	}
	return nil
}

// appendResourceProfileChunk marshals a single resource's profiles as OTLP protobuf
// and appends it to the correct per-type buffer in the slot.
func appendResourceProfileChunk(store *ProfileStore, sourceKey string, incomingBatch pprofile.Profiles, resourceProfiles pprofile.ResourceProfilesSlice, resourceIndex int) {
	log := commonlogger.LoggerCompat().With("subsystem", "backend-profiling")

	profType := profileTypeKeyFromRP(incomingBatch.Dictionary(), resourceProfiles.At(resourceIndex))

	singleResourceChunk := buildSingleResourceProfilesFromBatch(incomingBatch, resourceProfiles, resourceIndex)
	exportReq := pprofileotlp.NewExportRequestFromProfiles(singleResourceChunk)
	chunkBytes, marshalErr := exportReq.MarshalProto()
	if marshalErr != nil {
		log.Warn("store_chunk", "sourceKey", sourceKey, "err", marshalErr)
		return
	}
	store.AddProfileData(sourceKey, profType, chunkBytes)
}

// buildSingleResourceProfilesFromBatch builds a standalone pprofile.Profiles message
// holding one ResourceProfiles entry from the batch.
func buildSingleResourceProfilesFromBatch(
	incomingBatch pprofile.Profiles,
	resourceProfiles pprofile.ResourceProfilesSlice,
	resourceIndex int,
) pprofile.Profiles {
	out := pprofile.NewProfiles()
	// Copy the full batch dictionary into each stored chunk.
	// OTLP Profiles wire format uses a shared dictionary (string table, mappings, attribute tables).
	incomingBatch.Dictionary().CopyTo(out.Dictionary())
	resourceProfiles.At(resourceIndex).CopyTo(out.ResourceProfiles().AppendEmpty())
	return out
}

// profileTypeKeyFromRP extracts a human-readable profile sub-type key
// ("cpu", "memory", or "gpu") from the first Profile found in rp, using the
// batch-level string dictionary to resolve TypeStrindex values.
// Falls back to "cpu" when the type cannot be determined.
func profileTypeKeyFromRP(dict pprofile.ProfilesDictionary, rp pprofile.ResourceProfiles) string {
	strtable := dict.StringTable()
	resolve := func(idx int32) string {
		if idx >= 0 && int(idx) < strtable.Len() {
			return strings.ToLower(strtable.At(int(idx)))
		}
		return ""
	}

	sps := rp.ScopeProfiles()
	for j := 0; j < sps.Len(); j++ {
		ps := sps.At(j).Profiles()
		for k := 0; k < ps.Len(); k++ {
			p := ps.At(k)
			period := resolve(p.PeriodType().TypeStrindex())
			sample := resolve(p.SampleType().TypeStrindex())
			return classifyProfileType(period, sample)
		}
	}
	return "cpu"
}

// classifyProfileType maps raw OTLP type strings to the three canonical keys
// the cache uses: "cpu", "memory", "gpu".
func classifyProfileType(periodType, sampleType string) string {
	for _, s := range []string{periodType, sampleType} {
		switch {
		case strings.Contains(s, "cpu") || strings.Contains(s, "wall"):
			return "cpu"
		case strings.Contains(s, "alloc") || strings.Contains(s, "heap") ||
			strings.Contains(s, "inuse") || strings.Contains(s, "memory"):
			return "memory"
		case strings.Contains(s, "gpu"):
			return "gpu"
		}
	}
	return "cpu"
}
