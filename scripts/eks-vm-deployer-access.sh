#!/usr/bin/env bash
# Creates a dedicated IAM user + minimal API policy + EKS access entry so a VM can
# use: aws eks update-kubeconfig && kubectl / helm against ONE cluster.
#
# Run this ONCE from a machine whose AWS identity can manage IAM and EKS access
# (e.g. cluster admin account). Do NOT commit access keys or .kube/config.
#
# Usage:
#   export CLUSTER_NAME=eBPF-Profiler-Testbed
#   export AWS_REGION=ap-southeast-2
#   export DEPLOYER_USER_NAME=eks-deploy-ebpf-profiler-testbed   # optional; default is lowercase
#   ./scripts/eks-vm-deployer-access.sh
#
# Prerequisite: cluster must use authenticationMode API or API_AND_CONFIG_MAP (not CONFIG_MAP-only).
#
# Optional:
#   CREATE_ACCESS_KEY=false   # skip creating a new key (user already has keys)
#   K8S_ACCESS_POLICY_ARN=arn:aws:eks::aws:cluster-access-policy/AmazonEKSViewPolicy
#                             # default is ClusterAdmin (needed for helm install)

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-eBPF-Profiler-Testbed}"
AWS_REGION="${AWS_REGION:-ap-southeast-2}"
# Lowercase default user name — mixed-case IAM user names have triggered EKS
# CreateAccessEntry "invalid principal" for some accounts; override if you need a fixed name.
_default_sanitized="$(echo "${CLUSTER_NAME}" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_-]/-/g')"
DEPLOYER_USER_NAME="${DEPLOYER_USER_NAME:-eks-deploy-${_default_sanitized}}"
CREATE_ACCESS_KEY="${CREATE_ACCESS_KEY:-true}"
ACCESS_ENTRY_RETRIES="${ACCESS_ENTRY_RETRIES:-6}"
ACCESS_ENTRY_RETRY_SLEEP_SEC="${ACCESS_ENTRY_RETRY_SLEEP_SEC:-5}"
# ClusterAdmin is required for typical odigos helm install/upgrade; tighten later if needed.
K8S_ACCESS_POLICY_ARN="${K8S_ACCESS_POLICY_ARN:-arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy}"

die() { echo "error: $*" >&2; exit 1; }

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "missing command: $1 (install AWS CLI v2)"; }
need_cmd aws
need_cmd jq

ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
[[ -n "${ACCOUNT_ID}" && "${ACCOUNT_ID}" != "None" ]] || die "could not resolve AWS account (configure admin credentials)"

CLUSTER_ARN="arn:aws:eks:${AWS_REGION}:${ACCOUNT_ID}:cluster/${CLUSTER_NAME}"
echo "Using caller account=${ACCOUNT_ID} cluster=${CLUSTER_NAME} region=${AWS_REGION}"

CLUSTER_JSON="$(aws eks describe-cluster --name "${CLUSTER_NAME}" --region "${AWS_REGION}" --output json)" \
  || die "cluster not found or no permission to describe it: ${CLUSTER_NAME}"

CLUSTER_ACCOUNT="$(echo "${CLUSTER_JSON}" | jq -r '.cluster.arn | split(":")[4]')"
AUTH_MODE="$(echo "${CLUSTER_JSON}" | jq -r '.cluster.accessConfig // {} | .authenticationMode // empty')"
[[ "${CLUSTER_ACCOUNT}" == "${ACCOUNT_ID}" ]] \
  || die "caller account (${ACCOUNT_ID}) != cluster account (${CLUSTER_ACCOUNT}). Use credentials in the cluster's account, or create the IAM user there."

if [[ "${AUTH_MODE}" == "CONFIG_MAP" || -z "${AUTH_MODE}" ]]; then
  die "cluster authenticationMode must be API or API_AND_CONFIG_MAP for access entries (got: ${AUTH_MODE:-empty}). Run as cluster admin:

  aws eks update-cluster-config --name ${CLUSTER_NAME} --region ${AWS_REGION} \\
    --access-config authenticationMode=API_AND_CONFIG_MAP

  Wait until the cluster finishes updating, then re-run this script."
fi

echo "Cluster accessConfig.authenticationMode=${AUTH_MODE}"

# --- IAM: minimal policy (EKS DescribeCluster on this cluster only) ---
POLICY_NAME="EksDescribeCluster-${CLUSTER_NAME//[^a-zA-Z0-9]/-}"
POLICY_DOC="$(jq -nc \
  --arg cluster_arn "${CLUSTER_ARN}" \
  '{Version:"2012-10-17",Statement:[{Sid:"DescribeThisCluster",Effect:"Allow",Action:["eks:DescribeCluster"],Resource:$cluster_arn}]}')"

POLICY_ARN=""
if POLICY_ARN="$(
  aws iam list-policies --scope Local --output json | jq -r --arg n "${POLICY_NAME}" '.Policies[]? | select(.PolicyName == $n) | .Arn' | head -1
)" && [[ -n "${POLICY_ARN}" ]]; then
  echo "IAM policy already exists: ${POLICY_ARN}"
else
  POLICY_ARN="$(
    aws iam create-policy --policy-name "${POLICY_NAME}" --policy-document "${POLICY_DOC}" --query Policy.Arn --output text
  )"
  echo "Created IAM policy: ${POLICY_ARN}"
fi

# --- IAM user ---
if aws iam get-user --user-name "${DEPLOYER_USER_NAME}" >/dev/null 2>&1; then
  echo "IAM user already exists: ${DEPLOYER_USER_NAME}"
