#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 4 ]; then
	printf 'usage: %s <general|open-topology> <zero-based-index> <total> <output-directory>\n' "$0" >&2
	exit 2
fi

group=$1
index=$2
total=$3
output=$4
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
package_open=kicadai/internal/opentopologysynthesis
timeout=${GO_TEST_TIMEOUT:-40m}
skip=${COVER_TEST_SKIP:-}
cache_root=${COVERAGE_PROOF_CACHE:-"$root/.cache/coverage-proofs"}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

sha256_text() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum | awk '{print $1}'
	else
		shasum -a 256 | awk '{print $1}'
	fi
}

if [ -e "$output" ]; then
	printf 'coverage shard output already exists: %s\n' "$output" >&2
	exit 1
fi

source_fingerprint=$("$root/scripts/coverage-source-fingerprint.sh")
environment_fingerprint=$(
	{
		go version
		go env GOOS GOARCH CGO_ENABLED GOFLAGS
		env | LC_ALL=C sort | awk -F= '$1 ~ /^(KICADAI_|GOMAXPROCS$)/ {print}'
		printf 'timeout=%s\nskip=%s\ngroup=%s\nindex=%s\ntotal=%s\n' "$timeout" "$skip" "$group" "$index" "$total"
	} | sha256_text
)
go_identity=$(go version | tr ' /' '__')
cache_key="v1-${go_identity}-${group}-${index}-of-${total}-${source_fingerprint}-${environment_fingerprint}"
cache_dir="$cache_root/$cache_key"

mkdir -p "$cache_root"
if [ -d "$cache_dir" ]; then
	COVERAGE_VERIFY_CURRENT_SOURCE=1 "$root/scripts/verify-coverage-shard.sh" "$cache_dir"
	mkdir -p "$output"
	cp -R "$cache_dir/." "$output/"
	printf 'true\n' >"$output/cache-hit.txt"
	printf 'coverage proof cache hit: %s\n' "$group-$index-of-$total"
	exit 0
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/kicadai-coverage-shard.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$output"

plan="$work/plan.txt"
"$root/scripts/coverage-shard-plan.sh" "$group" "$index" "$total" >"$plan"
if [ ! -s "$plan" ]; then
	printf 'coverage shard plan is empty: %s-%s-of-%s\n' "$group" "$index" "$total" >&2
	exit 1
fi

list_tests() {
	local package=$1
	go test -list '^(Test|Example|Fuzz)' "$package" |
		awk -v package="$package" -v skip="$skip" '
			/^(Test|Example|Fuzz)[[:alnum:]_]*$/ {
				if (skip == "" || $0 !~ skip) print package "\t" $0
			}
		'
}

packages=()
tests=()
case "$group" in
	general)
		while IFS= read -r package; do
			packages+=("$package")
			list_tests "$package" >>"$work/tests.unsorted"
		done <"$plan"
		;;
	open-topology)
		packages+=("$package_open")
		while IFS= read -r test_name; do
			if [ -n "$skip" ] && [[ "$test_name" =~ $skip ]]; then
				continue
			fi
			tests+=("$test_name")
			printf '%s\t%s\n' "$package_open" "$test_name" >>"$work/tests.unsorted"
		done <"$plan"
		if [ "${#tests[@]}" -eq 0 ]; then
			printf 'coverage open-topology shard has no runnable tests\n' >&2
			exit 1
		fi
		;;
	*)
		printf 'unknown coverage shard group: %s\n' "$group" >&2
		exit 2
		;;
esac

printf '%s\n' "${packages[@]}" | LC_ALL=C sort -u >"$output/packages.txt"
if [ -f "$work/tests.unsorted" ]; then
	LC_ALL=C sort -u "$work/tests.unsorted" >"$output/tests.txt"
else
	: >"$output/tests.txt"
fi

command=(go test -short -count=1 -p=1 -timeout "$timeout" -covermode=set -coverprofile="$output/profile.out")
if [ -n "$skip" ]; then
	command+=(-skip "$skip")
fi
if [ "$group" = open-topology ]; then
	regex="^($(IFS='|'; printf '%s' "${tests[*]}"))$"
	command+=(-run "$regex")
fi
command+=("${packages[@]}")

printf 'coverage shard start: %s packages=%s tests=%s\n' "$group-$index-of-$total" "${#packages[@]}" "$(wc -l <"$output/tests.txt" | tr -d ' ')"
start=$(date +%s)
case "$(uname -s)" in
	Linux)
		/usr/bin/time -v -o "$output/resource.txt" "${command[@]}"
		;;
	Darwin)
		# Extended -l metrics query kern.clockrate and fail in sandboxed macOS
		# runners. Portable timing still records wall, user, and system CPU.
		/usr/bin/time -p -o "$output/resource.txt" "${command[@]}"
		;;
	*)
		/usr/bin/time -p -o "$output/resource.txt" "${command[@]}"
		;;
esac
end=$(date +%s)
printf 'wall_seconds\t%s\n' "$((end - start))" >>"$output/resource.txt"
printf 'false\n' >"$output/cache-hit.txt"

{
	printf 'schema\tkicadai.coverage-shard.v1\n'
	printf 'source_fingerprint\t%s\n' "$source_fingerprint"
	printf 'environment_fingerprint\t%s\n' "$environment_fingerprint"
	printf 'go_version\t%s\n' "$(go version | tr '\t' ' ')"
	printf 'shard_id\t%s\n' "$group-$index-of-$total"
	printf 'profile_sha256\t%s\n' "$(sha256_file "$output/profile.out")"
	printf 'tests_sha256\t%s\n' "$(sha256_file "$output/tests.txt")"
	printf 'packages_sha256\t%s\n' "$(sha256_file "$output/packages.txt")"
	printf 'resource_sha256\t%s\n' "$(sha256_file "$output/resource.txt")"
} >"$output/artifact.manifest"
sha256_file "$output/artifact.manifest" >"$output/artifact.manifest.sha256"

COVERAGE_VERIFY_CURRENT_SOURCE=1 "$root/scripts/verify-coverage-shard.sh" "$output"

cache_temp=$(mktemp -d "$cache_root/.tmp.XXXXXX")
cp -R "$output/." "$cache_temp/"
if [ -d "$cache_dir" ]; then
	COVERAGE_VERIFY_CURRENT_SOURCE=1 "$root/scripts/verify-coverage-shard.sh" "$cache_dir"
	rm -rf "$cache_temp"
else
	mv "$cache_temp" "$cache_dir"
fi
printf 'coverage shard complete: %s wall=%ss\n' "$group-$index-of-$total" "$((end - start))"
