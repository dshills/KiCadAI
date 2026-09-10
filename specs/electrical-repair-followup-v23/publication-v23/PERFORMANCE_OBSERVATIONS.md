# V23 native run and retained evidence measurements

These are observations from the one original frozen 24-case / two-replay run,
not a controlled benchmark or justification for changing execution budgets.
Source: `16013c7d3f4e44ae8cfbda18be655c853ef8f111`; Go 1.26.8,
darwin/arm64, CGO=1; installed KiCad 10.0.3. The original launcher's completion
marker and post-run assessment passed without restarting numerical work.

## Native process

| Measurement | Recorded value |
| --- | ---: |
| Real elapsed | 17,843.38 s (4 h 57 m 23.38 s) |
| User CPU | 51,097.22 s |
| System CPU | 850.64 s |
| Maximum resident set size | 47,085,027,328 bytes |
| Peak memory footprint | 75,661,812,976 bytes |
| Swaps reported by native timing | 0 |

Maximum RSS and peak memory footprint are different macOS measurements. Neither
is the compact publication size; zero reported swaps is not proof that the host
experienced no memory pressure. Memory requirements remain substantial. CPU
seconds are not wall-clock seconds. The `/usr/bin/time -l` output is retained
unaltered as `PROCESS_METRICS.txt` in the checksum-pinned publication.

## Per-replay measurements and storage

Across all 48 replays, recorded synthesis time sums to 16,619.064967959 s,
canonical hashing to 1,133.664742584 s, and physical promotion to 28.969834583 s.
These stage measurements do not include every loader, report, and process
overhead and therefore are not a complete decomposition of native elapsed time.

The run streamed 121,026,790,256 canonical synthesis bytes through hashing and
wrote zero synthesis spools. The retained compact publication is 521,456 bytes
across 154 files, including `SHA256SUMS`. This is a diagnostic/evidence
projection, not lossless storage of all canonical synthesis bytes or numerical
waveforms. The existing native projects and their scratch evidence remain at
`/tmp/kicadai-v23-public-evaluation-1/cases` outside Git; their storage is not
included in the compact publication size.

The canonical byte volume equals V22's recorded volume. All full synthesis
replay hashes also match V22, but the selected V23 repair sidecars separately
bind the new solver implementation and changed trial-1 evaluation in case 018.
Matching synthesis bytes do not erase that sidecar change or establish an
additional passing circuit.

Clean-checkout quality, coverage, release, and preservation checks overlapped
the run. Comparing its 4 h 57 m elapsed time to V22's 4 h 51 m therefore does
not establish a controlled speedup or slowdown. No search budget, concurrency,
solver tolerance, or safety guard was tuned from these timings.
