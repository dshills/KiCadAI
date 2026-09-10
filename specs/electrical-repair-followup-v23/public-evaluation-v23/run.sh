#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
freeze="$root/specs/electrical-repair-followup-v23/public-evaluation-v23"
if [ "$#" -ne 1 ] || [[ "$1" != /* ]]; then
  printf 'usage: bash %s /absolute/fresh/scratch-root\n' "$0" >&2
  exit 2
fi
scratch=$1
cd "$root"
test -z "$(git status --porcelain --untracked-files=all)"
revision=$(git rev-parse HEAD)
git diff --exit-code 9413c881ace95065b62d4cdaa31ee532f0b7cf87 HEAD -- . \
  ':(exclude)internal/capabilityexecutorv10/v23_executor.go' \
  ':(exclude)internal/capabilityexecutorv10/v23_runner.go' \
  ':(exclude)internal/capabilityexecutorv10/v23_runner_test.go' \
  ':(exclude)cmd/kicadai-discovery-baseline-v23' \
  ':(exclude)specs/electrical-repair-followup-v23/V23_PUBLIC_POPULATION.json' \
  ':(exclude)specs/electrical-repair-followup-v23/public-evaluation-v23'
(cd "$freeze" && shasum -a 256 -c FREEZE.sha256 && shasum -a 256 -c EVALUATOR.sha256)
(cd "$root/specs/electrical-repair-followup-v23" && shasum -a 256 -c REPAIR_IMPLEMENTATION.sha256 && shasum -a 256 -c ADAPTER_IMPLEMENTATION.sha256 && shasum -a 256 -c SOLVER_IMPLEMENTATION.sha256 && shasum -a 256 -c CORRECTION_SCOPE.sha256 && shasum -a 256 -c EVIDENCE.sha256 && shasum -a 256 -c DIAGNOSTIC.sha256)
(cd "$root/specs/post-topology-electrical-blockers" && shasum -a 256 -c V22_IMPLEMENTATION.sha256 && shasum -a 256 -c EVIDENCE.sha256 && shasum -a 256 -c DIAGNOSTIC.sha256)
(cd "$root/specs/post-topology-electrical-blockers/public-evaluation-v22" && shasum -a 256 -c FREEZE.sha256 && shasum -a 256 -c EVALUATOR.sha256)
(cd "$root/internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1" && shasum -a 256 -c SHA256SUMS)
(cd "$root/specs/generic-causal-topology-repair" && shasum -a 256 -c V21_CONTRACT.sha256 && shasum -a 256 -c V21_EVALUATOR.sha256)
(cd "$root/specs/generic-analysis-model-solver-admission" && shasum -a 256 -c V20_CONTRACT.sha256 && shasum -a 256 -c V20_EVALUATOR.sha256)
export GOTOOLCHAIN=go1.26.8
export GOENV=off GOWORK=off GOFLAGS= GOEXPERIMENT= CGO_ENABLED=1
export GOCACHE="$root/.cache/go/build" GOMODCACHE="$root/.cache/go/mod"
test "$(go env GOVERSION)" = go1.26.8
test "$(go env GOOS)/$(go env GOARCH)" = darwin/arm64
go mod verify
go test ./specs/electrical-repair-followup-v23/public-evaluation-v23 -run '^TestV23Freeze$' -count=1
report="$root/internal/capabilityfeedback/testdata/closed_loop_open_set_v23_public_1/report.json"
test ! -e "$(dirname "$report")"
test ! -e "$scratch"
mkdir "$scratch"
printf '%s\n' "$revision" > "$scratch/source-commit.txt"
go env GOVERSION GOOS GOARCH CGO_ENABLED GOENV GOWORK GOFLAGS GOEXPERIMENT > "$scratch/go-environment.txt"
go build -trimpath -o "$scratch/evaluator" ./cmd/kicadai-discovery-baseline-v23
shasum -a 256 "$scratch/evaluator" > "$scratch/evaluator.sha256"
/usr/bin/time -l "$scratch/evaluator" --repository-root "$root" --working-root "$scratch/cases" \
  --report "$report" --timeout 6h --keep-artifacts=true \
  > "$scratch/summary.json" 2> "$scratch/evaluator.stderr.log"
test "$(git rev-parse HEAD)" = "$revision"
git diff --exit-code "$revision"
go test ./specs/electrical-repair-followup-v23/public-evaluation-v23 \
  -run '^TestPublishedV23Evaluation$' -count=1 -v > "$scratch/assessment.log"
printf 'V23 public report published without replacement: %s\n' "$report"
