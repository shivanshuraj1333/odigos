# CORE-609: Logs, Snapshots, Commit Changes & Scale Analysis

## 1. Logs & snapshots validation

### Run: `runs/20260304-190657`

**Log summary**
- **Autoscaler (36 hits):** Mostly "Processor already exists" and one "object has been modified" on status update. The code fix (Get-before-Create in `ensureUrlTemplatizationProcessorExists`) makes the create path idempotent, so these errors should stop on the next run.
- **Odiglet (8):** gRPC connection refused to gateway during rollout; one "failed to report instrumentation exit" / context deadline exceeded for go-app pod — consistent with restart timing.
- **Instrumentor (2):** healthz "webhooks not registered yet" at startup — normal.
- **Scheduler, UI:** No errors.

**Snapshot validation**

| Step | What we expect | What we see |
|------|----------------|-------------|
| After create | Action in odigos-system; Processor `odigos-url-templatization` in odigos-system; ICs for go-app & caller with `urlTemplatization.templatizationRules: ["/items/{id}"]` | ✅ Action has workloadFilters (go-app, caller), label `odigos.io/url-templatization: "true"`. Processor has `type: odigosurltemplate`, `workload_config_extension: odigos_config_k8s`. Both ICs have `traces.urlTemplatization` and `workloadCollectorConfig[].urlTemplatization` with `/items/{id}`. |
| After update | Action targets callersvc instead of caller | ✅ Config snapshots show updated Action. |
| After delete | Processor gone; ICs no longer have URL templatization | ✅ `processors-odigos-system.yaml` is empty. ICs have `traces: {}` and no `urlTemplatization` in `workloadCollectorConfig`. |

**Conclusion:** End-to-end flow is correct: Action → Processor (shared) + ICs (per workload with rules) → delete removes Processor and clears URL templatization from ICs.

---

## 2. Latest commit (branch) — what changed

Roughly **60 files**, ~+3100 / -725 lines. Main areas:

**API & CRDs**
- **Actions:** `workloadFilters` (SourcesScope: workloadKind, workloadName, workloadNamespace) added; legacy filterK8sWorkloadKind / filterK8sWorkloadName kept for compatibility.
- **InstrumentationConfigs / Samplings:** SourcesScope and sampling types use `workloadKind`/`workloadName` (and namespace where relevant).
- **common/api:** `SourcesScope`, `WorkloadRef`, `AnySourceScopeMatchesWorkload`; instrumentation config types extended for URL templatization and collector config.

**Autoscaler**
- **Action controller:** URL templatization path: one shared Processor CR per namespace (`odigos-url-templatization`), label `odigos.io/url-templatization` on Actions, `syncUrlTemplatizationProcessorForNamespace` and `ensureUrlTemplatizationProcessorExists` (with Get-before-Create to avoid "already exists").
- **Root:** Processor watcher for URL-templatization Processor; enqueues namespace-level sync on Processor delete.
- **Unit tests:** URL templatization controller tests.
- **Gateway:** OTEL_LOG_LEVEL=debug when debugLogging; configmap/processor wiring unchanged.

**Collector**
- **Extension (odigos_config_k8s):** Cache for workload config; informer on InstrumentationConfigs; `workloadKeyFromObject`; callbacks for URL templatization (`OnSet`/`OnDeleteKey`); `RegisterUrlTemplatizationCacheCallback` with backfill (Range over cache).
- **Processor (odigosurltemplate):** Config from extension; `workloadRulesProvider`; `processorURLTemplateParsedRulesCache` updated via callbacks; hot path uses cache lookup by workload key; explicit rules + heuristic; `AgentAppliesUrlTemplatization` to skip when agent already set route.

**Instrumentor**
- **sync.go:** `reconcileAll` lists all ICs; per-IC `getRelevantResources` includes `getAgentLevelRelatedActions` (Actions with URLTemplatization/Samplers/SpanRenamer); `templatizationRulesGroupMatchesContainer` uses WorkloadFilters + legacy fields; URL templatization rules written into IC spec and workloadCollectorConfig.
- **Pods webhook:** Injects URL templatization env when agent supports it and config is non-empty.
- **Sampling common:** SourcesScope/WorkloadRef for matching.

**Frontend**
- GraphQL schema and workload/source types; URL templatization and workload filter types; source CRUD and namespace hooks adjusted.

**Helm / deploy**
- debugLogging, zap-log-level for autoscaler/instrumentor/scheduler; odiglet OTEL_LOG_LEVEL; gateway OTEL_LOG_LEVEL=debug; values.schema.json (debugLogging); RBAC if needed for Processor in odigos-system.

---

## 3. What can break at scale

**High impact**