else
  aws iam create-user --user-name "${DEPLOYER_USER_NAME}" >/dev/null
  echo "Created IAM user: ${DEPLOYER_USER_NAME}"
fi

# Attach describe policy
ATTACHED="$(aws iam list-attached-user-policies --user-name "${DEPLOYER_USER_NAME}" --output json \
  | jq -r --arg arn "${POLICY_ARN}" '.AttachedPolicies[]? | select(.PolicyArn == $arn) | .PolicyArn' | head -1 || true)"
if [[ "${ATTACHED}" == "${POLICY_ARN}" ]]; then
  echo "Describe policy already attached to user."
else
  aws iam attach-user-policy --user-name "${DEPLOYER_USER_NAME}" --policy-arn "${POLICY_ARN}"
  echo "Attached describe policy to user."
fi

# --- EKS access entry + Kubernetes API access ---
# Always use the canonical ARN from IAM (includes path if any). Constructed ARNs can mismatch.
PRINCIPAL_ARN="$(aws iam get-user --user-name "${DEPLOYER_USER_NAME}" --query User.Arn --output text)" \
  || die "could not read IAM user ARN for ${DEPLOYER_USER_NAME}"
echo "Using principal ARN from IAM: ${PRINCIPAL_ARN}"

if aws eks describe-access-entry \
  --cluster-name "${CLUSTER_NAME}" \
  --principal-arn "${PRINCIPAL_ARN}" \
  --region "${AWS_REGION}" \
  >/dev/null 2>&1; then
  echo "EKS access entry already exists for ${PRINCIPAL_ARN}"
else
  n=1
  while true; do
    if aws eks create-access-entry \
      --cluster-name "${CLUSTER_NAME}" \
      --principal-arn "${PRINCIPAL_ARN}" \
      --region "${AWS_REGION}" \
      --type STANDARD \
      >/dev/null 2>&1; then
      echo "Created EKS access entry for ${PRINCIPAL_ARN}"
      break
    fi
    if [[ "${n}" -ge "${ACCESS_ENTRY_RETRIES}" ]]; then
      echo "aws eks create-access-entry failed after ${ACCESS_ENTRY_RETRIES} attempts. Last error:" >&2
      aws eks create-access-entry \
        --cluster-name "${CLUSTER_NAME}" \
        --principal-arn "${PRINCIPAL_ARN}" \
        --region "${AWS_REGION}" \
        --type STANDARD >&2 || true
      die "giving up (IAM propagation, auth mode, or principal type). If this user was created with an older uppercase-heavy name, create a new IAM user with DEPLOYER_USER_NAME=... (lowercase) or use an IAM role + sts:AssumeRole."
    fi
    echo "create-access-entry not accepted yet (attempt ${n}/${ACCESS_ENTRY_RETRIES}); waiting ${ACCESS_ENTRY_RETRY_SLEEP_SEC}s (IAM/EKS propagation)..."
    sleep "${ACCESS_ENTRY_RETRY_SLEEP_SEC}"
    n=$((n + 1))
  done
fi

# Idempotent associate: list policies on entry, attach if missing
EXISTING_POLICIES="$(aws eks list-associated-access-policies \
  --cluster-name "${CLUSTER_NAME}" \
  --principal-arn "${PRINCIPAL_ARN}" \
  --region "${AWS_REGION}" \
  --output json 2>/dev/null | jq -r '.associatedAccessPolicies[]?.policyArn' || true)"

if echo "${EXISTING_POLICIES}" | grep -qF "${K8S_ACCESS_POLICY_ARN}"; then
  echo "Kubernetes access policy already associated: ${K8S_ACCESS_POLICY_ARN}"
else
  aws eks associate-access-policy \
    --cluster-name "${CLUSTER_NAME}" \
    --principal-arn "${PRINCIPAL_ARN}" \
    --policy-arn "${K8S_ACCESS_POLICY_ARN}" \
    --access-scope type=cluster \
    --region "${AWS_REGION}" \
    >/dev/null
  echo "Associated Kubernetes access policy: ${K8S_ACCESS_POLICY_ARN}"
fi

# --- Access key for the VM (optional) ---
if [[ "${CREATE_ACCESS_KEY}" == "true" ]]; then
  KEY_COUNT="$(aws iam list-access-keys --user-name "${DEPLOYER_USER_NAME}" --query 'length(AccessKeyMetadata)' --output text)"
  if [[ "${KEY_COUNT}" -ge 2 ]]; then
    echo "warning: user already has 2 access keys; cannot create another. Set CREATE_ACCESS_KEY=false or delete an old key."
  else
    echo ""
    echo "========== SAVE THESE CREDENTIALS NOW (shown once) =========="
    aws iam create-access-key --user-name "${DEPLOYER_USER_NAME}" --output table
    echo "=============================================================="
    echo "Store the Access Key ID and Secret in a password manager. Do not commit or paste into tickets."
  fi
else
  echo "Skipping access key creation (CREATE_ACCESS_KEY=false)."
fi

echo ""
echo "----------- On the VM (deployer identity) -----------"
echo "  export AWS_REGION=${AWS_REGION}"
echo "  export AWS_ACCESS_KEY_ID=...   # from the key above, or use aws configure"
echo "  export AWS_SECRET_ACCESS_KEY=..."
echo "  aws eks update-kubeconfig --name ${CLUSTER_NAME} --region ${AWS_REGION}"
echo "  kubectl get nodes"
echo ""
echo "This user can ONLY call eks:DescribeCluster on this cluster via IAM;"
echo "Kubernetes permissions follow the EKS access policy you set (currently cluster-scoped)."
