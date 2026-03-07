package api

import (
	"github.com/odigos-io/odigos/common"
)

// define conditions to match specific sources (containers) managed by odigos.
// a source container matches, if ALL non empty fields match (AND semantics)
//
// common patterns:
//   - Specific kubernetes workload by name (WorkloadNamespace + WorkloadKind + WorkloadName):
//     all containers (usually there is only one with agent injection)
//   - Specific container in a kubernetes workload (WorkloadNamespace + WorkloadKind + WorkloadName + ContainerName):
//     only this container
//   - All services in a kubernetes namespace (WorkloadNamespace):
//     all containers in all sources in the namespace
//   - All services implemented in a specific programming language (WorkloadLanguage):
//     all container which are running odigos agent for this language
//
// +kubebuilder:object:generate=true
// +kubebuilder:deepcopy-gen=true
type SourcesScope struct {
	Name          string `json:"Name,omitempty"`
	Kind          string `json:"Kind,omitempty"` // e.g. "Deployment"
	Namespace     string `json:"Namespace,omitempty"`
	ContainerName string `json:"ContainerName,omitempty"`

	Language common.ProgrammingLanguage `json:"Language,omitempty"`
}