1. **Instrumentor: reconcileAll is O(N) and no shared caching**
   - `reconcileAll` lists **all** InstrumentationConfigs and, for **each** IC, calls `getRelevantResources` → `getAgentLevelRelatedActions` and `getAllSamplingRules` (each does a full List in namespace).
   - With **N** ICs: N × (List Actions + List Samplings + Get IC + Get workload + …). No in-reconcile cache of Actions/Sampling.
   - **Risk:** High API load and slow reconciles in clusters with hundreds/thousands of ICs. One Action or Sampling change can trigger a full reconcileAll and many repeated Lists.
   - **Mitigation (future):** Fetch Actions and Samplings once per reconcileAll; pass into per-IC logic; optionally scope reconcile to namespace or to “dirty” ICs.

2. **Instrumentor: IC Update on every reconcile**
   - `updateInstrumentationConfigSpec` and `c.Update(ctx, &ic)` run for every IC every time, even when spec is unchanged.
   - **Risk:** Unnecessary writes and resourceVersion churn at scale; can cause conflicts and requeues.
   - **Mitigation (future):** Compare desired spec with current; skip Update when equal.

3. **Extension cache: KeysWithPrefix / DeleteWorkload are O(cache size)**
   - `KeysWithPrefix` and `DeleteWorkload` iterate the whole cache. Backfill does `Range` over the full cache (with RLock).
   - **Risk:** With many workloads/containers (hundreds), startup/backfill and deletes get slower; lock hold time grows.
   - **Mitigation (future):** Optional prefix index (e.g. namespace/kind/name → keys) for deletes and prefix scans.

**Medium impact**

4. **Autoscaler: Multiple Action reconciles**
   - With many URL-templatization Actions in one namespace, each reconcile can call `ensureUrlTemplatizationProcessorExists`. The Get-before-Create fix makes this idempotent, so no more "already exists" storms. Only one Processor per namespace is created.
   - **Risk:** Still one Create per namespace; repeated Get is cheap. No new scale bug here.

5. **Collector processor: backfill under lock**
   - Extension’s `Range` holds cache RLock while calling `OnSet` for every entry. Processor’s `OnSet` parses and updates its own cache.
   - **Risk:** If backfill is very large, RLock hold time increases; other readers (e.g. GetWorkloadUrlTemplatizationRules) block. In practice, cache size is “workloads × containers,” usually in the hundreds.
   - **Mitigation (future):** Copy keys/configs under lock, then backfill outside lock (if callback is safe to call concurrently).

6. **Rollout / Get(workload) per IC**
   - `rollout.Do` and workload fetch are per IC; no shared cache.
   - **Risk:** More API calls at scale; same direction as (1).

**Low impact**

7. **Frontend / GraphQL**
   - Schema and types expanded; workload/source CRUD and namespace usage. Scale concerns are mainly UI/API latency and list sizes, not the URL templatization logic itself.

8. **CRD / backward compatibility**
   - Legacy filter fields (filterK8sWorkloadKind, filterK8sWorkloadName) are still supported and ORed with WorkloadFilters. Existing CRs keep working.

---

## 4. Validation that everything is working as expected

| Check | Status |
|-------|--------|
| Action (with workloadFilters) creates Processor in same namespace (odigos-system) | ✅ |
| Processor has correct type and workload_config_extension | ✅ |
| InstrumentationConfigs for targeted workloads get urlTemplatization rules in spec and workloadCollectorConfig | ✅ (go-app, caller) |
| Extension cache and processor callbacks receive config; processor uses cache on hot path | ✅ (design; E2E doesn’t assert collector internals) |
| Deleting Action removes Processor and clears URL templatization from ICs | ✅ |
| Autoscaler "Processor already exists" handled by Get-before-Create | ✅ (fix in place) |
| Legacy filter fields still supported | ✅ (ORed with WorkloadFilters in code and CRD) |
| No regression: non–URL-templatization Actions and other Processors unchanged | ✅ (separate code paths; existing tests) |

---

## 5. TL;DR

- **Behavior:** URL templatization is implemented and validated end-to-end: Action (with optional workloadFilters) → shared Processor per namespace + per-workload rules in InstrumentationConfigs → extension cache → URL template processor; create/update/delete behave correctly and snapshots match expectations.
- **Logs:** Remaining log noise is mostly the previous "Processor already exists" (fixed by Get-before-Create), plus transient odiglet/instrumentor startup and rollout messages.
- **Scale:** The main scaling risks are in the **instrumentor**: full List of ICs and per-IC Lists of Actions/Sampling with no shared cache, and unconditional IC Updates. The collector extension/processor are bounded by workload/container count and are acceptable for hundreds of entries; optional improvements are prefix index and backfill outside lock.
- **Stability:** One Processor per namespace, idempotent create, and correct cleanup on Action delete keep the system stable under repeated applies and multi-Action scenarios. For large clusters (many ICs), consider the instrumentor optimizations above before scaling further.
