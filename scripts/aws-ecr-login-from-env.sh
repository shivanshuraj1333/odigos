#!/usr/bin/env bash
# Load scripts/eks-vm-aws.env (gitignored) then log docker in to registries.
# Usage: ./scripts/aws-ecr-login-from-env.sh [public|private|both]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${AWS_ENV_FILE:-$ROOT/scripts/eks-vm-aws.env}"
MODE="${1:-both}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "$ENV_FILE"
  set +a
fi

if ! aws sts get-caller-identity >/dev/null 2>&1; then
  echo "aws: no valid credentials (set keys in $ENV_FILE or use AWS_PROFILE)" >&2
  exit 1
fi

login_public() {
  echo "Logging in to public.ecr.aws ..."
  aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws
}

login_private() {
  local region="${AWS_REGION:-ap-southeast-2}"
  local acct
  acct="$(aws sts get-caller-identity --query Account --output text)"
  local ep="${acct}.dkr.ecr.${region}.amazonaws.com"
  echo "Logging in to ${ep} ..."
  aws ecr get-login-password --region "$region" | docker login --username AWS --password-stdin "$ep"
}

case "$MODE" in
  public) login_public ;;
  private) login_private ;;
  both)
    login_public || { echo "public ECR login failed (needs ecr-public:GetAuthorizationToken on the IAM principal)" >&2; exit 1; }
    login_private || { echo "private ECR login failed (needs ecr:GetAuthorizationToken + registry permissions)" >&2; exit 1; }
    ;;
  *) echo "usage: $0 [public|private|both]" >&2; exit 1 ;;
esac
echo "docker login ok."
