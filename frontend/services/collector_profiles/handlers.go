package collectorprofiles

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/frontend/services/common"
	"github.com/odigos-io/odigos/frontend/services/collector_profiles/flamegraph"
)

// RegisterProfilingRoutes adds routes for "enable continuous profiling" and "get profile data".
// namespace, kind, name are path params (e.g. /api/sources/:namespace/:kind/:name/profiling).
func RegisterProfilingRoutes(r *gin.RouterGroup, store ProfileStoreRef) {
	if store == nil {
		return
	}
	// Enable continuous profiling for a source (creates/refreshes slot).
	r.PUT("/sources/:namespace/:kind/:name/profiling/enable", func(c *gin.Context) {
		handleEnableProfiling(c, store)
	})
	// Get profile data for a source (snapshot of buffer).
	r.GET("/sources/:namespace/:kind/:name/profiling", func(c *gin.Context) {
		handleGetProfileData(c, store)
	})
	// Debug: list active profiling slots and which have data (for debugging empty flame graphs).
	r.GET("/debug/profiling-slots", func(c *gin.Context) {
		active, withData := store.DebugSlots()
		c.JSON(http.StatusOK, gin.H{"activeKeys": active, "keysWithData": withData})
	})
	// Debug: return raw first chunk JSON for a source (to inspect dictionary: stringTable, functionTable, locationTable).
	r.GET("/debug/sources/:namespace/:kind/:name/profiling-chunk", func(c *gin.Context) {
		handleGetProfilingChunkDebug(c, store)
	})
	// Debug: list and download raw profile dumps (for copying from pod when kubectl cp is not available).
	if dir := GetProfileDumpDir(); dir != "" {
		r.GET("/debug/profile-dumps", handleListProfileDumps)
		r.GET("/debug/profile-dumps/:filename", handleGetProfileDumpFile)
	}
}

func handleEnableProfiling(c *gin.Context, store ProfileStoreRef) {
	out, err := EnableProfilingForSource(store, c.Param("namespace"), c.Param("kind"), c.Param("name"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": out.Status, "sourceKey": out.SourceKey, "maxSlots": out.MaxSlots, "activeSlots": out.ActiveSlots})
}

func handleGetProfilingChunkDebug(c *gin.Context, store ProfileStoreRef) {
	id, err := SourceIDFromStrings(c.Param("namespace"), c.Param("kind"), c.Param("name"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := SourceKeyFromSourceID(id)
	chunks := store.GetProfileData(key)
	if len(chunks) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no chunks", "sourceKey": key})
		return
	}
	c.Data(http.StatusOK, "application/json", chunks[0])
}

func handleGetProfileData(c *gin.Context, store ProfileStoreRef) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[backend-profiling] GET panic: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("internal error: %v", r)})
		}
	}()
	wantDebug := c.Query("debug") == "1"
	out, err := GetProfilingForSource(store, c.Param("namespace"), c.Param("kind"), c.Param("name"), wantDebug)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if wantDebug {
		if out.EmptySlot {
			c.JSON(http.StatusOK, gin.H{"profile": out.Profile, "debug": out.Debug, "debugReason": out.DebugReason})
			return
		}
		c.JSON(http.StatusOK, gin.H{"profile": out.Profile, "debug": out.Debug})
		return
	}
	c.JSON(http.StatusOK, out.Profile)
}

// BuildPyroscopeProfileFromChunks parses OTLP profile chunks (dump format: resourceProfiles + dictionary),
// merges samples into a tree, and returns a Pyroscope-compatible response (version, flamebearer, metadata, timeline).
// Per-chunk transformation uses flamegraph.SamplesFromOTLPChunk (Pyroscope ConvertOtelToGoogle first, then JSON fallback).
//
// ProfileBuildDebug holds debug info from building a profile from chunks (for ?debug=1).
type ProfileBuildDebug struct {
	ChunkCount           int `json:"chunkCount"`
	NumTicks             int64 `json:"numTicks"`
	ParseErrors          int `json:"parseErrors"`
	ChunksWithSamples    int `json:"chunksWithSamples"`
	ChunksViaPyroscope   int `json:"chunksViaPyroscope"`
	ChunksViaJSONFallback int `json:"chunksViaJSONFallback"`
}

