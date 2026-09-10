#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
freeze="$root/specs/post-topology-electrical-blockers"
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ] || [[ "$1" != /* ]]; then
  printf 'usage: bash %s /absolute/fresh/scratch-root [--recovery-1]\n' "$0" >&2
  exit 2
fi
scratch=$1
config="$freeze/DIAGNOSTIC_RUN.json"
if [ "$#" -eq 2 ]; then
  test "$2" = --recovery-1
  config="$freeze/DIAGNOSTIC_RECOVERY_1.json"
fi
cd "$root"
test -z "$(git status --porcelain --untracked-files=all)"
revision=$(git rev-parse HEAD)
git diff --exit-code abe617d182369352df8fe0e16f9071a539881b21 HEAD -- . \
  ':(exclude)cmd/kicadai-electrical-diagnostics' \
  ':(exclude)internal/electricaldiagnostics' \
  ':(exclude)specs/post-topology-electrical-blockers'
(cd "$freeze" && shasum -a 256 -c DIAGNOSTIC.sha256)
(cd "$root/specs/generic-causal-topology-repair" && shasum -a 256 -c V21_EVALUATOR.sha256 && shasum -a 256 -c V21_CONTRACT.sha256)
(cd "$root/specs/generic-causal-topology-repair/maintenance-evaluation-1" && shasum -a 256 -c FREEZE.sha256)
(cd "$root/internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1" && shasum -a 256 -c report.sha256)
export GOTOOLCHAIN=go1.26.8 GOENV=off GOWORK=off GOFLAGS= GOEXPERIMENT=
export GOCACHE="$root/.cache/go/build" GOMODCACHE="$root/.cache/go/mod"
test "$(go env GOVERSION)" = go1.26.8
test "$(go env GOOS)/$(go env GOARCH)" = darwin/arm64
go mod verify
go test -count=1 ./cmd/kicadai-electrical-diagnostics ./internal/electricaldiagnostics \
  ./specs/generic-causal-topology-repair/maintenance-evaluation-1 \
  ./specs/post-topology-electrical-blockers
test ! -e "$scratch"
mkdir "$scratch"
printf '%s\n' "$revision" > "$scratch/source-commit.txt"
go env GOVERSION GOOS GOARCH CGO_ENABLED GOENV GOWORK GOFLAGS GOEXPERIMENT > "$scratch/go-environment.txt"
go build -trimpath -o "$scratch/diagnostic" ./cmd/kicadai-electrical-diagnostics
shasum -a 256 "$scratch/diagnostic" > "$scratch/diagnostic.sha256"
/usr/bin/time -l "$scratch/diagnostic" --repository-root "$root" \
  --output-root "$scratch/results" \
  --cases "$(jq -er '.cases | join(",")' "$config")" \
  --timeout "$(jq -er '.timeout' "$config")" \
  > "$scratch/progress.log" 2> "$scratch/resource-usage.log"
test "$(git rev-parse HEAD)" = "$revision"
git diff --exit-code "$revision"
printf 'diagnostic run complete; frozen V21 evaluation unchanged: %s\n' "$scratch/results"
