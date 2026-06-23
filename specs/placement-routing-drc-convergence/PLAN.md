# Placement Routing DRC Convergence Implementation Plan

## Objective

Implement larger-board placement-routing retry convergence evidence with
optional KiCad DRC-backed attempt ranking, while keeping retry opt-in and tests
hermetic by default.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep default tests independent of real KiCad CLI.
- Keep retry disabled unless explicitly requested.
- Preserve fixed, unowned, imported, and unsupported placement refs.
- Do not claim fabrication readiness from retry or DRC evidence alone.
- Prefer extending existing design workflow retry summaries and test harnesses.
- Run focused tests after each phase.
- Run full `go test ./...` before the final documentation commit.

## Phase 1: Retry Convergence Evidence Model

### Goal

Add a richer attempt-level convergence model without changing retry behavior.

### Work

- Add attempt convergence structs in `internal/designworkflow`:
  - attempt number;
  - routing status;
  - route score;
  - routed/failed/skipped net counts;
  - placement score;
  - board-validation issue counts;
  - DRC status/counts;
  - selected flag;
  - selected reason;
  - regression flags.
- Normalize status values and zero values deterministically.
- Add stable JSON summary fields under existing retry summaries.
- Keep existing summary fields backward compatible.

### Tests

- Unit tests for convergence summary normalization.
- Existing retry summary tests remain compatible.
- JSON selected-field tests for new fields.

### Acceptance

- Retry attempts can carry richer evidence, but retry behavior is unchanged.

### Commit

```text
Add placement routing convergence evidence model
```

## Phase 2: DRC Evidence Adapter

### Goal

Introduce a DRC evidence abstraction that supports fake fixture-backed evidence
and optional real KiCad CLI checks.

### Work

- Add DRC evidence adapter interface:
  - attempt/project path input;
  - status;
  - issue count;
  - blocking count;
  - issues;
  - artifacts;
  - evidence source (`fixture`, `kicad-cli`, `missing`, `skipped`).
- Implement fake adapter for tests.
- Implement optional KiCad CLI adapter using existing checks plumbing where
  practical.
- Add policy controls:
  - disabled;
  - optional;
  - required.
- Keep missing optional evidence visible but non-blocking.

### Tests

- Fake adapter returns deterministic pass/fail/missing evidence.
- Required missing DRC is blocking.
- Optional missing DRC is visible but non-blocking.
- Real adapter can be smoke-tested only when configured.

### Acceptance

- Retry ranking can consume DRC evidence without requiring KiCad in default
  tests.

### Commit

```text
Add retry DRC evidence adapter
```

## Phase 3: Attempt Ranking And Regression Detection

### Goal

Use route, validation, and DRC evidence to select the best attempt.

### Work

- Extend best-attempt ranking with:
  - required DRC failures;
  - board-validation blocking count;
  - DRC blocking count;
  - routed required nets;
  - failed required nets;
  - route quality score;
  - placement quality score;
  - attempt number tie-breaker.
- Add selected-attempt reason strings.
- Add regression detection:
  - DRC regression;
  - board-validation regression;
  - route-quality regression.
- Add stop reasons:
  - `drc_regression`;
  - `board_validation_regression`.

### Tests

- Ranking prefers fewer validation blockers over more routed nets.
- Ranking rejects DRC regressions when DRC is required.
- Optional DRC failure affects evidence but does not crash retry.
- Earlier attempt wins deterministic ties.
- Stop reasons are stable.

### Acceptance

- Best-attempt selection is based on electrical and validation quality, not
  only route count.

### Commit

```text
Rank retry attempts with validation and DRC evidence
```

## Phase 4: Larger Generated Fixture Corpus

### Goal

Add deterministic larger generated-board fixtures that exercise convergence.

### Work

- Add fixtures under existing design workflow retry testdata patterns.
- Required fixture families:
  - `generated_multiblock_converges`;
  - `generated_multiblock_drc_regression`;
  - `generated_multiblock_no_convergence`;
  - `generated_fixed_boundary`;
  - `generated_local_route_boundary`.
- Add metadata expectations for:
  - attempts;
  - stop reason;
  - selected attempt;
  - route counts;
  - validation deltas;
  - DRC deltas;
  - moved/blocked refs;
  - local route mobility.
- Keep fixture data compact and deterministic.

### Tests

- Harness loads every fixture.
- Fixture metadata validates expected convergence fields.
- Retry output preserves fixed boundaries.
- Local route mobility remains consistent after retry.

### Acceptance

- Larger generated boards exercise retry behavior end to end without real
  KiCad.

### Commit

```text
Add larger generated retry convergence fixtures
```

## Phase 5: Workflow And CLI Evidence

### Goal

Expose convergence and DRC evidence through `design create --json`.

### Work

- Add convergence evidence to routing stage summaries.
- Include:
  - selected attempt;
  - selected reason;
  - DRC status/counts/source;
  - board validation issue deltas;
  - route-quality deltas;
  - moved/blocked refs and groups where available.
- Add selected-field CLI tests.
- Keep evidence compact and deterministic.

### Tests

- CLI JSON includes convergence fields.
- Missing optional DRC evidence is visible.
- Required DRC failure blocks acceptance.
- Existing CLI retry tests remain compatible.

### Acceptance

- AI callers can inspect why a retry attempt was selected or rejected.

### Commit

```text
Expose retry convergence evidence in design CLI
```

## Phase 6: Optional Real KiCad DRC Smoke Test

### Goal

Add a local-only smoke test that proves real KiCad DRC evidence can be attached
to retry attempts.

### Work

- Gate test behind environment variable, for example:
  `KICADAI_REAL_KICAD_CLI`.
- Use a compact generated project fixture.
- Run with timeout.
- Capture DRC status, issue counts, and report artifacts.
- Skip cleanly when KiCad CLI is unavailable.

### Tests

- Default `go test ./...` skips smoke test.
- Configured smoke test fails loudly on KiCad CLI failure.

### Acceptance

- Developers can verify real DRC-backed convergence locally without affecting
  CI/default tests.

### Commit

```text
Add optional retry DRC smoke test
```

## Phase 7: Documentation And Roadmap

### Goal

Document larger-board convergence behavior and update roadmap status.

### Work

- Update README:
  - retry convergence evidence;
  - optional/required DRC policy;
  - larger-board fixture caveats;
  - not fabrication readiness.
- Update `specs/ROADMAP.md`:
  - mark larger-board placement-routing convergence foundation implemented;
  - move next priority to BOM/CPL component identity and manufacturer profile
    evidence.
- Run full test suite.

### Tests

- `go test ./...`
- Prism review staged docs.

### Acceptance

- Documentation matches implemented behavior and points to the next roadmap
  item.

### Commit

```text
Document placement routing DRC convergence
```

## Final Acceptance Checklist

- Retry attempt summaries include richer convergence fields.
- DRC evidence adapter supports fake evidence and optional real KiCad.
- Best-attempt ranking accounts for route, validation, and DRC evidence.
- Larger generated fixtures exercise convergence and regression behavior.
- CLI JSON exposes selected-attempt reason and DRC evidence.
- Default tests pass without KiCad.
- Optional real DRC smoke coverage is available.
- README and roadmap are updated.
