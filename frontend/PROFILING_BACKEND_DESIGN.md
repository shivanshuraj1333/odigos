# Profiling backend: design, steps, and layout

## Design (high level)

- **Single OTLP port (4317):** One gRPC server listens on 4317. It serves both OTLP Metrics (existing) and OTLP Profiles. The gateway sends metrics and profiles to the same UI endpoint.
- **Source-scoped storage:** Profile data is stored per “source” = `namespace/kind/name` (e.g. `otel-demo/Deployment/recommendation`). Only sources that the UI has asked to “view” (PUT enable) get a slot and accept data.
- **Filtering on ingest:** For each incoming OTLP profile batch we derive a source key from resource attributes (e.g. `k8s.namespace.name`, `k8s.deployment.name`). We only write to the store if that key is active (slot exists).
- **Storage shape:** Either (A) keep a bounded list of raw OTLP/JSON chunks per slot (current), or (B) merge incoming profiles into one profile per slot with a size cap and return a single chunk on GET (recommended).
- **HTTP API:** PUT enable creates/refreshes the slot; GET returns the stored data (chunks or one merged chunk). TTL and max slots limit memory.

---

## Refined design: key decisions

### 1) Raw samples vs aggregated values

- **Raw samples:** Store each OTLP batch as a chunk. Simple ingest; more memory; GET sends many chunks; frontend merges every time.
- **Aggregated:** Merge into one profile per slot (e.g. `pprofile.MergeFrom`). One coherent view; smaller store and response; UI gets one blob.

**Choice:** **Aggregated.** One merged profile per slot. GET returns one chunk.

### 2) How much to buffer before sharing in the UI

- Don't show empty: only return data once we have something useful.
- **Choice:** Return whatever we have on GET. UI shows "Loading..." until first non-empty response. Optional later: "only return if sample count >= N".

### 3) Live view with a rolling buffer

- **Rolling buffer:** Fixed window of recent data; new data merged in; old dropped so we don't grow unbounded.
- **Choice:** One merged profile per slot; merge each batch in. When merged size exceeds cap (e.g. 2–5 MB), replace with latest batch (or fresh merge). GET returns current snapshot = live rolling view.

### 4) User leaves → 30s TTL → stop receiving

- No GET (and no PUT enable) for 30s for that source ⇒ treat as "user left".
- **Mechanism:** Cleanup job evicts slots where `now - LastRequestAt > 30s`. Eviction = delete slot. After that, `IsActive(key)` is false, so consumer drops new profiles for that key.
- **Choice:** **TTL = 30 seconds.** Cleanup runs periodically; evicted slots stop accepting data until user opens profiling again (PUT enable).

### 5) Flush cache and restore original state

- **Eviction = flush:** When we evict a slot, we delete it and its buffer/merged profile. No separate flush call.
- **Original state:** No slot ⇒ no data; GET returns empty. Next PUT enable creates a new slot and we start fresh.

**End-to-end:** Frontend opens profiling for a service (same keys we use today) → PUT enable creates slot → we accept profiles on gRPC for that key only, merge into a rolling buffer (aggregated, size cap) → GET returns current snapshot; each GET refreshes 30s TTL → user leaves → 30s with no GET → cleanup evicts slot (flush) → we stop accepting for that key and return to original state until next PUT enable.

---

## Steps we will execute (backend)

| Step | What |
|------|------|
| 1 | **Single port 4317:** Create one gRPC server bound to 4317. Register the OTLP Metrics gRPC service (existing metrics consumer) and the OTLP Profiles gRPC service (existing profiles consumer). Refactor so `collector_metrics.Run` and `collector_profiles.RunWithStore` do **not** start their own listeners; they only provide consumers. Start the shared server from `main` and pass both consumers into it. |
| 2 | **Gateway config:** Point the gateway’s profiles export (or verification) at the UI at `odigos-ui.<namespace>.svc.cluster.local:4317` so profiles use the same port as metrics. |
| 3 | **Debug logging:** Add `profiles:`-prefixed logs: in the profiles consumer (batch received, per-resource key, active/drop), in the store (StartViewing, optional AddProfileData), in the handlers (enable, get), and in the profiles gRPC handler (ExportProfiles received). |
| 4 | **Storage: aggregated + rolling:** One merged `pprofile.Profiles` per slot; consumer calls `MergeAndStore(key, pd)`. Size cap (e.g. 2–5 MB); when over cap, replace with latest batch. GET returns one chunk. |
| 5 | **TTL 30s + flush:** Slot TTL = 30s (no GET/enable for 30s ⇒ evict). Cleanup runs every 15–30s. Eviction = delete slot and buffer (flush); original state until next PUT enable. |
| 6 | **(Optional) Label fallback:** If a debug export shows different resource attributes, add fallback in `SourceKeyFromResource`. |

