#!/bin/sh
set -eu

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
work_root=${PROMOTION_ROOT:-"$repo_root/.tmp/clean-checkout-promotion"}
cache_root=${PROMOTION_CACHE_DIR:-"$repo_root/.cache/kicadai-promotion-toolchain"}
matrix_path=${PROMOTION_MATRIX:-"$repo_root/testdata/external-review-mitigation/matrix.json"}
scenario_timeout=${PROMOTION_SCENARIO_TIMEOUT:-20m}

case "$work_root" in
	/*) ;;
	*) work_root="$repo_root/$work_root" ;;
esac
case "$cache_root" in
	/*) ;;
	*) cache_root="$repo_root/$cache_root" ;;
esac
case "$matrix_path" in
	/*) ;;
	*) matrix_path="$repo_root/$matrix_path" ;;
esac

if [ -e "$work_root" ]; then
	printf 'promotion output already exists: %s\n' "$work_root" >&2
	exit 1
fi
if [ ! -f "$matrix_path" ]; then
	printf 'promotion matrix is not a regular file: %s\n' "$matrix_path" >&2
	exit 1
fi
if [ -n "$(git -C "$repo_root" status --porcelain --untracked-files=normal)" ]; then
	printf 'clean-checkout promotion requires an unmodified checkout\n' >&2
	exit 1
fi

revision=$(git -C "$repo_root" rev-parse --verify HEAD)
bin_root="$work_root/bin"
run_root="$work_root/run"
bundle_root="$work_root/bundles"
promotion_cli="$bin_root/kicadai-promotion"
kicadai_cli="$bin_root/kicadai"

mkdir -p "$bin_root" "$bundle_root"
(
	cd "$repo_root"
	go build -o "$kicadai_cli" ./cmd/kicadai
	go build -o "$promotion_cli" ./cmd/kicadai-promotion
)

"$promotion_cli" promote \
	--repository "$repo_root" \
	--lock "$repo_root/toolchain/kicad-promotion.lock.json" \
	--matrix "$matrix_path" \
	--kicadai "$kicadai_cli" \
	--output "$run_root" \
	--bundle-output "$bundle_root" \
	--revision "$revision" \
	--bootstrap \
	--cache-dir "$cache_root" \
	--scenario-timeout "$scenario_timeout" \
	>"$work_root/promotion.json"

if [ "$(git -C "$repo_root" rev-parse --verify HEAD)" != "$revision" ] ||
	[ -n "$(git -C "$repo_root" status --porcelain --untracked-files=normal)" ]; then
	printf 'checkout changed during promotion; refusing to publish its bundle\n' >&2
	exit 1
fi

bundle_path=
bundle_count=0
for candidate in "$bundle_root"/sha256-*; do
	if [ ! -d "$candidate" ]; then
		continue
	fi
	bundle_path=$candidate
	bundle_count=$((bundle_count + 1))
done
if [ "$bundle_count" -ne 1 ]; then
	printf 'expected one content-addressed bundle, found %s\n' "$bundle_count" >&2
	exit 1
fi

"$promotion_cli" verify --bundle "$bundle_path" --receipt >"$work_root/verification.json"
printf '%s\n' "$bundle_path" >"$work_root/bundle-path.txt"
printf 'verified promotion bundle: %s\n' "$bundle_path"
