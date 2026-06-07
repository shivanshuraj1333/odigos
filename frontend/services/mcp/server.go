package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	collectormetrics "github.com/odigos-io/odigos/frontend/services/collector_metrics"
	fecommon "github.com/odigos-io/odigos/frontend/services/common"
)

const (
	serverName    = "odigos-mcp"
	serverVersion = "0.4.0"

	// EnvVarMCPEnabled gates the /mcp endpoint behind a single feature flag.
	// Set ODIGOS_MCP_ENABLED=true on the frontend pod to register /mcp.
	// No auth in this build — intended for kubectl port-forwarded local
	// dev / demo. Don't expose publicly until a bearer-token mode is added.
	EnvVarMCPEnabled = "ODIGOS_MCP_ENABLED"
)

// Deps is the dependency bundle every domain register-function receives. It
// mirrors the GraphQL Resolver fields so MCP tools reuse the exact services
// the UI does. Add a field here only when a new domain genuinely needs it.
type Deps struct {
	Logger          logr.Logger
	MetricsConsumer *collectormetrics.OdigosMetricsConsumer
	PromAPI         v1.API
	K8sCacheClient  client.Client
	ProfileStore    fecommon.ProfileStoreRef
}

// NewServer builds the MCP server with every domain registered. Adding a new
// domain = one new file in this package, one new register*Tools call below.
func NewServer(deps Deps) *server.MCPServer {
	s := server.NewMCPServer(serverName, serverVersion,
		server.WithToolHandlerMiddleware(auditMiddleware),
	)

	// Resources (CRD-shaped).
	registerSourceTools(s, deps)
	registerWorkloadTools(s, deps)
	registerDestinationTools(s, deps)
	registerActionTools(s, deps)
	registerRuleTools(s, deps)
	registerCustomInstrumentationTools(s, deps)
	registerDataStreamTools(s, deps)

	// Cluster control + diagnostics.
	registerControlTools(s, deps)
	registerSamplingTools(s, deps)
	registerDiagnoseTools(s, deps)

	// Live introspection + topology.
	registerMetricsTools(s, deps)
	registerTopologyTools(s, deps)
	registerCollectorTools(s, deps)

	// Configuration + presets + raw K8s + profiling.
	registerConfigTools(s, deps)
	registerPresetTools(s, deps)
	registerProfilingTools(s, deps)
	registerManifestTools(s, deps)

	return s
}

// Handler returns the http.Handler for /mcp. Returns nil if the feature flag
// is not set — the caller must not mount in that case (safe default).
func Handler(deps Deps) http.Handler {
	if !mcpEnabled() {
		return nil
	}
	return server.NewStreamableHTTPServer(NewServer(deps),
		server.WithStateLess(true),
		server.WithEndpointPath("/mcp"),
	)
}

func mcpEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvVarMCPEnabled)))
	return v == "1" || v == "true" || v == "yes"
}

// auditMiddleware writes one glanceable line per tool call to stderr (the pod
// log), tagging each READ or WRITE, so an operator can see at a glance what an
// agent did against the cluster — e.g.:
//
//	14:34:39.092  MCP-AUDIT  READ   cluster  (k8s)  list_sources           ok      12ms
//	14:34:41.310  MCP-AUDIT  WRITE  cluster  (k8s)  create_sampling_group  ok      34ms
func auditMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		res, err := next(ctx, req)
		outcome := "ok"
		if err != nil {
			outcome = "error"
		} else if res != nil && res.IsError {
			outcome = "tool_error"
		}
		fmt.Fprintf(os.Stderr, "%s  MCP-AUDIT  %-5s  %-8s (%-3s)  %-28s  %-10s  %5dms\n",
			time.Now().Format("15:04:05.000"), toolCategory(req.Params.Name), "cluster", "k8s",
			req.Params.Name, outcome, time.Since(start).Milliseconds())
		return res, err
	}
}

// toolCategory classifies a tool READ or WRITE from its verb prefix; the Odigos
// MCP read tools are all list_/get_/describe_/test_, everything else mutates.
func toolCategory(name string) string {
	for _, p := range []string{"list_", "get_", "describe_", "test_"} {
		if strings.HasPrefix(name, p) {
			return "READ"
		}
	}
	return "WRITE"
}
