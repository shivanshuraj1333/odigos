# PR-B: migrate the frontend GraphQL onto the `api` contract repo

> Stacked on `refactor/extract-frontend-server-pkg` (PR #4933). Pairs with the
> `api` repo PR-A (`feat(api): host the Odigos edge GraphQL as oss/enterprise
> flavors`) and the enterprise PR-C. LOCAL — not pushed.

## What changes (and what does not)

This is a behavior-preserving move. The **wire schema is unchanged** — the 8
`graph/*.graphqls` files were copied byte-for-byte into the `api` repo
(`graphql/odigos/`), proven identical there. The frontend stops *owning* the
schema + generated code and starts *consuming* the `api` repo's `odigos_oss`
flavor instead.

## Dependency wiring (mirrors the enterprise `ui/go.mod` pattern)

```
# frontend/go.mod
require github.com/odigos-io/agents-api/go/server vX.Y.Z      # the api tag
replace github.com/odigos-io/agents-api/go/server => ../../agents-api/go/server  # local dev only
```
At release time the replace is dropped and the version pinned to the api tag
(same post-merge step documented for the enterprise UI in ui/POST_MERGE_TODO.md).

## Cutover steps

1. **Delete** the now-relocated generated + schema in the frontend:
   - `graph/*.graphqls` (moved to api `graphql/odigos/`)
   - `graph/generated.go`, `graph/model/models_gen.go`
   - `gqlgen.yml`
2. **Re-point types.** 58 files import `frontend/graph` / `frontend/graph/model`.
   Two equivalent options:
   - **(a) Alias shim (lowest churn):** keep `graph/model/` as a thin package of
     `type X = apimodel.X` re-exports of the api `odigos_oss/model` types, so the
     58 files compile unchanged. Generate the 252 aliases from the schema.
   - **(b) Import sweep:** rewrite the 58 files' imports to the api model package
     (`gofmt -r` / sed), no shim.
   Recommend (a) first to land the cutover with a minimal diff, then (b) as cleanup.
3. **Resolvers.** The hand-written `graph/*.resolvers.go` stay in the frontend and
   keep delegating to `frontend/services/*` — they just implement the api flavor's
   `ResolverRoot` interface instead of the in-repo one. Signatures match because
   the field-resolver settings were carried into the api flavor's gqlgen config
   verbatim.
4. **Wire the executable schema.** In `server/router.go`, swap
   `graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{…}})`
   for the api flavor's `generated.NewExecutableSchema(...)`, passing the same
   `Resolver` (now implementing the api interface).

## Finding from the wiring probe (important)

Adding the api module and running `go mod tidy` makes the frontend's **existing**
`graph/generated.go` fail to compile:

```
graph/generated.go: *executableSchema does not implement graphql.ExecutableSchema
  (Complexity has (string,string,int,map) — interface wants (context.Context, …))
```

gqlgen is the **same version** in both modules (v0.17.81); the frontend's committed
`generated.go` is simply **stale** relative to it. Consequence: you cannot add the
api dependency *alongside* the old generated code — step 1 (delete old generated
graph) and step 4 (use the api flavor) must land **together**. The api flavor is
freshly generated under v0.17.81, so it has the correct interface.

## Verification bar for this PR

- `go build ./...` and `go vet ./...` in `frontend/` green against the api module.
- `go run . ...` serves the UI + GraphQL; introspection identical to pre-migration
  (CI breaking-change gate: `graphql-inspector diff` old vs new = 0 breaking).
- The MCP enterprise build (PR-C) still compiles on top.

## Status

Dependency-consumability proven (the api `odigos_oss` flavor imports cleanly from
the frontend module once the stale generated graph is removed). The cutover above
(steps 1–4, ~58 files via the alias shim) is the remaining mechanical work tracked
by this PR.
