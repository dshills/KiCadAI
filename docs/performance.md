# Performance and Test Tiers

KiCadAI optimizes for deterministic evidence and bounded resource use before
raw throughput. Concurrency must not change candidate ordering, hashes,
diagnostics, generated KiCad bytes, or promotion results.

## Reproducible benchmarks

Run the representative processing benchmarks with:

```sh
make performance-report
make performance-scaling
```

`performance-report` covers routing, placement, project writing, project
comparison, transient simulation, resistor-network path discovery, and a
bounded powered-low-pass synthesis. Override `PERFORMANCE_BENCHTIME` and
`PERFORMANCE_COUNT` for a longer sample. `performance-scaling` runs the same
synthesis with worker limits 1, 2, 4, 8, and the host default.

The initial 2026-08-20 Apple M4 Pro profile established these reference points:

| Workload | Reference result |
|---|---:|
| Route a golden detour | 187 µs, 300 KB, 1,056 allocations |
| Place a moderate board | 13.2 ms, 18.0 MB, 36,393 allocations |
| Write a project directory | 30.7 ms, 131 KB, 3,022 allocations |
| Compare two minimal projects | 64.8 µs, 9.2 KB, 78 allocations |
| Evaluate a transient switch | 2.9 ms, 834 KB, 7,423 allocations |
| Synthesize the powered low-pass fixture | 1.10 s, 219 MB, 1.72 M allocations |

These are comparison baselines, not cross-machine pass/fail thresholds. CI
should detect correctness and determinism regressions; benchmark history should
identify performance regressions on comparable hardware.

CPU and allocation profiles of the previously slowest package identified MNA
system construction/cloning, nonlinear device result construction, topology
path discovery, plan cloning, and evidence serialization as the dominant costs.
The following bounded changes address those sources:

- evidence hashes stream canonical JSON directly into SHA-256 instead of first
  retaining another complete encoded document;
- indexed plans build resistor adjacency once, and bounded path queries reuse
  scratch state;
- noise-transfer solves reuse bounded MNA scratch storage;
- a shared runtime budget caps nested worker pools;
- promotion scenarios and release cross-builds run concurrently but publish in
  canonical order.

The resistor-path microbenchmark improved from approximately 13.3 µs, 13.8 KB,
and 139 allocations per uncached query to 2.8 µs and zero allocations per
cached query. Closed-loop state evidence now streams directly into SHA-256 and
has an explicit byte-compatibility test against the prior buffered
`encoding/json` representation.

## Fast, bounded, and exhaustive tests

Use the cheapest tier that answers the current question:

```sh
make test-fast       # ordinary edit/compile feedback
make test            # alias of the bounded local gate
make test-bounded    # bounded local and coverage behavior
make test-exhaustive # full non-optional release verification
```

`test-fast` uses `-short` and omits the six individually classified
open-topology proofs that dominated package time. On the reference machine the
open-topology package fell from 434 seconds to 174 seconds, a 60% reduction.
`test-bounded` explicitly enables those proofs while retaining legacy `-short`
behavior for separately promoted corpora and installed-tool checks.
`test-exhaustive` removes `-short`; tests that require external KiCad or frozen
campaign opt-ins retain their existing explicit environment gates.

The fast tier's exact skip expression is centralized in `FAST_TEST_SKIP`; the
sealed historical test files remain byte-identical. Coverage uses the bounded
tier and serializes Go packages by default to avoid memory contention on small
CI and developer machines. Override `GO_TEST_PACKAGE_PARALLELISM` only after
measuring the host.

## Concurrency budget

`KICADAI_MAX_WORKERS` may reduce the process-wide worker budget. The default is
`GOMAXPROCS`; an explicit value cannot raise the budget above `GOMAXPROCS`.
Outer corner and plan schedulers reserve the known inner simulation-analysis
fan-out from the shared budget, preventing nested pools from multiplying CPU
and memory demand.

The 1/2/4/8/default scaling sample for the powered-low-pass synthesis remained
within roughly 1.05–1.06 seconds. That workload has a serial critical path, so
more goroutines do not improve its latency. Concurrency is retained where work
is independent—multiple analyses, corners, promotion scenarios, library
lookups, file stats, and release targets—and is bounded primarily for stable
throughput and memory use.

Promotion scenario pairs run concurrently across scenarios, but run 1 and run
2 of each scenario remain sequential. Results, errors, comparisons, manifests,
and hashes are assembled in frozen matrix order. Release targets build in
bounded parallel batches and the manifest/checksum order remains canonical.