---

## ASCII art: backend layout and data flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  GATEWAY / NODE COLLECTORS                                                        │
│  (OTLP Metrics + OTLP Profiles)                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
                    │
                    │  gRPC (single port 4317)
                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  FRONTEND BACKEND                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │  SHARED OTLP gRPC SERVER  :4317                                              │ │
│  │  ┌─────────────────────────────┐  ┌─────────────────────────────┐          │ │
│  │  │  OTLP Metrics service       │  │  OTLP Profiles service       │          │ │
│  │  │  (existing)                 │  │  (ExportProfiles)            │          │ │
│  │  └──────────────┬──────────────┘  └──────────────┬──────────────┘          │ │
│  └─────────────────┼─────────────────────────────────┼─────────────────────────┘ │
│                     │                                 │                           │
│                     ▼                                 ▼                           │
│  ┌──────────────────────────────┐    ┌──────────────────────────────────────────┐ │
│  │  OdigosMetricsConsumer       │    │  ProfilesConsumer (collector_profiles)    │ │
│  │  (metrics logic, notifications)│   │  ConsumeProfiles(pd)                     │ │
│  └──────────────────────────────┘    │    for each ResourceProfile:             │ │
│                                       │      key, ok := SourceKeyFromResource()  │ │
│                                       │      if !store.IsActive(key) → drop      │ │
│                                       │      else store.AddProfileData(key, blob)│ │
│                                       │        or MergeAndStore(key, pd)         │ │
│                                       └──────────────────┬───────────────────────┘ │
│                                                          │                         │
│                                                          ▼                         │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │  ProfileStore                                                                │  │
│  │  slots: map[sourceKey]*Slot    sourceKey = "namespace/kind/name"             │  │
│  │  ┌─────────────────────────────────────────────────────────────────────┐   │  │
│  │  │  Slot "otel-demo/Deployment/recommendation"                           │   │  │
│  │  │    LastRequestAt,  Buffer (chunks) or MergedProfile (one pprofile)    │   │  │
│  │  │  Slot "default/Deployment/frontend"                                   │   │  │
│  │  │    ...                                                                │   │  │
│  │  │  (max 10 slots, TTL 30s after last GET, evict → flush, stop receiving) │   │  │
│  │  └─────────────────────────────────────────────────────────────────────┘   │  │
│  │  StartViewing(key) → create/refresh slot                                     │  │
│  │  GetProfileData(key) → snapshot of buffer or merged blob                    │  │
│  │  IsActive(key) → has slot                                                   │  │
│  │  RunCleanup() → evict slots not requested in 30s; eviction = flush           │  │
│  └──────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                    │
│  HTTP API (Gin)  /api/sources/:namespace/:kind/:name/                               │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │  PUT .../profiling/enable   →  SourceKeyFromSourceID(id); StartViewing(key)   │  │
│  │  GET .../profiling          →  StartViewing(key); GetProfileData(key)         │  │
│  │                                return { chunks: [base64 or JSON strings] }    │  │
│  └──────────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
                    ▲
                    │  PUT enable (when user opens Profiling)
                    │  GET profiling (poll for chunks)
                    │
┌─────────────────────────────────────────────────────────────────────────────────┐
│  WEBAPP (browser)                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## ASCII art: source key derivation

```
Incoming OTLP ResourceProfiles
  Resource.Attributes():
    k8s.namespace.name     → namespace
    k8s.deployment.name    → name, kind=Deployment
    (or k8s.statefulset.name, k8s.daemonset.name, k8s.job.name, ...)

        SourceKeyFromResource(attrs)
                    │
                    ▼
        sourceKey = "otel-demo/Deployment/recommendation"

        SourceKeyFromSourceID(namespace, kind, name)  (from API path)
                    │
                    ▼
        same string  →  used as slot key in ProfileStore
```

---

## ASCII art: “only when viewing” flow

