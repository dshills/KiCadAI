#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
	printf 'usage: %s <expected-profile> <actual-profile>\n' "$0" >&2
	exit 2
fi

canonicalize() {
	local source=$1
	local target=$2
	local body=$target.body
	awk '
		NR == 1 {
			if ($0 != "mode: set") {
				printf "unexpected coverage mode in %s: %s\n", FILENAME, $0 > "/dev/stderr"
				exit 1
			}
			next
		}
		{
			region = $1
			statements[region] = $2
			count = $3 + 0
			if (!(region in covered) || count > covered[region]) covered[region] = count
		}
		END {
			for (region in statements) printf "%s %s %d\n", region, statements[region], covered[region]
		}
	' "$source" | LC_ALL=C sort -k1,1 >"$body"
	{
		printf 'mode: set\n'
		cat "$body"
	} >"$target"
	rm -f "$body"
}

work=$(mktemp -d "${TMPDIR:-/tmp}/kicadai-coverage-compare.XXXXXX")
trap 'rm -rf "$work"' EXIT
canonicalize "$1" "$work/expected"
canonicalize "$2" "$work/actual"

if ! cmp -s "$work/expected" "$work/actual"; then
	printf 'coverage profiles are not semantically identical\n' >&2
	diff -u "$work/expected" "$work/actual" >&2 || true
	exit 1
fi
printf 'coverage profiles are semantically identical\n'
