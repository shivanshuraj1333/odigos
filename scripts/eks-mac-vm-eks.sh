#!/usr/bin/env bash
# EKS from Mac (full AWS) vs VM (deployer IAM user only).
#
# You provide (env or defaults):
#   CLUSTER_NAME      default: eBPF-Profiler-Testbed
#   AWS_REGION        default: ap-southeast-2
#   DEPLOYER_USER     default: eks-deploy-eBPF-Profiler-Testbed
#   AWS_PROFILE       on Mac: your admin profile (optional)
#
# Mac (admin AWS): create a transfer bundle with deployer keys + example env for the VM
#   AWS_PROFILE=your-admin ./scripts/eks-mac-vm-eks.sh mac-bundle
#   scp -r ~/eks-vm-eks-transfer ubuntu@YOUR_VM:~/   # or rsync
#
# VM: install AWS CLI if needed, apply bundle, kubeconfig
#   ./scripts/eks-mac-vm-eks.sh vm-install-aws-cli   # if aws missing
#   ./scripts/eks-mac-vm-eks.sh vm-setup ~/eks-vm-eks-transfer
#
# vm-setup also works with repo file if you copied aws.env there:
#   cp ~/eks-vm-eks-transfer/aws.env scripts/eks-vm-aws.env
#   ./scripts/eks-mac-vm-eks.sh vm-setup

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-eBPF-Profiler-Testbed}"
AWS_REGION="${AWS_REGION:-ap-southeast-2}"
DEPLOYER_USER="${DEPLOYER_USER:-eks-deploy-eBPF-Profiler-Testbed}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TRANSFER_DIR="${EKS_VM_TRANSFER_DIR:-${HOME}/eks-vm-eks-transfer}"

die() { echo "error: $*" >&2; exit 1; }

need_aws() { command -v aws >/dev/null 2>&1 || die "aws CLI not found"; }

cmd_mac_bundle() {
  need_aws
  echo "Using caller: $(aws sts get-caller-identity --query Arn --output text)"
  echo "Creating IAM access key for user: ${DEPLOYER_USER} (max 2 keys per user; delete old keys in IAM if needed)"
  mkdir -p "${TRANSFER_DIR}"
  chmod 700 "${TRANSFER_DIR}"

  read -r AKID SECRET <<<"$(aws iam create-access-key --user-name "${DEPLOYER_USER}" \
    --query 'AccessKey.[AccessKeyId,SecretAccessKey]' --output text)" \
    || die "CreateAccessKey failed (need admin IAM on Mac; user may already have 2 keys)"
  [[ -n "${AKID}" && -n "${SECRET}" ]] || die "empty access key from AWS CLI"

  OUT="${TRANSFER_DIR}/aws.env"
  umask 077
  cat >"${OUT}" <<EOF
# Deployer-only IAM user — EKS + DescribeCluster on one cluster (do not commit)
export CLUSTER_NAME=${CLUSTER_NAME}
export AWS_REGION=${AWS_REGION}
export AWS_DEFAULT_REGION=${AWS_REGION}
export AWS_ACCESS_KEY_ID=${AKID}
export AWS_SECRET_ACCESS_KEY=${SECRET}
EOF
  chmod 600 "${OUT}"

  cat >"${TRANSFER_DIR}/README.txt" <<EOF
Files for your build VM (EKS kubectl/helm only via this IAM user).

1. Copy this folder to the VM:
     scp -r ${TRANSFER_DIR} USER@VM:~/

2. On the VM (in odigos repo):
     ./scripts/eks-mac-vm-eks.sh vm-install-aws-cli   # if needed
     ./scripts/eks-mac-vm-eks.sh vm-setup ~/eks-vm-eks-transfer

3. Optional: move aws.env into repo (still gitignored):
     cp ~/eks-vm-eks-transfer/aws.env ${REPO_ROOT}/scripts/eks-vm-aws.env
     set -a && source ${REPO_ROOT}/scripts/eks-vm-aws.env && set +a
     aws eks update-kubeconfig --name ${CLUSTER_NAME} --region ${AWS_REGION}

Delete this folder on the Mac after transfer or chmod -R go-rwx.
EOF

  echo ""
  echo "Wrote: ${OUT} (mode 600) and ${TRANSFER_DIR}/README.txt"
  echo "Transfer to VM, e.g.:"
  echo "  scp -r ${TRANSFER_DIR} ubuntu@YOUR_VM_HOST:~/"
  echo ""
  echo "This key authenticates as IAM user ${DEPLOYER_USER} (not admin AWS)."
}

cmd_vm_install_aws_cli() {
  if command -v aws >/dev/null 2>&1; then
    aws --version
    return 0
  fi
  if ! command -v unzip >/dev/null 2>&1; then
    sudo apt-get update -qq && sudo apt-get install -y unzip curl
  fi
  TMP="$(mktemp -d)"
  trap 'rm -rf "${TMP}"' EXIT
  ARCH="$(uname -m)"
  case "${ARCH}" in
    x86_64) ZIP_URL="https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" ;;
    aarch64) ZIP_URL="https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip" ;;
    *) die "unsupported arch: ${ARCH}" ;;
  esac
  curl -fsSL "${ZIP_URL}" -o "${TMP}/awscliv2.zip"
  unzip -q -o "${TMP}/awscliv2.zip" -d "${TMP}"
  sudo "${TMP}/aws/install" --update
  aws --version
}

cmd_vm_setup() {
  local bundle="${1:-}"
  local envfile=""
  if [[ -n "${bundle}" ]]; then
    envfile="${bundle}/aws.env"
    [[ -f "${envfile}" ]] || die "missing ${envfile}"
  elif [[ -f "${REPO_ROOT}/scripts/eks-vm-aws.env" ]]; then
    envfile="${REPO_ROOT}/scripts/eks-vm-aws.env"
  elif [[ -f "${HOME}/eks-vm-eks-transfer/aws.env" ]]; then
    envfile="${HOME}/eks-vm-eks-transfer/aws.env"
  else
    die "usage: $0 vm-setup PATH_TO_TRANSFER_DIR   OR   create scripts/eks-vm-aws.env from example"
  fi

  need_aws
  # shellcheck disable=SC1090
  set -a && source "${envfile}" && set +a
  [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]] || die "aws.env missing keys — paste from ~/eks-vm-eks-transfer/aws.env (Mac) into ${envfile}"

  echo "Caller: $(aws sts get-caller-identity --query Arn --output text)"
  aws eks update-kubeconfig --name "${CLUSTER_NAME:-eBPF-Profiler-Testbed}" --region "${AWS_REGION:-ap-southeast-2}"
  kubectl get nodes
  echo "OK — kubectl works. Use same shell or: set -a && source ${envfile} && set +a"
}

usage() {
  sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'
  echo ""
  echo "Commands:"
  echo "  mac-bundle              (Mac + admin AWS) create ${TRANSFER_DIR}/ with deployer aws.env"
  echo "  vm-install-aws-cli      (VM) install AWS CLI v2 if missing"
  echo "  vm-setup [BUNDLE_DIR]   (VM) source aws.env, update-kubeconfig, kubectl get nodes"
}

main() {
  case "${1:-}" in
    mac-bundle) cmd_mac_bundle ;;
    vm-install-aws-cli) cmd_vm_install_aws_cli ;;
    vm-setup) cmd_vm_setup "${2:-}" ;;
    ""|-h|--help) usage ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"
