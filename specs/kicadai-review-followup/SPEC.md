# KiCadAI Review Follow-up Specification

Status: Proposed  
Review basis: `specs/kicadai-review.md`  
Audit date: 2026-07-15  
Baseline commit: `a12f2c9a`

## 1. Purpose

This specification converts the architecture review into an evidence-based
remediation scope. It distinguishes defects that are present in the current
tree from observations that are historical, approximate, or already addressed.
The goal is to improve release reliability without turning every desirable
future capability into a blocking refactor.

The remediation is ordered around operational risk:

1. Make the repository continuously verifiable.
2. Make the default component catalog deployable with the binary.
3. Remove misleading routing configuration and make routing limitations
   explicit.
4. Establish package seams that permit later decomposition without changing
   generated design behavior.
5. Preserve evidence for the generic design path while the older bounded intent
   planner is incrementally expanded.

## 2. Current Findings

### 2.1 Finding disposition

| Review observation | Current disposition | Decision |
| --- | --- | --- |
| Duplicated `AcceptanceLevel`, `ComponentRole`, and `NetRole` vocabularies | Addressed by `b8b4cb32` and `a12f2c9a` | Keep the shared `internal/domain` types and alias coverage. Do not re-open as a broad refactor. `schematicir.AcceptanceLevel` remains intentionally separate because its values describe schematic evidence (`erc_clean`, `readable`) rather than the cross-domain acceptance contract. |
| `internal/designworkflow` is a large orchestration package | Real structural risk | Add seams and characterization tests first. Decompose only behind stable interfaces; no behavior rewrite is implied by this specification. |
| Router is net-by-net and has no general rip-up/retry loop | Partly real | The planner builds deterministic trees and the route executor consumes them in order. The public `Strategy.RipupRetryLimit` is currently unused, so the immediate fix is to remove or explicitly deprecate that promise. A general rip-up router is a separate future capability, not an assumed deliverable here. |
| Placer computes HPWL but does not globally optimize it | Overstated but useful | Candidate scoring is implemented and tested, and HPWL is retained as a metric. There is no global optimizer. Treat this as a documented quality limitation with regression tests, not as a correctness bug. |
| Intent planner contains hardcoded topology mappings | Real, but scoped | `internal/intentplanner` is a deterministic bounded topology planner. It is not the only generation path: the generic catalog-resolved circuit path already exists. Add a capability matrix and route supported generic graphs through the generic path; do not promise arbitrary circuits until graph coverage and evidence exist. |
| No CI workflow | Real | Add a reproducible offline CI lane for formatting, tests, lint, and coverage. KiCad-backed checks remain optional and environment-gated. |
| golangci-lint exposes approximately 82 issues | Stale or unsupported as a current claim | The checked-in configuration enables only `govet`, and the installed golangci-lint currently reports `0 issues`. CI should pin and run the declared policy, but this remediation will not chase an unverified historical count. |
| Only a small fraction of errors use `%w` | Directionally real, metric stale | Current direct-line counts are approximately 113 wrapped `fmt.Errorf` calls versus 579 total `fmt.Errorf` calls. This is a policy gap, not proof that every unwrapped error is a bug. Add targeted error-boundary guidance and lint/test rules; do not perform a mechanical repository-wide rewrite. |
| Component catalog is filesystem-only | Real deployment gap | `LoadCatalog` and default catalog discovery require `data/components` on disk. Add an embedded default catalog with an explicit filesystem override for development and custom libraries. |
| Historical LED/VCC-to-AREF and round-trip issues | Historical evidence, not a current finding | Preserve the existing regression and round-trip corpus. Reproduce a historical defect before changing production code. |
| Package counts, commit counts, and single-author history | Descriptive snapshot | These are not remediation items. Keep them out of acceptance criteria. |

### 2.2 Baseline evidence

The baseline is considered healthy before remediation:

- `go test ./...` passes.
- `make lint` passes.
- `make coverage-check` passes with 82.4% generated-code-excluded coverage
  against the 75.0% floor.
- Direct golangci-lint execution reports zero issues under the checked-in
  `govet` policy.
- Shared cross-domain vocabulary has compile-time alias tests and exact wire
  spelling tests.
- Existing generic, bounded, and KiCad-backed fixtures must remain unchanged
  in behavior.

The review's line counts and package counts are approximate planning signals,
not contractual measurements. Future comparisons must record the command,
commit, tool version, and scope used to produce a metric.

## 3. Goals

### 3.1 Required goals

1. Every pull request receives the same offline checks developers can run
   locally.
2. A compiled or packaged KiCadAI binary can resolve its default catalog without
   depending on the caller's working directory.
3. Routing configuration describes implemented behavior only. Unsupported retry
   behavior fails clearly or is removed from the public request contract.
4. The workflow package has tested boundaries for request normalization,
   placement/routing execution, artifact production, and promotion checks.
5. Generic-circuit support has an explicit capability matrix that distinguishes
   supported graph composition from bounded topology mappings.
6. Error handling, historical regressions, and evidence artifacts are improved
   incrementally without broad behavior churn.

### 3.2 Non-goals

- A general-purpose autorouter with optimal rip-up and reroute.
- A global placement optimizer.
- Replacing the bounded intent planner in one change.
- Supporting every analog topology or arbitrary natural-language request.
- Enabling KiCad or OpenAI in the default CI job.
- Rewriting all error strings solely to increase a `%w` percentage.
- Reorganizing the entire repository to reduce a line-count metric.

## 4. Design Requirements

### 4.1 Continuous verification

CI shall run, on a supported Go version and macOS/Linux runner as applicable:

```text
gofmt -l .
go test ./...
make lint
make coverage-check
```

