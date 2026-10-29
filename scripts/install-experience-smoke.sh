#!/usr/bin/env bash
set -euo pipefail
if [[ $# != 1 ]]; then echo 'Usage: bash scripts/install-experience-smoke.sh BINARY' >&2; exit 2; fi
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
input=$(cd -- "$(dirname -- "$1")" && pwd)/$(basename -- "$1")
if [[ ! -x "$input" ]]; then echo 'Expected an executable KiCadAI binary' >&2; exit 2; fi
work=$(mktemp -d "${TMPDIR:-/tmp}/kicadai-install-experience.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/empty working directory" "$work/home" "$work/tmp" "$work/bin with spaces" "$work/libraries"
cp "$input" "$work/bin with spaces/kicadai"
binary="$work/bin with spaces/kicadai"
cp "$root/examples/install-experience/board-family.json" "$work/configuration.json"
cp "$root/examples/install-experience/adapter-request.json" "$work/adapter.json"
cp "$root/examples/educational/01_dc_voltage_source/generated/lib/"*.kicad_sym "$work/libraries/"
cd "$work/empty working directory"
# No repository-relative catalog, API keys, HOME configuration, or provider.
run_clean() { env -i PATH="$PATH" HOME="$work/home" TMPDIR="$work/tmp" "$binary" "$@"; }
doctor=$(run_clean doctor)
grep -Fq '"status": "offline_ready"' <<<"$doctor"
grep -Fq '"api_required": false' <<<"$doctor"
library=$(run_clean --symbols-root "$work/libraries" library index)
grep -Fq '"symbol_count":' <<<"$library"
if grep -Fq '"symbol_count": 0' <<<"$library"; then echo 'Relocated symbol libraries were not indexed' >&2; exit 1; fi
run_clean --output "$work/generated" schematic example divider >/dev/null
cp -R "$work/generated" "$work/relocated schematic"
run_clean --target "$work/relocated schematic" schematic verify >/dev/null
before=$(shasum -a 256 "$work/relocated schematic/divider.kicad_sch")
run_clean --target "$work/relocated schematic" --output "$work/review" review schematic >/dev/null
run_clean --target "$work/review" review verify >/dev/null
after=$(shasum -a 256 "$work/relocated schematic/divider.kicad_sch")
[[ "$before" == "$after" ]]
response=$(run_clean --request "$work/adapter.json" adapter)
grep -Fq '"schema": "netform.kicadai.response.v1"' <<<"$response"
grep -Fq '"ok": true' <<<"$response"
run_clean --request "$work/configuration.json" --output "$work/family" family generate >/dev/null
cp -R "$work/family" "$work/relocated family"
run_clean --target "$work/relocated family" family verify >/dev/null
if run_clean --kicad-cli "$work/missing-kicad" --require-erc doctor >/dev/null 2>&1; then echo 'Missing required tool was accepted' >&2; exit 1; fi
printf 'Relocated install smoke passed: offline catalog, libraries, schematic/review integrity, adapter, and board-family bundle. No native or physical approval.\n'
