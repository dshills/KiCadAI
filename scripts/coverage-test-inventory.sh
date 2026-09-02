#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
skip=${COVER_TEST_SKIP:-}
cd "$root"

go list ./... | while IFS= read -r package; do
	go test -list '^(Test|Example|Fuzz)' "$package" |
		awk -v package="$package" -v skip="$skip" '
			/^(Test|Example|Fuzz)[[:alnum:]_]*$/ {
				if (skip == "" || $0 !~ skip) print package "\t" $0
			}
		'
done | LC_ALL=C sort -u
