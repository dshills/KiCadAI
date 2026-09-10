#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
freeze="$root/specs/electrical-repair-followup-v23"
if [ "$#" -ne 1 ] || [[ "$1" != /* ]]; then
  printf 'usage: bash %s /absolute/fresh/scratch-root\n' "$0" >&2
  exit 2
fi
scratch=$1
cd "$root"
test -z "$(git status --porcelain --untracked-files=all)"
revision=$(git rev-parse HEAD)
# Every production/input byte remains at the V22 publication commit. Only the
# separately sealed diagnostic directory may differ before this invocation.
git diff --exit-code 39aff2fbff00f64c65cc568f4333f9591bd28824 HEAD -- . \
  ':(exclude)specs/electrical-repair-followup-v23'
(cd "$freeze" && shasum -a 256 -c DIAGNOSTIC.sha256)
(cd "$root/internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1" && shasum -a 256 -c SHA256SUMS)
export GOTOOLCHAIN=go1.26.8 GOENV=off GOWORK=off GOFLAGS= GOEXPERIMENT= CGO_ENABLED=1
export GOCACHE="$root/.cache/go/build" GOMODCACHE="$root/.cache/go/mod"
test "$(go env GOVERSION)" = go1.26.8
test "$(go env GOOS)/$(go env GOARCH)" = darwin/arm64
go mod verify
go test -count=1 ./specs/electrical-repair-followup-v23 \
  ./specs/post-topology-electrical-blockers/publication-v22 \
  ./specs/post-topology-electrical-blockers/public-evaluation-v22
test ! -e "$scratch"
mkdir "$scratch"
mkdir "$scratch/results"
printf '%s\n' "$revision" > "$scratch/source-commit.txt"
go env GOVERSION GOOS GOARCH CGO_ENABLED GOENV GOWORK GOFLAGS GOEXPERIMENT > "$scratch/go-environment.txt"
go test -c -trimpath -o "$scratch/diagnostic" ./specs/electrical-repair-followup-v23
shasum -a 256 "$scratch/diagnostic" > "$scratch/diagnostic.sha256"
cd "$freeze"
KICADAI_V23_DIAGNOSTIC_OUTPUT="$scratch/results" /usr/bin/time -l "$scratch/diagnostic" \
  -test.run '^TestFrozenResidualDiagnostic$' -test.count=1 -test.timeout=31m -test.v \
  > "$scratch/progress.log" 2> "$scratch/resource-usage.log"
cd "$root"
test "$(git rev-parse HEAD)" = "$revision"
test -z "$(git status --porcelain --untracked-files=all)"
git diff --exit-code "$revision"
printf 'V23 residual diagnostic complete; V22 unchanged: %s\n' "$scratch/results"
