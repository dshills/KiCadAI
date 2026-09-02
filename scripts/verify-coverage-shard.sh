#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
	printf 'usage: %s <coverage-shard-directory>\n' "$0" >&2
	exit 2
fi

dir=$1
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

field() {
	awk -F '\t' -v name="$1" '$1 == name {print $2}' "$dir/artifact.manifest"
}

for required in artifact.manifest artifact.manifest.sha256 profile.out tests.txt packages.txt resource.txt cache-hit.txt; do
	if [ ! -f "$dir/$required" ]; then
		printf 'coverage shard is missing %s: %s\n' "$required" "$dir" >&2
		exit 1
	fi
done

cache_hit=$(tr -d '[:space:]' <"$dir/cache-hit.txt")
if [ "$cache_hit" != true ] && [ "$cache_hit" != false ]; then
	printf 'coverage shard has invalid cache-hit marker: %s\n' "$dir" >&2
	exit 1
fi

expected_manifest=$(tr -d '[:space:]' <"$dir/artifact.manifest.sha256")
actual_manifest=$(sha256_file "$dir/artifact.manifest")
if [ "$actual_manifest" != "$expected_manifest" ]; then
	printf 'coverage shard manifest digest mismatch: %s\n' "$dir" >&2
	exit 1
fi

if [ "$(field schema)" != "kicadai.coverage-shard.v1" ]; then
	printf 'unsupported coverage shard schema: %s\n' "$dir" >&2
	exit 1
fi
if [ "$(field profile_sha256)" != "$(sha256_file "$dir/profile.out")" ] ||
	[ "$(field tests_sha256)" != "$(sha256_file "$dir/tests.txt")" ] ||
	[ "$(field packages_sha256)" != "$(sha256_file "$dir/packages.txt")" ] ||
	[ "$(field resource_sha256)" != "$(sha256_file "$dir/resource.txt")" ]; then
	printf 'coverage shard payload digest mismatch: %s\n' "$dir" >&2
	exit 1
fi
if [ "$(sed -n '1p' "$dir/profile.out")" != "mode: set" ]; then
	printf 'coverage shard does not use set mode: %s\n' "$dir" >&2
	exit 1
fi
if ! LC_ALL=C sort -c -u "$dir/tests.txt"; then
	printf 'coverage shard test inventory is not canonical: %s\n' "$dir" >&2
	exit 1
fi
if ! LC_ALL=C sort -c -u "$dir/packages.txt"; then
	printf 'coverage shard package inventory is not canonical: %s\n' "$dir" >&2
	exit 1
fi

if [ "${COVERAGE_VERIFY_CURRENT_SOURCE:-1}" = 1 ]; then
	current=$("$root/scripts/coverage-source-fingerprint.sh")
	if [ "$(field source_fingerprint)" != "$current" ]; then
		printf 'coverage shard source fingerprint is stale: %s\n' "$dir" >&2
		exit 1
	fi
fi
