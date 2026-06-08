#!/usr/bin/env bash
set -euo pipefail
shopt -s nullglob

repo_url="https://gitlab.com/kicad/code/kicad"
api_base="https://gitlab.com/api/v4/projects/kicad%2Fcode%2Fkicad/repository"
project_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
version_file="$project_root/third_party/kicad/VERSION"
tmp_dir=""

cleanup() {
	if [ -n "$tmp_dir" ]; then
		rm -rf -- "$tmp_dir"
	fi
}

for command_name in curl jq mktemp sed tar; do
	if ! command -v "$command_name" >/dev/null 2>&1; then
		echo "$command_name is required but was not found in PATH." >&2
		exit 1
	fi
done

if [ "${KICAD_REF:-}" ]; then
	ref="$KICAD_REF"
elif [ -f "$version_file" ]; then
	ref="$(sed -n 's/^commit: //p' "$version_file")"
else
	ref=""
fi

if [ -z "$ref" ]; then
	echo "KICAD_REF is empty and no commit is recorded in $version_file." >&2
	exit 1
fi

source_ref="$ref"
tmp_dir="$(mktemp -d)"
trap cleanup EXIT INT TERM

archive="$tmp_dir/kicad-src.tar.gz"
commit_json="$tmp_dir/commit.json"
extract_dir="$tmp_dir/extract"
tree_json="$tmp_dir/tree.json"
vendor_tmp="$tmp_dir/vendor"
version_tmp="$tmp_dir/VERSION"

curl -fsSL \
	--get \
	--data-urlencode "ref_name=$source_ref" \
	--data-urlencode "per_page=1" \
	-o "$commit_json" \
	"$api_base/commits"

ref="$(jq -r '.[0].id // empty' "$commit_json")"

if [ -z "$ref" ]; then
	echo "Could not resolve KiCad ref '$source_ref' to a commit SHA." >&2
	exit 1
fi

proto_archive_url="$api_base/archive.tar.gz?sha=$ref&path=api/proto"

mkdir -p "$extract_dir"
mkdir -p "$vendor_tmp/api"

curl -fsSL \
	--get \
	--data-urlencode "sha=$ref" \
	--data-urlencode "path=api/proto" \
	-o "$archive" \
	"$api_base/archive.tar.gz"
tar -xzf "$archive" -C "$extract_dir"

src_dir=""

for candidate in "$extract_dir"/kicad-*; do
	if [ -d "$candidate/api/proto" ]; then
		src_dir="$candidate"
		break
	fi
done

if [ -z "$src_dir" ]; then
	echo "Downloaded archive did not contain the expected KiCad api/proto tree." >&2
	exit 1
fi

cp -R "$src_dir/api/proto" "$vendor_tmp/api/proto"

curl -fsSL \
	--get \
	--data-urlencode "ref=$ref" \
	--data-urlencode "per_page=100" \
	-o "$tree_json" \
	"$api_base/tree"

license_paths="$(jq -r '.[] | select(.path | startswith("LICENSE")) | .path' "$tree_json")"

if [ -z "$license_paths" ]; then
	echo "No KiCad LICENSE files were found in the upstream tree." >&2
	exit 1
fi

while IFS= read -r license_file; do
	encoded_license_file="$(jq -rn --arg value "$license_file" '$value | @uri')"

	case "$license_file" in
		*/*) mkdir -p "$vendor_tmp/${license_file%/*}" ;;
	esac

	curl -fsSL \
		--get \
		--data-urlencode "ref=$ref" \
		-o "$vendor_tmp/$license_file" \
		"$api_base/files/$encoded_license_file/raw"
done <<< "$license_paths"

encoded_authors_file="$(jq -rn --arg value "AUTHORS.txt" '$value | @uri')"

curl -fsSL \
	--get \
	--data-urlencode "ref=$ref" \
	-o "$vendor_tmp/AUTHORS.txt" \
	"$api_base/files/$encoded_authors_file/raw"

cat > "$version_tmp" <<EOF
source: KiCad Source Code
repository: $repo_url
source_ref: $source_ref
commit: $ref
archive: $proto_archive_url
vendored_paths:
  - api/proto
  - LICENSE*
  - AUTHORS.txt
notes: Only the KiCad IPC API protobuf definitions and attribution files are vendored.
EOF

dest_dir="$project_root/third_party/kicad"

rm -rf "$dest_dir/api/proto"
mkdir -p "$dest_dir/api"
cp -R "$vendor_tmp/api/proto" "$dest_dir/api/proto"

rm -f "$dest_dir"/LICENSE* "$dest_dir/AUTHORS.txt"
license_files=("$vendor_tmp"/LICENSE*)

if [ "${#license_files[@]}" -eq 0 ]; then
	echo "No KiCad LICENSE files were staged for copying." >&2
	exit 1
fi

cp "${license_files[@]}" "$dest_dir/"
cp "$vendor_tmp/AUTHORS.txt" "$dest_dir/AUTHORS.txt"
mv "$version_tmp" "$version_file"
