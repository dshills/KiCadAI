#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
shard_root=${COVER_SHARD_DIR:-"$root/.coverage/shards"}
merged_profile=${COVER_PROFILE:-"$root/.coverage/kicadai.cover.out"}
profile_summary=${COVER_PROFILE_SUMMARY:-"$root/.coverage/profile.tsv"}
general_total=${COVERAGE_GENERAL_SHARDS:-4}
open_total=${COVERAGE_OPEN_TOPOLOGY_SHARDS:-6}
max_workers=${COVERAGE_MAX_WORKERS:-4}

case "$max_workers" in
	''|*[!0-9]*) printf 'COVERAGE_MAX_WORKERS must be a positive integer\n' >&2; exit 2 ;;
esac
if [ "$max_workers" -lt 1 ]; then
	printf 'COVERAGE_MAX_WORKERS must be a positive integer\n' >&2
	exit 2
fi

rm -rf "$shard_root"
mkdir -p "$shard_root"

jobs=()
max=$general_total
if [ "$open_total" -gt "$max" ]; then max=$open_total; fi
for ((i = 0; i < max; i++)); do
	if [ "$i" -lt "$open_total" ]; then jobs+=("open-topology:$i:$open_total"); fi
	if [ "$i" -lt "$general_total" ]; then jobs+=("general:$i:$general_total"); fi
done

run_batch() {
	local start=$1
	local stop=$2
	local pids=()
	local status=0
	local spec group index total output pid position
	for ((position = start; position < stop; position++)); do
		spec=${jobs[$position]}
		IFS=: read -r group index total <<<"$spec"
		output="$shard_root/$group-$index-of-$total"
		"$root/scripts/run-coverage-shard.sh" "$group" "$index" "$total" "$output" &
		pids+=("$!")
	done
	for pid in "${pids[@]}"; do
		if ! wait "$pid"; then status=1; fi
	done
	return "$status"
}

for ((start = 0; start < ${#jobs[@]}; start += max_workers)); do
	stop=$((start + max_workers))
	if [ "$stop" -gt "${#jobs[@]}" ]; then stop=${#jobs[@]}; fi
	run_batch "$start" "$stop"
done

"$root/scripts/merge-coverage-shards.sh" "$shard_root" "$merged_profile"
"$root/scripts/summarize-coverage-profile.sh" "$shard_root" "$profile_summary"
