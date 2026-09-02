#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
	printf 'usage: %s <shard-root> <summary-tsv>\n' "$0" >&2
	exit 2
fi

shard_root=$1
summary=$2
mkdir -p "$(dirname -- "$summary")"
printf 'shard\tcache_hit\twall_seconds\tuser_seconds\tsystem_seconds\tmax_rss_kb\tcpu_percent\n' >"$summary"

find "$shard_root" -mindepth 1 -maxdepth 1 -type d -print | LC_ALL=C sort |
while IFS= read -r shard; do
	resource="$shard/resource.txt"
	cache_hit=$(tr -d '[:space:]' <"$shard/cache-hit.txt")
	awk -v shard="$(basename -- "$shard")" -v cache_hit="$cache_hit" '
		BEGIN { wall = 0; user = 0; system_seconds = 0; rss = 0 }
		$1 == "wall_seconds" { wall = $2 }
		/^[[:space:]]*User time \(seconds\):/ { user = $NF }
		/^[[:space:]]*System time \(seconds\):/ { system_seconds = $NF }
		/^[[:space:]]*Maximum resident set size \(kbytes\):/ { rss = $NF }
		$1 == "user" { user = $2 }
		$1 == "sys" { system_seconds = $2 }
		/maximum resident set size/ && $0 !~ /kbytes/ { rss = $1 / 1024 }
		END {
			cpu = wall > 0 ? ((user + system_seconds) * 100 / wall) : 0
			printf "%s\t%s\t%.3f\t%.3f\t%.3f\t%.0f\t%.1f\n", shard, cache_hit, wall, user, system_seconds, rss, cpu
		}
	' "$resource" >>"$summary"
done

printf 'coverage profile summary written to %s\n' "$summary"
