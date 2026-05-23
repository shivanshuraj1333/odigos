package cmd

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/yaml"

	"github.com/odigos-io/odigos/api/k8sconsts"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/cli/cmd/resources"
	"github.com/odigos-io/odigos/cli/pkg/kube"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/destinations"
	"github.com/odigos-io/odigos/k8sutils/pkg/workload"
)

// registerWriteTools adds the 6 mutating tools. Caller is responsible for
// guarding registration behind the --allow-writes flag.
func registerWriteTools(s *server.MCPServer, client *kube.Client, odigosNs func(context.Context) (string, error)) {
	destrAnno := mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    boolPtr(false),
		DestructiveHint: boolPtr(true),
		IdempotentHint:  boolPtr(false),
		OpenWorldHint:   boolPtr(true),
	})

	dryRunDesc := "Default true. When true, returns a JSON diff of what would change without touching the cluster. Pass false to actually apply."

	s.AddTool(
		mcp.NewTool("instrument_source",
			destrAnno,
			mcp.WithDescription("Instrument a workload by creating an Odigos Source CRD. WARNING: when this workload is first instrumented, its pods are restarted by the Odigos controller to inject the agent."),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("Workload namespace.")),
			mcp.WithString("kind", mcp.Required(), mcp.Description("Workload kind: Deployment, DaemonSet, or StatefulSet.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Workload name.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		instrumentSourceHandler(client),
	)

	s.AddTool(
		mcp.NewTool("uninstrument_source",
			destrAnno,
			mcp.WithDescription("Uninstrument a workload by deleting its Odigos Source CRD. WARNING: pods are restarted to remove the agent and telemetry stops flowing for this workload."),
			mcp.WithString("namespace", mcp.Required(), mcp.Description("Workload namespace.")),
			mcp.WithString("kind", mcp.Required(), mcp.Description("Workload kind.")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Workload name.")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		uninstrumentSourceHandler(client),
	)

	s.AddTool(
		mcp.NewTool("add_destination",
			destrAnno,
			mcp.WithDescription("Add a new telemetry destination (e.g. Datadog, Honeycomb). Call get_destination_schema first to learn which fields are required and which are secrets. Secret fields are stored in a Kubernetes Secret owned by the Destination CRD."),
			mcp.WithString("type", mcp.Required(), mcp.Description("Destination type identifier (e.g. 'datadog'). Discover via list_destination_types.")),
			mcp.WithString("destination_name", mcp.Required(), mcp.Description("Human-readable name for this destination instance.")),
			mcp.WithArray("signals", mcp.Required(),
				mcp.Description("Telemetry signals to export: any subset of [\"TRACES\",\"METRICS\",\"LOGS\"]."),
				mcp.Items(map[string]any{"type": "string", "enum": []string{"TRACES", "METRICS", "LOGS"}}),
			),
			mcp.WithObject("fields", mcp.Required(),
				mcp.Description("Map of field name -> string value, matching the schema from get_destination_schema. Both plain and secret fields go here; the server splits them based on the schema."),
				mcp.AdditionalProperties(map[string]any{"type": "string"}),
			),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		addDestinationHandler(client, odigosNs),
	)

	s.AddTool(
		mcp.NewTool("delete_destination",
			destrAnno,
			mcp.WithDescription("Delete a Destination by name. Telemetry will stop flowing to this backend. The owned Secret is garbage-collected."),
			mcp.WithString("destination_name", mcp.Required(), mcp.Description("Destination resource name (as returned by list_destinations).")),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		deleteDestinationHandler(client, odigosNs),
	)

	s.AddTool(
		mcp.NewTool("enable_profiling",
			destrAnno,
			mcp.WithDescription("Enable cluster-wide continuous profiling by setting Profiling.Enabled=true in the OdigosConfiguration ConfigMap. WARNING: this triggers a reconcile across the Odigos control plane and may cause rollouts of instrumented workloads."),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		toggleProfilingHandler(client, odigosNs, true),
	)

	s.AddTool(
		mcp.NewTool("disable_profiling",
			destrAnno,
			mcp.WithDescription("Disable cluster-wide continuous profiling by setting Profiling.Enabled=false. WARNING: triggers a reconcile across the Odigos control plane."),
			mcp.WithBoolean("dry_run", mcp.Description(dryRunDesc)),
		),
		toggleProfilingHandler(client, odigosNs, false),
	)
}

// ----- destination-type registry tools (read-only, registered with reads) -

func listDestinationTypesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := destinations.Load(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load destinations registry: %v", err)), nil
		}
		type row struct {
			Type        string `json:"type"`
			DisplayName string `json:"displayName"`
			Category    string `json:"category"`
			Traces      bool   `json:"traces"`
			Metrics     bool   `json:"metrics"`
			Logs        bool   `json:"logs"`
			Profiles    bool   `json:"profiles"`
		}
		all := destinations.Get()
		out := make([]row, 0, len(all))
		for _, d := range all {
			out = append(out, row{
				Type:        string(d.Metadata.Type),
				DisplayName: d.Metadata.DisplayName,
				Category:    d.Metadata.Category,
				Traces:      d.Spec.Signals.Traces.Supported,
				Metrics:     d.Spec.Signals.Metrics.Supported,
				Logs:        d.Spec.Signals.Logs.Supported,
				Profiles:    d.Spec.Signals.Profiles.Supported,
			})
		}
		return jsonResult(out)
	}
}

func getDestinationSchemaHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		destType, err := req.RequireString("type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := destinations.Load(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load destinations registry: %v", err)), nil
		}
		dest, ok := destinations.GetDestinationByType(destType)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("destination type %q not found; use list_destination_types", destType)), nil
		}
		type fieldOut struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Required    bool   `json:"required"`
			Secret      bool   `json:"secret"`
		}
		out := struct {
			Type          string     `json:"type"`
			DisplayName   string     `json:"displayName"`
			Fields        []fieldOut `json:"fields"`
			SignalsTraces bool       `json:"supportsTraces"`
			SignalsMetric bool       `json:"supportsMetrics"`
			SignalsLogs   bool       `json:"supportsLogs"`
		}{
			Type:          string(dest.Metadata.Type),
			DisplayName:   dest.Metadata.DisplayName,
			SignalsTraces: dest.Spec.Signals.Traces.Supported,
			SignalsMetric: dest.Spec.Signals.Metrics.Supported,
			SignalsLogs:   dest.Spec.Signals.Logs.Supported,
		}
		for _, f := range dest.Spec.Fields {
			required, _ := f.ComponentProps["required"].(bool)
			out.Fields = append(out.Fields, fieldOut{
				Name:        f.Name,
				DisplayName: f.DisplayName,
				Required:    required,
				Secret:      f.Secret,
			})
		}
		return jsonResult(out)
	}
}

