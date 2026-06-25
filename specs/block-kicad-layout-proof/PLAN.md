# Block KiCad-Backed Layout Proof Implementation Plan

Date: 2026-06-25

## Objective

Make circuit-block verification produce stable, explicit KiCad ERC/DRC evidence
for generated block fixtures, without requiring KiCad for normal test runs.

## Phase 1: Evidence Contract Audit

Audit and tighten the existing ERC/DRC stage contract.

Tasks:

- Review `ExpectedERCDRC`, `RunOptions`, `runERCDRCStage`, and CLI output.
- Ensure stage names, status values, summaries, issues, and artifacts are stable
  for pass, warning, blocked, and skipped outcomes.
- Add unit tests for manifest policy validation around `required`,
  `require_erc`, `require_drc`, `allowed_codes`, and `expected_issues`.
- Add regression tests for no-check-request behavior so unrequested checks do
  not appear as clean passes.

Review gate:

- `go test ./internal/blocks/verification`
- Prism review staged changes.
- Commit: `Harden block KiCad evidence contract`

## Phase 2: Fake Runner Coverage

Add deterministic ERC/DRC behavior tests using the existing `CheckRunner` hook.

Tasks:

- Add fake-runner tests for required missing KiCad behavior.
- Add fake-runner tests for required DRC finding behavior.
- Add fake-runner tests for allowed finding behavior.
- Add fake-runner tests for passing ERC and DRC with artifacts.
- Assert issue paths, stage summaries, and status transitions.
- Ensure global `RequireERC`/`RequireDRC` options can strengthen manifest
  requirements without weakening explicit manifest requirements.

Review gate:

- `go test ./internal/blocks/verification`
- Prism review staged changes.
- Commit: `Add deterministic KiCad check runner tests`

## Phase 3: Manifest Policy Expansion

Update representative block verification manifests with explicit KiCad evidence
policy.

Tasks:

- Add optional `erc_drc` policy to simple known-clean fixtures.
- Add optional or required-by-local-policy `erc_drc` intent to:
  - `esd_protection_5v`;
  - `reverse_polarity_schottky`;
  - `crystal_oscillator_default`;
  - `canned_oscillator_default`;
  - `reset_programming_header_isp`.
- Keep checked-in behavior optional unless fake-runner or local opt-in tests
  provide deterministic required evidence.
- Update manifest validation tests or golden snapshots if needed.

Review gate:

- `go test ./internal/blocks/verification`
- Prism review staged changes.
- Commit: `Declare block KiCad evidence policies`

## Phase 4: Artifact And Summary Stability

Make generated KiCad report artifacts stable enough for AI callers and local
debugging.

Tasks:

- Normalize artifact descriptions and paths from ERC/DRC stages.
- Ensure generated project preparation verifies freshness before reusing
  writer/board-validation artifacts. If freshness cannot be proven, force a
  re-export so KiCad checks run against the current manifest and block output.
  Freshness should be based on a hash of the manifest, instantiated block
  output, writer options, relevant check options, and detected `kicad-cli`
  version.
- Ensure optional skipped checks include actionable suggestions.
- Ensure required failed checks include check kind, CLI path, target, and
  report path when available.
- Add tests for artifact deduplication and stable summary strings.

Review gate:

- `go test ./internal/blocks/verification`
- Prism review staged changes.
- Commit: `Stabilize block KiCad check artifacts`

## Phase 5: Local Opt-In KiCad Smoke Harness

Add a local-only smoke path for developers with KiCad installed.

Tasks:

- Add a test or CLI fixture path that runs selected manifests with real
  `kicad-cli` only when explicitly configured.
- Ensure skipped local smoke evidence is explicit when KiCad is absent.
- Avoid adding environment-dependent failures to `go test ./...`.
- Document the command for running the opt-in smoke suite.

Review gate:

- `go test ./internal/blocks/verification`
- `go test ./...`
- Prism review staged changes.
- Commit: `Add opt-in block KiCad smoke coverage`

## Phase 6: Documentation And Roadmap

Update public documentation and roadmap status.

Tasks:

- Update `README.md` block verification docs with ERC/DRC evidence behavior.
- Update `specs/ROADMAP.md` Priority 2 current foundation and remaining work.
- Document that KiCad-backed proof is stronger layout evidence, not
  fabrication readiness by itself.
- Document how optional skipped evidence should be interpreted by AI callers.

Review gate:

- `go test ./...`
- Prism review staged changes.
- Commit: `Document block KiCad layout proof`

## Final Completion Criteria

- All phases are committed independently.
- Prism review has been run before each phase commit.
- `go test ./...` passes without requiring KiCad.
- Fake-runner tests prove pass, skipped/warning, and blocked KiCad evidence
  behavior.
- Representative block manifests explicitly declare KiCad proof intent.
- README and roadmap describe the new evidence accurately.
