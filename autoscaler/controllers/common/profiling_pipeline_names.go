package common

// OpenTelemetry component instance names for continuous profiling pipelines. Keys must be unique
// within each collector's merged config (node collector and cluster gateway are separate binaries).
const (
	// ProfilingReceiver is the contrib profiling receiver on the node collector.
	ProfilingReceiver = "profiling"

	// Node collector profiles domain — receive on host, forward to cluster gateway.
	ProfilingNodeFilterProcessor        = "filter/profiles-node"
	ProfilingNodeK8sAttributesProcessor = "k8s_attributes/profiles-node"
	// ProfilingNodeServiceNameTransformProcessor sets resource service.name from k8s workload metadata
	// when missing, so Pyroscope and OTLP destinations get a stable service dimension after k8s_attributes.
	ProfilingNodeServiceNameTransformProcessor = "transform/profiles-service-name"
	// Use the otlp exporter type (not otlp_grpc) so configs work with released odigos-collector
	// images (e.g. v1.23.x) that do not register a separate otlp_grpc exporter component ID.
	ProfilingNodeToGatewayExporter = "otlp/profiles-to-gateway"

	// Cluster gateway profiles pipeline — OTLP in from nodes, export to UI (and destination OTLP exporters when configured).
	ProfilingGatewayToUIExporter = "otlp/profiles-to-ui"
)
