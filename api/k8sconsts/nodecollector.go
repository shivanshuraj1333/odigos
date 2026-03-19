package k8sconsts

const (
	OdigosNodeCollectorDaemonSetName           = "odigos-data-collection"
	OdigosNodeCollectorConfigMapName           = OdigosNodeCollectorDaemonSetName
	OdigosNodeCollectorCollectorGroupName      = OdigosNodeCollectorDaemonSetName
	OdigosNodeCollectorOwnTelemetryPortDefault = int32(55682)

	OdigosNodeCollectorConfigMapConfigDomainsName = "odigos-node-collector-config-domains"
	// OdigosNodeCollectorProfilesConfigMapName is the ConfigMap that can override the profiles pipeline config (e.g. k8sattributes pod_association). Created by Helm from collectorNode.profiles.config. Autoscaler reads it; when key "profiles" is present, that YAML is used as the profiles config domain instead of the built-in default.
	OdigosNodeCollectorProfilesConfigMapName = "odigos-node-collector-profiles-config"

	OdigosNodeCollectorLocalTrafficServiceName = "odigos-data-collection-local-traffic"

	OdigosNodeCollectorConfigMapKey = "conf" // this key is different than the cluster collector value. not sure why

	OdigosNodeCollectorServiceAccountName     = "odigos-data-collection"
	OdigosNodeCollectorRoleName               = "odigos-data-collection"
	OdigosNodeCollectorRoleBindingName        = "odigos-data-collection"
	OdigosNodeCollectorClusterRoleName        = "odigos-data-collection"
	OdigosNodeCollectorClusterRoleBindingName = "odigos-data-collection"

	OdigosNodeCollectorContainerName    = "data-collection"
	OdigosNodeCollectorContainerCommand = "/odigosotelcol"
)
