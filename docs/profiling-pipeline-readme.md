# Continuous profiling pipeline (eBPF → OTLP Profiles → Odigos UI backend)

This document describes the **internal design** of the profiling feature in this branch, the **components** involved, **environment variables**, and **how to verify** behavior with HTTP (`curl`) and optional debug endpoints. It is written so another agent or engineer can operate or extend the system without prior context.

---

## 1. End-to-end data flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Node: odigos-data-collection (collector pod in odiglet DaemonSet)          │
│                                                                              │
│  Receivers:                                                                  │
│    • "profiling"  → go.opentelemetry.io/ebpf-profiler/collector (CPU samples)│
│    • "otlp/in"    → optional OTLP profiles on 4317/4318 from elsewhere        │
│                                                                              │
│  Pipeline "profiles": receivers → memory_limiter → resource/node-name →      │
│    resourcedetection → k8sattributes/profiles → resource/profiles-service-  │
│    name → exporter otlp/out-cluster-collector-profiles                        │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │ OTLP gRPC (profiles signal) :4317
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Cluster: odigos-gateway (cluster collector Deployment)                       │
│                                                                              │
│  Receiver "otlp" (gRPC :4317, HTTP :4318) with --feature-gates=              │
│    service.profilesSupport                                                   │
│                                                                              │
│  Pipeline "profiles" (when env sets at least one exporter endpoint):       │
│    receivers: [otlp] → exporters: [otlp/profiles-ui, …]                      │
│    (no batch processor; OTLP batch does not support profiles signal)          │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │ OTLP gRPC profiles
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Odigos UI backend (Go, same process as GraphQL + static UI)                  │
│                                                                              │
│  • OTLP **Profiles** gRPC server on **0.0.0.0:4318** (dedicated port)         │
│    (go.opentelemetry.io/collector receiver/otlp xreceiver profiles consumer) │
│  • In-memory **ProfileStore**: up to N “slots” (sources), TTL, bounded buffer│
│  • HTTP **/api/sources/:ns/:kind/:name/profiling** (enable + get flame graph)│
└─────────────────────────────────────────────────────────────────────────────┘
```

**Source identity:** Profile batches are keyed by a string `namespace/WorkloadKind/name` derived from resource attributes (`k8s.namespace.name` + one of `k8s.deployment.name`, `k8s.statefulset.name`, etc., or fallback `service.name` with `Deployment` kind). The UI must use the **same** namespace/kind/name as Odigos sources (PascalCase kind).

**“Only when viewing”:** The OTLP consumer **drops** profile data unless a **slot** exists for that source key. Slots are created/refreshed by `PUT …/profiling/enable` or by `GET …/profiling` (both call `StartViewing`).

---

## 2. Repository layout (what lives where)

| Area | Role |
|------|------|
| `collector/builder-config.yaml` | Adds `odigosebpfreceiver` and `go.opentelemetry.io/ebpf-profiler` profiling receiver to the distribution. |
| `collector/odigosotelcol/components.go` | Registers receiver factories. |
| `collector/odigosotelcol/ebpf_receivers_test.go` | Asserts `odigosebpf` and `profiling` types are present. |
| `autoscaler/.../nodecollector/collectorconfig/profiles.go` | Profiles pipeline + k8sattributes for pod association (container.id, etc.). |
| `autoscaler/.../clustercollector/configmap.go` | Gateway `profiles` pipeline + `PROFILES_OTLP_ENDPOINT_UI` / verification exporters. |
| `autoscaler/.../clustercollector/deployment.go` | `--feature-gates=service.profilesSupport` on gateway. |
| `api/k8sconsts/nodecollector.go` | Name for optional ConfigMap `odigos-node-collector-profiles-config`. |
| `frontend/services/collector_profiles/*` | OTLP ingest, store, consumer, HTTP handlers, Flamebearer aggregation, tests. |
| `helm/odigos/templates/ui/*.yaml` | Expose UI port **4318**, profiling-related env. |
| `helm/odigos/templates/autoscaler/deployment.yaml` | `PROFILES_OTLP_ENDPOINT_UI=dns:///ui.<namespace>:4318` when `ui.profiling.enabled`. |

---

## 3. Environment variables (quick reference)

### Odigos UI pod (Helm sets many of these when `ui.profiling` is enabled)

| Variable | Meaning |
|----------|---------|
| `ENABLE_PROFILES_RECEIVER` | If `false`, no gRPC listener on 4318 (default true when unset). |
| `PROFILES_MAX_SLOTS` | Max concurrent sources with profiling slots (default 10). |
| `PROFILES_SLOT_TTL_SECONDS` | Idle TTL for a slot (default 600). |
| `PROFILES_SLOT_MAX_BYTES` | Max bytes of raw OTLP JSON chunks per slot (default 20 MiB). |
| `PROFILES_CLEANUP_INTERVAL_SECONDS` | Background cleanup tick (default 15). |
| `PROFILING_DEBUG` | Verbose logs when not `0`/`false`. |
| `PROFILE_DEBUG_DUMP_DIR` | If set, write raw chunks to disk under that dir (default dir name `profile-dumps` unless `off`). |

### Autoscaler → gateway ConfigMap generation

| Variable | Meaning |
|----------|---------|
| `PROFILES_OTLP_ENDPOINT_UI` | If set (e.g. `dns:///ui.odigos-system:4318`), gateway gets an `otlp` exporter and a `profiles` pipeline to the UI. |
| `PROFILE_VERIFICATION_OTLP_ENDPOINT` | Optional second sink for debugging. |
| `PROFILE_DEBUG_EXPORT` | If `true`, adds `debug` exporter to gateway profiles pipeline. |
| `PROFILES_EXPORTER_*` | Timeout / retry for profile OTLP exporters. |

---

## 4. HTTP API (same origin as UI)

Base path: **`/api`** (Gin group). **CSRF** middleware skips paths whose URL contains **`/profiling`**, so `curl` can use `PUT` without a CSRF cookie for these routes.

### 4.1 Enable / refresh slot

```http
PUT /api/sources/{namespace}/{kind}/{name}/profiling/enable
```

Example:

```bash
NS=default
KIND=Deployment
NAME=frontend

curl -sS -X PUT "http://127.0.0.1:3000/api/sources/${NS}/${KIND}/${NAME}/profiling/enable"
```

Success: JSON includes `status`, `sourceKey`, `maxSlots`, `activeSlots`.

### 4.2 Get aggregated profile (Flamebearer / Pyroscope-shaped JSON)

```http
GET /api/sources/{namespace}/{kind}/{name}/profiling
GET /api/sources/{namespace}/{kind}/{name}/profiling?debug=1
```

Example:

```bash
curl -sS "http://127.0.0.1:3000/api/sources/${NS}/${KIND}/${NAME}/profiling" | head -c 2000
```

`debug=1` adds parse/build diagnostics in the JSON body.

### 4.3 Debug helpers (optional)

```bash
# Active slots and which have buffered data
curl -sS "http://127.0.0.1:3000/api/debug/profiling-slots"

# First raw OTLP JSON chunk for a source (if any)
curl -sS "http://127.0.0.1:3000/api/debug/sources/${NS}/${KIND}/${NAME}/profiling-chunk"
```

---

## 5. Kubernetes: how to reach the UI HTTP API

The UI listens on **port 3000** by default (`--port`). In-cluster Service name is typically **`ui`** in the Odigos namespace.

```bash
kubectl port-forward -n odigos-system svc/ui 3000:3000
# Then use http://127.0.0.1:3000/... as in section 4.
```

The OTLP **profiles** gRPC port on the UI pod is **4318** (used **gateway → UI**, not for `curl`).

---

## 6. What “correct” looks like

1. **Helm:** `ui` Service exposes port **4318** (name `otlp-profiles`); autoscaler pod has **`PROFILES_OTLP_ENDPOINT_UI`** when `ui.profiling.enabled` is true (default in values).  
2. **Gateway** ConfigMap includes a **`profiles`** pipeline with exporter endpoint pointing at `ui.<ns>:4318`.  
3. **Node collector** merged config includes a **`profiles`** pipeline exporting to **`odigos-gateway.<ns>:4317`**.  
4. **UI logs** show OTLP profiles receiver on **4318** and `[profiling]` lines when data arrives and slots are active.  
5. **Without** `PUT`/`GET` to create a slot, incoming OTLP profiles are **dropped** for that source (by design).

---

## 7. Tests (Go)

From repo `frontend/` module:

```bash
go test ./services/collector_profiles/... -count=1
```

From `autoscaler/` module:

```bash
go test ./controllers/nodecollector/... -count=1
```

Collector distribution:

```bash
cd collector/odigosotelcol && go test -count=1 .
```

---

## 8. Known limitations / follow-ups

- **eBPF profiler** requires a suitable Linux node and container capabilities (see odiglet `data-collection` securityContext).  
- **Symbol resolution:** Prefer full OTLP dictionaries in each chunk (Pyroscope in-band path). Cross-chunk dictionary reuse was intentionally **not** used for name resolution (avoid wrong symbols across batches).  
- **Flame graph in browser:** This README covers the **backend** JSON; a separate UI can render `flamebearer` with a Pyroscope-compatible viewer.

---

## 9. Suggested PR split (this branch)

1. **Collector + autoscaler (+ `api/k8sconsts`):** ebpf-profiler in distribution, node profiles pipeline, gateway profiles pipeline + feature gate, tests/fixtures.  
2. **Frontend (Go):** `collector_profiles` package, `main.go`, CSRF, `go.mod` / `go.sum`.  
3. **Helm:** UI ports/env, autoscaler `PROFILES_OTLP_ENDPOINT_UI`, `values.yaml` / `values.schema.json`, plus this document under `docs/` for operators.
