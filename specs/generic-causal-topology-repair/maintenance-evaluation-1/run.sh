#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
freeze="$root/specs/generic-causal-topology-repair/maintenance-evaluation-1"
if [ "$#" -ne 1 ] || [[ "$1" != /* ]]; then
  printf 'usage: bash %s /absolute/fresh/scratch-root\n' "$0" >&2
  exit 2
fi
scratch=$1
cd "$root"
test -z "$(git status --porcelain --untracked-files=all)"
revision=$(git rev-parse HEAD)
git diff --exit-code b961b7a73aa9d19398d2adbaeee89de008e5d390 HEAD -- . \
  ':(exclude)specs/generic-causal-topology-repair/maintenance-evaluation-1'
(cd "$freeze" && shasum -a 256 -c FREEZE.sha256)
(cd "$root/specs/generic-causal-topology-repair" && shasum -a 256 -c V21_CONTRACT.sha256 && shasum -a 256 -c V21_EVALUATOR.sha256)
(cd "$root/specs/generic-analysis-model-solver-admission" && shasum -a 256 -c V20_EVALUATOR.sha256)
export GOTOOLCHAIN=go1.26.8
export GOENV=off GOWORK=off GOFLAGS= GOEXPERIMENT=
export GOCACHE="$root/.cache/go/build" GOMODCACHE="$root/.cache/go/mod"
test "$(go env GOVERSION)" = go1.26.8
test "$(go env GOOS)/$(go env GOARCH)" = darwin/arm64
go mod verify
go test ./specs/generic-causal-topology-repair/maintenance-evaluation-1 -run '^TestMaintenanceFreeze$' -count=1
report="$root/internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json"
test ! -e "$(dirname "$report")"
test ! -e "$scratch"
mkdir "$scratch"
printf '%s\n' "$revision" > "$scratch/source-commit.txt"
go env GOVERSION GOOS GOARCH CGO_ENABLED GOENV GOWORK GOFLAGS GOEXPERIMENT > "$scratch/go-environment.txt"
go build -trimpath -o "$scratch/evaluator" ./cmd/kicadai-discovery-baseline-v21
shasum -a 256 "$scratch/evaluator" > "$scratch/evaluator.sha256"
"$scratch/evaluator" --repository-root "$root" --working-root "$scratch/cases" \
  --report "$report" --timeout 6h --keep-artifacts=true \
  > "$scratch/summary.json" 2> "$scratch/evaluator.stderr.log"
test "$(git rev-parse HEAD)" = "$revision"
git diff --exit-code "$revision"
go test ./specs/generic-causal-topology-repair/maintenance-evaluation-1 \
  -run '^TestPublishedMaintenanceEvaluation$' -count=1 -v > "$scratch/assessment.log"
printf 'V21 maintenance report published without replacement: %s\n' "$report"
