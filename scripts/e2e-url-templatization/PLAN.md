# E2E Test: URL Templatization (CORE-609)

## Goal
Validate the full flow of URL templatization: Action CR → Processor (shared per namespace) → InstrumentationConfig (per workload) → collector extension/processor. Capture all logs and generated configs to analyze delays and errors.

## Flow Under Test
1. **Autoscaler**: On Action with `urlTemplatization`, creates shared Processor CR `odigos-url-templatization` in the Action's namespace; sets label `odigos.io/url-templatization=true` on Action.
2. **Instrumentor**: Watches Actions with URLTemplatization; reconciles workloads; writes URL templatization rules into InstrumentationConfig per workload (traces.urlTemplatization.templatizationRules).
3. **Collector**: Extension watches InstrumentationConfigs; URL template processor gets config via callbacks (OnSet/OnDeleteKey) keyed by workload/container.

## Test Steps

| Step | Action | What we capture |
|------|--------|-----------------|
| 0 | Restart all Odigos components; start log capture to local files | logs/ (autoscaler, instrumentor, scheduler, gateway, odiglet, ui) |
| 1 | Record initial cluster state | cluster-states/01-initial/ |
| 2 | Create Action CR (namespace=odigos-action-demo, workloadFilters: go-app + caller, template /items/{id}) | config-snapshots/02-after-create-action.yaml |
| 3 | Wait for reconciliation; capture generated configs | Processor in odigos-action-demo; InstrumentationConfigs for go-app, caller; cluster-states/03-after-create/ |
| 4 | Update Action: change target (e.g. workloadName caller → callersvc or add another workload) | config-snapshots/04-after-update-action.yaml; cluster-states/04-after-update/ |
| 5 | Wait; record cluster state | cluster-states/05-after-update-settled/ |
| 6 | Delete Action CR | — |
| 7 | Record cluster state (Processor should be removed; ICs updated) | cluster-states/07-after-delete/ |
| 8 | Stop log capture; analyze logs for errors/warnings | analysis/errors-warnings.txt, summary |

## Targets (from user)
- **Namespace**: odigos-action-demo  
- **Workload 1**: Kind=Deployment, Name=go-app — template: `/items/{id}`  
- **Workload 2**: Kind=Deployment, Name=caller — template: `/items/{id}`  

## Generated Configs to Store
- **Action** (odigos.io/v1alpha1): full spec + status
- **Processor** (odigos.io/v1alpha1): name `odigos-url-templatization` in namespace odigos-action-demo (spec.processorConfig points to workload_config_extension)
- **InstrumentationConfig** (odigos.io/v1alpha1): one per workload (naming convention from instrumentor); check traces.urlTemplatization.templatizationRules = ["/items/{id}"]

## Success Criteria
- No errors in component logs (or document known acceptable ones).
- Processor created after Action create; deleted after Action delete.
- InstrumentationConfigs for go-app and caller contain the templatization rule.
- After update, configs reflect new target; after delete, Processor is gone and ICs no longer have URL templatization for this action.