The workflow shall pin action versions and record the Go and lint versions used
by each run. It shall not require KiCad, an OpenAI key, a local catalog path, or
network access after dependencies are available. Optional KiCad-backed jobs may
be added separately and must be explicitly environment-gated.

### 4.2 Catalog packaging

The component catalog loader shall support two sources with deterministic
precedence:

1. An explicitly supplied filesystem directory, for custom or development
   catalogs.
2. The embedded repository catalog when no directory is supplied or the
   packaged default is requested.

The API shall preserve current custom-directory behavior. Embedded catalog
records must be validated with the same parser and schema as filesystem records.
The loader shall expose the source used and a stable catalog fingerprint in
diagnostic/evidence output. A missing explicit custom directory is an error; a
missing embedded catalog is a build/test failure.

### 4.3 Routing contract

The current route planner may remain deterministic and ordered. It must not
advertise a retry feature that is not executed.

The implementation shall choose one of these compatible outcomes before the
routing phase is complete:

- Remove `RipupRetryLimit` from the public strategy and update schemas/examples;
  or
- Mark it deprecated, reject non-zero values with a structured unsupported
  diagnostic, and document that zero is the only supported value until a real
  rip-up implementation exists.

The chosen outcome must be backward-compatible with checked-in request fixtures
or include a migration diagnostic. No silent ignore is allowed.

If a future phase implements rip-up, it must use an explicit bounded budget,
restore all modified occupancy/net state between attempts, and produce evidence
of each attempt. That future work is outside this closeout.

### 4.4 Workflow seams

The workflow must expose tested stage boundaries without changing the existing
CLI contract. At minimum, the following responsibilities must be separable:

- request and intent normalization;
- trusted catalog/component resolution;
- schematic lowering and readability validation;
- PCB placement and routing;
- KiCad/evidence execution;
- promotion classification and artifact writing.

The first extraction may use interfaces or narrow stage functions inside the
existing package. New packages are justified only when they remove a dependency
cycle or make ownership testable. Package size alone is not an acceptance
criterion.

### 4.5 Generic capability boundary

The repository shall publish a machine-readable or Go-defined capability matrix
that states, for each input family:

- accepted component roles and pin/function evidence;
- supported net and endpoint multiplicity;
- supported multi-unit behavior;
- layout/routing limitations;
- required evidence gates;
- whether the path is generic, bounded, or unsupported.

Unsupported or ambiguous graphs must fail closed with structured diagnostics.
The generic catalog-resolved path must not silently dispatch through a
project-name or fixture-name special case.

### 4.6 Error and regression policy

Errors crossing package boundaries must preserve causal context with `%w` where
callers can classify or unwrap the cause. User-facing validation messages may
remain independently formatted when wrapping would reduce clarity. New code
shall follow this rule, and high-value existing boundaries shall be migrated
when touched.

Historical electronics defects shall be represented by focused regression
fixtures or normalized evidence tests. A historical review note alone does not
justify a production change.

## 5. Acceptance Criteria

### 5.1 CI and quality

- A clean checkout can run the required offline checks through one documented
  CI workflow.
- The workflow fails on test, lint, formatting, or coverage regressions.
- The workflow records Go and lint versions.
- Existing optional KiCad-backed tests remain skipped rather than failed when
  KiCad is unavailable.

### 5.2 Catalog

- A test builds/runs the binary or loader from a directory without
  `data/components` in the working directory and resolves the embedded default
  catalog.
- A custom directory overrides the embedded catalog deterministically.
- Catalog parse, validation, and fingerprint tests pass for both sources.
- Existing catalog-based fixtures produce equivalent design projections.

### 5.3 Routing

- No request field claims behavior that the executor silently ignores.
- Unsupported retry values produce a structured diagnostic, or the field is
  removed with migrated fixtures.
- Existing route completion, connectivity, DRC, writer, and round-trip tests
  remain green.

### 5.4 Architecture and generic support

- Stage-boundary tests identify the first failing stage without running the full
  CLI.
- The capability matrix identifies generic versus bounded paths.
- Generic unsupported graphs fail closed; existing supported graphs retain their
  current outputs.

### 5.5 Error and regression coverage

- New cross-package errors preserve causes where classification is required.
- At least one regression test covers each reproduced historical electronics or
  round-trip defect selected for remediation.
- No broad mechanical error rewrite or fixture weakening is accepted as a
  substitute for behavior evidence.

## 6. Test Strategy

Tests are layered:

1. Unit tests for catalog source selection, routing contract validation, stage
   boundaries, and capability classification.
2. Golden tests for normalized schematic/PCB output and evidence projection.
3. Existing package and full repository tests.
4. Offline CI checks without KiCad or credentials.
5. Optional environment-gated KiCad-backed promotion checks, run only when the
   local KiCad toolchain is explicitly available.

Every phase must record the baseline and rerun focused tests before the full
suite. A phase is not complete if it only makes static metrics look better.

## 7. Rollback and Compatibility

Changes must be independently revertible by phase. Catalog embedding must retain
the filesystem override. Routing contract changes must provide migration or a
structured error. Workflow extraction must preserve public CLI flags, JSON
schemas, artifact paths, and deterministic transaction output. CI changes must
be additive until the local equivalent passes.

## 8. Open Questions

- Should the packaged binary always ship the full catalog, or should a release
  build support a separately versioned catalog bundle?
- Which Go versions and operating systems are release-supported?
- Should unsupported routing options be rejected at JSON decode time or at
  planning validation time so diagnostics can include the field path?
- Which workflow seam should be extracted first after the stage characterization
  tests: evidence/promotion, catalog resolution, or placement/routing?
- Which historical review fixtures are still valuable after the current golden
  corpus is audited?
