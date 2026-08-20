#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary_root=$(mktemp -d "${TMPDIR:-/tmp}/kicadai-release-repro.XXXXXX")
trap 'rm -rf "$temporary_root"' EXIT HUP INT TERM

version=${VERSION:-$(tr -d '[:space:]' < "$root/VERSION")}
commit=${COMMIT:-$(git -C "$root" rev-parse HEAD)}
build_date=${BUILD_DATE:-$(git -C "$root" show -s --format=%cI "$commit")}

for copy in a b; do
	(
		cd "$root"
		VERSION="$version" COMMIT="$commit" BUILD_DATE="$build_date" OUTPUT_DIR="$temporary_root/$copy" \
			./scripts/build-release.sh
	)
done

diff -u "$temporary_root/a/SHA256SUMS" "$temporary_root/b/SHA256SUMS"
for artifact in "$temporary_root/a"/*; do
	name=${artifact##*/}
	cmp "$artifact" "$temporary_root/b/$name"
done

printf 'release build is byte-reproducible for version %s at %s\n' "$version" "$commit"