// ----- write tools: sources -----------------------------------------------

func instrumentSourceHandler(client *kube.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, kindStr, name, err := requireWorkloadTriple(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		kind := normalizeKind(kindStr)
		if kind == "" {
			return mcp.NewToolResultError(fmt.Sprintf("invalid kind %q", kindStr)), nil
		}
		dryRun := optBool(req, "dry_run", true)

		// Look up existing Source for this workload.
		selector := labels.SelectorFromSet(labels.Set{
			k8sconsts.WorkloadNameLabel:      name,
			k8sconsts.WorkloadNamespaceLabel: ns,
			k8sconsts.WorkloadKindLabel:      string(kind),
		})
		existing, err := client.OdigosClient.Sources(ns).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list existing sources: %v", err)), nil
		}

		if len(existing.Items) > 0 {
			src := &existing.Items[0]
			if !src.Spec.DisableInstrumentation {
				return jsonResult(map[string]any{
					"action":      "noop",
					"reason":      "workload already instrumented",
					"source_name": src.Name,
					"dry_run":     dryRun,
				})
			}
			// Re-enable a previously-disabled source.
			if dryRun {
				return jsonResult(map[string]any{
					"action":            "patch_source",
					"would_change":      "DisableInstrumentation: true -> false",
					"source_name":       src.Name,
					"namespace":         src.Namespace,
					"will_restart_pods": true,
					"dry_run":           true,
				})
			}
			src.Spec.DisableInstrumentation = false
			updated, uerr := client.OdigosClient.Sources(ns).Update(ctx, src, metav1.UpdateOptions{})
			if uerr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("re-enable source: %v", uerr)), nil
			}
			return jsonResult(map[string]any{
				"action":            "patched",
				"source_name":       updated.Name,
				"namespace":         updated.Namespace,
				"will_restart_pods": true,
			})
		}

		newSrc := &odigosv1.Source{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: workload.CalculateWorkloadRuntimeObjectName(name, kind),
				Namespace:    ns,
			},
			Spec: odigosv1.SourceSpec{
				Workload: k8sconsts.PodWorkload{Kind: kind, Name: name, Namespace: ns},
			},
		}
		if dryRun {
			return jsonResult(map[string]any{
				"action":              "create_source",
				"namespace":           ns,
				"workload_kind":       string(kind),
				"workload_name":       name,
				"will_restart_pods":   true,
				"source_generateName": newSrc.GenerateName,
				"dry_run":             true,
			})
		}
		created, cerr := client.OdigosClient.Sources(ns).Create(ctx, newSrc, metav1.CreateOptions{})
		if cerr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create source: %v", cerr)), nil
		}
		return jsonResult(map[string]any{
			"action":            "created",
			"source_name":       created.Name,
			"namespace":         created.Namespace,
			"will_restart_pods": true,
		})
	}
}

