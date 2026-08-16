#!/usr/bin/env bash

set -euo pipefail

repository_root="$(git rev-parse --show-toplevel)"
temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/kicadai-go-format.XXXXXX")"
format_paths="$temporary_root/format-paths"
format_diff="$temporary_root/gofmt.diff"
trap 'rm -rf "$temporary_root"' EXIT

cd "$repository_root"
: >"$format_paths"

# Check every Go file introduced or modified in the active change. This keeps
# immutable historical files byte-identical without allowing new or edited Go
# source to bypass formatting. The index, worktree, and untracked sets are
# included for local use; a clean CI checkout falls back to the committed
# HEAD^..HEAD change and therefore requires checkout depth two.
git diff --cached --name-only -z --diff-filter=ACMR -- '*.go' >>"$format_paths"
git diff --name-only -z --diff-filter=ACMR -- '*.go' >>"$format_paths"
git ls-files --others --exclude-standard -z -- '*.go' >>"$format_paths"

format_base="${KICADAI_FORMAT_BASE:-HEAD^}"
if [[ "$format_base" =~ ^0+$ ]]; then
	format_base="HEAD^"
fi
if ! git rev-parse --verify "$format_base^{commit}" >/dev/null 2>&1; then
	printf 'Go formatting base is unavailable: %s\n' "$format_base" >&2
	exit 2
fi
git diff --name-only -z --diff-filter=ACMR "$format_base" HEAD -- '*.go' >>"$format_paths"

: >"$format_diff"
if [[ -s "$format_paths" ]]; then
	set +e
	xargs -0 gofmt -d <"$format_paths" >"$format_diff"
	format_status=$?
	set -e
	if [[ "$format_status" -ne 0 && ! -s "$format_diff" ]]; then
		printf 'gofmt failed with status %d\n' "$format_status" >&2
		exit "$format_status"
	fi
fi
if [[ -s "$format_diff" ]]; then
	cat "$format_diff"
	exit 1
fi
