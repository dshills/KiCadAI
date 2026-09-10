# V22 public electrical-repair evaluation

This successor is frozen before any V22 public corpus execution. It reuses the
24 immutable public discovery requirements and corrected V21 predecessor. Only
the four cases selected in `../V22_PUBLIC_POPULATION.json` enter V22; the other
twenty use the exact selected V21/V20 predecessor path. Selection is evaluation
configuration, never production topology logic. No held-out key or plaintext
access, new corpus, component catalog, model, v1 CLI change, or automatic
capability admission is authorized by this run.

## Execution boundary

- Exactly 24 cases in canonical order, two fresh serial replays each. No resume,
  outcome-driven rerun, selection change, budget increase, or numerical tuning.
- V21 completion and all earlier policies stay unchanged. V22 uses depth 4,
  beam width 8, 4096 attempted bindings, 128 candidate evaluations and 4096
  corners. Existing simulation policy additionally constrains evaluation/corner
  consumption. Count limits decide capability outcomes; a six-hour process
  timeout is an infrastructure failure, never a capability result.
- Go 1.26.8, darwin/arm64, CGO enabled, no implicit Go environment overrides;
  installed KiCad 10.0.3 and libraries are authenticated by the existing lock.
  Run with native macOS access because sandboxed zone refill aborts.
- Every passing synthesis must complete the unchanged installed-KiCad promotion
  gate in both outer replays, including each promotion's two clean project
  roots. ERC, strict DRC, route completion, connectivity, writer correctness,
  zero round-trip diffs and exact project replay are mandatory.
- A non-capability error stops the run. Retain partial evidence; do not retry
  within this protocol. Infrastructure recovery requires a separate recorded
  boundary, not deletion or silent replacement of the interrupted run.

## Evidence and measurements

The V22 transport streams the same complete canonical synthesis bytes as V21
directly into SHA-256. A regression compares that digest and byte count with
the legacy disk spool. The prior diagnostic measured up to 4.8 GB for one raw
replay; removing that temporary destination is a transport-only correction,
not a change to synthesis, numerical evidence, or replay scope. It does not
claim to eliminate in-memory solver reports or all temporary KiCad files.

Each replay retains deterministic `ELECTRICAL_REPAIR.json`, clean-root evidence,
and promotion evidence when applicable. Both full synthesis hashes and repair
sidecar hashes must match. `METRICS.json` separately records synthesis, hashing,
and promotion elapsed nanoseconds, canonical byte count, and zero synthesis
spool bytes. Timing is intentionally excluded from deterministic evidence.
The invocation records exact source commit, executable digest, Go environment,
process wall time and peak RSS when the host permits. After execution, publish
the report, compact repair evidence, measurements and aggregate assessment;
retain bulky KiCad projects outside Git. Do not interpret one-run timings as a
controlled speed comparison.

## Success and preservation

Authenticate reports and every case hash. Require all 48 replay digests and the
twenty unselected full synthesis replay hashes/decisions/gates to match their
corrected V21 predecessor. Preserve metadata, every historical V18/V20/V21 pass
and unsafe refusal, and introduce no new unsafe outcome. Exact root/evaluator
commitments necessarily differ between version-isolated evaluations.

Success requires at least one **additional complete simulation-and-KiCad pass**
above V21's one of 24, with preservation passing. Renamed failures, later-stage
failures and structural-only advancement count as zero additional passes.
Publish the aggregate and typed frontier changes even if this threshold is not
met. A negative evaluation is valid evidence, not completion of the goal's
electrical-improvement criterion. No result expands the supported v1 envelope.

Run `bash specs/post-topology-electrical-blockers/public-evaluation-v22/run.sh
/absolute/fresh/scratch-root` from the clean freeze commit. Tests before this
invocation use synthetic engines or inspect authenticated public contracts;
they never execute this corpus's synthesis.
