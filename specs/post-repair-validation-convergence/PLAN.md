# Post-Repair Validation Convergence Implementation Plan

Date: 2026-06-24

This plan implements `specs/post-repair-validation-convergence/SPEC.md`.

Each phase should be implemented, reviewed with Prism, tested, and committed
before moving to the next phase.

## Implementation Status

Completed on 2026-06-24.

- Phase 1: `84fe163 Add normalized repair validation findings`
- Phase 2: `3114f2c Normalize KiCad check evidence for repair`
- Phase 3: `7756120 Normalize writer and board validation evidence`
- Phase 4: `b481475 Add normalized validation convergence deltas`
- Phase 5: `3fe5540 Add validation repair loop budget ledger`
- Phase 6: `2796dbb Add explicit KiCad zone refill policy`
- Phase 7: `8ae1869 Add opt-in KiCad validation fixtures`
- Phase 8: `204c956 Expose repair convergence evidence`
- Phase 9: final verification and closeout

## Phase 1: Normalized Finding Model

### Goal

Introduce a stable validation evidence model without changing existing repair
behavior.

### Work

- Add normalized finding, source, category, repairability, subject, and key
  models.
- Add conversion helpers from `reports.Issue` to normalized findings.
- Add deterministic key generation using structured fields before message text.
- Add sorting and grouping helpers by source, category, severity, and key.
- Keep the package placement narrow; start in `internal/repair` unless import
  cycles force a small shared package.

### Tests

- Keys are stable across input ordering.
- Keys prefer source/category/code/subject/path over message text.
- Empty or partially structured issues still normalize deterministically.
- Sorting is deterministic.
- Severity and operation IDs survive normalization.

### Acceptance

- Existing repair tests pass unchanged.
- New tests prove normalized evidence can be produced for generic issues.

### Commit

```text
Add normalized repair validation findings
```

## Phase 2: ERC/DRC And Parser Category Mapping

### Goal

Map KiCad check findings, parser issues, and tool failures into normalized
repair categories consistently.

### Work

- Add mapping helpers for `internal/kicadfiles/checks.CheckFinding`.
- Add mapping helpers for KiCad parser issues and context warnings.
- Add source/category rules for ERC, DRC, missing CLI, missing report, and tool
  execution failures.
- Reuse or align with existing routing DRC mapping where practical.
- Add table-driven mapping for known KiCad raw codes and current report parser
  outputs.
- Keep message-fragment fallbacks isolated and documented.

### Tests

- ERC violation maps to schematic ERC category.
- DRC clearance/track/outline/zone findings map to board DRC, route, outline,
  or zone categories as appropriate.
- Parser issue maps to external-tool or parse category with blocked
  repairability.
- Missing required KiCad evidence maps to external-tool-blocked.
- Fake-runner clean reports produce no blocking normalized findings.

### Acceptance

- KiCad check consumers can expose normalized finding evidence without changing
  their current public behavior.

### Commit

```text
Normalize KiCad check evidence for repair
```

## Phase 3: Writer And Board Validation Normalizers

### Goal

Normalize built-in writer correctness and board validation findings so they can
be compared with KiCad-backed evidence.

### Work

- Add adapters from writer correctness checks to normalized findings.
- Add adapters from board validation checks to normalized findings.
- Map parse, project structure, connectivity, pad-net, copper-net, zone,
  outline, route-completion, and optional DRC issues.
- Preserve artifact paths and adapter names.
- Add category-specific repairability defaults.

### Tests

- Schematic and PCB parse failures normalize as parse findings.
- Missing outline normalizes as outline and repairable when generated.
- Unrouted net normalizes as route/connectivity.
- Zone fill warning normalizes as zone with external KiCad evidence guidance.
- Round-trip diff normalizes as round-trip and blocks when required.

### Acceptance

- Post-repair validations can emit normalized evidence for all built-in
  adapters.

### Commit

```text
Normalize writer and board validation evidence
```

## Phase 4: Category-Level Delta And Convergence Summary

### Goal

Extend before/after comparison from raw issue counts to normalized evidence
categories and stable keys.

### Work

- Add normalized delta summary with cleared, repeated, new, worsened,
  unsupported, and external-tool-blocked counts.
- Add per-category delta grouping.
- Extend existing post-validation summary output additively.
- Add stop-reason selection helpers for clean, partial, no improvement,
  repeated evidence, unsupported findings, and validation errors.
- Ensure existing status decisions remain compatible but gain richer evidence.

### Tests

