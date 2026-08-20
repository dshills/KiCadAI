#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
	printf 'usage: %s <kicadai-binary>\n' "$0" >&2
	exit 2
fi

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
binary=$1
expected_version=$(tr -d '[:space:]' < "$root/VERSION")

if [ ! -x "$binary" ]; then
	printf 'release binary is not executable: %s\n' "$binary" >&2
	exit 1
fi

help_output=$($binary --help)
grep -Fq 'kicad-version Print connected KiCad version information' <<<"$help_output"

version_output=$($binary --json version)
grep -Fq '"name": "kicadai"' <<<"$version_output"
grep -Fq "\"version\": \"$expected_version\"" <<<"$version_output"

flag_output=$($binary --format text --version)
grep -Fq "kicadai $expected_version" <<<"$flag_output"

capability_output=$($binary --format text capability generation)
grep -Fq 'generic-circuit-v1' <<<"$capability_output"
grep -Fq 'not free-form or guaranteed for arbitrary' <<<"$capability_output"

example_output=$($binary --format json --document release-smoke plan-led-demo)
grep -Fq '"kind": "add_wire"' <<<"$example_output"

set +e
refusal_output=$($binary --format json \
	--catalog-dir "$root/data/components" \
	--request "$root/examples/circuit-graph/unsupported_unknown_component.json" \
	circuit preflight)
refusal_status=$?
set -e
if [ "$refusal_status" -ne 0 ] && [ "$refusal_status" -ne 1 ]; then
	printf 'refusal smoke returned unexpected status %s\n' "$refusal_status" >&2
	exit 1
fi
grep -Fq '"ok": false' <<<"$refusal_output"
grep -Fq '"code": "GRAPH_COMPONENT_UNRESOLVED"' <<<"$refusal_output"

printf 'release smoke test passed: %s\n' "$binary"
