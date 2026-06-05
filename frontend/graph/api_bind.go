package graph

// api_bind.go re-exports the generated GraphQL types from the `api` contract
// repo's odigos_oss flavor into this package, so the hand-written resolvers and
// the server wiring keep referring to them unqualified (QueryResolver, Config,
// NewExecutableSchema, …) exactly as they did with the in-repo generated.go.
// The schema + generated code now live in github.com/odigos-io/agents-api.

import (
	"github.com/99designs/gqlgen/graphql"
	gen "github.com/odigos-io/agents-api/go/server/graph/odigos_oss/generated"
)

type (
	Config                  = gen.Config
	ResolverRoot            = gen.ResolverRoot
	DirectiveRoot           = gen.DirectiveRoot
	ComplexityRoot          = gen.ComplexityRoot
	QueryResolver           = gen.QueryResolver
	MutationResolver        = gen.MutationResolver
	ComputePlatformResolver = gen.ComputePlatformResolver
	SamplingResolver        = gen.SamplingResolver
	SamplingConfigsResolver = gen.SamplingConfigsResolver
	SamplingRulesResolver   = gen.SamplingRulesResolver

	K8sActualNamespaceResolver          = gen.K8sActualNamespaceResolver
	K8sActualSourceResolver             = gen.K8sActualSourceResolver
	K8sNamespaceResolver                = gen.K8sNamespaceResolver
	K8sWorkloadResolver                 = gen.K8sWorkloadResolver
	K8sWorkloadPodContainerResolver     = gen.K8sWorkloadPodContainerResolver
	K8sWorkloadTelemetryMetricsResolver = gen.K8sWorkloadTelemetryMetricsResolver
)

// NewExecutableSchema builds the executable schema from the api flavor.
func NewExecutableSchema(cfg Config) graphql.ExecutableSchema {
	return gen.NewExecutableSchema(cfg)
}