- Cleared, repeated, new, and worsened findings are classified by key.
- Repeated blocking normalized evidence blocks repaired status.
- New external-tool-blocked evidence reports a specific stop reason.
- Warning-only remainder produces partial status when required validation is
  otherwise clean.
- Output ordering is deterministic.

### Acceptance

- Repair results explain not just whether validation changed, but which
  category changed and why the loop stopped.

### Commit

```text
Add normalized validation convergence deltas
```

## Phase 5: Full Loop Budget Ledger

### Goal

Track budgets across validate, repair, revalidate, and optional workflow loops.

### Work

- Add loop budget options and summary model.
- Track total cycles, repair attempts, per-category attempts, remaining budget,
  and exhaustion reason.
- Wire budget ledger into persisted repair apply.
- Wire the same ledger model into design workflow validation repair paths.
- Preserve existing placement-routing retry policy while reporting how it fits
  into the broader loop.
- Add repeated-evidence and no-improvement checks using normalized deltas.

### Tests

- Total budget exhaustion stops with `total_budget_exhausted`.
- Category budget exhaustion stops with `category_budget_exhausted`.
- Repeated evidence stops when enabled.
- No improvement stops when enabled.
- Existing repair apply behavior stays compatible under default budgets.

### Acceptance

- The repair loop always terminates with deterministic budget evidence.

### Commit

```text
Add validation repair loop budget ledger
```

## Phase 6: Explicit Zone Refill Policy

### Goal

Represent KiCad zone refill as an explicit, generated-project-only write
policy.

### Work

- Add zone refill policy model and options.
- Add a runner interface for KiCad zone refill.
- Implement fake-runner support for unit tests.
- Gate refill on generated ownership/provenance and configured KiCad CLI.
- Add evidence artifacts and structured failure issues.
- Wire policy before validation or after repair before validation depending on
  selected mode.
- Keep default policy as `never`.

### Tests

- Default validation never invokes refill.
- Requested refill without KiCad CLI blocks or skips according to policy.
- Requested refill on imported/preservation-only target blocks.
- Fake successful refill records artifact evidence.
- Failed refill blocks required validation.

### Acceptance

- Zone refill can be requested safely and is never implicit.

### Commit

```text
Add explicit KiCad zone refill policy
```

## Phase 7: Opt-In KiCad Artifact Fixtures

### Goal

Broaden real ERC/DRC evidence coverage without making default tests depend on
local KiCad.

### Work

- Add committed fake-runner report fixtures for clean ERC, ERC findings, clean
  DRC, DRC findings, parser errors, and missing report behavior.
- Add optional integration test hooks for local KiCad CLI report generation.
- Redact machine-specific paths, timestamps, and temp directories in golden
  summaries.
- Add CLI or test helper documentation for regenerating optional artifacts.
- Ensure default `go test ./...` skips real KiCad integration cleanly.

### Tests

- Fake report fixtures parse deterministically.
- Optional integration test skips when KiCad CLI is unavailable.
- Redacted summaries are stable.
- Required vs optional external evidence policy is covered by fixtures.

### Acceptance

- The project has a clear path to accumulate real KiCad ERC/DRC evidence while
  keeping normal tests hermetic.

### Commit

```text
Add opt-in KiCad validation fixtures
```

## Phase 8: CLI And Workflow Evidence Surface

### Goal

Expose normalized convergence evidence to humans and AI agents.

### Work

- Add normalized findings, convergence summary, and budget ledger to
  `repair apply` JSON output.
- Include convergence artifacts in `design create` repair output when repair is
  enabled.
- Include normalized validation evidence in repair bundles where useful for
  replay/debugging.
- Add selected-field golden tests for CLI output.
- Update README and `specs/ROADMAP.md` to reflect completed convergence work
  and remaining downstream gaps.

### Tests

- `repair apply` goldens include normalized findings and budget summary.
- `design create` goldens include convergence summary when repair is enabled.
- Existing JSON consumers remain backward-compatible.
- README examples match current flags and output behavior.

### Acceptance

- AI-facing workflows can see what changed, why repair stopped, and whether
  another attempt is useful.

### Commit

```text
Expose repair convergence evidence
```

## Phase 9: Final Verification And Review

### Goal

Validate the full implementation and close the spec.

### Work

- Run `go test ./...`.
- Run focused CLI golden tests.
- Run focused repair, checks, board validation, and workflow packages.
- Run Prism on staged changes.
- Resolve high and medium findings before commit.
- Leave any accepted low findings documented in the final response.

### Acceptance

- Full test suite passes.
- Prism has no unresolved high or medium findings.
- Worktree is committed phase by phase.

### Commit

```text
Complete post-repair validation convergence
```
