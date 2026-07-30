#!/bin/bash
set -eu

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
work_root=${COMPONENT_ONBOARDING_PROMOTION_ROOT:-"$repo_root/.tmp/component-onboarding-promotion"}
go_cache=${GOCACHE:-"$repo_root/.cache/go/build"}
module_cache=${GOMODCACHE:-"$repo_root/.cache/go/mod"}

case "$work_root" in
	/*) ;;
	*) work_root="$repo_root/$work_root" ;;
esac

if [ -e "$work_root" ]; then
	printf 'component-onboarding promotion output already exists: %s\n' "$work_root" >&2
	exit 1
fi
for variable in KICADAI_KICAD_CLI KICADAI_SYMBOLS_ROOT KICADAI_FOOTPRINTS_ROOT; do
	eval "value=\${$variable:-}"
	if [ -z "$value" ]; then
		printf '%s is required for component-onboarding promotion\n' "$variable" >&2
		exit 2
	fi
done

mkdir -p "$work_root" "$go_cache" "$module_cache"
file_list="$work_root/source-files.zlist"
(
	cd "$repo_root"
	git ls-files -z --cached --others --exclude-standard | LC_ALL=C sort -z >"$file_list"
)
if [ ! -s "$file_list" ]; then
	printf 'source snapshot is empty\n' >&2
	exit 1
fi

snapshot_hash=$(
	cd "$repo_root"
	while IFS= read -r -d '' path; do
		if [ -f "$path" ]; then
			digest=$(shasum -a 256 -- "$path" | awk '{print $1}')
			printf '%s\0%s\0' "$path" "$digest"
		fi
	done <"$file_list" | shasum -a 256 | awk '{print $1}'
)

for root_name in source-a source-b; do
	source_root="$work_root/$root_name"
	artifact_root="$work_root/artifacts-${root_name#source-}"
	mkdir -p "$source_root" "$artifact_root"
	(
		cd "$repo_root"
		tar --null -cf - -T "$file_list"
	) | (
		cd "$source_root"
		tar -xf -
	)
	(
		cd "$source_root"
		KICADAI_COMPONENT_ONBOARDING_ARTIFACT_DIR="$artifact_root" \
		GOCACHE="$go_cache" \
		GOMODCACHE="$module_cache" \
		go test ./internal/componentonboarding \
			-run '^TestHeldOutCorpusOptionalKiCadPromotion$' \
			-count=1 -timeout 20m -v
	) >"$work_root/$root_name.test.log" 2>&1
	if ! grep -Fq -- "--- PASS: TestHeldOutCorpusOptionalKiCadPromotion " "$work_root/$root_name.test.log"; then
		printf 'installed-KiCad promotion test did not run and pass in %s\n' "$root_name" >&2
		cat "$work_root/$root_name.test.log" >&2
		exit 1
	fi
	manifest="$work_root/$root_name.manifest.sha256"
	(
		cd "$artifact_root"
		find . -type f \( \( \
			! -path '*/.evidence/*' \( \
				-name '*.kicad_pcb' -o \
				-name '*.kicad_pro' -o \
				-name '*.kicad_sch' \
			\) \) -o \
			-path '*/simulation/report.json' \
		\) -print | LC_ALL=C sort | while IFS= read -r path; do
			shasum -a 256 "$path"
		done
	) >"$manifest"
	if [ ! -s "$manifest" ]; then
		printf 'promotion manifest is empty in %s\n' "$root_name" >&2
		exit 1
	fi
done

if ! cmp -s "$work_root/source-a.manifest.sha256" "$work_root/source-b.manifest.sha256"; then
	printf 'component-onboarding promotion differs across clean roots\n' >&2
	diff -u "$work_root/source-a.manifest.sha256" "$work_root/source-b.manifest.sha256" >&2 || true
	exit 1
fi

bundle_hash=$(
	{
		printf '%s  source-snapshot\n' "$snapshot_hash"
		cat "$work_root/source-a.manifest.sha256"
	} | shasum -a 256 | awk '{print $1}'
)
bundle_root="$work_root/bundles/sha256-$bundle_hash"
mkdir -p "$bundle_root"
cp "$file_list" "$bundle_root/source-files.zlist"
cp "$work_root/source-a.manifest.sha256" "$bundle_root/artifacts.sha256"
printf '%s\n' "$snapshot_hash" >"$bundle_root/source-snapshot.sha256"
printf '%s\n' "$bundle_hash" >"$bundle_root/bundle.sha256"
printf 'verified component-onboarding promotion bundle: %s\n' "$bundle_root"
