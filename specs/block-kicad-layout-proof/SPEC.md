# Block KiCad-Backed Layout Proof Specification

Date: 2026-06-25

## Summary

KiCadAI has a strong structural circuit-block verification harness: manifests
can validate schematic semantics, PCB realization metadata, required local
routes, timing evidence, writer correctness, board validation, and optional
KiCad ERC/DRC behavior. The next gap is stable KiCad-backed layout proof for
block-level generated projects.

This project makes block verification manifests produce deterministic,
inspectable ERC/DRC evidence when `kicad-cli` is available, while preserving
clear skipped evidence when it is not. The goal is to move protection,
timing-sensitive, and power-path blocks from "structurally modeled" toward
"layout behavior is proven by KiCad on generated fixtures."

## Problem

Current block verification can request ERC/DRC checks, but the evidence is not
yet broad or stable enough to use as a confidence gate across circuit blocks.
Several issues remain:

- not every block manifest that should have KiCad-backed proof declares it;
- optional versus required KiCad check behavior is easy to misconfigure;
- skipped external checks are not always distinguishable from clean KiCad
  checks in high-level status summaries;
- generated block fixtures may pass writer/board validation while still lacking
  DRC-clean KiCad evidence;
- protection and timing-sensitive blocks need stronger proof that their
  generated board fixtures are not merely parseable;
- local developer environments may have different KiCad versions, so checked-in
  tests must stay deterministic without requiring KiCad on every run.

## Goals

- Define a stable evidence contract for block-level KiCad ERC/DRC proof.
- Extend verification manifests so eligible blocks can request optional or
  required KiCad checks explicitly.
- Add deterministic skipped evidence when `kicad-cli` is unavailable or checks
  are configured as optional.
- Add checked-in regression coverage that proves:
  - required checks block when unavailable or failing;
  - optional checks report skipped/warning evidence without failing the suite;
  - passing check results produce stable stage summaries and artifacts.
- Add golden fixtures for representative blocks that need stronger layout proof:
  - ESD protection;
  - reverse-polarity protection;
  - crystal oscillator;
  - canned oscillator;
  - reset/programming fixture;
  - one simple known-clean passive block.
- Preserve generated-project cleanup and overwrite safety.
- Keep the evidence usable by AI callers: stage names, issue paths, artifacts,
  and summaries should explain what KiCad proved or skipped.

## Non-Goals

- Do not require every local `go test ./...` run to have KiCad installed.
- Do not guarantee fabrication readiness from ERC/DRC alone.
- Do not implement arbitrary block layout optimization in this project.
- Do not broaden the component catalog.
- Do not mutate imported user projects.
- Do not make KiCad version-specific expected warnings brittle unless the
  fixture explicitly declares the version scope.

## Current Foundation

The current harness already includes:

- `ExpectedERCDRC` manifest fields:
  - `required`;
  - `require_erc`;
  - `require_drc`;
  - `allowed_codes`;
  - `expected_issues`.
- `RunOptions` fields for `KiCadCLI`, `RequireERC`, `RequireDRC`,
  `CheckOptions`, and `CheckRunner`.
- Generated project preparation through `ensureProjectForExternalChecks`.
- Writer and board validation stages that can reuse generated artifacts.
- Optional versus required external-tool behavior in parts of the workflow.
- Checked-in block verification manifests under
  `internal/blocks/testdata/verification`.

This project should refine and broaden those existing paths rather than add a
parallel validation system.

## Evidence Contract

Each block verification run that evaluates KiCad checks should emit a stable
stage:

```json
{
  "name": "erc_drc",
  "status": "pass|warning|blocked|skipped",
  "summary": "erc/drc ...",
  "issues": []
}
```

The stage must distinguish:

- `pass`: requested checks ran and produced no blocking findings outside the
  manifest policy.
- `warning`: checks were optional and either unavailable, skipped, or produced
  only allowed findings.
- `blocked`: checks were required and unavailable, failed, or produced
  unallowed findings.
- `skipped`: no ERC/DRC evidence was requested for the case.

Artifacts should include paths to generated KiCad reports when available.
Summaries or artifacts must record the `kicad-cli` version when real KiCad
checks run so evidence can be reproduced across local and CI environments.
Warning summaries must include a machine-readable cause such as `tool_skipped`,
`tool_unavailable`, `allowed_findings`, or `partial_evidence` so callers do not
need to infer why the stage warned.
Issues use the repository-wide `reports.Issue` shape with stable fields:
`code`, `severity`, `path`, `message`, optional `refs`, optional `nets`,
optional `suggestion`, and optional `operation_id`.
For KiCad findings, `path` should preserve the most specific normalized design
object path available, such as a component, net, sheet, coordinate, or report
finding path. The KiCad finding code should stay in the issue `code` or message
rather than replacing object location. Object references, nets, coordinates,
and UUIDs should be mapped into `refs`, `nets`, `message`, and existing issue
location fields where available.

