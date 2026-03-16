# Profiles work – handoff for VM development

Use this file when opening the repo on the VM in Cursor: **@PROFILES_HANDOFF.md** or paste relevant parts into a new chat so the AI has context.

---

## Branch & commit

- **Branch:** `feature/profiles-node-collector-gateway`
- **Commit:** `e548ae69` – "Profiles: node collector + gateway pipeline, configurable verification, container.id default"
- **Repo on VM:** `~/odigos` (clone from github.com:shivanshuraj1333/odigos)

---

## What’s done

1. **Node collector (data collection / odiglet)**  
   - Profiles pipeline: OTLP in → batch, memory_limiter, nodeName, resourcedetection, **k8sattributes** → `otlp/out-cluster-collector-profiles`.  
   - Default **pod_association** for PID + container.id data: **container.id** first, then connection, then k8s.pod.uid, k8s.pod.ip.  
   - **Config override:** ConfigMap `odigos-node-collector-profiles-config` (key `profiles`) overrides the built-in profiles config. Helm: `collectorNode.profiles.config` (YAML string).  
   - Feature gate: `--feature-gates=service.profilesSupport` on the data-collection container (Helm).

2. **Gateway**  
   - When verification endpoint is set: profiles pipeline (otlp → batch → `otlp/profiles-verification`).  
   - Feature gate: `--feature-gates=service.profilesSupport` on gateway deployment.

3. **Verification endpoint (deploy-time)**  
   - Helm: `autoscaler.profileVerificationOtlpEndpoint`.  
   - Autoscaler gets env `PROFILE_VERIFICATION_OTLP_ENDPOINT`; gateway config gets the verification exporter.  
   - Can point to Pyroscope, Odigos UI, or e.g. `host.docker.internal:4040` for local.

4. **API**  
   - `OdigosNodeCollectorProfilesConfigMapName = "odigos-node-collector-profiles-config"`.

---

## Files touched (for reference)

- `api/k8sconsts/nodecollector.go`
- `autoscaler/controllers/nodecollector/collectorconfig/common.go`
- `autoscaler/controllers/nodecollector/collectorconfig/profiles.go` (new)
- `autoscaler/controllers/nodecollector/configmap.go`
- `autoscaler/controllers/nodecollector/configmap_test.go`
- `autoscaler/controllers/nodecollector/testdata/*.yaml`
- `autoscaler/controllers/clustercollector/configmap.go`
- `autoscaler/controllers/clustercollector/deployment.go`
- `helm/odigos/templates/odiglet/daemonset.yaml`
- `helm/odigos/templates/nodecollector-profiles-config.yaml` (new)
- `helm/odigos/templates/autoscaler/deployment.yaml`
- `helm/odigos/values.yaml`
- `helm/odigos/values.schema.json`

---

## Next steps (from here on the VM)

- Build collector image (with profiles support), push to registry, use in EKS.
- Deploy with `autoscaler.profileVerificationOtlpEndpoint` set; verify in Pyroscope/UI.
- If profiler doesn’t send `container.id`, override via `collectorNode.profiles.config` (pod_association).
- Frontend/backend profiling UI and 10-maps/session logic come in a later phase.

---

*Generated for handoff to VM development.*
