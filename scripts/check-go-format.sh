#!/usr/bin/env bash

set -euo pipefail

repository_root="$(git rev-parse --show-toplevel)"
seal_base="HEAD^"
temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/kicadai-go-format.XXXXXX")"
sealed_paths="$temporary_root/sealed-paths"
format_paths="$temporary_root/format-paths"
format_diff="$temporary_root/gofmt.diff"
trap 'rm -rf "$temporary_root"' EXIT

sha256_stream() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 | awk '{print $1}'
	elif command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 | awk '{print $NF}'
	else
		printf 'no SHA-256 implementation found (tried sha256sum, shasum, and openssl)\n' >&2
		return 127
	fi
}

cd "$repository_root"
if ! git rev-parse --verify "$seal_base" >/dev/null 2>&1; then
	printf 'Go formatting requires one parent commit to authenticate pre-existing sealed files\n' >&2
	exit 2
fi
: >"$sealed_paths"

# Historical protocol artifacts are immutable by full-byte SHA-256. A matching
# Go file that was already sealed in the parent commit is authenticated evidence
# rather than mutable source. Keep checking every other tracked Go file,
# including newly introduced manifest-bound files, and automatically resume
# checking a sealed path if its bytes stop matching the manifest entry.
while IFS= read -r manifest; do
	manifest_directory="$(dirname "$manifest")"
	while IFS= read -r line; do
		expected="${line:0:64}"
		separator="${line:64:2}"
		referenced="${line:66}"
		if [[ ! "$expected" =~ ^[0-9a-f]{64}$ ]] ||
			[[ "$separator" != "  " && "$separator" != " *" ]] ||
			[[ "$referenced" != *.go ]]; then
			continue
		fi
		if [[ ! -f "$manifest_directory/$referenced" ]]; then
			continue
		fi
		referenced_path="$manifest_directory/$referenced"
		resolved="$(cd "$(dirname "$referenced_path")" && printf '%s/%s' "$(pwd -P)" "$(basename "$referenced_path")")"
		case "$resolved" in
			"$repository_root"/*) ;;
			*) continue ;;
		esac
		repository_path="${resolved#"$repository_root"/}"
		if ! git ls-files --error-unmatch -- "$repository_path" >/dev/null 2>&1; then
			continue
		fi
		actual="$(sha256_stream <"$repository_path")"
		if ! git cat-file -e "$seal_base:$repository_path" 2>/dev/null; then
			continue
		fi
		parent_actual="$(git show "$seal_base:$repository_path" | sha256_stream)"
		if [[ "$actual" == "$expected" && "$parent_actual" == "$expected" ]]; then
			printf '%s\n' "$repository_path" >>"$sealed_paths"
		fi
	done <"$manifest"
done < <(git ls-files '*.sha256')

LC_ALL=C sort -u -o "$sealed_paths" "$sealed_paths"
: >"$format_paths"
while IFS= read -r -d '' path; do
	if ! grep -Fqx -- "$path" "$sealed_paths"; then
		printf '%s\0' "$path" >>"$format_paths"
	fi
done < <(git ls-files -z '*.go')

: >"$format_diff"
if [[ -s "$format_paths" ]]; then
	xargs -0 gofmt -d <"$format_paths" >"$format_diff"
fi
if [[ -s "$format_diff" ]]; then
	cat "$format_diff"
	exit 1
fi
