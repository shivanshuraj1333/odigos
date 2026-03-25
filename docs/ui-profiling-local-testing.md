# UI profiling: local testing (Mac / Linux)

Use this to exercise the **Profiling** page (`/profiling`) against a real cluster: either the Odigos UI **backend** in the cluster (port-forwarded) or a **fully embedded** UI from the running pod image.

## Prerequisites

- `kubectl` configured for the cluster where Odigos runs (e.g. `export KUBECONFIG=...` on EKS).
- Odigos installed in namespace `odigos-system` (adjust commands if yours differs).
- Optional: for a workload that returns profile data, a namespace and source name you know (e.g. a `Deployment` that is already collecting profiling samples).

---

## Option A — Local Next dev + port-forwarded backend (hot reload UI)

Use this when you change React/Next code and want GraphQL and REST (`/api/...`) to hit the **same** Odigos process that talks to the API server (Kubernetes).

1. **Port-forward the UI service** (backend listens on container port 3000 by default):

   ```bash
   kubectl port-forward svc/ui -n odigos-system 3000:3000
   ```

2. **Start the webapp on another port** and point Next at the forwarded backend:

   ```bash
   cd frontend/webapp
   yarn install
   ODIGOS_DEV_BACKEND=http://localhost:3000 yarn dev --turbopack -p 3001
   ```

3. Open **`http://localhost:3001/profiling`**.

`ODIGOS_DEV_BACKEND` enables **dev-only rewrites** in `next.config.ts` so `/graphql`, `/api/*`, `/auth/*`, etc. are proxied to the port-forwarded URL. The browser still talks to `localhost:3001`, which keeps cookies and CSRF consistent with how the embedded UI works.

---

## Option B — Port-forward only (embedded static UI from the pod)

Use this to validate **what is actually deployed** in the cluster (no local `yarn dev`).

1. Port-forward:

   ```bash
   kubectl port-forward svc/ui -n odigos-system 3000:3000
   ```

2. Open **`http://localhost:3000/profiling`**.

The UI is the built assets **inside the running image**; your local `frontend/webapp` changes are **not** visible until you rebuild and redeploy.

---

## What to click on `/profiling`

1. Enter **Namespace**, **Kind** (e.g. `Deployment`), and **Name** (workload name).
2. **Enable & load profile** — enables profiling for that source (if needed) and loads the latest aggregated profile.
3. **Refresh** — fetch again if profiling was already enabled.
4. Confirm a **flame graph** appears when the backend returns profile JSON; otherwise read the error message shown on the page.

Optional deep link:  
`http://localhost:3001/profiling?namespace=YOUR_NS&kind=Deployment&name=YOUR_WORKLOAD`

---

## Rebuilding the embedded UI for Option B

From the repo root (after `yarn build` in `frontend/webapp` so `frontend/webapp/out/` exists):

```bash
cd frontend/webapp && yarn build
cd ../.. && go build -o /tmp/odigos-ui ./frontend
```

Deploy or run the binary per your usual Odigos image / Helm flow. See [profiling-e2e-testing-kind-aws.md](./profiling-e2e-testing-kind-aws.md) for EKS / kind profiling testbed details.
