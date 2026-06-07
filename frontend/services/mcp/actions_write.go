package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	actionsv1 "github.com/odigos-io/odigos/api/actions/v1alpha1"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	actions "github.com/odigos-io/odigos/api/odigos/v1alpha1/actions"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/frontend/kube"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
)

// actions_write.go owns create_action + update_action + the sub-config
// patcher both use. Reads (list/get/delete) live in actions.go.
//
// create_action requires action_type and validates the matching sub-config
// fully (e.g. probabilistic_sampler requires sampling_percentage).
// update_action skips the action_type switch and patches whichever fields the
// caller sent via mutateActionSubConfig, leaving other sub-configs untouched.

func createActionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		actionType, err := req.RequireString("action_type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)

		signalsRaw := optStringSlice(req, "signals")
		if len(signalsRaw) == 0 {
			signalsRaw = []string{"TRACES"}
		}
		signals := make([]common.ObservabilitySignal, 0, len(signalsRaw))
		for _, sig := range signalsRaw {
			signals = append(signals, common.ObservabilitySignal(sig))
		}

		spec := odigosv1.ActionSpec{
			ActionName: optString(req, "action_name", ""),
			Signals:    signals,
		}

		switch actionType {
		case "pii_masking":
			cats := optStringSlice(req, "pii_categories")
			if len(cats) == 0 {
				return mcp.NewToolResultError("pii_masking requires non-empty pii_categories (e.g. [\"CREDIT_CARD\"])"), nil
			}
			pcats := make([]actionsv1.PiiCategory, 0, len(cats))
			for _, c := range cats {
				pcats = append(pcats, actionsv1.PiiCategory(c))
			}
			spec.PiiMasking = &actionsv1.PiiMaskingConfig{PiiCategories: pcats}
		case "delete_attribute":
			names := optStringSlice(req, "attribute_names")
			if len(names) == 0 {
				return mcp.NewToolResultError("delete_attribute requires non-empty attribute_names"), nil
			}
			spec.DeleteAttribute = &actionsv1.DeleteAttributeConfig{AttributeNamesToDelete: names}
		case "rename_attribute":
			renames, rerr := requireStringMap(req, "renames")
			if rerr != nil || len(renames) == 0 {
				return mcp.NewToolResultError("rename_attribute requires a non-empty renames object"), nil
			}
			spec.RenameAttribute = &actionsv1.RenameAttributeConfig{Renames: renames}
		case "k8s_attributes":
			spec.K8sAttributes = &actionsv1.K8sAttributesConfig{
				CollectContainerAttributes:  optBool(req, "collect_container_attributes", false),
				CollectReplicaSetAttributes: optBool(req, "collect_replicaset_attributes", false),
				CollectWorkloadUID:          optBool(req, "collect_workload_uid", false),
				CollectClusterUID:           optBool(req, "collect_cluster_uid", false),
			}
		case "add_cluster_info":
			cfg := &actionsv1.AddClusterInfoConfig{OverwriteExistingValues: optBool(req, "overwrite_existing_values", false)}
			if _, derr := decodeArg(req, "cluster_attributes", &cfg.ClusterAttributes); derr != nil {
				return mcp.NewToolResultError(derr.Error()), nil
			}
			spec.AddClusterInfo = cfg
		// NOTE: sampler "actions" (probabilistic/error/latency/…) were removed from
		// the Action API; sampling is now its own Sampling CRD. Use the dedicated
		// sampling tools (create_sampling_group + create_*_rule) instead.
		case "url_templatization":
			cfg := &actions.URLTemplatizationConfig{}
			ok, derr := decodeArg(req, "config", cfg)
			if derr != nil {
				return mcp.NewToolResultError(derr.Error()), nil
			}
			if !ok || len(cfg.Rules) == 0 {
				return mcp.NewToolResultError("url_templatization requires config.templatizationRulesGroups"), nil
			}
			spec.URLTemplatization = cfg
		case "span_renamer":
			cfg := &actions.SpanRenamerConfig{}
			ok, derr := decodeArg(req, "config", cfg)
			if derr != nil {
				return mcp.NewToolResultError(derr.Error()), nil
			}
			if !ok {
				return mcp.NewToolResultError("span_renamer requires a config object"), nil
			}
			spec.SpanRenamer = cfg
		case "extract_attribute":
			cfg := &actions.ExtractAttributeConfig{}
			ok, derr := decodeArg(req, "config", cfg)
			if derr != nil {
				return mcp.NewToolResultError(derr.Error()), nil
			}
			if !ok || len(cfg.Extractions) == 0 {
				return mcp.NewToolResultError("extract_attribute requires config.extractions"), nil
			}
			spec.ExtractAttribute = cfg
		default:
			return errorWithOptions(fmt.Sprintf("unknown action_type %q", actionType), []string{
				"pii_masking", "delete_attribute", "rename_attribute", "add_cluster_info", "k8s_attributes",
				"url_templatization", "span_renamer", "extract_attribute",
			})
		}

		ns := env.GetCurrentNamespace()
		if dryRun {
			return withGuidance(map[string]any{"action": "create_action", "action_type": actionType, "signals": signalsRaw},
				"Dry-run: action validated.", []NextStep{
					{When: "to actually apply", Tool: "create_action", Args: map[string]any{"action_type": actionType, "dry_run": false}},
				})
		}
		act := &odigosv1.Action{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "odigos-mcp-action-", Namespace: ns},
			Spec:       spec,
		}
		created, cerr := kube.DefaultClient.OdigosClient.Actions(ns).Create(ctx, act, metav1.CreateOptions{})
		if cerr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create action: %v", cerr)), nil
		}
		return withGuidance(map[string]any{"action": "created", "action_name": created.Name},
			"Created. The controller will compile it into a Processor for the collectors.", []NextStep{
				{When: "to confirm it appeared in the pipeline", Tool: "list_actions"},
			})
	}
}

func updateActionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("action_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)
		ns := env.GetCurrentNamespace()
		act, gerr := kube.DefaultClient.OdigosClient.Actions(ns).Get(ctx, name, metav1.GetOptions{})
		if gerr != nil {
			if apierrors.IsNotFound(gerr) {
				return mcp.NewToolResultError(fmt.Sprintf("action %q not found", name)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get action: %v", gerr)), nil
		}

		changed := map[string]any{}
		if v := optString(req, "new_action_name", ""); v != "" {
			act.Spec.ActionName = v
			changed["actionName"] = v
		}
		if v := optString(req, "notes", ""); v != "" {
			act.Spec.Notes = v
			changed["notes"] = v
		}
		if dp := optBoolPtr(req, "disabled"); dp != nil {
			act.Spec.Disabled = *dp
			changed["disabled"] = *dp
		}
		if sigs := optStringSlice(req, "signals"); len(sigs) > 0 {
			act.Spec.Signals = act.Spec.Signals[:0]
			for _, s := range sigs {
				act.Spec.Signals = append(act.Spec.Signals, common.ObservabilitySignal(s))
			}
			changed["signals"] = sigs
		}
		if err := mutateActionSubConfig(req, &act.Spec, changed); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(changed) == 0 {
			return withGuidance(map[string]any{"action": "noop"}, "Nothing to change; supply at least one field.", nil)
		}
		if dryRun {
			return withGuidance(map[string]any{"action": "update_action", "action_name": name, "would_change": changed}, "Dry-run.", []NextStep{
				{When: "to actually apply", Tool: "update_action", Args: map[string]any{"action_name": name, "dry_run": false}},
			})
		}
		updated, uerr := kube.DefaultClient.OdigosClient.Actions(ns).Update(ctx, act, metav1.UpdateOptions{})
		if uerr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("update action: %v", uerr)), nil
		}
		return withGuidance(map[string]any{"action": "updated", "action_name": updated.Name},
			"Action updated; controller will recompile to Processor.", nil)
	}
}

// mutateActionSubConfig applies whichever action sub-config fields the caller
// populated. It only TOUCHES the sub-configs the caller actually sent — other
// kinds are left as-is, so update_action can patch one piece without
// disturbing the rest.
func mutateActionSubConfig(req mcp.CallToolRequest, spec *odigosv1.ActionSpec, changed map[string]any) error {
	if cats := optStringSlice(req, "pii_categories"); len(cats) > 0 {
		pcats := make([]actionsv1.PiiCategory, 0, len(cats))
		for _, c := range cats {
			pcats = append(pcats, actionsv1.PiiCategory(c))
		}
		spec.PiiMasking = &actionsv1.PiiMaskingConfig{PiiCategories: pcats}
		changed["piiMasking"] = cats
	}
	if names := optStringSlice(req, "attribute_names"); len(names) > 0 {
		spec.DeleteAttribute = &actionsv1.DeleteAttributeConfig{AttributeNamesToDelete: names}
		changed["deleteAttribute"] = names
	}
	if rmap, rerr := requireStringMap(req, "renames"); rerr == nil && len(rmap) > 0 {
		spec.RenameAttribute = &actionsv1.RenameAttributeConfig{Renames: rmap}
		changed["renameAttribute"] = rmap
	}
	// Sampler patches removed: sampling is now the dedicated Sampling CRD
	// (use the sampling tools), not an Action field.
	// Typed 'config' object is reused across the advanced action kinds. We try
	// each in turn and accept the first that parses with non-empty content.
	cfgURL := &actions.URLTemplatizationConfig{}
	if ok, _ := decodeArg(req, "config", cfgURL); ok && len(cfgURL.Rules) > 0 {
		spec.URLTemplatization = cfgURL
		changed["urlTemplatization"] = "updated"
	}
	cfgRenamer := &actions.SpanRenamerConfig{}
	if ok, _ := decodeArg(req, "config", cfgRenamer); ok && cfgRenamer.ScopeName != "" {
		spec.SpanRenamer = cfgRenamer
		changed["spanRenamer"] = "updated"
	}
	cfgExtract := &actions.ExtractAttributeConfig{}
	if ok, _ := decodeArg(req, "config", cfgExtract); ok && len(cfgExtract.Extractions) > 0 {
		spec.ExtractAttribute = cfgExtract
		changed["extractAttribute"] = "updated"
	}
	return nil
}
