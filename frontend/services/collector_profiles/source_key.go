package collectorprofiles

import (
	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/frontend/services/common"
	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	k8sNamespaceName   = string(semconv.K8SNamespaceNameKey)
	k8sDeploymentName  = string(semconv.K8SDeploymentNameKey)
	k8sStatefulSetName = string(semconv.K8SStatefulSetNameKey)
	k8sDaemonSetName   = string(semconv.K8SDaemonSetNameKey)
	k8sCronJobName     = string(semconv.K8SCronJobNameKey)
	k8sJobName         = string(semconv.K8SJobNameKey)
	k8sRolloutName     = k8sconsts.K8SArgoRolloutNameAttribute
)

// SourceKeyFromSourceID returns a stable string key for the given SourceID.
// Format: "namespace/kind/name" so it matches keys derived from profile resource attributes.
func SourceKeyFromSourceID(id common.SourceID) string {
	return id.Namespace + "/" + string(id.Kind) + "/" + id.Name
}

// SourceKeyFromResource extracts namespace, kind and name from resource attributes
// (e.g. k8s.namespace.name, k8s.deployment.name) and returns the same key format as SourceKeyFromSourceID.
// Returns ("", false) if required attributes are missing.
func SourceKeyFromResource(attrs pcommon.Map) (string, bool) {
	ns, ok := attrs.Get(k8sNamespaceName)
	if !ok || ns.Str() == "" {
		return "", false
	}
	namespace := ns.Str()

	var kind k8sconsts.WorkloadKind
	var name string
	var found bool

	if name, found = getStr(attrs, k8sDeploymentName); found {
		kind = k8sconsts.WorkloadKindDeployment
	} else if name, found = getStr(attrs, k8sStatefulSetName); found {
		kind = k8sconsts.WorkloadKindStatefulSet
	} else if name, found = getStr(attrs, k8sDaemonSetName); found {
		kind = k8sconsts.WorkloadKindDaemonSet
	} else if name, found = getStr(attrs, k8sCronJobName); found {
		kind = k8sconsts.WorkloadKindCronJob
	} else if name, found = getStr(attrs, k8sJobName); found {
		kind = k8sconsts.WorkloadKindJob
	} else if name, found = getStr(attrs, k8sRolloutName); found {
		kind = k8sconsts.WorkloadKindArgoRollout
	}
	if !found || name == "" {
		return "", false
	}

	return namespace + "/" + string(kind) + "/" + name, true
}

func getStr(attrs pcommon.Map, key string) (string, bool) {
	v, ok := attrs.Get(key)
	if !ok {
		return "", false
	}
	return v.Str(), true
}
