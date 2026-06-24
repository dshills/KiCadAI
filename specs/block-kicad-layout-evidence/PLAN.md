# Block KiCad Layout Evidence Implementation Plan

## Overview

This plan upgrades block verification from mostly schematic assertions to
stronger PCB realization, internal board validation, and optional KiCad-backed
evidence for timing and protection seed blocks.

Each phase should be implemented, tested, reviewed with Prism, and committed
before moving to the next phase.

## Phase 1: Manifest Model And Validation

### Goal

Add first-class manifest fields for PCB realization evidence without changing
existing manifest behavior.

### Work

- Extend `verification.ExpectedPCB` with:
  - `RequireRealization bool`
  - `RequiredLocalRoutes []string`
  - `TimingFixtures []ExpectedTimingFixture`
  - `RequireBoardValidation bool`
- Add `ExpectedTimingFixture` with:
  - `ID string`
  - `Satisfied *bool`
  - optional `RequiredFindings []string`
  - optional `ForbiddenFindings []string`
- Update manifest validation:
  - reject duplicate `required_local_routes`;
  - reject duplicate timing fixture IDs;
  - reject empty timing fixture IDs;
  - keep existing `required_routes` semantics unchanged;
  - allow new fields to be omitted by older manifests.
- Add model and validation tests.

### Tests

- `go test ./internal/blocks/verification`

### Acceptance

- Existing manifests still validate unchanged.
- Invalid new PCB evidence fields produce stable issue paths.
- The new fields are documented in test fixtures or comments.

### Commit

`Add block PCB evidence manifest fields`

## Phase 2: RunCase PCB Realization Assertions

### Goal

Teach block verification to run and assert `RealizeBlockPCB` when requested by
the manifest.

### Work

- Add a `pcb_realization` stage in `verification.RunCase`.
- Run `blocks.RealizeBlockPCB` when any of these are set:
  - `expected.pcb.require_realization`;
  - `expected.pcb.required_local_routes`;
  - `expected.pcb.timing_fixtures`;
  - `expected.pcb.require_board_validation`.
- Assert:
  - no blocking realization issues;
  - expected realized local-route IDs exist;
  - expected timing fixture IDs exist;
  - timing fixture `satisfied` matches when specified;
  - required/forbidden finding IDs match when specified.
- Keep existing `pcb_assertions` stage for writer/project summary assertions.
- Add negative tests for:
  - missing local route;
  - missing timing fixture;
  - unsatisfied fixture when satisfied is expected.

### Tests

- `go test ./internal/blocks ./internal/blocks/verification`

### Acceptance

- PCB realization evidence fails before writer/KiCad stages when block metadata
  is wrong.
- Existing manifests that do not request realization remain unchanged.
- New stage output is deterministic.

### Commit

`Assert block PCB realization evidence`

## Phase 3: Internal Board Validation Stage

### Goal

Allow manifests to require internal board validation for generated block
projects when writer output exists.

### Work

- Add `board_validation` stage to the verification runner.
- Reuse writer-generated project artifacts when writer stage already ran.
- If board validation is requested without writer output, either:
  - generate a project through the same helper used by writer/KiCad checks; or
  - fail with a clear manifest configuration issue if generation cannot be
    performed.
- Respect `expected.pcb.allow_unrouted`.
- Summarize:
  - issue count;
  - blocking issue count;
  - unrouted count when available;
  - project path/artifact path when generated.
- Add tests with a small block fixture that passes board validation under
  `allow_unrouted`.

### Tests

- `go test ./internal/blocks/verification ./internal/boardvalidation`

### Acceptance

- Board validation can be required by a manifest without requiring KiCad CLI.
- Failures include stable paths under
  `verification.<case>.board_validation`.
- Existing writer and ERC/DRC stages continue to work.

### Commit

`Add block board validation evidence`

## Phase 4: Upgrade Timing Block Manifests

### Goal

Promote timing block manifests to request PCB realization evidence.

### Work

- Update `crystal_oscillator_default` manifest:
  - `require_realization: true`;
  - `required_local_routes`: `xtal1_load`, `xtal2_load`,
    `load_caps_ground`;
  - timing fixture `crystal_loop` satisfied true.
- Update `canned_oscillator_default` manifest:
  - `require_realization: true`;
  - `required_local_routes`: oscillator local routes;
  - timing fixture `canned_oscillator_core` satisfied true.
- Update `reset_programming_header_isp` manifest:
  - `require_realization: true`;
  - `required_local_routes`: reset/header routes;
  - timing fixture `reset_programming_path` satisfied true.
- Add optional UART/no-switch manifest if the conditional realization path
  needs corpus coverage.
- Update built-in verification CLI goldens.

### Tests

- `go test ./internal/blocks/verification ./cmd/kicadai`

### Acceptance

- Built-in verification reports PCB realization stages for timing blocks.
- The timing block manifests fail if expected local routes or timing fixtures
  disappear.
- CLI goldens reflect the new evidence stages.

### Commit

`Upgrade timing block verification evidence`

## Phase 5: Upgrade Protection Block Manifests

### Goal

Promote ESD and reverse-polarity block manifests to the strongest currently
truthful PCB evidence.

### Work

- Review current ESD and reverse-polarity PCB realization metadata.
- Add `require_realization` where supported.
- Add `required_local_routes` for local shunt/protection or power-path routes
  that exist today.
- Add `require_board_validation` only if generated project validation is stable
  without KiCad CLI and without false fabrication-readiness claims.
- Document blockers in manifest descriptions or README where stronger evidence
  depends on entry-anchor/power-path enforcement.
- Update CLI goldens.

### Tests

- `go test ./internal/blocks/verification ./cmd/kicadai ./internal/blocks`

### Acceptance

- Protection block manifests assert all supported PCB realization evidence.
- Unsupported KiCad-backed evidence remains clearly optional/pending.
- Built-in verification remains deterministic.

### Commit

`Upgrade protection block verification evidence`

## Phase 6: Optional KiCad ERC/DRC Policy Hardening

### Goal

Make KiCad-backed block evidence strict when requested and clearly skipped when
optional or unavailable.

### Work

- Review existing `ExpectedERCDRC` behavior in block verification.
- Ensure required ERC/DRC fails if `kicad-cli` is missing.
- Ensure optional ERC/DRC reports skipped status with reason when unavailable.
- Add artifact path summaries for KiCad reports when checks run.
- Add tests using fake/check fixture behavior where possible without requiring
  live KiCad.
- Avoid checking in KiCad-version-sensitive DRC output unless stable.

### Tests

- `go test ./internal/blocks/verification ./internal/kicadfiles/checks`

### Acceptance

- Required KiCad evidence cannot silently pass when not run.
- Optional KiCad evidence is visible as skipped, not silently absent.
- Stage summaries are AI-readable.

### Commit

`Harden block KiCad check evidence policy`

## Phase 7: Documentation And Roadmap Update

### Goal

Keep user-facing docs aligned with block verification evidence levels.

### Work

- Update README block verification section.
- Update `docs/circuit-block-verification.md`.
- Update `specs/ROADMAP.md`:
  - mark timing/protection manifest evidence progress;
  - keep remaining KiCad DRC-backed proof limitations explicit.
- Mention that commands assume compiled `kicadai`.

### Tests

- `go test ./...`

### Acceptance

- Full test suite passes.
- Documentation accurately states which evidence is deterministic by default
  and which requires KiCad CLI.

### Commit

`Document block layout evidence verification`
