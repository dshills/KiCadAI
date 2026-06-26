# Component Lifecycle And Availability Intelligence Implementation Plan

Date: 2026-06-26

## Implementation Rules

- Commit each phase independently after `prism review staged`.
- Keep default tests hermetic; no network, no live distributor APIs, no secrets.
- Treat local source evidence as snapshots, not real-time procurement truth.
- Preserve deterministic ordering in reports and issue lists.
- Do not weaken existing component confidence, rating, or pinmap gates.
- Do not auto-select alternate parts in this project.
- Run focused tests after each phase and `go test ./...` before closeout.

## Phase 1: Source Evidence Model And Fixtures

Objective: define and validate local lifecycle/availability source snapshots.

Tasks:

- Add source evidence structs under `internal/components` or a focused
  subpackage.
- Define enums for lifecycle status, availability status, and source confidence.
- Add loader support for a source directory with sorted JSON traversal.
- Add source file schema validation:
  - schema;
  - source ID;
  - generated date;
  - manufacturer/MPN normalization;
  - lifecycle status/confidence/source date;
  - availability status/confidence/source date;
  - duplicate records.
- Add checked-in test fixtures for:
  - valid curated source;
  - obsolete source;
  - stale source;
  - invalid status;
  - duplicate source records.
- Add unit tests for validation and deterministic ordering.

Review:

- `go test ./internal/components`
- `prism review staged`

Commit:

- Commit with a message like `Add component source evidence model`.

## Phase 2: Selection Policy Integration

Objective: join source evidence to candidates and enforce lifecycle gates.

Tasks:

- Add procurement policy structs and defaults.
- Join source evidence to concrete catalog records by normalized manufacturer
  and MPN.
- Extend selection request/options to accept source evidence and procurement
  policy.
- Add candidate issue codes for:
  - missing source;
  - blocked lifecycle;
  - stale lifecycle;
  - blocked availability;
  - stale availability.
- Apply default policy:
  - connectivity blocks EOL/obsolete;
  - fabrication-candidate requires fresh lifecycle source evidence;
  - availability warns unless explicitly required.
- Include procurement evidence in selected component/candidate output.
- Add selection tests for active, mature, NRND, unknown, EOL, obsolete, stale,
  unavailable, and missing evidence cases.

Review:

- `go test ./internal/components`
- `prism review staged`

Commit:

- Commit with a message like `Gate component selection on lifecycle evidence`.

## Phase 3: CLI Validation, Coverage, And Selection Output

Objective: expose source evidence through component CLI commands.

Tasks:

- Add CLI flag support for an explicit component source directory.
- Extend `component validate` to validate source evidence when provided.
- Extend `component coverage` with lifecycle/availability coverage counts.
- Extend `component select` JSON output with selected procurement evidence and
  rejected-candidate procurement reasons.
- Add CLI tests/goldens for:
  - source validation success;
  - invalid source failure;
  - coverage counts;
  - AP2112K selection with active lifecycle evidence;
  - obsolete candidate blocking.
- Keep CLI output deterministic and backward-compatible where possible.

Review:

- `go test ./cmd/kicadai ./internal/components`
- `prism review staged`

Commit:

- Commit with a message like `Expose component source evidence in CLI`.

## Phase 4: Design Workflow And Rationale Evidence

Objective: propagate procurement evidence into generated-project workflows and
AI-facing rationale.

Tasks:

- Add request/CLI plumbing for procurement policy and source directory in
  `design create`.
- Extend the `component_selection` workflow stage summary with lifecycle and
  availability counts, warnings, and blockers.
- Persist selected procurement evidence in workflow result output.
- Extend rationale evidence conversion so selected lifecycle/availability source
  facts appear in `design-rationale.json`.
- Add workflow tests for:
  - active sourced AP2112K selection evidence;
  - stale lifecycle warning/blocking depending on acceptance;
  - fabrication-candidate lifecycle source requirement.
- Keep missing source evidence non-blocking for draft/structural unless policy
  explicitly requires it.

Review:

- `go test ./internal/designworkflow ./internal/rationale ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Propagate procurement evidence through workflows`.

## Phase 5: Fabrication And BOM Snapshot Reporting

Objective: include local source snapshot evidence in fabrication/BOM reports
without implying live availability.

Tasks:

- Add optional lifecycle/availability fields to BOM/readiness rows where
  component identity exists.
- Mark source evidence as snapshot/local with source ID and source date.
- Add fabrication preview warnings when fabrication-candidate output lacks
  required fresh lifecycle evidence.
- Add tests for:
  - BOM row includes lifecycle source status;
  - unavailable/unknown availability appears as warning, not live failure, by
    default;
  - required availability policy blocks when requested.
- Avoid adding network or live provider assumptions.

Review:

- `go test ./internal/fabrication ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Report procurement snapshots in fabrication output`.

## Phase 6: Documentation And Roadmap Updates

Objective: document the new procurement evidence contract.

Tasks:

- Update:
  - `README.md`;
  - `docs/component-intelligence.md`;
  - `docs/fabrication.md`;
  - `docs/kicadai-agent-skill.md`;
  - `specs/ROADMAP.md`.
- Document source file schema and policy examples.
- Document that source evidence is local snapshot evidence and not live
  availability.
- Add examples under `examples/components/` if useful.
- Check documentation for stale source-tree `go run` commands.

Review:

- `rg -n "go run ./cmd/kicadai|go run" README.md docs`
- `git diff --check`
- `prism review staged`

Commit:

- Commit with a message like `Document component procurement evidence`.

## Phase 7: Final Compatibility Sweep

Objective: prove the source evidence layer does not regress existing selection
or generation behavior.

Tasks:

- Run `go test ./...`.
- Run focused component, workflow, fabrication, and CLI tests.
- Verify default tests do not require source evidence, network, or credentials.
- Verify generated JSON ordering and issue paths are stable.
- Update final status in this spec directory if implementation revealed
  follow-up gaps.

Review:

- `go test ./...`
- `prism review staged` only if files changed in this phase.

Commit:

- Commit only if final sweep requires file changes.
