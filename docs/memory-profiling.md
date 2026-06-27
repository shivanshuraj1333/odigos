# Odigos Memory Profiling — Enablement & Operations Runbook

Continuous, out-of-process heap profiling across 8 language families, alongside CPU
profiling, exported as OTLP profiles. This is the customer-facing guide: how to turn
it on, per-language requirements, overhead expectations, observability, and known
limitations.

## Enable

In `values.yaml` (or `--set`):

```yaml
profiling:
  enabled: true
  symbolization:
    native: true          # on-host symbolization of native/Go frames
  memory:
    enabled: true
    sampleSizeBytes: 262144   # 128KiB / 256KiB / 512KiB; default 256KiB
    go: true
    java: true
    native: true          # C/C++/Rust + the interpreted family (Python/Ruby/PHP)
    dotnet: true
    node: true
    inject: false         # ptrace no-restart enablement for Go binaries w/o pprof (opt-in)
    metrics: true         # self-observability metrics on the node collector :8888
```

Then instrument workloads (namespace label `odigos-instrumentation=enabled` or a
`Source`), configure a destination, and enable profiling per source. Disabling
`profiling.memory.metrics` (or `helm uninstall`) removes the metrics endpoint
entirely.

## Per-language: how it works & what it needs

| Language | Mechanism | Restart? | Requirement |
|---|---|---|---|
| **Go** | reads `runtime.mbuckets` out-of-process | none | binaries that import `runtime/pprof`; otherwise enable `inject` (ptrace writes `MemProfileRate`) |
| **.NET** | EventPipe diagnostic socket (`GCAllocationTick`) | none | CoreCLR (libcoreclr.so); profiles the app process, not the SDK driver |
| **Node.js** | V8 Inspector sampling heap profiler (SIGUSR1) | none | a writable path for the inspector socket |
| **Java / JVM** (incl. Kotlin/Scala) | async-profiler alloc + live JFR | none (ptrace attach) | **JDK ≥ 17**; the odiglet stages async-profiler into the pod and provides a writable attach dir (see Limitations: read-only rootfs) |
| **C / C++ / Rust** (native) | `libmemsample` `LD_PRELOAD` sampler, or jemalloc/tcmalloc prof dumps | 1 (preload) | the instrumentor injects the preload; jemalloc needs `--enable-prof` builds |
| **Python** | `libmemsample` + CPython stack walk | 1 (preload) | instrumentor injects `LD_PRELOAD` + `ODIGOS_MEMSAMPLE_*` |
| **Ruby** | `libmemsample` + Ruby stack walk | 1 (preload) | same |
| **PHP** | `libmemsample` + Zend stack walk | 1 (preload) | same **plus `USE_ZEND_ALLOC=0`** (so Zend routes allocations through libc malloc) |

**Enablement modes:** (A) runtime protocol — Go/Java/.NET/Node, no restart; (B)
ptrace-inject — `.so`/mallctl, no restart; (C) instrumentor preload — native/Python/
Ruby/PHP, one restart. None of the native/scripting runtimes are zero-touch out of
the box; they use mode B or C, which the odigos instrumentor performs automatically.

## Signals

Four heap signals per allocation site, symbolized: `alloc_space`, `alloc_objects`
(cumulative), `inuse_space`, `inuse_objects` (live, where the runtime supports it).
Rendered in the Odigos UI and/or Pyroscope under `memory:<signal>:bytes:memory:bytes`.

## Overhead expectations

Measured on a mixed 8-language workload (the node collector runs CPU + memory
profilers + the pipeline in one process):

- **CPU:** ~1–2% of one core for the whole node collector.
- **RSS:** ~300–350 MB for the whole node collector.
- **In-target:** mode-C languages add the LD_PRELOAD sampler cost inside the app
  (sampling, default 256KiB interval — sub-percent at that rate).

Bound the agent with resource limits; keep `metrics.perService` **off** at fleet
scale (per-`service.name` cardinality).

## Observability (prove it yourself)

With `profiling.memory.metrics: true`, the node collector exposes Prometheus on
`:8888`. Key series (all labeled by `language` where applicable):

- `profiler_cpu_samples_total{cpu_mode}` — CPU samples (rate ⇒ samples/s)
- `profiler_mem_samples_total{language,signal}` — memory samples/s by language
- `profiler_mem_alloc_bytes_sampled_bytes_total{language}` — allocation visibility
- `profiler_mem_inuse_bytes{language}` — live heap
- `profiler_mem_records_total{language}`, `profiler_mem_read_errors_total{language}`, `profiler_mem_readers_active{language}`
- `profiler_process_rss_bytes`, `profiler_process_cpu_millis_milliseconds_total` — overhead
- `profiler_export_bytes_bytes_total{signal}`, `profiler_node_network_receive_bytes_bytes_total`

A ready Grafana dashboard ships at `deploy/grafana/memprof-dashboard.json` and a
Prometheus + Grafana quickstart at `deploy/grafana/monitoring.yaml`.

## Known limitations

- **Elixir/Erlang (BEAM):** unsupported (no reader).
- **Envoy and other hook-stripped C++:** the LD_PRELOAD sampler can't see allocations
  (Envoy strips malloc hooks). Use jemalloc-prof builds where available.
- **Read-only root filesystem (Java):** the JVM attach mechanism needs a writable
  path for its socket; the odiglet provides a writable attach volume. If a pod
  hard-pins read-only `/tmp` with no override, Java attach fails gracefully (logged,
  no crash).
- **Pyroscope direct render:** use the Odigos UI as the supported render surface;
  Pyroscope-direct works but is sensitive to OTLP export timing (tuned by default).