func uninstrumentSourceHandler(client *kube.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, kindStr, name, err := requireWorkloadTriple(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		kind := normalizeKind(kindStr)
		dryRun := optBool(req, "dry_run", true)

		selector := labels.SelectorFromSet(labels.Set{
			k8sconsts.WorkloadNameLabel:      name,
			k8sconsts.WorkloadNamespaceLabel: ns,
			k8sconsts.WorkloadKindLabel:      string(kind),
		})
		existing, err := client.OdigosClient.Sources(ns).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list sources: %v", err)), nil
		}
		if len(existing.Items) == 0 {
			return jsonResult(map[string]any{
				"action":  "noop",
				"reason":  "no Source CRD found for this workload",
				"dry_run": dryRun,
			})
		}
		src := &existing.Items[0]
		if dryRun {
			return jsonResult(map[string]any{
				"action":            "delete_source",
				"source_name":       src.Name,
				"namespace":         src.Namespace,
				"will_restart_pods": true,
				"dry_run":           true,
			})
		}
		if derr := client.OdigosClient.Sources(ns).Delete(ctx, src.Name, metav1.DeleteOptions{}); derr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete source: %v", derr)), nil
		}
		return jsonResult(map[string]any{
			"action":            "deleted",
			"source_name":       src.Name,
			"namespace":         src.Namespace,
			"will_restart_pods": true,
		})
	}
}

// ----- write tools: destinations -----------------------------------------

func addDestinationHandler(client *kube.Client, odigosNs func(context.Context) (string, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		destType, err := req.RequireString("type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		destName, err := req.RequireString("destination_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		signalsRaw, err := req.RequireStringSlice("signals")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fields, err := requireStringMap(req, "fields")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)

		if err := destinations.Load(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load destinations registry: %v", err)), nil
		}
		schema, ok := destinations.GetDestinationByType(destType)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("unknown destination type %q", destType)), nil
		}

		// Validate fields against schema and split secret/plain.
		dataFields, secretFields, validationErrs := splitAndValidateFields(&schema, fields)
		if len(validationErrs) > 0 {
			return jsonResult(map[string]any{
				"action":            "rejected",
				"validation_errors": validationErrs,
				"dry_run":           dryRun,
			})
		}

		signals := make([]common.ObservabilitySignal, 0, len(signalsRaw))
		for _, s := range signalsRaw {
			signals = append(signals, common.ObservabilitySignal(s))
		}

		ns, err := odigosNs(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("resolve odigos namespace: %v", err)), nil
		}

		if dryRun {
			return jsonResult(map[string]any{
				"action":               "create_destination",
				"odigos_namespace":     ns,
				"destination_type":     destType,
				"destination_name":     destName,
				"signals":              signalsRaw,
				"plain_field_names":    keysOf(dataFields),
				"secret_field_names":   keysOf(secretFields),
				"will_create_secret":   len(secretFields) > 0,
				"telemetry_starts_flowing_to_external_endpoint": true,
				"dry_run": true,
			})
		}

		// Apply: create secret (if any), then Destination, then patch OwnerReference on secret.
		var secretRef *corev1.LocalObjectReference
		if len(secretFields) > 0 {
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "odigos.io.dest." + destType + "-",
					Namespace:    ns,
				},
				StringData: secretFields,
			}
			created, sErr := client.Clientset.CoreV1().Secrets(ns).Create(ctx, sec, metav1.CreateOptions{})
			if sErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("create secret: %v", sErr)), nil
			}
			secretRef = &corev1.LocalObjectReference{Name: created.Name}
		}

		dest := &odigosv1.Destination{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "odigos-destination-" + destType + "-",
				Namespace:    ns,
			},
			Spec: odigosv1.DestinationSpec{
				Type:            common.DestinationType(destType),
				DestinationName: destName,
				Data:            dataFields,
				SecretRef:       secretRef,
				Signals:         signals,
			},
		}
		createdDest, dErr := client.OdigosClient.Destinations(ns).Create(ctx, dest, metav1.CreateOptions{})
		if dErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create destination: %v", dErr)), nil
		}
		return jsonResult(map[string]any{
			"action":           "created",
			"destination_name": createdDest.Name,
			"namespace":        createdDest.Namespace,
			"secret_created":   secretRef != nil,
		})
	}
}

