package profiles

import (
	"context"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

var protoMarshaler pprofile.ProtoMarshaler

type OdigosProfilesConsumer struct {
	store *ProfileStore
	otlp  xconsumer.Profiles
}

// NewOdigosProfilesConsumer builds a profiles consumer for the given store.
func NewOdigosProfilesConsumer(store *ProfileStore) (*OdigosProfilesConsumer, error) {
	c := &OdigosProfilesConsumer{store: store}
	otlp, err := xconsumer.NewProfiles(
		c.consume,
		consumer.WithCapabilities(consumer.Capabilities{MutatesData: false}),
	)
	if err != nil {
		return nil, err
	}
	c.otlp = otlp
	return c, nil
}

func (c *OdigosProfilesConsumer) OTLPProfiles() xconsumer.Profiles {
	return c.otlp
}

func (c *OdigosProfilesConsumer) consume(ctx context.Context, incomingBatch pprofile.Profiles) error {
	log := commonlogger.LoggerCompat().With("subsystem", "backend-profiling")
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
			// Open a profiling slot in the UI for this exact key (namespace/Kind/name) or batches are dropped.
			log.Debug("profile_batch_skip_inactive_slot", "sourceKey", sourceKey)
			continue
		}
		appendResourceProfileChunk(c.store, sourceKey, incomingBatch, resourceProfiles, idx)
	}
	return nil
}

// appendResourceProfileChunk marshals a single resource's profiles as OTLP protobuf and appends
// to the store slot. sourceKey is already derived and validated by the caller.
func appendResourceProfileChunk(store *ProfileStore, sourceKey string, incomingBatch pprofile.Profiles, resourceProfiles pprofile.ResourceProfilesSlice, resourceIndex int) {
	log := commonlogger.LoggerCompat().With("subsystem", "backend-profiling")
	singleResourceChunk := buildSingleResourceProfilesFromBatch(incomingBatch, resourceProfiles, resourceIndex)
	chunkBytes, marshalErr := protoMarshaler.MarshalProfiles(singleResourceChunk)
	if marshalErr != nil {
		log.Warn("store_chunk", "sourceKey", sourceKey, "err", marshalErr)
		return
	}
	store.AddProfileData(sourceKey, chunkBytes)
	log.Debug("stored_profile_chunk", "sourceKey", sourceKey, "bytes", len(chunkBytes))
}

// buildSingleResourceProfilesFromBatch builds a standalone pprofile.Profiles message
// holding one ResourceProfiles entry from the batch.
func buildSingleResourceProfilesFromBatch(
	incomingBatch pprofile.Profiles,
	resourceProfiles pprofile.ResourceProfilesSlice,
	resourceIndex int,
) pprofile.Profiles {
	out := pprofile.NewProfiles()
	// Copy the full batch dictionary into each stored chunk: OTLP Profiles wire format uses a shared
	// dictionary (string table, mappings, attribute tables). Indices in ResourceProfiles / Profiles /
	// Samples point into that dictionary; without copying it, a standalone marshaled chunk would have
	// dangling indices once the batch is released or when decoded in isolation.
	incomingBatch.Dictionary().CopyTo(out.Dictionary())
	resourceProfiles.At(resourceIndex).CopyTo(out.ResourceProfiles().AppendEmpty())
	return out
}
