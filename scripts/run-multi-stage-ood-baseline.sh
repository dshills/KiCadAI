#!/usr/bin/env bash

set -euo pipefail
shopt -s nullglob

if [[ "$#" -ne 8 ]]; then
  echo "usage: $0 BINARY CORPUS_ROOT OUTPUT_ROOT CATALOG_ROOT SYMBOLS_ROOT FOOTPRINTS_ROOT KICAD_CLI JOBS" >&2
  exit 2
fi

binary=$1
corpus_root=$2
output_root=$3
catalog_root=$4
symbols_root=$5
footprints_root=$6
kicad_cli=$7
jobs=$8

if [[ ! -x "$binary" || ! -x "$kicad_cli" || ! "$jobs" =~ ^[1-9][0-9]*$ ]]; then
  echo "binary, kicad-cli, or job count is invalid" >&2
  exit 2
fi

run_one() {
  local repeat=$1
  local requirement=$2
  local id
  local log_root
  local project_root
  local status

  id=$(basename "$requirement" .json)
  log_root="$output_root/logs/run-$repeat"
  project_root="$output_root/projects/run-$repeat/$id"
  mkdir -p "$log_root" "$(dirname "$project_root")"

  if "$binary" \
      --request "$requirement" \
      --output "$project_root" \
      --overwrite \
      --catalog-dir "$catalog_root" \
      --symbols-root "$symbols_root" \
      --footprints-root "$footprints_root" \
      --kicad-cli "$kicad_cli" \
      open-topology create \
      >"$log_root/$id.stdout.json" \
      2>"$log_root/$id.stderr"; then
    status=0
  else
    status=$?
  fi
  printf '%s\n' "$status" >"$log_root/$id.exit"
  printf 'run=%s case=%s exit=%s\n' "$repeat" "$id" "$status"
}

requirements=()
for requirement in "$corpus_root"/*.json; do
  if [[ "$(basename "$requirement")" != "manifest.json" ]]; then
    requirements+=("$requirement")
  fi
done
if [[ "${#requirements[@]}" -eq 0 ]]; then
  echo "no corpus requirements found" >&2
  exit 2
fi

export binary catalog_root symbols_root footprints_root kicad_cli output_root
export -f run_one
for repeat in 1 2; do
  for requirement in "${requirements[@]}"; do
    printf '%s\0%s\0' "$repeat" "$requirement"
  done | xargs -0 -n 2 -P "$jobs" bash -c 'run_one "$1" "$2"' _
done

for requirement in "${requirements[@]}"; do
  id=$(basename "$requirement" .json)
  for suffix in stdout.json stderr exit; do
    first="$output_root/logs/run-1/$id.$suffix"
    second="$output_root/logs/run-2/$id.$suffix"
    if [[ ! -f "$first" || ! -f "$second" ]]; then
      echo "missing replay evidence for $id.$suffix" >&2
      exit 1
    fi
    if ! cmp -s "$first" "$second"; then
      echo "replay differs for $id.$suffix" >&2
      exit 1
    fi
  done
done

printf 'baseline complete: cases=%s invocations=%s replay=identical\n' "${#requirements[@]}" "$((2 * ${#requirements[@]}))"
