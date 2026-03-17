package collectorprofiles

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
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
	// Debug: list and download raw profile dumps (for copying from pod when kubectl cp is not available).
	if dir := GetProfileDumpDir(); dir != "" {
		r.GET("/debug/profile-dumps", handleListProfileDumps)
		r.GET("/debug/profile-dumps/:filename", handleGetProfileDumpFile)
	}
}

func handleEnableProfiling(c *gin.Context, store ProfileStoreRef) {
	id, err := sourceIDFromParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := SourceKeyFromSourceID(id)
	store.StartViewing(key)
	profilingDebugLog("[profiling] enable: sourceKey=%q (namespace=%q kind=%q name=%q)", key, id.Namespace, id.Kind, id.Name)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "sourceKey": key})
}

func handleGetProfileData(c *gin.Context, store ProfileStoreRef) {
	id, err := sourceIDFromParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := SourceKeyFromSourceID(id)
	store.StartViewing(key)
	chunks := store.GetProfileData(key)
	if chunks == nil {
		profilingDebugLog("[profiling] get: sourceKey=%q chunks=0 (no slot or empty)", key)
		c.JSON(http.StatusOK, flamegraph.FlamebearerProfile{
			Version: 1,
			Flamebearer: flamegraph.Flamebearer{
				Names:    []string{"total"},
				Levels:   [][]int64{},
				NumTicks: 0,
				MaxSelf:  0,
			},
			Metadata: pyroscopeMetadata(0),
		})
		return
	}
	profilingDebugLog("[profiling] get: sourceKey=%q chunks=%d", key, len(chunks))
	tree := flamegraph.NewTree()
	for _, b := range chunks {
		parsed, err := flamegraph.ParseOTLPChunk(b)
		if err != nil {
			continue
		}
		for _, s := range parsed.Samples {
			tree.InsertStack(s.Value, s.Stack...)
		}
	}
	fb := flamegraph.TreeToFlamebearer(tree, 1024)
	total := fb.NumTicks
	c.JSON(http.StatusOK, flamegraph.FlamebearerProfile{
		Version:     1,
		Flamebearer: fb,
		Metadata:    pyroscopeMetadata(total),
		Timeline:    pyroscopeTimeline(total),
		Groups:      nil,
		Heatmap:     nil,
		Symbols:     tree.SymbolTable(),
	})
}

// pyroscopeMetadata returns metadata in Pyroscope API shape (format, spyName, sampleRate, units, name).
func pyroscopeMetadata(numTicks int64) flamegraph.FlamebearerMetadata {
	return flamegraph.FlamebearerMetadata{
		Format:     "single",
		SpyName:    "ebpf",
		SampleRate: 1000000000, // 1 GHz typical for CPU profiling
		Units:      "samples",
		Name:       "cpu",
	}
}

// pyroscopeTimeline returns a minimal timeline so the response matches Pyroscope (single bucket with total).
func pyroscopeTimeline(numTicks int64) *flamegraph.FlamebearerTimeline {
	if numTicks == 0 {
		return nil
	}
	return &flamegraph.FlamebearerTimeline{
		StartTime:     0,
		Samples:       []int64{0, numTicks},
		DurationDelta: 15,
		Watermarks:    nil,
	}
}

var errMissingParams = errors.New("missing namespace, kind, or name")

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
	namespace := c.Param("namespace")
	kindStr := c.Param("kind")
	name := c.Param("name")
	if namespace == "" || kindStr == "" || name == "" {
		return common.SourceID{}, errMissingParams
	}
	kind := normalizeWorkloadKind(kindStr)
	return common.SourceID{Namespace: namespace, Kind: kind, Name: name}, nil
}
