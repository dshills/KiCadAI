# V22 performance observations

These are read-only observations of the original frozen process at source
`d3b6088fbaa4f318b5685d62c4fc83eeba3eb4a2`. The original 24-case/two-replay
evaluation completed successfully on 2026-09-09. This was not a controlled
benchmark. No source, search limit, environment variable, or worker count was
changed in response to these measurements.

## Complete-run measurements

The authenticated 48 `METRICS.json` files and retained native
[`PROCESS_METRICS.txt`](../../../internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/PROCESS_METRICS.txt)
report:

| Measurement | Observed total |
| --- | ---: |
| Process elapsed time | 17,466.23 s (4 h 51 min 6.23 s) |
| Process user CPU time | 50,883.32 s |
| Process system CPU time | 759.18 s |
| Synthesis elapsed time, summed over 48 replays | 16,243.890083750 s |
| Outer canonical hashing elapsed time, summed | 1,139.542973665 s |
| Installed-KiCad promotion elapsed time, summed | 31.714079374 s |
| Canonical synthesis bytes hashed | 121,026,790,256 bytes |
| Synthesis spool bytes written | 0 bytes |
| Native maximum resident set size | 48,518,250,496 bytes |
| Native peak memory footprint, separately reported | 81,925,367,600 bytes |
| Retained publication, including checksum manifest | 154 files; 488,885 bytes |

The memory rows preserve the native tool's distinct labels and reported values;
they must not be conflated with each other or the earlier `ps`/sample snapshots.
Stage sums exclude other process work and are not the process wall time. The
publication inventory is independently checked and its checksum manifest is
pinned in the audit source. These compact records authenticate the full replay
identities and selected repair evidence; they are **not lossless compression**
of all numerical reports. Bulky generated KiCad projects and local validation
logs are retained outside this compact publication.

Zero synthesis spooling did not eliminate large in-memory evidence. The final
resource measurements strengthen the case for profiling retained report
ownership, repeated model/catalog work, and hashing before adding case-level
concurrency. They do not establish a comparative speed or memory improvement.

## Completed-prefix timing and evidence volume

At the 2026-09-09 14:46 CDT observation, the first twelve cases had completed
both replays and case 013 was still running its first replay. The 24 completed
`METRICS.json` records totaled:

| Measurement | Observed total |
| --- | ---: |
| Synthesis elapsed time | 3,609.037328918 s |
| Outer canonical replay hashing | 87.085100623 s |
| Installed-KiCad promotion | 31.714079374 s |
| Canonical synthesis bytes hashed | 10,283,291,474 bytes |
| Synthesis spool bytes written | 0 bytes |

These totals exclude the in-progress case. They are sums of measured replay
stages, not the process's total wall time. Internal hashing performed during
synthesis belongs to the synthesis measurement; it is not included in the
outer-hashing row. Earlier local preservation/release checks ran concurrently,
so the timings cannot establish a speedup over V21 or isolate hardware effects.
Those validation jobs had finished before the third CPU sample below.

Removing the synthesis disk destination preserves the complete canonical replay
digest and count. It does not remove the solver reports retained in memory,
compress those reports losslessly, or establish a memory improvement.

Case 013's first replay subsequently completed. Its retained metrics report
1,956.779038291 seconds of synthesis, 34.797131875 seconds of outer hashing,
4,440,086,410 canonical synthesis bytes, and zero synthesis-spool bytes. The
second replay's clean-root marker was created at 14:50:01 CDT. This is an
unselected predecessor-path case; it does not execute the new V22 electrical
repair. These measurements describe one completed replay, not an authenticated
two-replay case outcome or an additional pass.

## Short native CPU samples

All three samples used the same live evaluator process (PID 39446), launched
at 13:14:33.886 CDT. Each sampled for three seconds at a 10 ms interval.
The footprint values below preserve `/usr/bin/sample`'s display units and are
not interchangeable with `ps` RSS or `/usr/bin/time` maximum RSS.

| Local sample time | Physical footprint | Process peak reported so far |
| --- | ---: | ---: |
| 13:34:16.610 | 3.7G | 4.1G |
| 14:13:00.292 | 3.4G | 7.6G |
| 14:46:11.357 | 7.5G | 10.2G |

Observed stacks include nonlinear residual/MNA/thermal evaluation in the first
sample; simulation-harness and catalog/model-admission work in the second; and
the inherited V20 causal repair/value search, graph canonicalization, repeated
model-provenance normalization/lookup, allocation, and garbage collection in the
third. The third sample is from an unselected case using the unchanged V20
path, not the new electrical continuation. A nearby `ps` snapshot at 14:45:51
reported 355.1% CPU and cumulative CPU time 279:35.30 after 1:31:18 elapsed.
This establishes active computation, not useful progress per unit of CPU.

These brief samples cannot assign whole-run hotspot percentages or prove which
allocation dominates retained memory. They do justify a separate profiling
follow-up: measure heap ownership and repeated immutable catalog/admission work
before choosing caching, report-lifetime changes, or additional concurrency.
Increasing case concurrency without measuring per-worker peak memory could
increase memory pressure. No such optimization is part of the frozen run.

## Retained raw observations

The machine-local raw samples are retained outside Git; the digests below bind
the exact observations used here. They are not final publication artifacts or
a substitute for a reproducible benchmark.

| Local file | SHA-256 |
| --- | --- |
| `/tmp/kicadai-v22-live-sample-1.txt` | `998558c8f3cfa4368d812867c81a9a8f637e41c4287f1f2ec34d801d24c49f58` |
| `/tmp/kicadai-v22-live-sample-2.txt` | `cfd2978065d14cc34df200b0174d276628e312f943d54dd8edb8d59fec069232` |
| `/tmp/kicadai-v22-live-sample-3.txt` | `3e0991da865856b8951f9fbe2f31c93712a6ab572b264f748e0a0ee830e40861` |
