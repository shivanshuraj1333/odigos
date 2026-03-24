#!/usr/bin/env bash
# Bootstrap a fresh Ubuntu EC2 (or any Ubuntu VM) to mirror the Odigos EKS dev machine
# (zsh + Oh My Zsh + Powerlevel10k, Docker, AWS/kubectl/helm/kind/Go, EKS kubeconfig).
#
# Run on the NEW instance:
#   sudo apt-get update && sudo apt-get install -y git
#   git clone --depth 1 -b <your-branch> <your-fork-or-upstream-url> ~/work/odigos
#   cd ~/work/odigos
#   ./scripts/bootstrap-ec2-odigos-dev.sh --env-file ~/eks-vm-aws.env [options]
#
# ----------------------------------------------------------------------------
# INPUTS YOU MUST PROVIDE (outside this script)
# ----------------------------------------------------------------------------
#
# 1) eks-vm-aws.env — same shape as scripts/eks-vm-aws.env.example, with real keys.
#    Create on a trusted machine, then:  scp eks-vm-aws.env ubuntu@NEW_EC2:~/
#    Pass:  --env-file ~/eks-vm-aws.env
#
# 2) Git over SSH (optional but typical) — EITHER:
#    - Copy your existing key:  scp ~/.ssh/id_ed25519 ~/.ssh/id_ed25519.pub ubuntu@NEW_EC2:~/.ssh/
#    - Or pass:  --ssh-private-key /path/on-ec2/to/id_ed25519
#    - Or generate on EC2:  ssh-keygen -t ed25519 -C "you@github"  then add ~/.ssh/id_ed25519.pub to GitHub
#
# 3) Odigos repo — clone before running (see example above), or use --clone with --repo-url.
#
# ----------------------------------------------------------------------------
# OPTIONAL FLAGS
# ----------------------------------------------------------------------------
#
#   --env-file PATH       Required for EKS/ECR wiring; copied to ODIGOS_ROOT/scripts/eks-vm-aws.env
#   --odigos-root PATH    Default: ~/work/odigos
#   --repo-url URL        With --clone (default upstream; use your fork SSH URL if private)
#   --repo-branch NAME    Checkout branch after clone (optional)
#   --clone               Clone repo into --odigos-root (fails if directory exists and is non-empty)
#   --ssh-private-key P   Install key as ~/.ssh/id_ed25519 (mode 600); pubkey optional
#   --ssh-public-key P    If you only have private key from export; else derived from .pub
#   --skip-shell          Do not install zsh / Oh My Zsh / snippet (tools + AWS only)
#   --skip-p10k           Oh My Zsh stays on the default theme (no Powerlevel10k clone)
#   --go-version V        Default: 1.24.1
#   --no-eks-config       Skip aws eks update-kubeconfig + kubectl get nodes
#   --yes                 Non-interactive (no confirmation before apt installs)
#
# ----------------------------------------------------------------------------
# NOT AUTOMATED (install manually if you need them)
# ----------------------------------------------------------------------------
#
# - Cursor CLI (cursor-agent) — follow Cursor docs for Linux install on that machine.
# - ODIGOS_ONPREM_TOKEN — only for Helm installs against Odigos commercial/on-prem charts.
# - IAM / EKS access — must already match scripts/eks-vm-deployer-access.sh expectations.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SNIPPET_TEMPLATE="${SCRIPT_DIR}/ec2-dev-env/zshrc-odigos-eks.snippet.in"

ENV_FILE=""
ODIGOS_ROOT="${ODIGOS_ROOT:-${HOME}/work/odigos}"
REPO_URL="${REPO_URL:-https://github.com/odigos-io/odigos.git}"
REPO_BRANCH=""
DO_CLONE=false
SSH_PRIVATE_KEY_SRC=""
SSH_PUBLIC_KEY_SRC=""
SKIP_SHELL=false
SKIP_P10K=false
GO_VERSION="${GO_VERSION:-1.24.1}"
NO_EKS_CONFIG=false
ASSUME_YES=false

die() { echo "error: $*" >&2; exit 1; }

