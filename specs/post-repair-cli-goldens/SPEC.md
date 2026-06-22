# Post-Repair CLI Golden Fixtures Specification

## 1. Purpose

Add deterministic CLI golden coverage for post-repair validation evidence.

The post-repair validation foundation is implemented: generated-project repair
can replay transactions, run built-in post-validation adapters, compute issue
deltas, persist validation summaries, and expose repair bundle artifacts from
`design create`. The remaining gap is confidence at the external CLI boundary.
AI callers need a stable JSON contract that explains:

- what validation failed before repair;
- what repair attempted;
- what changed after repair;
- which validators ran, skipped, or blocked;
- whether the generated repair bundle is present and parseable;
- why final status is `repaired`, `partial`, or `blocked`.

This project adds golden CLI fixtures and selected-field assertions so future
changes cannot silently break that evidence contract.

## 2. Goals

This project must:

- add small, deterministic CLI fixtures for generated-project repair behavior;
- cover post-repair validation summaries from writer correctness, board
  validation, optional KiCad checks, and skipped validators where practical;
- cover validation deltas, including cleared, repeated, new, and worsened
  issues;
- prove generated repair bundle artifacts are written by `design create` repair
  flows and can be parsed by repair tooling;
- keep default tests independent of local KiCad CLI, global KiCad libraries, and
  network access;
- normalize output paths and unstable artifact locations before comparisons;
- prefer selected-field golden assertions over brittle full JSON snapshots;
- document how AI callers should interpret repair evidence.

## 3. Non-Goals

This project does not:

- add new repair executors;
- change repair planning policy;
- change post-validation adapter behavior except to expose stable evidence if
  tests reveal missing fields;
- make imported-project mutation safe;
- require real KiCad ERC/DRC in default tests;
- add fabrication export gates;
- implement natural-language design planning.

## 4. Current Baseline

Implemented foundations include:

- `repair apply` bundle and target flows;
- generated-project ownership checks and imported-project write blockers;
- persisted generated-project repair apply;
- built-in post-validation adapters for writer correctness, board validation,
  optional KiCad checks, optional round-trip checks, and validation deltas;
- `design create` validation repair stage and generated repair bundle artifact;
- workflow summaries containing repair attempt counts, validation adapter
  summaries, validation deltas, and artifact counts;
- CLI support for `design create`, `repair plan`, and `repair apply`.

Current gaps:

- no dedicated CLI golden fixture suite locks down post-repair validation JSON;
- repair bundle artifacts are not covered by a CLI parseability round trip;
- skipped and required external validator behavior is not covered at the CLI
  boundary;
- issue delta shape is covered mostly by package tests, not by end-to-end CLI
  output;
- AI-facing docs do not yet show a stable post-repair evidence interpretation
  contract.

## 5. Golden Fixture Categories

### 5.1 Clean Generated Repair Bundle

Run `design create` with validation repair bundle export enabled on a generated
fixture that reaches post-repair validation.

Expected evidence:

- `validation_repair` stage is present when repair is enabled;
- repair bundle artifact exists in the output project;
- bundle parses successfully;
- bundle identifies the project as generated;
- bundle contains transaction, stage issues, repair options, and ownership
  metadata;
- artifact paths are normalized in assertions.

### 5.2 Writer Correctness Summary

Use a small generated repair fixture that runs writer correctness after apply.

Expected evidence:

- validation list contains stable writer correctness adapter name;
- summary reports adapter status, issue counts, artifact count, and skipped
  state;
- blocking writer correctness issues affect final repair status.

### 5.3 Board Validation Summary

Use a generated PCB fixture that produces board validation evidence after
repair apply.

Expected evidence:

- validation list contains stable board validation adapter name;
- summary reports outline, pad-net, unrouted-net, route-completion, and zone
  evidence where available;
- disconnected pads or unrouted required nets remain blocking;
- schematic-only projects skip board validation with explicit non-blocking
  evidence.

### 5.4 Validation Delta Contract

Use fixtures or harnessed CLI inputs that produce known before/after issue
sets.

Expected evidence:

- delta includes cleared, repeated, new, and worsened counts where applicable;
- issue keys are deterministic and do not include absolute temp paths;
- `repaired` is reported only when required validation is clean;
- `partial` is reported only when blockers are cleared and non-blocking issues
  remain;
- `blocked` is reported for repeated or new blocking issues.

### 5.5 Optional KiCad Check Policy

Use fake or missing KiCad CLI behavior at the CLI boundary.

Expected evidence:

- optional missing KiCad CLI produces skipped validator evidence without failing
  the default test;
- required missing KiCad CLI produces a blocking issue;
- report artifact paths are retained when fake check artifacts are produced;
- default tests do not call real KiCad unless explicitly configured.

### 5.6 Target Apply Bundle Flow

Run `repair apply --target` from a generated repair bundle fixture.

Expected evidence:

- target apply uses built-in validators;
- validation summary and delta appear in CLI JSON;
- overwrite and generated-project ownership policy are enforced;
- imported or preservation-only targets remain blocked.

## 6. CLI Evidence Contract

The golden tests must preserve these fields in CLI JSON output.

For `design create`:

- `data.stages[].name`;
- `data.stages[].status`;
- `data.stages[].summary.status`;
- `data.stages[].summary.validation_count`;
- `data.stages[].summary.validation_delta`;
- `data.stages[].summary.artifact_count`;
- `data.stages[].artifacts[]`.

For `repair apply`:

- top-level `ok`;
- repair status;
- attempt and applied counts;
- validation adapter list;
- validation delta;
- artifacts;
- blocking issues surfaced in `issues`.

The tests should use helpers such as `stageByName`, `summaryField`, and
`normalizedArtifactPaths` instead of full raw JSON comparisons.

## 7. Determinism Requirements

- Fixture output directories must be under `t.TempDir()`.
- Absolute paths must be normalized before comparison.
- Timestamps, UUIDs, and operation IDs must be asserted only for presence or
  stable shape unless they are deterministic.
- Fixture request JSON must be stable and minimal.
- Tests must not depend on local KiCad library roots.
- Tests must not require real `kicad-cli` unless guarded behind explicit opt-in
  integration test configuration.

## 8. Safety Requirements

- CLI tests must not mutate imported user projects.
- Generated target apply must require explicit overwrite when rewriting output.
- Bundle parse tests must not execute arbitrary files outside the fixture
  output directory.
- Required external validation failures must remain blocking.
- Optional skipped validators must be visible, not silently ignored.

## 9. Documentation Requirements

Update README and roadmap documentation to explain:

- what post-repair validation evidence appears in CLI output;
- how AI agents should read validation deltas;
- where generated repair bundles are written;
- how to rerun `repair apply` from a bundle or target;
- which KiCad-backed checks are optional vs required.

## 10. Acceptance Gates

This project is complete when:

- `go test ./cmd/kicadai ./internal/designworkflow ./internal/repair` includes
  golden CLI coverage for post-repair validation summaries and deltas;
- at least one generated `design create` fixture emits and parses a repair
  bundle artifact;
- at least one `repair apply --target` fixture proves built-in validators are
  visible in CLI JSON;
- optional and required external validator behavior is covered without requiring
  real KiCad CLI in default tests;
- README and `specs/ROADMAP.md` identify this roadmap item as implemented and
  point to the next priority.

