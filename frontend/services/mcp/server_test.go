package mcp

import (
	"sort"
	"testing"
)

// TestCatalog asserts the full tool catalog appears and the read/write split
// is intact. This is the "did the comprehensive coverage actually land" test —
// it also doubles as a regression guard when domains are added.
func TestCatalog(t *testing.T) {
	s := NewServer(Deps{}) // deps may be nil; we're only checking registration
	tools := s.ListTools()

	// Expected names by category. Add to these lists when adding a new tool.
	expectRead := []string{
		// sources & workloads
		"list_sources", "describe_source", "get_runtime_detection",
		"get_pod_details", "list_workloads_in_namespace",
		// destinations
		"list_destination_types", "get_destination_schema", "list_destinations", "test_destination_connection",
		"get_destination", "list_potential_destinations",
		// actions / rules
		"list_actions", "get_action",
		"list_instrumentation_rules", "get_instrumentation_rule",
		// sampling
		"get_sampling", "get_sampling_group",
		// streams / config / collectors / metrics / topology / diagnose / profiling / presets / manifest
		"list_data_streams", "get_effective_config", "get_config_yamls",
		"get_gateway_info", "get_odiglet_info", "list_odiglet_pods",
		"list_gateway_pods", "get_collector_pod",
		"get_overview_metrics", "get_service_map", "get_peer_sources",
		"describe_odigos", "get_instrumentation_health", "get_source_conditions",
		"list_profiling_slots", "get_source_profile", "get_source_hot_functions",
		"list_profiles", "get_k8s_manifest",
	}
	expectWrite := []string{
		// sources
		"instrument_source", "uninstrument_source", "update_source",
		"instrument_namespace", "uninstrument_namespace",
		// destinations
		"create_destination", "delete_destination", "update_destination",
		// actions
		"create_action", "delete_action", "update_action",
		// rules / custom
		"create_instrumentation_rule", "delete_instrumentation_rule", "update_instrumentation_rule", "add_custom_instrumentation",
		// sampling groups + per-rule CRUD
		"create_sampling_group", "delete_sampling_group",
		"create_noisy_operation_rule", "update_noisy_operation_rule", "delete_noisy_operation_rule",
		"create_highly_relevant_operation_rule", "update_highly_relevant_operation_rule", "delete_highly_relevant_operation_rule",
		"create_cost_reduction_rule", "update_cost_reduction_rule", "delete_cost_reduction_rule",
		// data streams
		"update_data_stream", "delete_data_stream",
		// control
		"restart_pod", "restart_workloads", "recover_from_rollback", "pause_odigos", "uninstrument_cluster",
		// profiling writes
		"enable_source_profiling", "disable_source_profiling", "clear_source_profiling_buffer",
		// config writes
		"set_component_log_level", "update_remote_config", "update_local_ui_config", "reset_local_ui_config",
		// diagnose + presets
		"collect_diagnose_bundle", "apply_profile",
	}

	for _, name := range expectRead {
		st, ok := tools[name]
		if !ok {
			t.Errorf("read tool %q not registered", name)
			continue
		}
		if a := st.Tool.Annotations.ReadOnlyHint; a == nil || !*a {
			t.Errorf("tool %q expected ReadOnlyHint=true, got %v", name, a)
		}
		if a := st.Tool.Annotations.DestructiveHint; a != nil && *a {
			t.Errorf("tool %q expected DestructiveHint=false", name)
		}
	}
	for _, name := range expectWrite {
		st, ok := tools[name]
		if !ok {
			t.Errorf("write tool %q not registered", name)
			continue
		}
		if a := st.Tool.Annotations.DestructiveHint; a == nil || !*a {
			t.Errorf("tool %q expected DestructiveHint=true, got %v", name, a)
		}
	}

	// All tools must have a non-empty description (so the agent has guidance).
	var missingDesc []string
	for name, st := range tools {
		if st.Tool.Description == "" {
			missingDesc = append(missingDesc, name)
		}
	}
	sort.Strings(missingDesc)
	if len(missingDesc) > 0 {
		t.Errorf("tools missing description: %v", missingDesc)
	}

	t.Logf("catalog size: %d tools registered", len(tools))
}
