#!/usr/bin/env bash
# Automated tests referenced by the profiling E2E plan (no kubebuilder/etcd required).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== verify-profiling-helm (make) =="
make verify-profiling-helm

echo "== autoscaler clustercollector profiling unit tests =="
(cd autoscaler/controllers/clustercollector && go test -run 'TestShould|TestGateway' -count=1 -timeout 60s ./...)

echo "== autoscaler nodecollector =="
(cd autoscaler/controllers/nodecollector && go test -count=1 -timeout 180s ./...)

echo "== collector odigosotelcol =="
(cd collector/odigosotelcol && go test --ldflags="-checklinkname=0" -count=1 ./...)

echo "== frontend collector_profiles =="
(cd frontend && go test ./services/collector_profiles/... -count=1 -short -timeout 120s)

echo "== parse_profiles_jsonl sample =="
python3 scripts/parse_profiles_jsonl.py --min-lines 1 --require-key service.name \
  scripts/testdata/profiles_jsonl_sample.jsonl >/dev/null

echo "== parse_profiles_jsonl --require-nonempty-dictionary (fixture with stringTable) =="
python3 scripts/parse_profiles_jsonl.py --min-lines 1 --require-nonempty-dictionary \
  scripts/testdata/profiles_jsonl_with_dictionary.jsonl >/dev/null

echo "All profiling CI tests passed."
