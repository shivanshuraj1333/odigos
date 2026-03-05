# E2E Test: URL Templatization (CORE-609)

Run a full flow test for the URL templatization feature: create Action → capture configs → update Action → delete Action, while capturing all Odigos component logs and cluster state.

## Prerequisites

- `kubectl` configured for your kind cluster (e.g. `kind-local-dev-cluster`).
- Odigos installed in `odigos-system` (default).
- Optional: namespace `odigos-action-demo` with Deployments `go-app` and `caller` (script creates the namespace if missing).

## Run

From repo root or this directory:

```bash
cd /path/to/odigos
chmod +x scripts/e2e-url-templatization/run-e2e.sh
./scripts/e2e-url-templatization/run-e2e.sh
```

To save output under a specific directory (e.g. your home):

```bash
./scripts/e2e-url-templatization/run-e2e.sh ~/odigos-e2e-runs
```

Each run creates a timestamped directory, e.g.:

- `scripts/e2e-url-templatization/runs/20260304-185000/` (default), or  
- `~/odigos-e2e-runs/20260304-185000/`

## Output Layout

```
<run-dir>/
├── logs/                    # Component logs (autoscaler, instrumentor, scheduler, gateway, odiglet, ui)
├── config-snapshots/        # Action, Processor, InstrumentationConfig YAML at each step
├── cluster-states/          # Full cluster state (actions, processors, ICs, pods) per step
│   ├── 01-initial/
│   ├── 03-after-create/
│   ├── 04-after-update/
│   ├── 05-after-update-settled/
│   └── 07-after-delete/
└── analysis/
    ├── errors-warnings.txt  # Grep of error/warn/panic/fail in each log file
    └── summary.txt         # Per-file counts and paths
```

## Environment

- `ODIGOS_NS` – Odigos system namespace (default: `odigos-system`).
- `DEMO_NS` – Namespace for the Action and workloads (default: `odigos-action-demo`).
- `ACTION_NAME` – Name of the Action CR (default: `url-templatization-e2e`).
- `WAIT_SETTLE` – Seconds to wait after apply/delete (default: `25`).
- `WAIT_RECONCILE` – Max seconds to wait for Processor create/delete (default: `45`).

## Spans flow (go-app only → both → cleanup)

To verify templated spans in Jaeger with go-app and caller running:

1. **First action (go-app only):** Apply URL templatization for `go-app` only; generate traffic and check Jaeger (e.g. `http.route` → `/items/{id}`).
2. **Second action (both):** Apply for both `go-app` and `caller`; check spans for both services.
3. **Cleanup:** Delete the Action; Processor is removed.

```bash
chmod +x scripts/e2e-url-templatization/run-spans-flow.sh
./scripts/e2e-url-templatization/run-spans-flow.sh
```

The script is interactive: it prints port-forward and curl commands and pauses so you can check spans. Set `SKIP_PROMPTS=1` to run without prompts.

**View traces:** If Jaeger is in namespace `tracing`, run `kubectl port-forward -n tracing svc/jaeger 16686:16686` and open http://localhost:16686. Select service `go-app` or `caller` and look for spans with `http.route=/items/{id}`.

**Action files used:**
- Step 1: `action-url-templatization-go-app-only.yaml`
- Step 2: `action-url-templatization.yaml` (both workloads)
- Cleanup: Action is deleted.

## Test Steps (see PLAN.md)

1. Restart Odigos components and start log capture.
2. Record initial cluster state.
3. Create Action CR (go-app + caller, template `/items/{id}`).
4. Wait for Processor and capture configs.
5. Update Action (caller → callersvc).
6. Record cluster state after update.
7. Delete Action.
8. Record cluster state after delete; stop logs; analyze errors/warnings.
