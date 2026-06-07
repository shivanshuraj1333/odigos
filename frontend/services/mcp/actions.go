package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/frontend/kube"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
)

// Actions are the telemetry transformation pipeline — samplers, PII masking,
// attribute manipulation, URL templatization, span renaming, attribute
// extraction. All 13 kinds map onto Action.spec sub-config structs.
//
// This file owns the read surface + tool registration. The create/update
// handlers + sub-config patcher live in actions_write.go.

func registerActionTools(s *server.MCPServer, deps Deps) {
	// Reads
	s.AddTool(
		mcp.NewTool("list_actions",
			readAnno(),
			mcp.WithDescription("List configured Actions (samplers + attribute transformers + PII masking + URL templatization + …). Each row shows kind, signals, and disabled state."),
		),
		listActionsHandler(),
	)
	s.AddTool(
		mcp.NewTool("get_action",
			readAnno(),
			mcp.WithDescription("Return one Action with its full spec (whichever sub-config — pii_masking, samplers.probabilisticSampler, url_templatization config, etc — is populated)."),
			mcp.WithString("action_name", mcp.Required(), mcp.Description("Action resource name from list_actions.")),
		),
		getActionHandler(),
	)

	// Writes — see actions_write.go
	s.AddTool(
		mcp.NewTool("create_action",
			writeAnno(),
			mcp.WithDescription("Create an Action. action_type covers all 13 kinds: pii_masking, delete_attribute, rename_attribute, add_cluster_info, k8s_attributes, probabilistic_sampler, error_sampler, latency_sampler, service_name_sampler, span_attribute_sampler, url_templatization, span_renamer, extract_attribute. Provide the parameters relevant to the chosen type; advanced types take a typed config object."),
			mcp.WithString("action_type", mcp.Required(), mcp.Description("One of the 13 action types listed above.")),
			mcp.WithString("action_name", mcp.Description("Optional human-readable name.")),
			mcp.WithArray("signals",
				mcp.Description("Signals to operate on. Defaults to [\"TRACES\"]."),
				mcp.Items(map[string]any{"type": "string", "enum": []string{"TRACES", "METRICS", "LOGS"}}),
			),
			// pii_masking
			mcp.WithArray("pii_categories",
				mcp.Description("pii_masking: e.g. [\"CREDIT_CARD\"]."),
				mcp.Items(map[string]any{"type": "string"}),
			),
			// delete_attribute
			mcp.WithArray("attribute_names",
				mcp.Description("delete_attribute: attribute names to drop."),
				mcp.Items(map[string]any{"type": "string"}),
			),
			// rename_attribute
			mcp.WithObject("renames",
				mcp.Description("rename_attribute: map of oldName -> newName."),
				mcp.AdditionalProperties(map[string]any{"type": "string"}),
			),
			// k8s_attributes
			mcp.WithBoolean("collect_container_attributes", mcp.Description("k8s_attributes: collect container-level attrs.")),
			mcp.WithBoolean("collect_replicaset_attributes", mcp.Description("k8s_attributes: collect replicaset attrs.")),
			mcp.WithBoolean("collect_workload_uid", mcp.Description("k8s_attributes: collect workload UID.")),
			mcp.WithBoolean("collect_cluster_uid", mcp.Description("k8s_attributes: collect cluster UID.")),
			// add_cluster_info
			mcp.WithBoolean("overwrite_existing_values", mcp.Description("add_cluster_info: overwrite existing values.")),
			mcp.WithArray("cluster_attributes",
				mcp.Description("add_cluster_info: array of {attributeName, attributeStringValue}."),
				mcp.Items(map[string]any{"type": "object"}),
			),
			// samplers
			mcp.WithString("sampling_percentage", mcp.Description("probabilistic_sampler: 0-100 as a string, e.g. \"25\".")),
			mcp.WithNumber("fallback_sampling_ratio", mcp.Description("error_sampler: 0-100 of non-error traces to keep.")),
			mcp.WithArray("endpoints_filters",
				mcp.Description("latency_sampler: array of {service_name, http_route, minimum_latency_threshold, fallback_sampling_ratio}."),
				mcp.Items(map[string]any{"type": "object"}),
			),
			mcp.WithArray("services_name_filters",
				mcp.Description("service_name_sampler: array of {service_name, sampling_ratio, fallback_sampling_ratio}."),
				mcp.Items(map[string]any{"type": "object"}),
			),
			mcp.WithObject("config",
				mcp.Description("For advanced types, typed config: span_attribute_sampler={attribute_filters:[…]}; url_templatization={templatizationRulesGroups:[…]}; span_renamer={programmingLanguage,scopeName,regexReplacements:[…]}; extract_attribute={extractions:[{target,source,dataFormat,regex}]}."),
				mcp.AdditionalProperties(true),
			),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		createActionHandler(),
	)
	s.AddTool(
		mcp.NewTool("update_action",
			writeAnno(),
			mcp.WithDescription("Update an existing Action. Pass action_name plus only the fields you want to change: new_action_name (label), notes, disabled (toggle without delete), signals[], or any of the action_type-specific param sets accepted by create_action. The sub-config you provide REPLACES the existing one for that kind."),
			mcp.WithString("action_name", mcp.Required(), mcp.Description("Action resource name to update.")),
			mcp.WithString("new_action_name", mcp.Description("Optional: change the human-readable name.")),
			mcp.WithString("notes", mcp.Description("Optional: free-form notes.")),
			mcp.WithBoolean("disabled", mcp.Description("Optional: temporarily disable without deleting.")),
			mcp.WithArray("signals",
				mcp.Description("Optional: replace the signals list."),
				mcp.Items(map[string]any{"type": "string", "enum": []string{"TRACES", "METRICS", "LOGS"}}),
			),
			// Same parameter shapes as create_action — caller supplies any sub-config they want to replace.
			mcp.WithArray("pii_categories", mcp.Description("Replace PiiMasking categories."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithArray("attribute_names", mcp.Description("Replace DeleteAttribute targets."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithObject("renames", mcp.Description("Replace RenameAttribute map."), mcp.AdditionalProperties(map[string]any{"type": "string"})),
			mcp.WithString("sampling_percentage", mcp.Description("Replace ProbabilisticSampler percentage.")),
			mcp.WithNumber("fallback_sampling_ratio", mcp.Description("Replace ErrorSampler fallback ratio.")),
			mcp.WithArray("endpoints_filters", mcp.Description("Replace LatencySampler filters."), mcp.Items(map[string]any{"type": "object"})),
			mcp.WithArray("services_name_filters", mcp.Description("Replace ServiceNameSampler filters."), mcp.Items(map[string]any{"type": "object"})),
			mcp.WithObject("config", mcp.Description("Typed config for span_attribute_sampler / url_templatization / span_renamer / extract_attribute (same shape as create_action)."), mcp.AdditionalProperties(true)),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		updateActionHandler(),
	)
	s.AddTool(
		mcp.NewTool("delete_action",
			writeAnno(),
			mcp.WithDescription("Delete an Action by resource name."),
			mcp.WithString("action_name", mcp.Required(), mcp.Description("Action resource name from list_actions.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		deleteActionHandler(),
	)
}

// ---- read handlers ---------------------------------------------------------

func listActionsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns := env.GetCurrentNamespace()
		list, err := kube.DefaultClient.OdigosClient.Actions(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list actions: %v", err)), nil
		}
		type row struct {
			Name       string   `json:"name"`
			ActionName string   `json:"actionName"`
			Kinds      []string `json:"actionKinds"`
			Signals    []string `json:"signals"`
			Disabled   bool     `json:"disabled"`
		}
		out := make([]row, 0, len(list.Items))
		for _, a := range list.Items {
			signals := make([]string, 0, len(a.Spec.Signals))
			for _, sig := range a.Spec.Signals {
				signals = append(signals, string(sig))
			}
			out = append(out, row{Name: a.Name, ActionName: a.Spec.ActionName, Kinds: actionKinds(&a.Spec), Signals: signals, Disabled: a.Spec.Disabled})
		}
		return withGuidance(out, fmt.Sprintf("%d actions configured.", len(out)), []NextStep{
			{When: "to add a new sampler/transformer", Tool: "create_action"},
			{When: "to configure tail sampling rule groups instead (richer rules)", Tool: "get_sampling"},
		})
	}
}

func getActionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("action_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		ns := env.GetCurrentNamespace()
		act, gerr := kube.DefaultClient.OdigosClient.Actions(ns).Get(ctx, name, metav1.GetOptions{})
		if gerr != nil {
			if apierrors.IsNotFound(gerr) {
				return mcp.NewToolResultError(fmt.Sprintf("action %q not found", name)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get action: %v", gerr)), nil
		}
		return withGuidance(act, "Full ActionSpec returned. Look at the populated sub-config (one of: piiMasking, samplers, addClusterInfo, deleteAttribute, renameAttribute, k8sAttributes, urlTemplatization, spanRenamer, extractAttribute) to see what this action does.", []NextStep{
			{When: "to change a field", Tool: "update_action", Args: map[string]any{"action_name": name}},
			{When: "to remove this action entirely", Tool: "delete_action", Args: map[string]any{"action_name": name}},
		})
	}
}

func deleteActionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("action_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)
		ns := env.GetCurrentNamespace()
		if _, gerr := kube.DefaultClient.OdigosClient.Actions(ns).Get(ctx, name, metav1.GetOptions{}); gerr != nil {
			if apierrors.IsNotFound(gerr) {
				return withGuidance(map[string]any{"action": "noop"}, "Action not found — nothing to delete.", nil)
			}
			return mcp.NewToolResultError(fmt.Sprintf("get action: %v", gerr)), nil
		}
		if dryRun {
			return withGuidance(map[string]any{"action": "delete_action", "action_name": name}, "Dry-run.", nil)
		}
		if derr := kube.DefaultClient.OdigosClient.Actions(ns).Delete(ctx, name, metav1.DeleteOptions{}); derr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete action: %v", derr)), nil
		}
		return withGuidance(map[string]any{"action": "deleted", "action_name": name}, "Deleted.", nil)
	}
}

// actionKinds returns a short list of populated sub-config kinds for the
// list view. Used by listActionsHandler.
func actionKinds(spec *odigosv1.ActionSpec) []string {
	kinds := []string{}
	if spec.AddClusterInfo != nil {
		kinds = append(kinds, "AddClusterInfo")
	}
	if spec.DeleteAttribute != nil {
		kinds = append(kinds, "DeleteAttribute")
	}
	if spec.RenameAttribute != nil {
		kinds = append(kinds, "RenameAttribute")
	}
	if spec.PiiMasking != nil {
		kinds = append(kinds, "PiiMasking")
	}
	if spec.K8sAttributes != nil {
		kinds = append(kinds, "K8sAttributes")
	}
	if spec.URLTemplatization != nil {
		kinds = append(kinds, "URLTemplatization")
	}
	if spec.SpanRenamer != nil {
		kinds = append(kinds, "SpanRenamer")
	}
	if spec.ExtractAttribute != nil {
		kinds = append(kinds, "ExtractAttribute")
	}
	return kinds
}