usage() {
  sed -n '2,/^set -euo pipefail$/p' "$0" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
  case "${1}" in
    --env-file) ENV_FILE="${2:-}"; shift 2 ;;
    --odigos-root) ODIGOS_ROOT="${2:-}"; shift 2 ;;
    --repo-url) REPO_URL="${2:-}"; shift 2 ;;
    --repo-branch) REPO_BRANCH="${2:-}"; shift 2 ;;
    --clone) DO_CLONE=true; shift ;;
    --ssh-private-key) SSH_PRIVATE_KEY_SRC="${2:-}"; shift 2 ;;
    --ssh-public-key) SSH_PUBLIC_KEY_SRC="${2:-}"; shift 2 ;;
    --skip-shell) SKIP_SHELL=true; shift ;;
    --skip-p10k) SKIP_P10K=true; shift ;;
    --go-version) GO_VERSION="${2:-}"; shift 2 ;;
    --no-eks-config) NO_EKS_CONFIG=true; shift ;;
    --yes) ASSUME_YES=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

[[ "$(id -u)" -ne 0 ]] || die "run as normal user (ubuntu), not root — script uses sudo where needed"

if [[ "${ASSUME_YES}" != true ]]; then
  read -r -p "Install packages and dev tools on this host? [y/N] " ans
  [[ "${ans}" =~ ^[yY] ]] || die "aborted"
fi

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "missing command after install: $1"; }

sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y \
  apt-transport-https ca-certificates curl gnupg unzip git jq build-essential

if [[ "${SKIP_SHELL}" != true ]]; then
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y zsh
fi

# --- Docker ---
if ! command -v docker >/dev/null 2>&1; then
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y docker.io
fi
sudo systemctl enable --now docker 2>/dev/null || true
sudo usermod -aG docker "${USER}"

# --- AWS CLI v2 (same pattern as scripts/eks-mac-vm-eks.sh) ---
install_aws_cli() {
  if command -v aws >/dev/null 2>&1; then
    aws --version
    return 0
  fi
  local arch zip_url tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' RETURN
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) zip_url="https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" ;;
    aarch64) zip_url="https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip" ;;
    *) die "unsupported arch for AWS CLI: ${arch}" ;;
  esac
  curl -fsSL "${zip_url}" -o "${tmp}/awscliv2.zip"
  unzip -q -o "${tmp}/awscliv2.zip" -d "${tmp}"
  sudo "${tmp}/aws/install" --update
  aws --version
}
install_aws_cli

# --- kubectl ---
if ! command -v kubectl >/dev/null 2>&1; then
  kver="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) karch=amd64 ;;
    aarch64) karch=arm64 ;;
    *) die "unsupported arch for kubectl: ${arch}" ;;
  esac
  tmp="$(mktemp)"
  curl -fsSL "https://dl.k8s.io/release/${kver}/bin/linux/${karch}/kubectl" -o "${tmp}"
  sudo install -m 0755 "${tmp}" /usr/local/bin/kubectl
  rm -f "${tmp}"
fi
kubectl version --client >/dev/null

# --- Helm ---
if ! command -v helm >/dev/null 2>&1; then
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi
helm version >/dev/null

# --- kind ---
if ! command -v kind >/dev/null 2>&1; then
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) karch=amd64 ;;
    aarch64) karch=arm64 ;;
    *) die "unsupported arch for kind: ${arch}" ;;
  esac
  kind_ver="$(curl -fsSL https://api.github.com/repos/kubernetes-sigs/kind/releases/latest | jq -r .tag_name)"
  tmp="$(mktemp)"
  curl -fsSL "https://kind.sigs.k8s.io/dl/${kind_ver}/kind-linux-${karch}" -o "${tmp}"
  sudo install -m 0755 "${tmp}" /usr/local/bin/kind
  rm -f "${tmp}"
fi
kind version >/dev/null

# --- Go ---
if ! command -v go >/dev/null 2>&1; then
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) garch=amd64 ;;
    aarch64) garch=arm64 ;;
    *) die "unsupported arch for Go: ${arch}" ;;
  esac
  tgz="go${GO_VERSION}.linux-${garch}.tar.gz"
  tmp="$(mktemp -d)"
  curl -fsSL "https://go.dev/dl/${tgz}" -o "${tmp}/${tgz}"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "${tmp}/${tgz}"
  rm -rf "${tmp}"
