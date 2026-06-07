package mcp

import (
	"net/http"
	"os"
	"strings"

	"github.com/go-logr/logr"
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
	s := server.NewMCPServer(serverName, serverVersion)

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
