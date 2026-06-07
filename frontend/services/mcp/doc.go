// Package mcp is the Odigos MCP (Model Context Protocol) server. It lives in
// the frontend module because it wraps the existing frontend/services/* layer
// the UI/GraphQL already use — same process, no HTTP/GraphQL round-trip, full
// access to the in-memory ProfileStore + Prometheus + K8s cache client.
//
// # Architecture
//
//   - One package, flat layout (no nested directories). Mirrors mcp-grafana
//     and SigNoz, the two closest reference servers built on mark3labs/mcp-go.
//   - One file per domain. Adding a feature = drop a new file with a
//     registerXTools(s, deps) function and add one line to server.go.
//   - A small set of shared infrastructure files at package root:
//     annotations.go, args.go, scope.go, responses.go.
//   - Large domains (sampling) are split by axis: sampling.go owns the group
//     lifecycle + tool registration, with one file per rule family
//     (sampling_noisy.go / sampling_highly_relevant.go /
//     sampling_cost_reduction.go) and one helper file (sampling_matchers.go).
//     This mirrors mcp-grafana's alerting_*.go split.
//
// # Standardized tool template
//
//	func registerXTools(s *server.MCPServer, deps Deps) {
//	    s.AddTool(
//	        mcp.NewTool("name", readAnno()|writeAnno(),
//	            mcp.WithDescription("what + when-to-use + size warning + which compact tool to prefer"),
//	            mcp.WithString("param", mcp.Required(), mcp.Description("...")),
//	        ),
//	        handler(deps),
//	    )
//	}
//
//	func handler(deps Deps) server.ToolHandlerFunc {
//	    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
//	        // 1. Parse via the args.go / scope.go helpers; bad input -> errorWithOptions(...)
//	        // 2. Call frontend/services/* (or kube.DefaultClient) — never reimplement
//	        // 3. Wrap the success path with withGuidance(data, ctxNote, nextSteps)
//	    }
//	}
//
// # Self-documenting responses
//
// Every successful tool response is wrapped via withGuidance(...) which adds
// two fields beyond the data: a human-readable "context" note explaining what
// the data means, and a "next_steps" list pre-filled with sibling tool calls
// the agent likely wants next. This is what lets an AI agent independently
// diagnose and configure Odigos without a human in the loop.
//
// Error responses use errorWithOptions(...) which surfaces valid enum values
// so the agent can self-correct on the next call (cribl-mcp pattern).
//
// # Write-safety contract
//
// Every mutating tool exposes a dry_run parameter defaulting to true. Tools
// annotate themselves with DestructiveHint so MCP clients can prompt before
// executing. The feature flag ODIGOS_MCP_ENABLED gates whether /mcp is
// mounted at all.
//
// # Deployment
//
//	1. Set ODIGOS_MCP_ENABLED=true on the frontend pod.
//	2. kubectl -n odigos-system port-forward svc/ui 3000:3000
//	3. Point Cursor / Claude Desktop at http://localhost:3000/mcp
//
// No CLI required, no auth in this build.
package mcp