func BuildPyroscopeProfileFromChunks(chunks [][]byte) flamegraph.FlamebearerProfile {
	profile, _ := BuildPyroscopeProfileFromChunksWithDebug(chunks)
	return profile
}

func BuildPyroscopeProfileFromChunksWithDebug(chunks [][]byte) (flamegraph.FlamebearerProfile, ProfileBuildDebug) {
	debug := ProfileBuildDebug{ChunkCount: len(chunks)}
	tree := flamegraph.NewTree()
	bpInfof("build_profile: start chunk_count=%d", len(chunks))
	for i, b := range chunks {
		samples, st := flamegraph.SamplesFromOTLPChunk(b)
		switch st.Route {
		case flamegraph.RoutePyroscopeOTLP:
			debug.ChunksViaPyroscope++
		case flamegraph.RouteJSONFallback:
			debug.ChunksViaJSONFallback++
		case flamegraph.RouteError:
			debug.ParseErrors++
			bpInfof("build_profile: chunk[%d] transform_error bytes=%d pyroscope_reason=%q json_err=%v",
				i, st.ByteLen, st.PyroscopeFailReason, st.JSONFallbackErr)
			continue
		}
		if len(samples) > 0 {
			debug.ChunksWithSamples++
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
	debug.NumTicks = fb.NumTicks
	startTimeSec := extractStartTimeFromChunks(chunks)
	meta := pyroscopeMetadata(fb.NumTicks)
	if allNamesArePlaceholders(fb.Names) {
		meta.SymbolsHint = "Symbols unavailable. Ensure the collector sends full OTLP profile dictionaries (Pyroscope-shaped path)."
	}
	if os.Getenv("PROFILE_BUILD_SUMMARY") != "" {
		b, _ := json.Marshal(debug)
		bpInfof("build_profile: summary_json=%s", string(b))
	}
	bpInfof("build_profile: done num_ticks=%d names=%d levels=%d pyroscope_chunks=%d json_fallback_chunks=%d parse_errors=%d",
		fb.NumTicks, len(fb.Names), len(fb.Levels), debug.ChunksViaPyroscope, debug.ChunksViaJSONFallback, debug.ParseErrors)
	return flamegraph.FlamebearerProfile{
		Version:     1,
		Flamebearer: fb,
		Metadata:    meta,
		Timeline:    pyroscopeTimeline(fb.NumTicks, startTimeSec),
		Groups:      nil,
		Heatmap:     nil,
		Symbols:     nil,
	}, debug
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

// pyroscopeMetadata returns metadata in Pyroscope API shape (format, spyName, sampleRate, units, name).
func pyroscopeMetadata(numTicks int64) flamegraph.FlamebearerMetadata {
	return flamegraph.FlamebearerMetadata{
		Format:     "single",
		SpyName:    "", // match Pyroscope (empty)
		SampleRate: 1000000000,
		Units:      "samples",
		Name:       "cpu",
	}
}

// pyroscopeTimeline returns a minimal timeline so the response matches Pyroscope (single bucket with total).
// startTimeSec is Unix seconds from earliest profile in chunks (0 if unknown).
func pyroscopeTimeline(numTicks int64, startTimeSec int64) *flamegraph.FlamebearerTimeline {
	if numTicks == 0 {
		return nil
	}
	return &flamegraph.FlamebearerTimeline{
		StartTime:     startTimeSec,
		Samples:       []int64{0, numTicks},
		DurationDelta: 15,
		Watermarks:    nil, // Pyroscope uses null
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

func getKey(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}


func handleListProfileDumps(c *gin.Context) {
	dir := GetProfileDumpDir()
	if dir == "" {
		c.JSON(http.StatusOK, gin.H{"files": []string{}})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	c.JSON(http.StatusOK, gin.H{"files": names})
}

func handleGetProfileDumpFile(c *gin.Context) {
	dir := GetProfileDumpDir()
	if dir == "" {
		c.Status(http.StatusNotFound)
		return
	}
	filename := c.Param("filename")
	if filename == "" || strings.Contains(filename, "..") || filepath.Clean(filename) != filename {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}
	path := filepath.Join(dir, filename)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dir)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.Status(http.StatusNotFound)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "application/json", data)
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

func sourceIDFromParams(c *gin.Context) (common.SourceID, error) {
	return SourceIDFromStrings(c.Param("namespace"), c.Param("kind"), c.Param("name"))
}
