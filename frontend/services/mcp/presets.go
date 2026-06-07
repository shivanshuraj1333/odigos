package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/frontend/kube"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	"github.com/odigos-io/odigos/profiles"
)

// Presets are curated bundles of OdigosConfiguration tweaks (full-payload-collection,
// db-payload-collection, java-enterprise, ebpf-log-capture, mount-method-*, …).
// They're applied by adding the profile name to OdigosConfiguration.profiles[]
// in the odigos-configuration ConfigMap.
func registerPresetTools(s *server.MCPServer, deps Deps) {
	s.AddTool(
		mcp.NewTool("list_profiles",
			readAnno(),
			mcp.WithDescription("List curated profiles (presets) available for this Odigos tier (community / cloud / onprem). Each profile bundles a set of OdigosConfiguration tweaks (e.g. full-payload-collection, ebpf-log-capture)."),
			mcp.WithString("tier", mcp.Description("Optional: 'community' | 'cloud' | 'onprem'. Defaults to community.")),
		),
		listProfilesHandler(),
	)
	s.AddTool(
		mcp.NewTool("apply_profile",
			writeAnno(),
			mcp.WithDescription("Apply a curated profile by adding it to OdigosConfiguration.profiles[] in the odigos-configuration ConfigMap. WARNING: this triggers a control-plane reconcile and may roll out instrumented workloads."),
			mcp.WithString("profile_name", mcp.Required(), mcp.Description("Profile name from list_profiles.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		applyProfileHandler(),
	)
}

func listProfilesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tier := common.OdigosTier(optString(req, "tier", string(common.CommunityOdigosTier)))
		ps := profiles.GetAvailableProfilesForTier(tier)
		type row struct {
			Name             string   `json:"name"`
			ShortDescription string   `json:"shortDescription"`
			Dependencies     []string `json:"dependencies,omitempty"`
		}
		out := make([]row, 0, len(ps))
		for _, p := range ps {
			deps := make([]string, 0, len(p.Dependencies))
			for _, d := range p.Dependencies {
				deps = append(deps, string(d))
			}
			out = append(out, row{Name: string(p.ProfileName), ShortDescription: p.ShortDescription, Dependencies: deps})
		}
		return withGuidance(out, fmt.Sprintf("%d profiles available for tier %q.", len(out), tier), []NextStep{
			{When: "to apply one", Tool: "apply_profile"},
		})
	}
}

func applyProfileHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("profile_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)
		ns := env.GetCurrentNamespace()

		cm, gerr := kube.DefaultClient.CoreV1().ConfigMaps(ns).Get(ctx, consts.OdigosConfigurationName, metav1.GetOptions{})
		if gerr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get odigos-configuration: %v", gerr)), nil
		}
		raw, ok := cm.Data[consts.OdigosConfigurationFileName]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("key %q missing from ConfigMap", consts.OdigosConfigurationFileName)), nil
		}
		var cfg common.OdigosConfiguration
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("parse config: %v", err)), nil
		}

		profileName := common.ProfileName(name)
		for _, p := range cfg.Profiles {
			if p == profileName {
				return withGuidance(map[string]any{"action": "noop", "profile_name": name}, "Profile already applied.", nil)
			}
		}
		if dryRun {
			return withGuidance(map[string]any{"action": "apply_profile", "profile_name": name, "would_append_to": "profiles[]"},
				"Dry-run: would add this profile to OdigosConfiguration.profiles[] and trigger reconcile.", []NextStep{
					{When: "to actually apply", Tool: "apply_profile", Args: map[string]any{"profile_name": name, "dry_run": false}},
				})
		}

		cfg.Profiles = append(cfg.Profiles, profileName)
		buf, mErr := yaml.Marshal(&cfg)
		if mErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal config: %v", mErr)), nil
		}
		cm.Data[consts.OdigosConfigurationFileName] = string(buf)
		if _, uErr := kube.DefaultClient.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{}); uErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("update configmap: %v", uErr)), nil
		}
		return withGuidance(map[string]any{"action": "applied", "profile_name": name},
			"Profile applied. Control plane will reconcile and may roll out instrumented workloads.", []NextStep{
				{When: "to confirm via the effective config", Tool: "get_effective_config"},
			})
	}
}
