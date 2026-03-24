# Profiling E2E testing: kind cluster, EC2 checks, and AWS/EKS/ECR

**Cursor agents on the profiling VM:** see repo root **`AGENTS.md`** and **`.cursor/rules/profiling-ec2-eks-dev-vm.mdc`** (enable the rule or open matching files so it applies).

This guide covers:

1. Running **end-to-end profiling validation** on a **local kind** cluster.
2. On an **EC2** (or any build box), verifying **AWS**, **kubectl → EKS**, **Docker**, and **ECR Public pull/push**.
3. Which **scripts and Helm changes** in this repo support those flows (pull the branch on EC2 and run the same steps).

## Prerequisites

- **kind**, **kubectl**, **helm** (v3), **docker** with **buildx**
- **AWS CLI v2** (for EKS/ECR checks and image push)
- **Go**, **Python 3** (for optional `make verify-profiling-ci` / parsers)
- Network access to pull base images (`registry.odigos.io`, `public.ecr.aws`, `gcr.io`, etc.)

## Pull the latest Odigos repo on EC2

```bash
git fetch origin
git checkout <your-branch>   # e.g. feat/eBPF-profiling-support
git pull
```

## Verify AWS, EKS kubectl, Docker, and ECR (EC2 or laptop)

Use the bundled script (read it for details):

```bash
# Optional: kubeconfig for EKS
export KUBECONFIG="$HOME/.kube/eks.config"
export EKS_CONTEXT="arn:aws:eks:REGION:ACCOUNT:cluster/CLUSTER_NAME"

# Default profiler repo prefix (matches Makefile / helm-upgrade-profiling-eks.sh)
export PROFILER_ECR_IMAGE="public.ecr.aws/odigos/odigos/core/profiler"

chmod +x scripts/verify-ec2-aws-kube-ecr.sh
./scripts/verify-ec2-aws-kube-ecr.sh
```

**What “good” looks like**

| Check | Command (manual) | Success |
|--------|------------------|--------|
| AWS identity | `aws sts get-caller-identity` | Account and Arn printed |
| EKS API | `kubectl cluster-info` / `kubectl get ns` | Reaches cluster |
| Docker | `docker info` | No error |
| Pull a **known** profiler tag | `PROFILER_PULL_TAG=ui-<sha> ./scripts/verify-ec2-aws-kube-ecr.sh` | Image pulls after you have pushed that tag |
| Authenticated push | After `make profiler-ecr-public-login` | `docker push $PROFILER_ECR_IMAGE:your-tag` succeeds |

**Credentials**

- For **ECR Public login**, use IAM permissions aligned with `scripts/iam-ecr-profiler-push.json`.
- Optional env file (gitignored): `scripts/eks-vm-aws.env` — sourced by `scripts/aws-ecr-login-from-env.sh`.

```bash
make profiler-ecr-public-login   # uses aws-ecr-login-from-env.sh public
```

## Why these repo changes matter (kind and EKS)

- **Node collector `profiles` pipeline** requires the collector flag  
  `--feature-gates=service.profilesSupport`.  
  The **gateway** already had this in autoscaler code; the **Helm chart** now adds the same flag to the **odiglet `data-collection`** container when `profiling.enabled` is true (`helm/odigos/templates/odiglet/daemonset.yaml`).
- **Odiglet multi-arch builds** on an amd64 builder: `go generate` must run with the **builder’s** `GOARCH` (`BUILDARCH`), then the binary is built for `TARGETARCH` (`odiglet/Dockerfile`).

Without these, the node collector can **CrashLoop** with  
`pipeline "profiles": ... gated under the "service.profilesSupport" feature gate`.

## kind: end-to-end profiling test (local cluster)

### 1) Create a kind cluster

```bash
kind create cluster --name odigos-profiling
kubectl config use-context kind-odigos-profiling
```

(Optional) Use the repo’s kind config if you need extra mounts:

```bash
kind create cluster --config=tests/common/apply/kind-config.yaml
```

### 2) Install Odigos with profiling enabled

First install a **baseline** Odigos release (version must match your chart expectations), then merge profiling values and **branch images**.

