#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
	printf 'usage: %s <general|open-topology> <zero-based-index> <total>\n' "$0" >&2
	exit 2
fi

group=$1
index=$2
total=$3
case "$index:$total" in
	*[!0-9:]*|:*|*:) printf 'coverage shard index and total must be integers\n' >&2; exit 2 ;;
esac
if [ "$total" -lt 1 ] || [ "$index" -ge "$total" ]; then
	printf 'invalid coverage shard %s of %s\n' "$index" "$total" >&2
	exit 2
fi

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
costs="$root/scripts/testdata/coverage-costs.tsv"
weighted=$(mktemp "${TMPDIR:-/tmp}/kicadai-coverage-weighted.XXXXXX")
trap 'rm -f "$weighted"' EXIT

case "$group" in
	general)
		go list ./... | awk '$0 != "kicadai/internal/opentopologysynthesis"' |
			awk -F '\t' '
				NR == FNR {
					if ($0 !~ /^#/ && NF == 3 && $1 == "package") cost[$2] = $3
					next
				}
				{
					weight = ($1 in cost) ? cost[$1] : 1
					printf "%.6f\t%s\n", weight, $1
				}
			' "$costs" - >"$weighted"
		;;
	open-topology)
		go test -list '^(Test|Example|Fuzz)' ./internal/opentopologysynthesis |
			awk '/^(Test|Example|Fuzz)[[:alnum:]_]*$/ {print}' |
			awk -F '\t' '
				NR == FNR {
					if ($0 !~ /^#/ && NF == 3 && $1 == "test") cost[$2] = $3
					next
				}
				{
					weight = ($1 in cost) ? cost[$1] : 1
					printf "%.6f\t%s\n", weight, $1
				}
			' "$costs" - >"$weighted"
		;;
	*)
		printf 'unknown coverage shard group: %s\n' "$group" >&2
		exit 2
		;;
esac

if [ ! -s "$weighted" ]; then
	printf 'coverage shard candidate set is empty for %s\n' "$group" >&2
	exit 1
fi

# Longest-processing-time assignment is deterministic: descending recorded
# cost, then lexical identity, then the lowest-index least-loaded shard.
LC_ALL=C sort -t $'\t' -k1,1nr -k2,2 "$weighted" |
	awk -F '\t' -v target="$index" -v total="$total" '
		BEGIN { for (i = 0; i < total; i++) load[i] = 0 }
		{
			selected = 0
			for (i = 1; i < total; i++) {
				if (load[i] < load[selected]) selected = i
			}
			if (selected == target) print $2
			load[selected] += $1
		}
	'
