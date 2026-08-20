#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=${VERSION:-$(tr -d '[:space:]' < "$root/VERSION")}
version=${version#v}
commit=${COMMIT:-$(git -C "$root" rev-parse HEAD)}
build_date=${BUILD_DATE:-$(git -C "$root" show -s --format=%cI "$commit")}
output_dir=${OUTPUT_DIR:-"$root/dist"}

if [ "${ALLOW_DIRTY_RELEASE:-0}" != 1 ] && [ -n "$(git -C "$root" status --porcelain --untracked-files=normal)" ]; then
	printf 'release builds require a clean repository; ALLOW_DIRTY_RELEASE=1 is for pre-commit verification only\n' >&2
	exit 2
fi

directory_is_empty() {
	for entry in "$1"/* "$1"/.[!.]* "$1"/..?*; do
		if [ -e "$entry" ] || [ -L "$entry" ]; then
			return 1
		fi
	done
	return 0
}

if ! printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$'; then
	printf 'invalid release version: %s\n' "$version" >&2
	exit 2
fi
if ! printf '%s\n' "$commit" | grep -Eq '^[0-9a-f]{40}$'; then
	printf 'release commit must be a full lowercase Git SHA-1: %s\n' "$commit" >&2
	exit 2
fi
if [ -e "$output_dir" ]; then
	if [ ! -d "$output_dir" ] || ! directory_is_empty "$output_dir"; then
		printf 'release output directory must be absent or empty: %s\n' "$output_dir" >&2
		exit 2
	fi
fi

# The guard above makes stale artifacts impossible; never clean a caller-owned
# directory implicitly.
mkdir -p "$output_dir"

ldflags="-s -w -X kicadai/internal/buildinfo.Version=$version -X kicadai/internal/buildinfo.Commit=$commit -X kicadai/internal/buildinfo.BuildDate=$build_date"
targets='darwin/amd64 darwin/arm64 linux/amd64 linux/arm64'
for target in $targets; do
	goos=${target%/*}
	goarch=${target#*/}
	artifact="$output_dir/kicadai_v${version}_${goos}_${goarch}"
	printf 'building %s/%s\n' "$goos" "$goarch"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
		go build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$artifact" ./cmd/kicadai
	chmod 0755 "$artifact"
done

go_version=$(go version | awk '{ print $3 }')
{
cat <<EOF
{
  "schema": "kicadai.release-manifest.v1",
  "version": "$version",
  "commit": "$commit",
  "build_date": "$build_date",
  "go_version": "$go_version",
  "cgo_enabled": false,
  "artifacts": [
EOF
	first=true
	for target in $targets; do
		goos=${target%/*}
		goarch=${target#*/}
		if [ "$first" = true ]; then
			first=false
		else
			printf ',\n'
		fi
		printf '    "kicadai_v%s_%s_%s"' "$version" "$goos" "$goarch"
	done
	printf '\n  ]\n}\n'
} > "$output_dir/RELEASE_MANIFEST.json"

checksum_hash() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

(
	for artifact in "$output_dir"/kicadai_v* "$output_dir/RELEASE_MANIFEST.json"; do
		if [ ! -f "$artifact" ]; then
			printf 'expected release artifact is missing: %s\n' "$artifact" >&2
			exit 1
		fi
		printf '%s  %s\n' "$(checksum_hash "$artifact")" "${artifact##*/}"
	done
) | LC_ALL=C sort -k2 > "$output_dir/SHA256SUMS"

printf 'release artifacts written to %s\n' "$output_dir"