**Option A — Images pulled by kind nodes from ECR Public** (simplest if the cluster has internet):

```bash
export PROFILER_SHA="$(git rev-parse --short=8 HEAD)"
export PROFILER_ECR_IMAGE="public.ecr.aws/odigos/odigos/core/profiler"

helm upgrade --install odigos ./helm/odigos -n odigos-system --create-namespace \
  --set image.tag=v1.22.0 \
  -f scripts/profiling-enable-values.yaml \
  --set "images.ui=${PROFILER_ECR_IMAGE}:ui-${PROFILER_SHA}" \
  --set "images.autoscaler=${PROFILER_ECR_IMAGE}:autoscaler-${PROFILER_SHA}" \
  --set "images.collector=${PROFILER_ECR_IMAGE}:collector-${PROFILER_SHA}" \
  --set "images.scheduler=${PROFILER_ECR_IMAGE}:scheduler-${PROFILER_SHA}" \
  --set "images.instrumentor=${PROFILER_ECR_IMAGE}:instrumentor-${PROFILER_SHA}" \
  --set "images.odiglet=${PROFILER_ECR_IMAGE}:odiglet-${PROFILER_SHA}"
```

You must have **pushed** those tags first (`make profiler-ecr-public-login` and `make push-profiler-images-eks`, or per-component targets). If the collector multi-arch build exhausts disk, build **amd64** and **arm64** separately and use `docker buildx imagetools create` (see comments in the root `Makefile` near `push-profiler-collector`).

**Option B — Load images into kind** (air-gapped / no registry):

```bash
export TAG="$(git rev-parse --short=8 HEAD)"
export ORG="registry.odigos.io"   # or the org you build with

make build-ui build-collector build-scheduler build-autoscaler build-instrumentor build-odiglet TAG="$TAG" ORG="$ORG"
make load-to-kind-ui load-to-kind-collector load-to-kind-scheduler load-to-kind-autoscaler load-to-kind-instrumentor load-to-kind-odiglet TAG="$TAG" ORG="$ORG"
# … plus agents/init if your chart needs them — see `make load-to-kind`

helm upgrade --install odigos ./helm/odigos -n odigos-system --create-namespace \
  --set image.tag="$TAG" \
  --set imagePrefix="$ORG/odigos" \
  -f scripts/profiling-enable-values.yaml
```

Adjust `imagePrefix` / `image.tag` to match how your images are tagged after `kind load`.

### 3) Rollouts (scheduler first)

After any upgrade that changes **scheduler** or **effective-config**:

```bash
kubectl rollout restart deployment/odigos-scheduler -n odigos-system
kubectl rollout status  deployment/odigos-scheduler -n odigos-system --timeout=180s
kubectl rollout restart deployment/odigos-autoscaler deployment/odigos-gateway deployment/odigos-ui -n odigos-system
kubectl rollout status  deployment/odigos-autoscaler -n odigos-system --timeout=300s
kubectl rollout restart daemonset/odiglet -n odigos-system
kubectl rollout status  daemonset/odiglet -n odigos-system --timeout=400s
```

### 4) Validate ConfigMaps (pipeline wiring)

```bash
kubectl get cm effective-config -n odigos-system -o jsonpath='{.data.config\.yaml}' | grep -A8 '^profiling:'
kubectl get cm odigos-gateway -n odigos-system -o jsonpath='{.data.collector-conf}' | grep -E 'profiles:|gateway-profiles|profiles-ui'
kubectl get cm odigos-data-collection -n odigos-system -o jsonpath='{.data.conf}' | grep -E 'profiles:|profiling'
```

After a **node** `CollectorsGroup` exists and the **data-collection** container is healthy, the last command should show **profiles** pipeline fragments. If `data-collection` crashes, check logs for the **feature gate** error above.

### 5) Optional app + Destination + Source (non-empty profiles)

Odigos only creates rich **node** collector config when there is a **Destination** (signals non-empty) and **Source** objects.

```bash
kubectl apply -f tests/debug-exporter.yaml
```

Deploy a demo namespace (example: Google **Online Boutique**):