```
  User clicks "Profiling" in source drawer
            │
            ▼
  PUT /api/sources/otel-demo/Deployment/recommendation/profiling/enable
            │
            ▼
  StartViewing("otel-demo/Deployment/recommendation")
            │
            ├── creates or refreshes slot
            └── LastRequestAt = now

  Gateway keeps sending OTLP profiles (all sources)
            │
            ▼
  ConsumeProfiles(pd)
    for each resource:
      key = "otel-demo/Deployment/recommendation"  (from attrs)
      IsActive(key)?  ──yes──►  AddProfileData(key, chunk)  or  MergeAndStore(key, pd)
                  └──no───►  drop

  User (or polling) calls GET .../profiling
            │
            ▼
  GetProfileData(key)  →  returns chunks (or one merged chunk)
  Slot’s LastRequestAt refreshed  →  TTL extended (30s). User leaves → 30s no GET → cleanup evicts slot (flush); next PUT enable starts fresh.
```

---

## Pyroscope symbolization (OTLP dictionary)

Pyroscope **does not** do external symbolization for OTLP profiles in the ingest path. It **extracts all symbol data from the OTLP packet**:

- **ingest_handler.go**: Requires `er.Dictionary` and returns error if missing (`"missing profile metadata dictionary"`).
- **convert.go**: `ConvertOtelToGoogle(profile, dictionary)` uses only the dictionary:
  - `dictionary.StringTable` → type/unit, function names, filenames, mapping names, attribute keys/values
  - `dictionary.FunctionTable` → Function (NameStrindex, FilenameStrindex, etc.) resolved via StringTable
  - `dictionary.LocationTable` → Location (MappingIndex, Address, Lines with FunctionIndex)
  - `dictionary.MappingTable` → Mapping (FilenameStrindex, MemoryStart/Limit, FileOffset, build_id attribute)
  - `dictionary.StackTable` → Stack (LocationIndices = call stack)
  - `dictionary.AttributeTable` → sample/resource attributes (e.g. service.name)

So when Pyroscope shows correct flame graphs from the same data, the **dictionary in the OTLP request must be populated** when it reaches them. If our pipeline receives the same gRPC but we see `dictionary: {}` in dumps, either (1) the collector pdata JSON marshaler does not emit the dictionary, or (2) the dictionary is not being copied/preserved (e.g. it lives per-ResourceProfiles in the wire format and we only copy top-level). Options:

1. **Use Pyroscope's conversion**: Depend on `github.com/grafana/pyroscope/pkg/ingester/otlp` (or copy `ConvertOtelToGoogle`); unmarshal incoming request as `ExportProfilesServiceRequest` (same proto as Pyroscope: `go.opentelemetry.io/proto/otlp/.../v1development`), call their converter to get Google pprof, then build our flame graph from the pprof. This requires receiving the same proto (our receiver may use collector pdata which might differ).
2. **Verify dictionary preservation**: Log or dump the raw gRPC request (before pdata) and confirm whether the dictionary is present on the wire. If it is, fix our copy/marshal path so the dictionary is included in stored chunks and in `ParseOTLPChunk`.
3. **Backend symbolization** (if dictionary is truly empty from sender): Add a separate step that resolves stack frames (e.g. file ID + offset from mapping/location) via debuginfod/DWARF; see Pyroscope PR #3799. Only needed if the eBPF profiler never sends a filled dictionary.

## File roles (backend)

| File / area | Role |
|-------------|------|
| `main.go` | Create ProfileStore, RunCleanup, call RunWithStore (or shared server + consumers), pass store to startHTTPServer; register profiling routes. |
| `collector_profiles/receiver.go` | RunWithStore: build profiles consumer, create OTLP profiles receiver (or register on shared server). Currently starts listener on 4318; after refactor only provides consumer. |
| `collector_profiles/consumer.go` | NewProfilesConsumer(store). ConsumeProfiles: derive key from resource attrs, if IsActive(key) then AddProfileData (or MergeAndStore). |
| `collector_profiles/source_key.go` | SourceKeyFromResource(attrs), SourceKeyFromSourceID(id). K8s attribute names. |
| `collector_profiles/store.go` | ProfileStore: slots, StartViewing, AddProfileData, GetProfileData, IsActive, cleanup. Slot = Buffer (chunks) or merged profile. |
| `collector_profiles/buffer.go` | BoundedBuffer (chunks, max bytes). Optional: replaced by merged profile per slot. |
| `collector_profiles/handlers.go` | RegisterProfilingRoutes; PUT enable, GET profiling; sourceIDFromParams, SourceKeyFromSourceID. |
| `collector_metrics/collector_metrics.go` | Run: today starts OTLP metrics receiver on 4317. After refactor: only provide consumer; shared server registers metrics on 4317. |
| New (e.g. `otlpserver/` or in main) | Create one gRPC server on 4317, register Metrics + Profiles services, start server; both consumers provided by existing packages. |