The existing `blocked` status covers both execution blockers and unallowed
design findings. Consumers should distinguish those causes through issue code
and message: missing tools and command failures should use external-tool issue
codes, while unallowed ERC/DRC findings should use validation issue codes. The
implementation must keep those code families partitioned so CI and AI callers
can separate environment failures from design regressions even when both map to
the existing `blocked` stage status.

Implementations that invoke `kicad-cli` must use argument-vector execution such
as Go's `exec.Command` and must not interpolate manifest-derived values into a
shell string. Manifest-derived filenames and paths must reject directory
traversal and shell metacharacters.

## Manifest Policy

Each manifest can declare:

```json
"erc_drc": {
  "required": false,
  "require_erc": true,
  "require_drc": true,
  "allowed_codes": [],
  "expected_issues": [],
  "runner": "optional|fake|required_real",
  "min_kicad_version": "9.0.0",
  "max_kicad_version": ""
}
```

Policy rules:

- `required: true` means missing `kicad-cli`, command failure, missing report,
  or unallowed findings block the case. Checked-in manifests that are exercised
  by default `go test ./...` should not require real KiCad unless the test uses
  a fake runner. Real required KiCad proof belongs in explicit local opt-in or
  CI jobs that provision KiCad.
- `required: false` means the stage can warn or skip, but must still report why
  evidence is missing.
- `require_erc` and `require_drc` select which checks run.
- `runner` declares the expected execution capability:
  - `optional`: real KiCad may run when configured, but absence is warning or
    skipped evidence;
  - `fake`: deterministic tests use `CheckRunner` and do not require real
    KiCad;
  - `required_real`: real `kicad-cli` must be available and compatible.
- `runner: required_real` implies required behavior even if `required` is
  omitted or false. `required: true` with `runner: optional` is invalid and
  should be rejected by manifest validation.
- `min_kicad_version` and `max_kicad_version` constrain version-sensitive
  finding policies when real KiCad checks are required.
- Version mismatches block `required_real` cases and warn/skip optional cases
  with explicit version evidence.
- An empty `allowed_codes` list means no KiCad finding codes are allowed.
- Findings matching `allowed_codes` should remain visible as informational
  issues or summarized allowed findings rather than disappearing from evidence.
- `expected_issues` should be used only for negative fixtures that intentionally
  prove failure parsing.
- Global CLI flags/options can strengthen requirements but should not weaken a
  manifest that explicitly requires checks.

## Required Fixtures

The implementation should add or update fixtures in three categories.

### Optional KiCad Evidence Fixtures

These fixtures should remain stable without KiCad installed:

- LED indicator;
- connector breakout;
- voltage regulator.

They should prove optional KiCad evidence produces a skipped/warning stage and
does not fail `go test ./...`.

### Required KiCad Evidence Unit Fixtures

These should use a fake `CheckRunner` in tests so behavior is deterministic:

- required checks unavailable blocks the case;
- required DRC finding blocks unless allowed;
- optional DRC finding warns when allowed;
- passing ERC/DRC produces a pass stage and artifacts.

### Real Layout Proof Candidates

These manifests should be prepared for local opt-in KiCad runs:

- `esd_protection_5v`;
- `reverse_polarity_schottky`;
- `crystal_oscillator_default`;
- `canned_oscillator_default`;
- `reset_programming_header_isp`.

The checked-in tests should not require KiCad, but the manifests and CLI output
should make it clear which cases are intended to become KiCad-backed proof
fixtures.

## CLI And Output Behavior

`kicadai block verify` should make KiCad evidence clear:

- suite output should include stage status for `erc_drc`;
- skipped optional external checks should not be confused with pass;
- required missing external checks should fail the case;
- `--output` should keep generated project and report artifacts when requested;
- `--overwrite` should remain required for replacing existing generated
  verification artifacts.

## Acceptance Criteria

- Manifests validate ERC/DRC policy consistently.
- Required KiCad checks block when unavailable or failing.
- Optional KiCad checks produce explicit warning/skipped evidence when
  unavailable.
- Passing fake KiCad check results produce stable pass summaries and artifacts.
- Representative protection and timing-sensitive manifests declare the intended
  KiCad evidence policy.
- `go test ./...` passes without requiring KiCad.
- Prism review is run before each phase commit.

## Open Questions

- Which KiCad versions should be treated as supported proof sources for
  checked-in external artifacts?
- Should a passing KiCad DRC stage become a prerequisite for
  `fabrication-candidate` block evidence, or remain separate until more block
  variants exist?
- Should block verification add a future `keep_on_failure` mode? The default
  behavior for this project should continue to respect `KeepArtifacts` to avoid
  unbounded disk growth during suite runs.
- How much KiCad report normalization is needed to keep evidence stable across
  KiCad 9 patch releases?
