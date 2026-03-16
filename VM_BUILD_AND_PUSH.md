# VM: Build collector image and push to ECR

**Put this in the VM (e.g. ~/odigos). When on the VM in Cursor, give the AI this context so it can build and push.**

---

## Short AI instructions (copy into VM chat)

```
We're in ~/odigos on branch feature/profiles-node-collector-gateway. Build the Odigos collector image and push it to public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo. Use: make build-collector with ORG=public.ecr.aws/odigos/dev/coretestbed and TAG=odigos-collector-demo, then log in to public ECR (us-east-1) and docker push. See VM_BUILD_AND_PUSH.md for exact commands.
```

---

## Target image

- **ECR URI:** `public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo`
- **Repo on VM:** `~/odigos` (branch `feature/profiles-node-collector-gateway`)

---

## Instructions for the AI on the VM

1. **Sync code** (if changes were made on Mac):  
   `cd ~/odigos && git fetch origin && git checkout feature/profiles-node-collector-gateway && git pull`

2. **Build the collector image** (from repo root):
   ```bash
   cd ~/odigos
   make build-collector ORG=public.ecr.aws/odigos/dev/coretestbed TAG=odigos-collector-demo
   ```
   This produces the image `public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo` locally.

3. **Log in to Public ECR** (if not already):
   ```bash
   aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws
   ```
   (Requires AWS CLI configured with credentials that can push to that repo.)

4. **Push the image:**
   ```bash
   docker push public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo
   ```

5. **Tell the user:** "Image built and pushed. Use the Mac/Cursor instructions to deploy and verify in the cluster."

---

## One-liner (after sync)

```bash
cd ~/odigos && make build-collector ORG=public.ecr.aws/odigos/dev/coretestbed TAG=odigos-collector-demo && \
aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws && \
docker push public.ecr.aws/odigos/dev/coretestbed:odigos-collector-demo
```
