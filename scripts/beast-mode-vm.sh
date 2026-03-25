#!/usr/bin/env bash
# beast-mode-vm.sh — Configure this EC2 VM for maximum parallel multiarch Docker image building.
# Run as root: sudo ./scripts/beast-mode-vm.sh
#
# What this does:
#   1. Adds a 16 GB swapfile (prevents OOM during parallel Go builds)
#   2. Installs QEMU binfmt for linux/arm64 cross-compilation
#   3. Writes /etc/docker/daemon.json  — parallel downloads/uploads, log limits
#   4. Writes /etc/buildkitd.toml     — caps build cache at 25 GB, uses all 32 CPUs
#   5. Restarts Docker
#   6. Recreates the odigos-multi buildx builder with the new config
#   7. Bootstraps the builder and verifies arm64 support
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

echo "=== [1/7] Swapfile (16 GB) ==="
SWAPFILE=/swapfile
if swapon --show | grep -q "$SWAPFILE"; then
  echo "Swapfile already active — skipping."
else
  if [[ ! -f "$SWAPFILE" ]]; then
    fallocate -l 16G "$SWAPFILE" || dd if=/dev/zero of="$SWAPFILE" bs=1M count=16384 status=progress
  fi
  chmod 600 "$SWAPFILE"
  mkswap "$SWAPFILE"
  swapon "$SWAPFILE"
  # Persist across reboots
  grep -q "$SWAPFILE" /etc/fstab || echo "$SWAPFILE none swap sw 0 0" >> /etc/fstab
  echo "Swapfile active: $(swapon --show)"
fi

# Keep swap pressure low — only use it under real memory pressure
sysctl -w vm.swappiness=10
grep -q "vm.swappiness" /etc/sysctl.conf \
  && sed -i 's/^vm.swappiness=.*/vm.swappiness=10/' /etc/sysctl.conf \
  || echo "vm.swappiness=10" >> /etc/sysctl.conf

echo ""
echo "=== [2/7] QEMU binfmt (arm64 cross-compilation) ==="
# Install QEMU static binaries for all architectures (required for linux/arm64 builds on amd64)
docker run --privileged --rm tonistiigi/binfmt --install all
echo "Registered binfmt handlers:"
ls /proc/sys/fs/binfmt_misc/ | grep -v "^register\|^status" || true

echo ""
echo "=== [3/7] Docker daemon config ==="
cat > /etc/docker/daemon.json <<'EOF'
{
  "storage-driver": "overlay2",
  "max-concurrent-downloads": 12,
  "max-concurrent-uploads": 12,
  "max-download-attempts": 5,
  "default-shm-size": "256m",
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
EOF
echo "Written /etc/docker/daemon.json"

echo ""
echo "=== [4/7] BuildKit config (cap cache at 25 GB, 32-way parallelism) ==="
# maxUsedSpace=25GiB keeps the cache from filling the 80 GB root volume.
# minFreeSpace=12GiB triggers aggressive GC before the disk gets tight.
# max-parallelism=32 uses all 32 vCPUs inside the single BuildKit daemon.
cat > /etc/buildkitd.toml <<'EOF'
[worker.oci]
  max-parallelism = 32

# Evict short-lived local/exec caches after 48 h, cap at 512 MiB
[[worker.oci.gcpolicy]]
  keepDuration = "48h"
  filters = ["type==source.local", "type==exec.cachemount", "type==source.git.checkout"]
  maxUsedSpace = "512MiB"

# Keep layer cache up to 60 days, hard cap 25 GB, always leave 12 GB free
[[worker.oci.gcpolicy]]
  keepDuration = "1440h"
  reservedSpace = "4GiB"
  maxUsedSpace  = "25GiB"
  minFreeSpace  = "12GiB"

[[worker.oci.gcpolicy]]
  reservedSpace = "4GiB"
  maxUsedSpace  = "25GiB"
  minFreeSpace  = "12GiB"

# Nuclear option: evict everything if needed to stay under limits
[[worker.oci.gcpolicy]]
  all           = true
  reservedSpace = "4GiB"
  maxUsedSpace  = "25GiB"
  minFreeSpace  = "12GiB"
EOF
echo "Written /etc/buildkitd.toml"

echo ""
echo "=== [5/7] Restart Docker ==="
systemctl restart docker
sleep 2
docker info | grep -E "CPUs|Memory|Storage Driver|Root Dir"

echo ""
echo "=== [6/7] Recreate odigos-multi buildx builder ==="
# Copy buildx plugin to system-wide path so root can use it too
UBUNTU_BUILDX="$(getent passwd ubuntu | cut -d: -f6)/.docker/cli-plugins/docker-buildx"
if [[ -f "$UBUNTU_BUILDX" ]]; then
  mkdir -p /usr/local/lib/docker/cli-plugins
  cp "$UBUNTU_BUILDX" /usr/local/lib/docker/cli-plugins/docker-buildx
  chmod +x /usr/local/lib/docker/cli-plugins/docker-buildx
fi

# Create the builder as the ubuntu user so it lives in the right context
BUILDX_CMD="docker buildx"
RUN_AS="sudo -u ubuntu"

$RUN_AS $BUILDX_CMD rm odigos-multi 2>/dev/null && echo "Removed old builder." || echo "No existing builder to remove."
$RUN_AS $BUILDX_CMD create \
  --name odigos-multi \
  --driver docker-container \
  --buildkitd-config /etc/buildkitd.toml \
  --driver-opt network=host \
  --buildkitd-flags '--allow-insecure-entitlement=network.host' \
  --use
echo "Builder created."

echo ""
echo "=== [7/7] Bootstrap builder & verify platforms ==="
$RUN_AS $BUILDX_CMD inspect --bootstrap
echo ""
echo "Checking arm64 support..."
$RUN_AS $BUILDX_CMD inspect odigos-multi | grep -i "platform" | head -3 || true

echo ""
echo "=== Beast Mode Active ==="
echo ""
echo "Disk:"
df -h /
echo ""
echo "Memory + Swap:"
free -h
echo ""
echo "Builder:"
docker buildx ls
echo ""
echo "--- Recommended build commands ---"
echo "  # 4 core images in parallel (ui, autoscaler, collector, scheduler):"
echo "  make push-profiler-images-eks PROFILER_PUSH_JOBS=4"
echo ""
echo "  # All 7 images in parallel:"
echo "  make push-profiler-images-eks-full PROFILER_PUSH_JOBS=4"
echo ""
echo "  # amd64-only (fastest, ~2x speedup — use if EKS nodes are all x86):"
echo "  make push-profiler-images-eks PROFILER_PUSH_JOBS=4 PROFILER_PLATFORMS=linux/amd64"
echo ""
echo "  # Then helm upgrade:"
echo "  ./scripts/helm-upgrade-profiling-eks.sh"
echo ""
echo "  # Prune cache if disk gets tight:"
echo "  docker buildx prune --keep-storage 20GB -f"
