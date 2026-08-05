#!/usr/bin/env bash
set -euo pipefail

demo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${demo_dir}/../../.." && pwd)"
mode="${1:-positive}"
output_root="${KICADAI_PUBLIC_DEMO_OUTPUT:-${repo_root}/examples/.generated/public-demo}"

find_kicad_cli() {
  if [[ -n "${KICADAI_KICAD_CLI:-}" ]]; then
    printf '%s\n' "${KICADAI_KICAD_CLI}"
  elif command -v kicad-cli >/dev/null 2>&1; then
    command -v kicad-cli
  elif [[ -x /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli ]]; then
    printf '%s\n' /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
  fi
}

find_library_root() {
  local configured="$1"
  local mac_path="$2"
  local linux_path="$3"
  if [[ -n "${configured}" ]]; then
    printf '%s\n' "${configured}"
  elif [[ -d "${mac_path}" ]]; then
    printf '%s\n' "${mac_path}"
  elif [[ -d "${linux_path}" ]]; then
    printf '%s\n' "${linux_path}"
  fi
}

kicad_cli="$(find_kicad_cli)"
symbols_root="$(find_library_root "${KICADAI_SYMBOLS_ROOT:-}" \
  /Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
  /usr/share/kicad/symbols)"
footprints_root="$(find_library_root "${KICADAI_FOOTPRINTS_ROOT:-}" \
  /Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
  /usr/share/kicad/footprints)"

if [[ -z "${kicad_cli}" || ! -x "${kicad_cli}" ]]; then
  printf 'kicad-cli was not found; set KICADAI_KICAD_CLI\n' >&2
  exit 2
fi
if [[ -z "${symbols_root}" || ! -d "${symbols_root}" ]]; then
  printf 'KiCad symbols were not found; set KICADAI_SYMBOLS_ROOT\n' >&2
  exit 2
fi
if [[ -z "${footprints_root}" || ! -d "${footprints_root}" ]]; then
  printf 'KiCad footprints were not found; set KICADAI_FOOTPRINTS_ROOT\n' >&2
  exit 2
fi

cd "${repo_root}"
make build

case "${mode}" in
  positive)
    output="${output_root}/protected-programmable-current-output"
    mkdir -p "${output}"
    printf 'Generating and independently replaying the protected current output...\n'
    printf 'Full search evidence is retained locally and can require about 1 GB.\n'
    "${repo_root}/bin/kicadai" \
      --request "${demo_dir}/requirement.json" \
      --output "${output}" \
      --overwrite \
      --catalog-dir "${repo_root}/data/components" \
      --symbols-root "${symbols_root}" \
      --footprints-root "${footprints_root}" \
      --kicad-cli "${kicad_cli}" \
      open-topology create | tee "${output}/result.json"
    printf '\nOpen the generated project:\n  %s\n' \
      "${output}/run-1/protected_programmable_current_output.kicad_pro"
    ;;
  refusal)
    output="${output_root}/refusal-excessive-step-down-stress"
    mkdir -p "${output}"
    printf 'Submitting a requirement outside the reviewed thermal/SOA envelope...\n'
    set +e
    "${repo_root}/bin/kicadai" \
      --request "${demo_dir}/refusal-requirement.json" \
      --output "${output}" \
      --overwrite \
      --catalog-dir "${repo_root}/data/components" \
      --symbols-root "${symbols_root}" \
      --footprints-root "${footprints_root}" \
      --kicad-cli "${kicad_cli}" \
      open-topology create | tee "${output}/result.json"
    status=${PIPESTATUS[0]}
    set -e
    if [[ "${status}" -eq 0 ]]; then
      printf 'expected fail-closed result, but generation succeeded\n' >&2
      exit 1
    fi
    if find "${output}" -name '*.kicad_pro' -print -quit | grep -q .; then
      printf 'refused request unexpectedly produced a KiCad project\n' >&2
      exit 1
    fi
    printf '\nRefusal verified: the command failed and produced no KiCad project.\n'
    ;;
  *)
    printf 'usage: %s [positive|refusal]\n' "$0" >&2
    exit 2
    ;;
esac
