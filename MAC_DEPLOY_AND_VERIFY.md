# Mac/Cursor: Deploy with custom collector image and verify in cluster

**Use this after the image is built and pushed from the VM.**  
Image: `public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo`

---

## 1. Deploy/upgrade with the collector image

Override the collector image so the **gateway** and **node collector (odiglet data-collection)** use your built image.

**Option A – Helm upgrade (release already installed)**

```bash
cd /path/to/odigos  # or your prof/odigos

helm upgrade <RELEASE_NAME> helm/odigos \
  --namespace <ODIGOS_NAMESPACE> \
  --set images.collector=public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo \
  --set autoscaler.profileVerificationOtlpEndpoint=<YOUR_OTLP_ENDPOINT> \
  # add other -f values.yaml or --set as you normally use
```

**Option B – Fresh install**

```bash
helm install <RELEASE_NAME> helm/odigos \
  --namespace <ODIGOS_NAMESPACE> --create-namespace \
  -f your-values.yaml
```

In `your-values.yaml` (or via `--set`):

```yaml
images:
  collector: public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo

autoscaler:
  profileVerificationOtlpEndpoint: "pyroscope.namespace:4040"  # or your Pyroscope/UI/local OTLP endpoint
```

---

## 2. Verify in the cluster

- **Gateway and odiglet use the image:**  
  Check gateway deployment and odiglet DaemonSet node collector container image:
  ```bash
  kubectl get deployment -n <ODIGOS_NAMESPACE> -l app.kubernetes.io/name=odigos-gateway -o jsonpath='{.items[0].spec.template.spec.containers[0].image}'
  kubectl get daemonset -n <ODIGOS_NAMESPACE> odiglet -o jsonpath='{.spec.template.spec.containers[?(@.name=="data-collection")].image}'
  ```
  Both should show `public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo`.

- **Pods running:**  
  `kubectl get pods -n <ODIGOS_NAMESPACE>` — gateway and odiglet (data-collection) should be Running.

- **Profiles in verification backend:**  
  If `autoscaler.profileVerificationOtlpEndpoint` is set, send some profile data and confirm it appears in Pyroscope (or your OTLP receiver).

---

## 3. Pipeline summary

| Step | Where | Action |
|------|--------|--------|
| 1 | Mac (Cursor) | Make code changes in `odigos` (this repo). Commit, push to `feature/profiles-node-collector-gateway`. |
| 2 | VM (Cursor) | Pull branch. Run build + push using **VM_BUILD_AND_PUSH.md**. |
| 3 | Mac (Cursor) | Deploy/upgrade with `images.collector=public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo`. Verify in cluster using steps above. |
