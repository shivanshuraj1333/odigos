#!/usr/bin/env bash
# After EBS resize: grow partition 1 on nvme root disk and resize ext4.
set -euo pipefail
ROOT_PART="${ROOT_PART:-/dev/nvme0n1p1}"
DISK="${DISK:-/dev/nvme0n1}"
PART_NUM="${PART_NUM:-1}"
[[ "$(id -u)" -eq 0 ]] || { echo "Run: sudo $0" >&2; exit 1; }
echo "Before:"; df -h /; lsblk "$DISK"
command -v growpart >/dev/null || { apt-get update -qq && apt-get install -y -qq cloud-guest-utils; }
set +e
gp_out="$(growpart "$DISK" "$PART_NUM" 2>&1)"
gp_rc=$?
set -e
echo "$gp_out"
if [[ "$gp_rc" -ne 0 ]] && ! echo "$gp_out" | grep -q NOCHANGE; then
  exit "$gp_rc"
fi
resize2fs "$ROOT_PART"
echo "After:"; df -h /; lsblk "$DISK"
