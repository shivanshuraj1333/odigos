package collectorprofiles

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/frontend/services/collector_profiles/flamegraph"
)

// ProfileBuildDebug holds debug info from building a profile from chunks (for ?debug=1).
type ProfileBuildDebug struct {
	ChunkCount         int   `json:"chunkCount"`
	NumTicks           int64 `json:"numTicks"`
	ParseErrors        int   `json:"parseErrors"`
	ChunksWithSamples  int   `json:"chunksWithSamples"`
	ChunksViaPyroscope int   `json:"chunksViaPyroscope"`
}

// BuildPyroscopeProfileFromChunks parses OTLP profile chunks and returns a Pyroscope-compatible response.
func BuildPyroscopeProfileFromChunks(chunks [][]byte) flamegraph.FlamebearerProfile {
	profile, _ := BuildPyroscopeProfileFromChunksWithDebug(chunks)
	return profile
}

// BuildPyroscopeProfileFromChunksWithDebug is like BuildPyroscopeProfileFromChunks but also returns debug info.
func BuildPyroscopeProfileFromChunksWithDebug(chunks [][]byte) (flamegraph.FlamebearerProfile, ProfileBuildDebug) {
	dbg := ProfileBuildDebug{ChunkCount: len(chunks)}
	tree := flamegraph.NewTree()
	bpInfof("build_profile: start chunk_count=%d", len(chunks))
	for i, b := range chunks {
		samples, st := flamegraph.SamplesFromOTLPChunk(b)
		switch st.Route {
		case flamegraph.RoutePyroscopeOTLP:
			dbg.ChunksViaPyroscope++
		case flamegraph.RouteError:
			dbg.ParseErrors++
			bpInfof("build_profile: chunk[%d] transform_error bytes=%d pyroscope_reason=%q",
				i, st.ByteLen, st.PyroscopeFailReason)
			continue
		}
		if len(samples) > 0 {
			dbg.ChunksWithSamples++
		}
		if len(samples) == 0 {
			bpInfof("build_profile: chunk[%d] no samples after transform route=%s bytes=%d", i, st.Route, st.ByteLen)
			continue
		}
		for _, s := range samples {
			tree.InsertStack(s.Value, s.Stack...)
		}
	}
	fb := flamegraph.TreeToFlamebearer(tree, 1024)
	dbg.NumTicks = fb.NumTicks
	startTimeSec := extractStartTimeFromChunks(chunks)
	meta := pyroscopeMetadata(fb.NumTicks)
	if allNamesArePlaceholders(fb.Names) {
		meta.SymbolsHint = "Symbols unavailable. Ensure the collector sends full OTLP profile dictionaries (Pyroscope-shaped path)."
	}
	if os.Getenv("PROFILE_BUILD_SUMMARY") != "" {
		b, _ := json.Marshal(dbg)
		bpInfof("build_profile: summary_json=%s", string(b))
	}
	bpInfof("build_profile: done num_ticks=%d names=%d levels=%d pyroscope_chunks=%d parse_errors=%d",
		fb.NumTicks, len(fb.Names), len(fb.Levels), dbg.ChunksViaPyroscope, dbg.ParseErrors)
	return flamegraph.FlamebearerProfile{
		Version:     1,
		Flamebearer: fb,
		Metadata:    meta,
		Timeline:    pyroscopeTimeline(fb.NumTicks, startTimeSec),
		Groups:      nil,
		Heatmap:     nil,
		Symbols:     nil,
	}, dbg
}

// normalizeWorkloadKind returns the canonical PascalCase kind so the source key matches
// keys built from OTLP resource attributes (e.g. "deployment" -> "Deployment").
func normalizeWorkloadKind(kindStr string) k8sconsts.WorkloadKind {
	switch strings.ToLower(kindStr) {
	case "deployment":
		return k8sconsts.WorkloadKindDeployment
	case "daemonset":
		return k8sconsts.WorkloadKindDaemonSet
	case "statefulset":
		return k8sconsts.WorkloadKindStatefulSet
	case "cronjob":
		return k8sconsts.WorkloadKindCronJob
	case "job":
		return k8sconsts.WorkloadKindJob
	case "deploymentconfig":
		return k8sconsts.WorkloadKindDeploymentConfig
	case "rollout":
		return k8sconsts.WorkloadKindArgoRollout
	case "namespace":
		return k8sconsts.WorkloadKindNamespace
	case "staticpod":
		return k8sconsts.WorkloadKindStaticPod
	default:
		return k8sconsts.WorkloadKind(kindStr)
	}
}

// pyroscopeMetadata returns metadata in Pyroscope API shape (format, spyName, sampleRate, units, name).
func pyroscopeMetadata(_ int64) flamegraph.FlamebearerMetadata {
	return flamegraph.FlamebearerMetadata{
		Format:     "single",
		SpyName:    "",
		SampleRate: 1000000000,
		Units:      "samples",
		Name:       "cpu",
	}
}

// pyroscopeTimeline returns a minimal timeline so the response matches Pyroscope (single bucket with total).
func pyroscopeTimeline(numTicks int64, startTimeSec int64) *flamegraph.FlamebearerTimeline {
	if numTicks == 0 {
		return nil
	}
	return &flamegraph.FlamebearerTimeline{
		StartTime:     startTimeSec,
		Samples:       []int64{0, numTicks},
		DurationDelta: 15,
		Watermarks:    nil,
	}
}

// extractStartTimeFromChunks returns the earliest timeUnixNano from chunks as Unix seconds, or 0 if none found.
func extractStartTimeFromChunks(chunks [][]byte) int64 {
	var minNano int64
	for _, b := range chunks {
		var raw map[string]interface{}
		if json.Unmarshal(b, &raw) != nil {
			continue
		}
		rps := getKey(raw, "resourceProfiles", "ResourceProfiles")
		arr, ok := rps.([]interface{})
		if !ok {
			continue
		}
		for _, rp := range arr {
			rpm, _ := rp.(map[string]interface{})
			if rpm == nil {
				continue
			}
			scopes := getKey(rpm, "scopeProfiles", "ScopeProfiles")
			sarr, ok := scopes.([]interface{})
			if !ok {
				continue
			}
			for _, s := range sarr {
				sm, _ := s.(map[string]interface{})
				if sm == nil {
					continue
				}
				profs := getKey(sm, "profiles", "Profiles")
				parr, ok := profs.([]interface{})
				if !ok || len(parr) == 0 {
					continue
				}
				p, _ := parr[0].(map[string]interface{})
				if p == nil {
					continue
				}
				ts := getKey(p, "timeUnixNano", "TimeUnixNano")
				if ts == nil {
					continue
				}
				var nano int64
				switch v := ts.(type) {
				case string:
					for _, c := range v {
						if c >= '0' && c <= '9' {
							nano = nano*10 + int64(c-'0')
						}
					}
				case float64:
					nano = int64(v)
				}
				if nano > 0 && (minNano == 0 || nano < minNano) {
					minNano = nano
				}
			}
		}
	}
	if minNano == 0 {
		return 0
	}
	return minNano / 1e9
}

// allNamesArePlaceholders returns true if every name is frame_N, 0x..., "total", or "other" (no real symbols).
func allNamesArePlaceholders(names []string) bool {
	for _, n := range names {
		if n == "" || n == "total" || n == "other" {
			continue
		}
		if len(n) > 6 && n[:6] == "frame_" {
			continue
		}
		if len(n) > 2 && n[:2] == "0x" {
			continue
		}
		return false
	}
	return true
}

func getKey(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}