fi
grep -q '/usr/local/go/bin' "${HOME}/.profile" 2>/dev/null || echo 'export PATH=$PATH:/usr/local/go/bin' >> "${HOME}/.profile"
export PATH="${PATH}:/usr/local/go/bin"
go version

# --- SSH key (optional) ---
if [[ -n "${SSH_PRIVATE_KEY_SRC}" ]]; then
  [[ -f "${SSH_PRIVATE_KEY_SRC}" ]] || die "SSH private key not found: ${SSH_PRIVATE_KEY_SRC}"
  mkdir -p "${HOME}/.ssh"
  chmod 700 "${HOME}/.ssh"
  install -m 0600 "${SSH_PRIVATE_KEY_SRC}" "${HOME}/.ssh/id_ed25519"
  if [[ -n "${SSH_PUBLIC_KEY_SRC}" ]]; then
    install -m 0644 "${SSH_PUBLIC_KEY_SRC}" "${HOME}/.ssh/id_ed25519.pub"
  elif [[ -f "${SSH_PRIVATE_KEY_SRC}.pub" ]]; then
    install -m 0644 "${SSH_PRIVATE_KEY_SRC}.pub" "${HOME}/.ssh/id_ed25519.pub"
  fi
fi
if [[ ! -f "${HOME}/.ssh/known_hosts" ]] || ! grep -q '^github.com ' "${HOME}/.ssh/known_hosts" 2>/dev/null; then
  mkdir -p "${HOME}/.ssh"
  chmod 700 "${HOME}/.ssh"
  ssh-keyscan -t ed25519 github.com >> "${HOME}/.ssh/known_hosts" 2>/dev/null || true
fi

# --- Clone Odigos (optional) ---
if [[ "${DO_CLONE}" == true ]]; then
  if [[ -e "${ODIGOS_ROOT}" ]] && [[ -n "$(ls -A "${ODIGOS_ROOT}" 2>/dev/null)" ]]; then
    die "refusing --clone: ${ODIGOS_ROOT} exists and is not empty"
  fi
  mkdir -p "$(dirname "${ODIGOS_ROOT}")"
  if [[ -n "${REPO_BRANCH}" ]]; then
    git clone --depth 1 --branch "${REPO_BRANCH}" "${REPO_URL}" "${ODIGOS_ROOT}"
  else
    git clone --depth 1 "${REPO_URL}" "${ODIGOS_ROOT}"
  fi
fi

[[ -d "${ODIGOS_ROOT}/scripts" ]] || die "Odigos repo not found at ${ODIGOS_ROOT} — clone first or use --clone"

# --- Secrets: eks-vm-aws.env ---
if [[ -n "${ENV_FILE}" ]]; then
  [[ -f "${ENV_FILE}" ]] || die "--env-file not found: ${ENV_FILE}"
  _env_dest="${ODIGOS_ROOT}/scripts/eks-vm-aws.env"
  if [[ "$(realpath "${ENV_FILE}")" == "$(realpath "${_env_dest}")" ]]; then
    chmod 600 "${_env_dest}" || true
  else
    install -m 0600 "${ENV_FILE}" "${_env_dest}"
  fi
else
  echo "warning: no --env-file; copy eks-vm-aws.env to ${ODIGOS_ROOT}/scripts/eks-vm-aws.env yourself" >&2
fi

# --- ~/.aws/config profile (region from env file if possible) ---
mkdir -p "${HOME}/.aws"
AWS_REGION_FALLBACK="ap-southeast-2"
if [[ -f "${ODIGOS_ROOT}/scripts/eks-vm-aws.env" ]]; then
  # shellcheck disable=SC1090
  set -a && source "${ODIGOS_ROOT}/scripts/eks-vm-aws.env" && set +a || true
