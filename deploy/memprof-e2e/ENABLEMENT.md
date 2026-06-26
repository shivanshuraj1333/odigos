# Memory profiling — enablement & remaining packaging

## How each runtime is enabled

| Mode | Runtimes | What odigos does | Restart |
|---|---|---|---|
| **Zero-touch (runtime protocol)** | Go, Java, .NET, Node | nothing in the workload — the node-collector profiles out-of-process via the runtime's own API | no |
| **Preload (interposer)** | Python, Ruby, PHP, C++, Rust (glibc/musl) | the pods webhook injects `libmemsample` via `LD_PRELOAD` + `ODIGOS_MEMSAMPLE_*` (+ `USE_ZEND_ALLOC=0` for PHP) | yes, once |

Enable with `profiling.enabled: true` + `profiling.memory.enabled: true`. The
preload injection fires automatically for the preload runtimes via
`InjectMemorySampler` in the pods webhook (gated on `MemoryEnabled()` +
`MemorySamplerLanguage`).

## DONE (committed)
- **Profiler readers** (odigos-ebpf-profiler `feat/memory-profiler-cat2`): all 11
  runtimes, dump reaper, `MaxTrackedProcs` bound, Python/Ruby/PHP interposers,
  Ruby ≥3.2 layout.
- **Control plane** (`memprof-main`): `ProfilingMemoryConfiguration` (go/java/
  native/dotnet/node), autoscaler receiver block, helm schema, the vendored
  collector build.
- **Instrumentor injection** (`memprof-main`): `InjectMemorySampler` + webhook
  call-site — unit-tested, compiles.

## REMAINING — agent-bundle packaging (the one cross-repo step)
`InjectMemorySampler` references `/var/odigos/memprof/libmemsample.so` (and
`-musl.so`). Those must be delivered into the odiglet agents bundle:

1. Build `libmemsample.so` + `libmemsample-musl.so` from the profiler repo
   (`native/libmemsample/Makefile` already does this, both libc variants).
2. In `odiglet/Dockerfile`, add to the `agents-builder` stage:
   ```dockerfile
   # memory-profiling allocation interposer (glibc + musl)
   COPY --from=<memprof-build> /libmemsample.so      /instrumentations/memprof/libmemsample.so
   COPY --from=<memprof-build> /libmemsample-musl.so /instrumentations/memprof/libmemsample-musl.so
   ```
   `agents-builder` → `/instrumentations/` is copied to `/var/odigos/` at runtime,
   so they land at `/var/odigos/memprof/…` — matching the injected `LD_PRELOAD`.
3. Decide the source of `<memprof-build>`: a published profiler-release artifact,
   or a build stage that compiles `libmemsample.c` (musl variant needs
   `musl-gcc -DUSE_FP_UNWIND -fno-omit-frame-pointer`).

## REMAINING — in-cluster validation (needs a stable cluster)
- Build + deploy the bounded collector; capture overhead numbers.
- Swap debug→OTLP/Pyroscope; confirm the new runtimes render.
- E2e the webhook injection on a Python/Ruby/PHP source with profiling on.
- Release: images + PRs.