```bash
kubectl create namespace online-boutique --dry-run=client -o yaml | kubectl apply -f -
curl -fsSL https://raw.githubusercontent.com/GoogleCloudPlatform/microservices-demo/main/release/kubernetes-manifests.yaml \
  | kubectl apply -n online-boutique -f -
kubectl apply -f scripts/online-boutique-odigos-source.yaml
```

Wait for workloads, then confirm `InstrumentationConfig` objects exist:

```bash
kubectl get instrumentationconfigs -n online-boutique
```

### 6) Helm / template-only verification (no cluster)

```bash
make verify-profiling-helm
# or
./scripts/verify-profiling-pipeline.sh --helm-only
```

### 7) CI-style unit / integration slice (no kind)

```bash
./scripts/run-profiling-ci-tests.sh
```

### 8) HTTP smoke: UI profiling API + artifact dump

```bash
# Terminal 1
kubectl port-forward -n odigos-system svc/ui 3000:3000

# Terminal 2 — use a real instrumented workload NS/KIND/NAME
chmod +x scripts/profiling-test-and-dump.sh
NS=online-boutique KIND=Deployment NAME=frontend \
  START_PORT_FORWARD=true \
  ./scripts/profiling-test-and-dump.sh
```

Artifacts land under `profiling-dumps/`. **Non-empty** `numTicks` needs OTLP profiles on the UI path (load on the workload, healthy **odiglet** + **data-collection**, and a supported runtime for eBPF profiling).

## EKS (EC2): reuse the same scripts

On EKS, after kubeconfig points at the cluster:

```bash
export KUBECONFIG="$HOME/.kube/eks.config"
export PROFILER_ECR_IMAGE="public.ecr.aws/odigos/odigos/core/profiler"
export PROFILER_SHA="$(git rev-parse --short=8 HEAD)"

make profiler-ecr-public-login
./scripts/helm-upgrade-profiling-eks.sh
# Extend with instrumentor + odiglet when testing full node path:
helm upgrade odigos ./helm/odigos -n odigos-system --reuse-values \
  -f scripts/profiling-enable-values.yaml \
  --set "images.instrumentor=${PROFILER_ECR_IMAGE}:instrumentor-${PROFILER_SHA}" \
  --set "images.odiglet=${PROFILER_ECR_IMAGE}:odiglet-${PROFILER_SHA}"
```

Then run the **rollouts** and **verification** sections above.

## Script reference (tracked in git)

| Path | Purpose |
|------|---------|
| `scripts/profiling-enable-values.yaml` | Helm values: enable profiling + gateway file export |
| `scripts/helm-upgrade-profiling-eks.sh` | Helm upgrade for EKS with profiler ECR tags (ui/autoscaler/collector/scheduler) |
| `scripts/profiling-test-and-dump.sh` | curl UI profiling APIs + optional gateway JSONL copy |
| `scripts/verify-profiling-pipeline.sh` | Helm/manifest checks for profiling wiring |
| `scripts/run-profiling-ci-tests.sh` | Go/Python tests without a cluster |
| `scripts/aws-ecr-login-from-env.sh` | Docker login to ECR Public / private using AWS creds |
| `scripts/iam-ecr-profiler-push.json` | Example IAM for ECR Public push |
| `scripts/online-boutique-odigos-source.yaml` | Namespace `Source` for microservices-demo |
| `scripts/verify-ec2-aws-kube-ecr.sh` | EC2/AWS/kubectl/Docker/ECR sanity checks |
| `docs/profiling-e2e-testing-kind-aws.md` | This document |

## Troubleshooting

- **Node collector CrashLoop**: logs show `service.profilesSupport` → ensure Helm revision includes `odiglet` template with `--feature-gates=service.profilesSupport` when `profiling.enabled` is true, and upgrade Odigos.
- **No `profiling` in `effective-config`**: use a **branch scheduler** image that persists profiling into `effective-config` (see `scheduler` controller); stock release schedulers may drop unknown fields.
- **Empty profile samples**: confirm **branch odiglet** and **collector** on all node architectures, a **Destination** + **Source**, and CPU load on the target workload.
