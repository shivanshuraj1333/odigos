package consts

// OTel component instance names for continuous profiling pipelines. Keys must be unique within
// each collector's merged config (node collector and cluster gateway are separate binaries).
const (
	// ProfilingReceiver is the contrib profiling receiver on the node collector.
	ProfilingReceiver = "profiling"

	// Node collector profiles domain — receive on host, forward to cluster gateway.
	ProfilingNodeFilterProcessor        = "filter/profiles-node"
	ProfilingNodeK8sAttributesProcessor = "k8s_attributes/profiles-node"
	// ProfilingNodeServiceNameTransformProcessor sets resource service.name from K8s workload attrs
	// when absent (Pyroscope otherwise labels series as unknown_service:<process>).
	ProfilingNodeServiceNameTransformProcessor = "transform/profiles-service-name"
	ProfilingNodeToGatewayExporter             = "otlp/profiles-to-gateway"

	// Cluster gateway profiles pipeline — OTLP in from nodes, export to UI (no extra processors).
	ProfilingGatewayToUIExporter = "otlp/profiles-to-ui"
)
