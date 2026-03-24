# Agent notes (Cursor / automation)

## Profiling + EKS / EC2 VM

For work on **eBPF profiling**, **ECR Public profiler images**, **EKS**, or **kind** E2E:

1. Follow **`docs/profiling-e2e-testing-kind-aws.md`** (human- and agent-oriented runbook).
2. Enable or follow the Cursor rule **“Profiling E2E on EC2/EKS or kind”** (`.cursor/rules/profiling-ec2-eks-dev-vm.mdc`) when editing profiling scripts, Helm, or that doc — it summarizes env vars, verification commands, and rollout order.
3. On the VM, export **`KUBECONFIG`** to the EKS kubeconfig used for the profiler testbed before running `kubectl` / Helm against that cluster.

## Repo-specific

- **Odigos Kind install / on-prem token**: follow the existing workspace rule for Kind + Helm when the user asks to install Odigos on kind (including `ODIGOS_ONPREM_TOKEN`).
- Profiler **ECR** flow does **not** use the on-prem Helm token; it uses AWS credentials and `make profiler-ecr-public-login`.
