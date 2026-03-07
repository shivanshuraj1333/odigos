package k8sconsts

const (
	// URLTemplatizationProcessorName is the name of the shared Processor CR and OTel pipeline
	// component used for URL templatization. Defined here (not in common) because it is
	// Kubernetes-specific and must not be included in non-k8s agent builds.
	URLTemplatizationProcessorName = "odigos-url-templatization"

	// OdigosConfigK8sExtensionType is the OTel component type identifier for the workload config
	// extension (odigos_config_k8s) that serves per-workload collector configuration at runtime.
	OdigosConfigK8sExtensionType = "odigos_config_k8s"
)
