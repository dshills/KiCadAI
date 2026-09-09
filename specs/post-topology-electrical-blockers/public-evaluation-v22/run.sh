#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
freeze="$root/specs/post-topology-electrical-blockers/public-evaluation-v22"
if [ "$#" -ne 1 ] || [[ "$1" != /* ]]; then
  printf 'usage: bash %s /absolute/fresh/scratch-root\n' "$0" >&2
  exit 2
fi
scratch=$1
cd "$root"
test -z "$(git status --porcelain --untracked-files=all)"
revision=$(git rev-parse HEAD)
git diff --exit-code a808d612769bcb4821a22cbf7dd6535feded1f6a HEAD -- . \
  ':(exclude)internal/capabilityexecutorv10/v22_executor.go' \
  ':(exclude)internal/capabilityexecutorv10/v22_runner.go' \
  ':(exclude)internal/capabilityexecutorv10/v22_runner_test.go' \
  ':(exclude)cmd/kicadai-discovery-baseline-v22' \
  ':(exclude)specs/post-topology-electrical-blockers/V22_PUBLIC_POPULATION.json' \
  ':(exclude)specs/post-topology-electrical-blockers/public-evaluation-v22'
(cd "$freeze" && shasum -a 256 -c FREEZE.sha256 && shasum -a 256 -c EVALUATOR.sha256)
(cd "$root/specs/post-topology-electrical-blockers" && shasum -a 256 -c V22_IMPLEMENTATION.sha256 && shasum -a 256 -c EVIDENCE.sha256 && shasum -a 256 -c DIAGNOSTIC.sha256)
(cd "$root/specs/generic-causal-topology-repair" && shasum -a 256 -c V21_CONTRACT.sha256 && shasum -a 256 -c V21_EVALUATOR.sha256)
(cd "$root/specs/generic-analysis-model-solver-admission" && shasum -a 256 -c V20_CONTRACT.sha256 && shasum -a 256 -c V20_EVALUATOR.sha256)
export GOTOOLCHAIN=go1.26.8
export GOENV=off GOWORK=off GOFLAGS= GOEXPERIMENT= CGO_ENABLED=1
export GOCACHE="$root/.cache/go/build" GOMODCACHE="$root/.cache/go/mod"
test "$(go env GOVERSION)" = go1.26.8
test "$(go env GOOS)/$(go env GOARCH)" = darwin/arm64
go mod verify
go test ./specs/post-topology-electrical-blockers/public-evaluation-v22 -run '^TestV22Freeze$' -count=1
report="$root/internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/report.json"
test ! -e "$(dirname "$report")"
test ! -e "$scratch"
mkdir "$scratch"
printf '%s\n' "$revision" > "$scratch/source-commit.txt"
go env GOVERSION GOOS GOARCH CGO_ENABLED GOENV GOWORK GOFLAGS GOEXPERIMENT > "$scratch/go-environment.txt"
go build -trimpath -o "$scratch/evaluator" ./cmd/kicadai-discovery-baseline-v22
shasum -a 256 "$scratch/evaluator" > "$scratch/evaluator.sha256"
/usr/bin/time -l "$scratch/evaluator" --repository-root "$root" --working-root "$scratch/cases" \
  --report "$report" --timeout 6h --keep-artifacts=true \
  > "$scratch/summary.json" 2> "$scratch/evaluator.stderr.log"
test "$(git rev-parse HEAD)" = "$revision"
git diff --exit-code "$revision"
go test ./specs/post-topology-electrical-blockers/public-evaluation-v22 \
  -run '^TestPublishedV22Evaluation$' -count=1 -v > "$scratch/assessment.log"
printf 'V22 public report published without replacement: %s\n' "$report"
