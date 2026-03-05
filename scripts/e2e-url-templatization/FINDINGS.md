# E2E Test Findings (CORE-609)

## 1. Autoscaler: "Processor already exists" errors

**What happened:** After creating the Action CR, the autoscaler logged many errors:
`Failed to ensure URL templatization processor ... error: processors.odigos.io "odigos-url-templatization" already exists`

**Cause:** The code path for "at least one non-disabled URLTemplatization Action exists" calls `ensureUrlTemplatizationProcessorExists`, which only does `Create()`. When multiple reconcile requests run (e.g. concurrent workers or requeues), the first Create succeeds and the rest get "already exists".

**Suggested fix:** In `autoscaler/controllers/actions/action_controller.go`, either:
- Use `syncUrlTemplatizationProcessorForNamespace` in that branch too (it does Get first and only creates if not found), or
- In `ensureUrlTemplatizationProcessorExists`, do a Get before Create and return nil if the Processor already exists (treat "already exists" as success).

## 2. Autoscaler: "Failed to update action status to success"

**What happened:** One error: `Operation cannot be fulfilled on actions.odigos.io "url-templatization-e2e": the object has been modified; please apply your changes to the latest version and try again`

**Cause:** Status update conflict (another writer updated the Action between read and write). Usually resolved by requeue; can be reduced by using patch with retry or ensuring status updates use the latest resource version.

## 3. Odiglet: Connection refused + instrumentation exit timeout

**What happened:** During rollout, odiglet logged grpc connection refused to the gateway and one error: `failed to report instrumentation exit ... error deleting instrumentation instance for pod go-app-64d697bbc8-4jhkf ... context deadline exceeded`.

**Cause:** Restart/rollout: gateway pods were not ready yet, and one instrumentation teardown hit a timeout. Likely transient; if it persists in steady state, worth investigating.

## 4. Instrumentor / Scheduler / UI

Only normal startup messages (e.g. healthz "webhooks not registered yet"). No errors specific to URL templatization.

## 5. Flow verification

- **Create:** Action created → Processor `odigos-url-templatization` appeared in `odigos-system`; InstrumentationConfigs in `odigos-action-demo` should contain the templatization rule for go-app and caller.
- **Update:** Action updated (caller → callersvc); cluster state and config snapshots captured.
- **Delete:** Action deleted → Processor was removed from `odigos-system` within the wait window.

Review the run’s `config-snapshots/` and `cluster-states/` to confirm InstrumentationConfig contents and timing.

---

## 6. Why most caller spans show as "not templated" in ClickHouse

**Observed:** For service `caller`, client spans to go-app: most have span name `GET` (no template), a minority have `GET /items/{id}` (templated). Same resource attributes (namespace, deployment, container) for both.

**Reasons:**

1. **Config timing** – The Action was updated to target **caller** only recently. Most spans in a 2h window were produced **before** the Action included caller, so they never had rules. Only spans after the update get templated.

2. **Cache-miss negative caching** – In the processor, on **cache miss** we call the extension once and **permanently** cache the result. If the first batch of caller spans hits a gateway pod where the extension **hasn't yet synced** the InstrumentationConfig for caller, we cache "no config" and **skip all later spans** for that workload on that pod until **OnSet** overwrites the cache.

3. **Where to look** – For **client** spans we set **span name** and **`url.template`**, not `http.route`. In ClickHouse/Signoz, check **span name** and **`attributes_string['url.template']`** for caller→go-app client spans, not only `http.route`.

**Quick ClickHouse check (caller, last 2h):**
```sql
SELECT name, resources_string['k8s.deployment.name'], count() AS cnt
FROM signoz_traces.distributed_signoz_index_v3
WHERE serviceName = 'caller' AND httpUrl LIKE '%/items/%' AND timestamp >= now() - INTERVAL 2 HOUR
GROUP BY name, resources_string['k8s.deployment.name'] ORDER BY cnt DESC;
```
If both templated and non-templated have the same k8s attributes, the difference is timing (config/cache) as above.
