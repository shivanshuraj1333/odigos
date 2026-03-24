# Cursor agent on EC2 — what to say and what to avoid

Use this when you start a chat with the agent on the **EC2** dev host (`~/work/odigos`). Paste the **session block** below (fill in the blanks). Update it as your task changes.

## Session block (copy into the first message)

```text
Workspace: ~/work/odigos on EC2 (Remote-SSH).
Branch: feat/eBPF-profiling-support (confirm with: git branch --show-current).
Goal: <one sentence — e.g. fix profiling pipeline, helm upgrade, e2e script>

Context:
- EKS cluster: eBPF-Profiler-Testbed, region ap-southeast-2 (credentials via ~/.zshrc sourcing scripts/eks-vm-aws.env — do not paste keys).
- If kubectl fails: say whether it is timeout vs auth; I can fix SG/VPC separately.

Constraints:
- Do not put secrets in git (see below).
- Prefer minimal diffs; match existing repo style.

Recent commands / errors:
<paste terminal output; redact account IDs or keys if you want>
```

## Never paste into Cursor chat

- Contents of `scripts/eks-vm-aws.env`
- Any **AWS access key / secret** or session token
- **PEM** or other private key material
- Raw **kubeconfig** if it embeds long-lived tokens

The agent can rely on the file existing on disk; you only confirm paths and symptoms.

## Never commit to git

These must stay local:

- `scripts/eks-vm-aws.env` (already in `.gitignore`)
- `*.pem`, ad-hoc `.env` files with secrets
- Large local dumps under `profiling-dumps/` (gitignored)

Before every `git commit`, run `git status` and `git diff --staged` and confirm no secret paths appear.

## Shell and tools on EC2

Open a **new terminal** after SSH (or use login zsh), then:

```bash
cd ~/work/odigos
source ~/.zshrc
```

Check AWS identity (should show `eks-deploy-eBPF-Profiler-Testbed`):

```bash
aws sts get-caller-identity
```

Check cluster (needs network path from EC2 to EKS API):

```bash
kubectl get nodes
```

Docker (after re-login if you were added to the `docker` group):

```bash
docker run --rm hello-world
```

## Git vocabulary (what you are seeing)

| What you see | Meaning |
|--------------|--------|
| **Changes not staged** | Edited files; not yet in the next commit |
| **Untracked files** | New files Git is not tracking yet |
| **Ahead of origin by N** | You have **N local commits** not pushed; run `git push origin feat/eBPF-profiling-support` from this clone (or the machine that has those commits) so EC2 can `git pull` |
| **Shallow clone** | `git rev-parse --is-shallow-repository` → `true` → run `git fetch --unshallow` on EC2 for full history |

## Keeping EC2 in sync with another machine

1. On the machine with the latest work: `git push origin feat/eBPF-profiling-support`
2. On EC2: `cd ~/work/odigos && git pull origin feat/eBPF-profiling-support`
3. If the clone was shallow: `git fetch --unshallow` once (see above)

## Repo-specific pointers

- EKS / VM env template: `scripts/eks-vm-aws.env.example`
- Bootstrap a fresh Ubuntu EC2: `scripts/bootstrap-ec2-odigos-dev.sh` (documented in its header)
- ECR login: `scripts/aws-ecr-login-from-env.sh`
