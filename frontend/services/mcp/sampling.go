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

// Sampling tools cover collector-side tail-sampling. The Sampling CRD models
// three rule families:
//
//   - noisy operations (drop)         → sampling_noisy.go
//   - highly-relevant ops (always keep) → sampling_highly_relevant.go
//   - cost-reduction rules            → sampling_cost_reduction.go
//
// Each Sampling CR is a named "rule group". This file owns the group lifecycle
// (list / get / create / delete) and the tool registration; per-rule CRUD
// lives in the family-specific files, and the shared decoder + read-modify-
// write helpers live in sampling_matchers.go.
//
// Identification used by all tools:
//
//	sampling_id = Sampling CR resource name
//	rule_name   = the Name field on a rule inside the group
//
// For AGENT-side head sampling, use create_action with
// action_type=probabilistic_sampler, or set head_sampling_fraction on
// create_instrumentation_rule.

func registerSamplingTools(s *server.MCPServer, deps Deps) {
	// Groups
	s.AddTool(
		mcp.NewTool("get_sampling",
			readAnno(),
			mcp.WithDescription("List Sampling CRs (rule groups) with per-group counts of noisy / highly-relevant / cost-reduction rules. Each Sampling CR is one group."),
		),
		listSamplingGroupsHandler(),
	)
	s.AddTool(
		mcp.NewTool("get_sampling_group",
			readAnno(),
			mcp.WithDescription("Return one Sampling group's full spec (every rule of every kind)."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Sampling CR resource name (from get_sampling).")),
		),
		getSamplingGroupHandler(),
	)
	s.AddTool(
		mcp.NewTool("create_sampling_group",
			writeAnno(),
			mcp.WithDescription("Create an empty Sampling group (a named rule container). Add individual rules to it with the create_*_rule tools below."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Human-readable group name.")),
			mcp.WithString("notes", mcp.Description("Optional free-form notes.")),
			mcp.WithBoolean("disabled", mcp.Description("Optional: create disabled (rules in this group will be ignored).")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		createSamplingGroupHandler(),
	)
	s.AddTool(
		mcp.NewTool("delete_sampling_group",
			writeAnno(),
			mcp.WithDescription("Delete a Sampling group and all its rules."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Sampling CR resource name.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		deleteSamplingGroupHandler(),
	)

	// Noisy-operation rules — sampling_noisy.go
	s.AddTool(
		mcp.NewTool("create_noisy_operation_rule",
			writeAnno(),
			mcp.WithDescription("Add a noisy-operation tail-sampling rule (drop traces matching this operation, keeping at most percentageAtMost%). Targets like health checks, metrics scrapes."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name to add the rule to.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Short rule name, e.g. 'health-checks'.")),
			mcp.WithObject("operation", mcp.Required(),
				mcp.Description("HeadSamplingOperationMatcher: typed config matching the operation (e.g. by http target, span name)."),
				mcp.AdditionalProperties(true),
			),
			mcp.WithNumber("percentage_at_most", mcp.Required(), mcp.Description("0-100 — keep at most this fraction of matching traces.")),
			mcp.WithObject("source_scopes", mcp.Description("Optional SourcesScopes to limit which sources this rule applies to."), mcp.AdditionalProperties(true)),
			mcp.WithString("notes", mcp.Description("Optional notes.")),
			mcp.WithBoolean("disabled", mcp.Description("Optional: rule is created disabled.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		createNoisyRuleHandler(),
	)
	s.AddTool(
		mcp.NewTool("update_noisy_operation_rule",
			writeAnno(),
			mcp.WithDescription("Update a noisy-operation rule. Identifies by sampling_id + rule_name."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("rule_name", mcp.Required(), mcp.Description("Rule name to update.")),
			mcp.WithObject("operation", mcp.Description("Replace operation matcher."), mcp.AdditionalProperties(true)),
			mcp.WithNumber("percentage_at_most", mcp.Description("Replace percentageAtMost.")),
			mcp.WithObject("source_scopes", mcp.Description("Replace SourcesScopes."), mcp.AdditionalProperties(true)),
			mcp.WithString("notes", mcp.Description("Replace notes.")),
			mcp.WithBoolean("disabled", mcp.Description("Toggle disabled.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		updateNoisyRuleHandler(),
	)
	s.AddTool(
		mcp.NewTool("delete_noisy_operation_rule",
			writeAnno(),
			mcp.WithDescription("Remove a noisy-operation rule from a Sampling group."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("rule_name", mcp.Required(), mcp.Description("Rule name to delete.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		deleteNoisyRuleHandler(),
	)

	// Highly-relevant operation rules — sampling_highly_relevant.go
	s.AddTool(
		mcp.NewTool("create_highly_relevant_operation_rule",
			writeAnno(),
			mcp.WithDescription("Add a highly-relevant tail-sampling rule (always keep traces matching). Mark errors or slow requests."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Short rule name.")),
			mcp.WithBoolean("error", mcp.Description("Match traces with an error span.")),
			mcp.WithNumber("duration_at_least_ms", mcp.Description("Match traces longer than this duration.")),
			mcp.WithObject("operation", mcp.Description("TailSamplingOperationMatcher — additional matching by name/attributes."), mcp.AdditionalProperties(true)),
			mcp.WithNumber("percentage_at_least", mcp.Description("0-100 — keep at least this fraction.")),
			mcp.WithObject("source_scopes", mcp.Description("Optional SourcesScopes."), mcp.AdditionalProperties(true)),
			mcp.WithString("notes", mcp.Description("Optional.")),
			mcp.WithBoolean("disabled", mcp.Description("Optional.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		createHROHandler(),
	)
	s.AddTool(
		mcp.NewTool("update_highly_relevant_operation_rule",
			writeAnno(),
			mcp.WithDescription("Update a highly-relevant rule."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("rule_name", mcp.Required(), mcp.Description("Rule name.")),
			mcp.WithBoolean("error", mcp.Description("Replace.")),
			mcp.WithNumber("duration_at_least_ms", mcp.Description("Replace.")),
			mcp.WithObject("operation", mcp.Description("Replace."), mcp.AdditionalProperties(true)),
			mcp.WithNumber("percentage_at_least", mcp.Description("Replace.")),
			mcp.WithObject("source_scopes", mcp.Description("Replace."), mcp.AdditionalProperties(true)),
			mcp.WithString("notes", mcp.Description("Replace.")),
			mcp.WithBoolean("disabled", mcp.Description("Toggle.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		updateHROHandler(),
	)
	s.AddTool(
		mcp.NewTool("delete_highly_relevant_operation_rule",
			writeAnno(),
			mcp.WithDescription("Remove a highly-relevant rule from a group."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("rule_name", mcp.Required(), mcp.Description("Rule name.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		deleteHROHandler(),
	)

	// Cost-reduction rules — sampling_cost_reduction.go
	s.AddTool(
		mcp.NewTool("create_cost_reduction_rule",
			writeAnno(),
			mcp.WithDescription("Add a cost-reduction tail-sampling rule — sample matching traces at percentage_at_most%. Like noisy but keyword'd for cost optimization."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Short rule name.")),
			mcp.WithObject("operation", mcp.Required(), mcp.Description("TailSamplingOperationMatcher."), mcp.AdditionalProperties(true)),
			mcp.WithNumber("percentage_at_most", mcp.Required(), mcp.Description("0-100.")),
			mcp.WithObject("source_scopes", mcp.Description("Optional."), mcp.AdditionalProperties(true)),
			mcp.WithString("notes", mcp.Description("Optional.")),
			mcp.WithBoolean("disabled", mcp.Description("Optional.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		createCRRHandler(),
	)
	s.AddTool(
		mcp.NewTool("update_cost_reduction_rule",
			writeAnno(),
			mcp.WithDescription("Update a cost-reduction rule."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("rule_name", mcp.Required(), mcp.Description("Rule name.")),
			mcp.WithObject("operation", mcp.Description("Replace."), mcp.AdditionalProperties(true)),
			mcp.WithNumber("percentage_at_most", mcp.Description("Replace.")),
			mcp.WithObject("source_scopes", mcp.Description("Replace."), mcp.AdditionalProperties(true)),
			mcp.WithString("notes", mcp.Description("Replace.")),
			mcp.WithBoolean("disabled", mcp.Description("Toggle.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		updateCRRHandler(),
	)
	s.AddTool(
		mcp.NewTool("delete_cost_reduction_rule",
			writeAnno(),
			mcp.WithDescription("Remove a cost-reduction rule from a group."),
			mcp.WithString("sampling_id", mcp.Required(), mcp.Description("Group CR name.")),
			mcp.WithString("rule_name", mcp.Required(), mcp.Description("Rule name.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		deleteCRRHandler(),
	)
}

// ---- group handlers --------------------------------------------------------

func listSamplingGroupsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns := env.GetCurrentNamespace()
		list, err := kube.DefaultClient.OdigosClient.Samplings(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list sampling: %v", err)), nil
		}
		type group struct {
			ResourceName, Name              string
			Disabled                        bool
			NoisyCount, HROCount, CRRCount int
		}
		out := make([]group, 0, len(list.Items))
		for _, s := range list.Items {
			out = append(out, group{
				ResourceName: s.Name, Name: s.Spec.Name, Disabled: s.Spec.Disabled,
				NoisyCount: len(s.Spec.NoisyOperations),
				HROCount:   len(s.Spec.HighlyRelevantOperations),
				CRRCount:   len(s.Spec.CostReductionRules),
			})
		}
		return withGuidance(out, fmt.Sprintf("%d sampling groups.", len(out)),
			[]NextStep{
				{When: "to inspect one group's rules", Tool: "get_sampling_group"},
				{When: "to create a new group", Tool: "create_sampling_group"},
			})
	}
}

func getSamplingGroupHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("sampling_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		ns := env.GetCurrentNamespace()
		cr, gerr := kube.DefaultClient.OdigosClient.Samplings(ns).Get(ctx, id, metav1.GetOptions{})
		if gerr != nil {
			if apierrors.IsNotFound(gerr) {
				return mcp.NewToolResultError(fmt.Sprintf("sampling group %q not found", id)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("get sampling: %v", gerr)), nil
		}
		return withGuidance(cr, "All three rule arrays returned. Identify a rule by its Name when calling update/delete.",
			[]NextStep{
				{When: "to add a noisy rule", Tool: "create_noisy_operation_rule", Args: map[string]any{"sampling_id": id}},
				{When: "to add a highly-relevant rule", Tool: "create_highly_relevant_operation_rule", Args: map[string]any{"sampling_id": id}},
				{When: "to add a cost-reduction rule", Tool: "create_cost_reduction_rule", Args: map[string]any{"sampling_id": id}},
			})
	}
}

func createSamplingGroupHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)
		ns := env.GetCurrentNamespace()
		cr := &odigosv1.Sampling{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "odigos-mcp-sampling-", Namespace: ns},
			Spec: odigosv1.SamplingSpec{
				Name: name, Notes: optString(req, "notes", ""), Disabled: optBool(req, "disabled", false),
			},
		}
		if dryRun {
			return withGuidance(map[string]any{"action": "create_sampling_group", "name": name}, "Dry-run.", nil)
		}
		created, cerr := kube.DefaultClient.OdigosClient.Samplings(ns).Create(ctx, cr, metav1.CreateOptions{})
		if cerr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create sampling group: %v", cerr)), nil
		}
		return withGuidance(map[string]any{"action": "created", "sampling_id": created.Name, "name": created.Spec.Name},
			"Group created (empty). Add rules to populate it.", []NextStep{
				{When: "to add a noisy rule", Tool: "create_noisy_operation_rule", Args: map[string]any{"sampling_id": created.Name}},
			})
	}
}

func deleteSamplingGroupHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("sampling_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)
		ns := env.GetCurrentNamespace()
		if _, gerr := kube.DefaultClient.OdigosClient.Samplings(ns).Get(ctx, id, metav1.GetOptions{}); gerr != nil {
			if apierrors.IsNotFound(gerr) {
				return withGuidance(map[string]any{"action": "noop"}, "Group not found.", nil)
			}
			return mcp.NewToolResultError(fmt.Sprintf("get sampling: %v", gerr)), nil
		}
		if dryRun {
			return withGuidance(map[string]any{"action": "delete_sampling_group", "sampling_id": id}, "Dry-run: would delete the group and ALL its rules.", nil)
		}
		if derr := kube.DefaultClient.OdigosClient.Samplings(ns).Delete(ctx, id, metav1.DeleteOptions{}); derr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete sampling: %v", derr)), nil
		}
		return withGuidance(map[string]any{"action": "deleted", "sampling_id": id}, "Group deleted.", nil)
	}
}
