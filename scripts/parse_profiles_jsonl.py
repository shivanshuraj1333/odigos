#!/usr/bin/env python3
"""Parse gateway file exporter profiles.jsonl (OTLP JSON) and print resource attributes.

Each line should be JSON matching ExportProfilesServiceRequest (or compatible).
Usage:
  kubectl exec -n odigos-system deploy/odigos-gateway -- cat /var/odigos/profiles-export/profiles.jsonl | \\
    python3 scripts/parse_profiles_jsonl.py [--min-lines N] [--require-key k8s.pod.name]
"""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any, Iterator, TextIO


def iter_resource_profile_blobs(obj: Any) -> Iterator[dict[str, Any]]:
    """Yield resource profile dicts from a decoded OTLP-ish JSON object."""
    if not isinstance(obj, dict):
        return
    # OTLP JSON (development profiles)
    rps = obj.get("resourceProfiles")
    if isinstance(rps, list):
        for rp in rps:
            if isinstance(rp, dict):
                yield rp
        return
    # Some encodings nest differently
    for key in ("profiles", "resource_profiles"):
        v = obj.get(key)
        if isinstance(v, list):
            for item in v:
                if isinstance(item, dict):
                    yield item


def attrs_from_resource(resource: dict[str, Any]) -> dict[str, str]:
    out: dict[str, str] = {}
    attrs = resource.get("attributes")
    if not isinstance(attrs, list):
        return out
    for a in attrs:
        if not isinstance(a, dict):
            continue
        k = a.get("key")
        val = a.get("value", {})
        if not k:
            continue
        if isinstance(val, dict):
            if "stringValue" in val:
                out[str(k)] = str(val["stringValue"])
            elif "intValue" in val:
                out[str(k)] = str(val["intValue"])
            elif "boolValue" in val:
                out[str(k)] = str(val["boolValue"]).lower()
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--min-lines", type=int, default=1, help="Minimum non-empty lines to succeed")
    ap.add_argument(
        "--require-key",
        action="append",
        default=[],
        help="Require this attribute key on at least one resource (repeatable)",
    )
    ap.add_argument("path", nargs="?", help="File path (default: stdin)")
    args = ap.parse_args()

    nonempty = 0
    keys_seen: set[str] = set()

    def process_lines(stream: TextIO) -> None:
        nonlocal nonempty, keys_seen
        for line in stream:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError as e:
                print(f"parse_profiles_jsonl: bad JSON line: {e}", file=sys.stderr)
                raise SystemExit(2) from e
            nonempty += 1
            for rp in iter_resource_profile_blobs(obj):
                res = rp.get("resource")
                if isinstance(res, dict):
                    amap = attrs_from_resource(res)
                    keys_seen.update(amap.keys())
                    if amap:
                        print(json.dumps(amap, sort_keys=True))

    if args.path:
        with open(args.path, encoding="utf-8") as f:
            process_lines(f)
    else:
        process_lines(sys.stdin)

    if nonempty < args.min_lines:
        print(
            f"parse_profiles_jsonl: expected at least {args.min_lines} JSON lines, got {nonempty}",
            file=sys.stderr,
        )
        return 3

    missing = [k for k in args.require_key if k not in keys_seen]
    if missing:
        print(
            f"parse_profiles_jsonl: missing required attribute keys (never seen): {missing}",
            file=sys.stderr,
        )
        return 4

    print(f"# ok: lines={nonempty} distinct_attr_keys={len(keys_seen)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