func deleteDestinationHandler(client *kube.Client, odigosNs func(context.Context) (string, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("destination_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dryRun := optBool(req, "dry_run", true)

		ns, err := odigosNs(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("resolve odigos namespace: %v", err)), nil
		}
		dest, gerr := client.OdigosClient.Destinations(ns).Get(ctx, name, metav1.GetOptions{})
		if gerr != nil {
			if apierrors.IsNotFound(gerr) {
				return jsonResult(map[string]any{"action": "noop", "reason": "not found", "dry_run": dryRun})
			}
			return mcp.NewToolResultError(fmt.Sprintf("get destination: %v", gerr)), nil
		}
		if dryRun {
			return jsonResult(map[string]any{
				"action":           "delete_destination",
				"destination_name": dest.Name,
				"destination_type": string(dest.Spec.Type),
				"telemetry_stops_flowing_to_external_endpoint": true,
				"dry_run": true,
			})
		}
		if dErr := client.OdigosClient.Destinations(ns).Delete(ctx, name, metav1.DeleteOptions{}); dErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete destination: %v", dErr)), nil
		}
		return jsonResult(map[string]any{"action": "deleted", "destination_name": dest.Name})
	}
}

// ----- write tools: profiling --------------------------------------------

func toggleProfilingHandler(client *kube.Client, odigosNs func(context.Context) (string, error), enable bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dryRun := optBool(req, "dry_run", true)

		ns, err := odigosNs(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("resolve odigos namespace: %v", err)), nil
		}

		cfg, err := resources.GetCurrentConfig(ctx, client, ns)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get odigos configuration: %v", err)), nil
		}

		current := cfg.ProfilingEnabled()
		if current == enable {
			return jsonResult(map[string]any{
				"action":  "noop",
				"reason":  fmt.Sprintf("profiling already %v", current),
				"dry_run": dryRun,
			})
		}

		if dryRun {
			return jsonResult(map[string]any{
				"action":                "patch_configmap",
				"configmap":             consts.OdigosConfigurationName,
				"namespace":             ns,
				"field":                 "profiling.enabled",
				"from":                  current,
				"to":                    enable,
				"triggers_cluster_wide": "Odigos control plane reconciles; instrumented workloads may roll out.",
				"dry_run":               true,
			})
		}

		// Apply: re-fetch the configmap, mutate the embedded YAML, write back.
		cm, gerr := client.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, consts.OdigosConfigurationName, metav1.GetOptions{})
		if gerr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get configmap: %v", gerr)), nil
		}
		if cfg.Profiling == nil {
			cfg.Profiling = &common.ProfilingConfiguration{}
		}
		v := enable
		cfg.Profiling.Enabled = &v
		buf, mErr := yaml.Marshal(cfg)
		if mErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal config: %v", mErr)), nil
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data[consts.OdigosConfigurationFileName] = string(buf)
		if _, uErr := client.Clientset.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{}); uErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("update configmap: %v", uErr)), nil
		}
		return jsonResult(map[string]any{
			"action": "patched",
			"field":  "profiling.enabled",
			"from":   current,
			"to":     enable,
		})
	}
}

// ----- helpers ------------------------------------------------------------

func requireWorkloadTriple(req mcp.CallToolRequest) (ns, kind, name string, err error) {
	if ns, err = req.RequireString("namespace"); err != nil {
		return "", "", "", err
	}
	if kind, err = req.RequireString("kind"); err != nil {
		return "", "", "", err
	}
	if name, err = req.RequireString("name"); err != nil {
		return "", "", "", err
	}
	return ns, kind, name, nil
}

func optBool(req mcp.CallToolRequest, key string, def bool) bool {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return def
	}
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func requireStringMap(req mcp.CallToolRequest, key string) (map[string]string, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing arguments")
	}
	raw, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("missing required parameter %q", key)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parameter %q must be an object of string->string", key)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("field %q must be a string", k)
		}
		out[k] = s
	}
	return out, nil
}

func splitAndValidateFields(schema *destinations.Destination, fields map[string]string) (data, secret map[string]string, errs []string) {
	data = map[string]string{}
	secret = map[string]string{}

	// Build name->field index for the schema.
	byName := make(map[string]destinations.Field, len(schema.Spec.Fields))
	for _, f := range schema.Spec.Fields {
		byName[f.Name] = f
	}

	// Required-field check.
	for _, f := range schema.Spec.Fields {
		required, _ := f.ComponentProps["required"].(bool)
		if !required {
			continue
		}
		v, ok := fields[f.Name]
		if !ok || v == "" {
			errs = append(errs, fmt.Sprintf("required field %q is missing", f.Name))
		}
	}

	// Split provided fields, reject unknowns.
	for name, val := range fields {
		if val == "" {
			continue
		}
		f, ok := byName[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("field %q is not in schema for destination type %s", name, schema.Metadata.Type))
			continue
		}
		if f.Secret {
			secret[name] = val
		} else {
			data[name] = val
		}
	}
	return data, secret, errs
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
