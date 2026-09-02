#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if ! command -v git >/dev/null 2>&1; then
	printf 'coverage proof fingerprint requires Git\n' >&2
	exit 1
fi
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	printf 'coverage proof fingerprint requires a Git working tree\n' >&2
	exit 1
fi

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

paths=$(mktemp "${TMPDIR:-/tmp}/kicadai-coverage-paths.XXXXXX")
hashes=$(mktemp "${TMPDIR:-/tmp}/kicadai-coverage-hashes.XXXXXX")
manifest=$(mktemp "${TMPDIR:-/tmp}/kicadai-coverage-source.XXXXXX")
trap 'rm -f "$paths" "$hashes" "$manifest"' EXIT

# Include tracked and relevant untracked repository files. Ignored build, cache,
# coverage, and promotion outputs are intentionally absent. Hashing the whole
# source tree is conservative: any change invalidates every cached proof.
git ls-files --cached --others --exclude-standard | LC_ALL=C sort >"$paths"
git hash-object --stdin-paths <"$paths" >"$hashes"
if [ "$(wc -l <"$paths" | tr -d ' ')" -ne "$(wc -l <"$hashes" | tr -d ' ')" ]; then
	printf 'coverage fingerprint did not hash every source path\n' >&2
	exit 1
fi
paste "$hashes" "$paths" >"$manifest"
# Preserve tracked executable/type changes as part of the proof identity while
# hashing working-tree bytes above so unstaged edits also invalidate the cache.
git ls-files -s | LC_ALL=C sort >>"$manifest"

if [ ! -s "$manifest" ]; then
	printf 'coverage fingerprint input set is empty\n' >&2
	exit 1
fi

sha256_file "$manifest"