fi
AWS_REGION_EFFECTIVE="${AWS_REGION:-${AWS_DEFAULT_REGION:-${AWS_REGION_FALLBACK}}}"
if [[ ! -f "${HOME}/.aws/config" ]] || ! grep -q '^\[profile ebpf-eks-deploy\]' "${HOME}/.aws/config" 2>/dev/null; then
  cat >> "${HOME}/.aws/config" <<EOF

[profile ebpf-eks-deploy]
region = ${AWS_REGION_EFFECTIVE}
EOF
else
  echo "note: ~/.aws/config already has [profile ebpf-eks-deploy] — leaving as-is" >&2
fi

# --- EKS kubeconfig ---
if [[ -f "${ODIGOS_ROOT}/scripts/eks-vm-aws.env" && "${NO_EKS_CONFIG}" != true ]]; then
  if [[ -x "${ODIGOS_ROOT}/scripts/eks-mac-vm-eks.sh" ]]; then
    bash "${ODIGOS_ROOT}/scripts/eks-mac-vm-eks.sh" vm-setup || die "vm-setup failed — check keys and cluster name"
  else
    die "missing ${ODIGOS_ROOT}/scripts/eks-mac-vm-eks.sh"
  fi
fi

# --- zsh + Oh My Zsh + Powerlevel10k + snippet ---
if [[ "${SKIP_SHELL}" != true ]]; then
  [[ -f "${SNIPPET_TEMPLATE}" ]] || die "missing snippet template: ${SNIPPET_TEMPLATE}"
  ODIGOS_ROOT_ABS="$(cd "${ODIGOS_ROOT}" && pwd)"
  SNIPPET_CONTENT="$(sed "s|__ODIGOS_ROOT__|${ODIGOS_ROOT_ABS}|g" "${SNIPPET_TEMPLATE}")"

  if [[ ! -d "${HOME}/.oh-my-zsh" ]]; then
    RUNZSH=no CHSH=no sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended || die "Oh My Zsh install failed"
  fi

  if [[ "${SKIP_P10K}" != true ]]; then
    ZSH_CUSTOM="${HOME}/.oh-my-zsh/custom"
    P10K_DIR="${ZSH_CUSTOM}/themes/powerlevel10k"
    if [[ ! -d "${P10K_DIR}" ]]; then
      git clone --depth=1 https://github.com/romkatv/powerlevel10k.git "${P10K_DIR}"
    fi
    # Quiet instant prompt + skip wizard on headless machines
    if ! grep -q 'POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD' "${HOME}/.zshrc" 2>/dev/null; then
      awk '1; /^export ZSH=/ { print ""; print "typeset -g POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true"; print "typeset -g POWERLEVEL9K_INSTANT_PROMPT=quiet"; print "" }' "${HOME}/.zshrc" > "${HOME}/.zshrc.tmp.$$" && mv "${HOME}/.zshrc.tmp.$$" "${HOME}/.zshrc"
    fi
    sed -i.bak 's/^ZSH_THEME=.*/ZSH_THEME="powerlevel10k\/powerlevel10k"/' "${HOME}/.zshrc" || true
    rm -f "${HOME}/.zshrc.bak"
  fi

  if ! grep -q 'odigos-eks-env-begin' "${HOME}/.zshrc" 2>/dev/null; then
    printf '\n%s\n' "${SNIPPET_CONTENT}" >> "${HOME}/.zshrc"
  fi

  if command -v chsh >/dev/null 2>&1 && command -v zsh >/dev/null 2>&1; then
    echo "Setting default login shell to zsh (sudo may prompt once)."
    sudo chsh -s "$(command -v zsh)" "${USER}" || echo "note: chsh failed — run: sudo chsh -s $(command -v zsh) ${USER}" >&2
  fi
fi

echo ""
echo "Done."
echo "  Odigos:     ${ODIGOS_ROOT}"
echo "  AWS env:    ${ODIGOS_ROOT}/scripts/eks-vm-aws.env"
echo "  Kubeconfig: \${KUBECONFIG:-~/.kube/eks-ebpf-profiler.config}"
echo ""
echo "Log out and back in (or new SSH session) so the docker group and default shell apply."
echo "Then: cd ${ODIGOS_ROOT} && source ~/.zshrc && kubectl get nodes"
