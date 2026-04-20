package common

import (
	odigoscommon "github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/config"
)

// MergeProfilingOtlpExporter merges Profiling.Exporter into an OTLP exporter config map.
//
// The returned map is always a shallow copy of base (even when otlp is nil) so callers can
// safely mutate the result without affecting the map they passed in.
//
// We map fields explicitly rather than json.Marshal/Unmarshal: OdigosConfiguration uses camelCase
// JSON tags (e.g. retryOnFailure, initialInterval) while the OpenTelemetry Collector exporter
// block expects snake_case keys (retry_on_failure, initial_interval, etc.).
func MergeProfilingOtlpExporter(base config.GenericMap, otlp *odigoscommon.OtlpExporterConfiguration) config.GenericMap {
	out := cloneGenericMap(base)
	if otlp == nil {
		return out
	}
	if otlp.EnableDataCompression != nil {
		if *otlp.EnableDataCompression {
			out["compression"] = "gzip"
		} else {
			out["compression"] = "none"
		}
	}
	if otlp.Timeout != "" {
		out["timeout"] = otlp.Timeout
	}
	if otlp.RetryOnFailure != nil {
		retry := config.GenericMap{}
		if otlp.RetryOnFailure.Enabled != nil {
			retry["enabled"] = *otlp.RetryOnFailure.Enabled
		} else {
			retry["enabled"] = true
		}
		if otlp.RetryOnFailure.InitialInterval != "" {
			retry["initial_interval"] = otlp.RetryOnFailure.InitialInterval
		}
		if otlp.RetryOnFailure.MaxInterval != "" {
			retry["max_interval"] = otlp.RetryOnFailure.MaxInterval
		}
		if otlp.RetryOnFailure.MaxElapsedTime != "" {
			retry["max_elapsed_time"] = otlp.RetryOnFailure.MaxElapsedTime
		}
		out["retry_on_failure"] = retry
	}
	if otlp.SendingQueue != nil {
		q := config.GenericMap{}
		if otlp.SendingQueue.Enabled != nil {
			q["enabled"] = *otlp.SendingQueue.Enabled
		}
		if otlp.SendingQueue.QueueSize > 0 {
			q["queue_size"] = otlp.SendingQueue.QueueSize
		}
		out["sending_queue"] = q
	}
	return out
}

func cloneGenericMap(m config.GenericMap) config.GenericMap {
	if len(m) == 0 {
		return config.GenericMap{}
	}
	out := make(config.GenericMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// K8sAttributesProfilesProcessorConfig is the k8sattributes processor config for profiles pipelines.
func K8sAttributesProfilesProcessorConfig() config.GenericMap {
	return config.GenericMap{
		"auth_type":   "serviceAccount",
		"passthrough": false,
		"extract": config.GenericMap{
			"metadata": []string{
				"k8s.namespace.name",
				"k8s.pod.name",
				"k8s.pod.uid",
				"k8s.deployment.name",
				"k8s.statefulset.name",
				"k8s.daemonset.name",
				"container.id",
			},
		},
		// Primary association by container.id (CRI/container runtime id on profile resource).
		// k8s.pod.ip is a secondary path for cases where container id is missing or the processor needs IP-based correlation.
		"pod_association": []config.GenericMap{
			{
				"sources": []config.GenericMap{
					{"from": "resource_attribute", "name": "container.id"},
				},
			},
			{
				"sources": []config.GenericMap{
					{"from": "resource_attribute", "name": "k8s.pod.ip"},
				},
			},
		},
	}
}

// ProfilingProfileDropConditions returns filterprocessor profile_conditions for the node
// profiles pipeline. These run after k8s_attributes so enrichment can use container.id or
// k8s.pod.ip pod_association; we then drop profiles that still have no Kubernetes namespace
// (host/system noise) instead of dropping everything missing container.id before enrichment.
func ProfilingProfileDropConditions() []string {
	return []string{
		`resource.attributes["k8s.namespace.name"] == nil`,
	}
}

// ProfilingFilterProcessorConfig is the filter processor block for profiles (contrib filterprocessor).
func ProfilingFilterProcessorConfig() config.GenericMap {
	return config.GenericMap{
		"error_mode":         "ignore",
		"profile_conditions": ProfilingProfileDropConditions(),
	}
}

// ProfilingServiceNameTransformProcessorConfig sets OpenTelemetry service.name on profile resources
// from Kubernetes workload attributes when service.name is absent. Runs after k8s_attributes so
// k8s.deployment.name (and friends) are populated; uses the contrib transform processor (profile_statements).
func ProfilingServiceNameTransformProcessorConfig() config.GenericMap {
	return config.GenericMap{
		"error_mode": "ignore",
		"profile_statements": []any{
			config.GenericMap{
				"context": "resource",
				"statements": []any{
					`set(attributes["service.name"], attributes["k8s.deployment.name"]) where not(IsString(attributes["service.name"])) and IsString(attributes["k8s.deployment.name"])`,
					`set(attributes["service.name"], attributes["k8s.statefulset.name"]) where not(IsString(attributes["service.name"])) and IsString(attributes["k8s.statefulset.name"])`,
					`set(attributes["service.name"], attributes["k8s.daemonset.name"]) where not(IsString(attributes["service.name"])) and IsString(attributes["k8s.daemonset.name"])`,
					`set(attributes["service.name"], attributes["k8s.cronjob.name"]) where not(IsString(attributes["service.name"])) and IsString(attributes["k8s.cronjob.name"])`,
					`set(attributes["service.name"], attributes["k8s.job.name"]) where not(IsString(attributes["service.name"])) and IsString(attributes["k8s.job.name"])`,
					`set(attributes["service.name"], attributes["k8s.argoproj.rollout.name"]) where not(IsString(attributes["service.name"])) and IsString(attributes["k8s.argoproj.rollout.name"])`,
				},
			},
		},
	}
}
