package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/cli/cmd/resources"
	cmdcontext "github.com/odigos-io/odigos/cli/pkg/cmd_context"
	"github.com/odigos-io/odigos/cli/pkg/kube"
	"github.com/odigos-io/odigos/k8sutils/pkg/describe"
)

// odigos mcp ...
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP (Model Context Protocol) server backed by the Odigos cluster",
	Long: `Exposes Odigos resources to MCP-compatible AI clients (Claude Desktop, Cursor, etc.)
via the cluster credentials of the current kubectl context.`,
}

var mcpAllowWritesFlag bool

// odigos mcp serve
var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the Odigos MCP over stdio",
	Long: `Serves an MCP server over stdio. Read-only tools are always registered.
Use --allow-writes to additionally register tools that mutate cluster state
(instrumenting workloads, adding destinations, toggling profiling).
Even with --allow-writes, write tools default to dry_run=true; the caller must
pass dry_run=false explicitly to actually apply changes.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		client := cmdcontext.KubeClientFromContextOrExit(ctx)

		// Lazy: resolve once on first use so the server can boot even if the
		// cluster API is briefly unreachable; each tool call surfaces its own error.
		var (
			odigosNs    string
			odigosNsErr error
			once        sync.Once
		)
		nsGetter := func(ctx context.Context) (string, error) {
			once.Do(func() {
				odigosNs, odigosNsErr = resources.GetOdigosNamespace(client, ctx)
			})
			return odigosNs, odigosNsErr
		}

		s := server.NewMCPServer("odigos-mcp", "0.1.0")
		registerReadTools(s, client, nsGetter)
		if mcpAllowWritesFlag {
			registerWriteTools(s, client, nsGetter)
		}

		if err := server.ServeStdio(s); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "MCP serve error: %v\n", err)
		}
	},
}

// ----- read tools --------------------------------------------------------

func registerReadTools(s *server.MCPServer, client *kube.Client, odigosNs func(context.Context) (string, error)) {
	readAnno := mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    boolPtr(true),
		DestructiveHint: boolPtr(false),
		IdempotentHint:  boolPtr(true),
		OpenWorldHint:   boolPtr(true),
	})

	s.AddTool(
		mcp.NewTool("list_sources",
			readAnno,
			mcp.WithDescription("List all Odigos Sources (instrumented workloads) across the cluster. Returns workload kind, name, namespace, and whether instrumentation is disabled."),
			mcp.WithString("namespace", mcp.Description("Optional namespace filter. Omit for all namespaces.")),
		),
		listSourcesHandler(client),
	)

	s.AddTool(
		mcp.NewTool("list_destinations",
			readAnno,
			mcp.WithDescription("List all configured Odigos Destinations (telemetry backends like Datadog, Honeycomb). Returns type, name, signals, and disabled state."),
		),
		listDestinationsHandler(client, odigosNs),
	)

	s.AddTool(
		mcp.NewTool("describe_source",
			readAnno,
			mcp.WithDescription("Deeply describe a single Odigos Source: instrumentation status, runtime detection, agent enabling, pod rollout state, per-container agent configuration. Use this to troubleshoot why a workload is or is not producing telemetry."),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace of the workload.")),
			mcp.WithString("kind", mcp.Required(), mcp.Description("Workload kind: Deployment, DaemonSet, or StatefulSet.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Workload name.")),
		),
		describeSourceHandler(client),
	)

	s.AddTool(
		mcp.NewTool("list_destination_types",
			readAnno,
			mcp.WithDescription("List every destination type Odigos supports (Datadog, Honeycomb, Jaeger, etc.), with their display name, category, and supported signals. Use this to discover what backends can be configured."),
		),
		listDestinationTypesHandler(),
	)

	s.AddTool(
		mcp.NewTool("get_destination_schema",
			readAnno,
			mcp.WithDescription("Get the configuration schema for a specific destination type — the fields the user must supply, which fields are secrets, which signals are supported. Always call this before add_destination to know what fields to collect."),
			mcp.WithString("type", mcp.Required(), mcp.Description("Destination type identifier (e.g. 'datadog', 'honeycomb', 'jaeger'). Use list_destination_types to discover valid values.")),
		),
		getDestinationSchemaHandler(),
	)
}

func listSourcesHandler(client *kube.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns := ""
		if v, err := req.RequireString("namespace"); err == nil {
			ns = v
		}
		list, err := client.OdigosClient.Sources(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list sources: %v", err)), nil
		}
		type row struct {
			Namespace              string `json:"namespace"`
			Name                   string `json:"name"`
			WorkloadKind           string `json:"workloadKind"`
			WorkloadName           string `json:"workloadName"`
			WorkloadNamespace      string `json:"workloadNamespace"`
			DisableInstrumentation bool   `json:"disableInstrumentation"`
		}
		out := make([]row, 0, len(list.Items))
		for _, src := range list.Items {
			out = append(out, row{
				Namespace:              src.Namespace,
				Name:                   src.Name,
				WorkloadKind:           string(src.Spec.Workload.Kind),
				WorkloadName:           src.Spec.Workload.Name,
				WorkloadNamespace:      src.Spec.Workload.Namespace,
				DisableInstrumentation: src.Spec.DisableInstrumentation,
			})
		}
		return jsonResult(out)
	}
}

func listDestinationsHandler(client *kube.Client, odigosNs func(context.Context) (string, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := odigosNs(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("resolve odigos namespace: %v", err)), nil
		}
		list, err := client.OdigosClient.Destinations(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list destinations: %v", err)), nil
		}
		type row struct {
			Name            string   `json:"name"`
			Type            string   `json:"type"`
			DestinationName string   `json:"destinationName"`
			Signals         []string `json:"signals"`
			Disabled        bool     `json:"disabled"`
		}
		out := make([]row, 0, len(list.Items))
		for _, d := range list.Items {
			signals := make([]string, 0, len(d.Spec.Signals))
			for _, sig := range d.Spec.Signals {
				signals = append(signals, string(sig))
			}
			disabled := false
			if d.Spec.Disabled != nil {
				disabled = *d.Spec.Disabled
			}
			out = append(out, row{
				Name:            d.Name,
				Type:            string(d.Spec.Type),
				DestinationName: d.Spec.DestinationName,
				Signals:         signals,
				Disabled:        disabled,
			})
		}
		return jsonResult(out)
	}
}

func describeSourceHandler(client *kube.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		kindStr, err := req.RequireString("kind")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var analyze any
		var dErr error
		switch normalizeKind(kindStr) {
		case k8sconsts.WorkloadKindDeployment:
			analyze, dErr = describe.DescribeDeployment(ctx, client, client.OdigosClient, ns, name)
		case k8sconsts.WorkloadKindDaemonSet:
			analyze, dErr = describe.DescribeDaemonSet(ctx, client, client.OdigosClient, ns, name)
		case k8sconsts.WorkloadKindStatefulSet:
			analyze, dErr = describe.DescribeStatefulSet(ctx, client, client.OdigosClient, ns, name)
		default:
			return mcp.NewToolResultError(fmt.Sprintf("unsupported kind %q (use Deployment, DaemonSet, or StatefulSet)", kindStr)), nil
		}
		if dErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("describe %s/%s in %s: %v", kindStr, name, ns, dErr)), nil
		}
		return jsonResult(analyze)
	}
}

// ----- helpers shared by read+write handlers ------------------------------

func boolPtr(b bool) *bool { return &b }

func jsonResult(v any) (*mcp.CallToolResult, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcp.NewToolResultText(string(buf)), nil
}

func normalizeKind(s string) k8sconsts.WorkloadKind {
	// Accept "deployment", "Deployment", "DEPLOYMENT", etc.
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	switch lower {
	case "deployment":
		return k8sconsts.WorkloadKindDeployment
	case "daemonset":
		return k8sconsts.WorkloadKindDaemonSet
	case "statefulset":
		return k8sconsts.WorkloadKindStatefulSet
	case "namespace":
		return k8sconsts.WorkloadKindNamespace
	case "cronjob":
		return k8sconsts.WorkloadKindCronJob
	}
	return k8sconsts.WorkloadKind(s)
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpServeCmd)
	mcpServeCmd.Flags().BoolVar(&mcpAllowWritesFlag, "allow-writes", false,
		"Register write tools (instrument_source, add_destination, enable_profiling, etc.). Off by default for safety.")
}
