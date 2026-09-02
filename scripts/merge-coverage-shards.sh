#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
	printf 'usage: %s <shard-root> <merged-profile>\n' "$0" >&2
	exit 2
fi

shard_root=$1
merged_profile=$2
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
expected_shards=$((${COVERAGE_GENERAL_SHARDS:-4} + ${COVERAGE_OPEN_TOPOLOGY_SHARDS:-6}))
work=$(mktemp -d "${TMPDIR:-/tmp}/kicadai-coverage-merge.XXXXXX")
trap 'rm -rf "$work"' EXIT

if [ ! -d "$shard_root" ]; then
	printf 'coverage shard root does not exist: %s\n' "$shard_root" >&2
	exit 1
fi

find "$shard_root" -mindepth 1 -maxdepth 1 -type d -print | LC_ALL=C sort >"$work/shards.txt"
actual_shards=$(wc -l <"$work/shards.txt" | tr -d ' ')
if [ "$actual_shards" -ne "$expected_shards" ]; then
	printf 'coverage shard count mismatch: got %s, want %s\n' "$actual_shards" "$expected_shards" >&2
	exit 1
fi

while IFS= read -r shard; do
	COVERAGE_VERIFY_CURRENT_SOURCE=1 "$root/scripts/verify-coverage-shard.sh" "$shard"
	cat "$shard/tests.txt" >>"$work/tests.unsorted"
	cat "$shard/packages.txt" >>"$work/packages.unsorted"
	printf '%s\n' "$shard/profile.out" >>"$work/profiles.txt"
done <"$work/shards.txt"

LC_ALL=C sort "$work/tests.unsorted" >"$work/tests.sorted"
LC_ALL=C sort "$work/tests.unsorted" | uniq -d >"$work/tests.duplicates"
if [ -s "$work/tests.duplicates" ]; then
	printf 'coverage shards execute tests more than once:\n' >&2
	cat "$work/tests.duplicates" >&2
	exit 1
fi
COVER_TEST_SKIP=${COVER_TEST_SKIP:-} "$root/scripts/coverage-test-inventory.sh" >"$work/tests.expected"
if ! cmp -s "$work/tests.expected" "$work/tests.sorted"; then
	printf 'coverage shard test selection differs from the bounded suite\n' >&2
	diff -u "$work/tests.expected" "$work/tests.sorted" >&2 || true
	exit 1
fi

LC_ALL=C sort -u "$work/packages.unsorted" >"$work/packages.actual"
(cd "$root" && go list ./... | LC_ALL=C sort -u) >"$work/packages.expected"
if ! cmp -s "$work/packages.expected" "$work/packages.actual"; then
	printf 'coverage shard package selection differs from ./...\n' >&2
	diff -u "$work/packages.expected" "$work/packages.actual" >&2 || true
	exit 1
fi

profiles=()
while IFS= read -r profile; do
	profiles+=("$profile")
done <"$work/profiles.txt"

awk '
	FNR == 1 {
		if ($0 != "mode: set") {
			printf "unexpected coverage mode in %s: %s\n", FILENAME, $0 > "/dev/stderr"
			exit 1
		}
		next
	}
	{
		region = $1
		statements = $2
		count = $3 + 0
		if ((region in statement_count) && statement_count[region] != statements) {
			printf "coverage statement-count mismatch for %s\n", region > "/dev/stderr"
			exit 1
		}
		statement_count[region] = statements
		if (!(region in covered) || count > covered[region]) covered[region] = count
	}
	END {
		for (region in statement_count) {
			printf "%s %s %d\n", region, statement_count[region], covered[region]
		}
	}
	' "${profiles[@]}" >"$work/profile.unsorted"
LC_ALL=C sort -k1,1 "$work/profile.unsorted" >"$work/profile.body"
mkdir -p "$(dirname -- "$merged_profile")"
{
	printf 'mode: set\n'
	cat "$work/profile.body"
} >"$merged_profile"

printf 'coverage merge complete: shards=%s tests=%s packages=%s profile=%s\n' \
	"$actual_shards" \
	"$(wc -l <"$work/tests.sorted" | tr -d ' ')" \
	"$(wc -l <"$work/packages.actual" | tr -d ' ')" \
	"$merged_profile"
