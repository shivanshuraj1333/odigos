package collectorprofiles

import (
	"errors"
	"log"

	"github.com/odigos-io/odigos/frontend/services/common"
	"github.com/odigos-io/odigos/frontend/services/collector_profiles/flamegraph"
)

// ErrMissingProfilingParams is returned when namespace, kind, or name is empty.
var ErrMissingProfilingParams = errors.New("missing namespace, kind, or name")

// SourceIDFromStrings builds a SourceID from HTTP/GraphQL path parameters.
func SourceIDFromStrings(namespace, kindStr, name string) (common.SourceID, error) {
	if namespace == "" || kindStr == "" || name == "" {
		return common.SourceID{}, ErrMissingProfilingParams
	}
	kind := normalizeWorkloadKind(kindStr)
	return common.SourceID{Namespace: namespace, Kind: kind, Name: name}, nil
}

// EnableProfilingOutput is the result of enabling on-demand profiling for a source.
type EnableProfilingOutput struct {
	Status      string `json:"status"`
	SourceKey   string `json:"sourceKey"`
	MaxSlots    int    `json:"maxSlots"`
	ActiveSlots int    `json:"activeSlots"`
}

// EnableProfilingForSource creates or refreshes a profiling slot for the given workload (REST and GraphQL).
func EnableProfilingForSource(store ProfileStoreRef, namespace, kindStr, name string) (*EnableProfilingOutput, error) {
	id, err := SourceIDFromStrings(namespace, kindStr, name)
	if err != nil {
		return nil, err
	}
	key := SourceKeyFromSourceID(id)
	store.StartViewing(key)
	activeKeys, _ := store.DebugSlots()
	log.Printf("[profiling] enable: sourceKey=%q namespace=%q kind=%q name=%q", key, id.Namespace, id.Kind, id.Name)
	profilingDebugLog("[profiling] enable: sourceKey=%q (namespace=%q kind=%q name=%q)", key, id.Namespace, id.Kind, id.Name)
	return &EnableProfilingOutput{
		Status:      "ok",
		SourceKey:   key,
		MaxSlots:    store.MaxSlots(),
		ActiveSlots: len(activeKeys),
	}, nil
}

// GetProfilingOutput is the resolved profile for a source (REST body or GraphQL fields).
type GetProfilingOutput struct {
	Profile     flamegraph.FlamebearerProfile
	Debug       *ProfileBuildDebug
	DebugReason string
	EmptySlot   bool
}

// GetProfilingForSource loads buffered OTLP chunks and builds a Pyroscope-shaped profile (REST and GraphQL).
func GetProfilingForSource(store ProfileStoreRef, namespace, kindStr, name string, wantDebug bool) (*GetProfilingOutput, error) {
	id, err := SourceIDFromStrings(namespace, kindStr, name)
	if err != nil {
		return nil, err
	}
	key := SourceKeyFromSourceID(id)
	store.StartViewing(key)
	chunks := store.GetProfileData(key)

	if chunks == nil {
		log.Printf("[profiling] get: sourceKey=%q chunks=0 (no slot or empty)", key)
		profilingDebugLog("[profiling] get: sourceKey=%q chunks=0 (no slot or empty)", key)
		out := &GetProfilingOutput{
			Profile: emptyFlamebearerProfile(),
			EmptySlot: true,
		}
		if wantDebug {
			z := ProfileBuildDebug{ChunkCount: 0, NumTicks: 0}
			out.Debug = &z
			out.DebugReason = "no_slot_or_empty"
		}
		return out, nil
	}

	log.Printf("[profiling] get: sourceKey=%q chunks=%d", key, len(chunks))
	profilingDebugLog("[profiling] get: sourceKey=%q chunks=%d", key, len(chunks))
	profile, buildDebug := BuildPyroscopeProfileFromChunksWithDebug(chunks)
	log.Printf("[profiling] build: sourceKey=%q chunkCount=%d numTicks=%d parseErrors=%d chunksWithSamples=%d namesCount=%d",
		key, buildDebug.ChunkCount, buildDebug.NumTicks, buildDebug.ParseErrors, buildDebug.ChunksWithSamples, len(profile.Flamebearer.Names))
	if buildDebug.ParseErrors > 0 {
		log.Printf("[profiling] build: sourceKey=%q parseErrors=%d (some chunks failed to parse)", key, buildDebug.ParseErrors)
	}
	if buildDebug.ChunkCount > 0 && buildDebug.ChunksWithSamples == 0 && buildDebug.NumTicks == 0 {
		log.Printf("[profiling] build: sourceKey=%q chunks have no samples or all failed (chunkCount=%d)", key, buildDebug.ChunkCount)
	}
	out := &GetProfilingOutput{Profile: profile, EmptySlot: false}
	if wantDebug {
		bd := buildDebug
		out.Debug = &bd
	}
	return out, nil
}

func emptyFlamebearerProfile() flamegraph.FlamebearerProfile {
	return flamegraph.FlamebearerProfile{
		Version: 1,
		Flamebearer: flamegraph.Flamebearer{
			Names:    []string{"total"},
			Levels:   [][]int64{},
			NumTicks: 0,
			MaxSelf:  0,
		},
		Metadata: pyroscopeMetadata(0),
	}
}
