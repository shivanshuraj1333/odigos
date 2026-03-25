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
from typing import Any, Iterator, Optional, TextIO, Tuple


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


def get_dictionary_blob(obj: Any) -> Optional[dict[str, Any]]:
    """OTLP JSON uses 'dictionary'; some encodings use 'Dictionary'."""
    if not isinstance(obj, dict):
        return None
    d = obj.get("dictionary")
    if d is None:
        d = obj.get("Dictionary")
    if isinstance(d, dict):
        return d
    return None


def string_table_len(dict_blob: dict[str, Any]) -> int:
    st = dict_blob.get("stringTable")
    if st is None:
        st = dict_blob.get("StringTable")
    if isinstance(st, list):
        return len(st)
    return 0


def audit_dictionary(obj: Any) -> tuple[bool, int, int]:
    """Return (dictionary_is_empty, string_table_len, dict_key_count for top-level dictionary)."""
    d = get_dictionary_blob(obj)
    if d is None or len(d) == 0:
        return True, 0, 0
    n = string_table_len(d)
    return False, n, len(d)


def dictionary_ok_for_symbols(obj: Any, min_string_table: int) -> tuple[bool, str]:
    """True if batch has a non-empty dictionary with stringTable long enough for in-band symbols."""
    d = get_dictionary_blob(obj)
    if d is None:
        return False, "missing_dictionary"
    if len(d) == 0:
        return False, "dictionary_object_empty"
    n = string_table_len(d)
    if n < min_string_table:
        return False, f"stringTable_too_short len={n} need>={min_string_table}"
    return True, ""


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--min-lines", type=int, default=1, help="Minimum non-empty lines to succeed")
    ap.add_argument(
        "--require-key",
        action="append",
        default=[],
        help="Require this attribute key on at least one resource (repeatable)",
    )
    ap.add_argument(
        "--audit-dictionary",
        action="store_true",
        help="Per line: print dictionary audit (empty dict / stringTable size) to stderr; still prints resource attrs",
    )
    ap.add_argument(
        "--require-nonempty-dictionary",
        action="store_true",
        help="Fail (exit 6) if any line lacks a usable dictionary (stringTable length below --min-string-table)",
    )
    ap.add_argument(
        "--min-string-table",
        type=int,
        default=2,
        help="With --require-nonempty-dictionary: require at least this many stringTable entries (default 2: '' + symbols)",
    )
    ap.add_argument("path", nargs="?", help="File path (default: stdin)")
    args = ap.parse_args()

    nonempty = 0
    keys_seen: set[str] = set()
    dict_fail_line: Optional[Tuple[int, str]] = None

    def process_lines(stream: TextIO) -> None:
        nonlocal nonempty, keys_seen, dict_fail_line
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
            if args.require_nonempty_dictionary:
                ok, reason = dictionary_ok_for_symbols(obj, args.min_string_table)
                if not ok and dict_fail_line is None:
                    dict_fail_line = (nonempty, reason)
            if args.audit_dictionary:
                empty, st_len, dk = audit_dictionary(obj)
                print(
                    f"# audit line {nonempty}: dictionary_empty={empty} stringTable_len={st_len} dictionary_keys={dk}",
                    file=sys.stderr,
                )
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

    if args.require_nonempty_dictionary and dict_fail_line is not None:
        ln, reason = dict_fail_line
        print(
            f"parse_profiles_jsonl: line {ln} failed dictionary check: {reason} "
            f"(gateway→UI path needs in-band dictionary; see ebpf-profiler / collector export)",
            file=sys.stderr,
        )
        return 6

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
