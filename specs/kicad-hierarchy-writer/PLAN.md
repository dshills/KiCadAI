# KiCad Hierarchy Writer Plan

Status: Complete (2026-07-16)

## Implementation rules

- Reproduce and test the shared writer defect before changing output.
- Use a minimal KiCad-authored two-sheet project as structural reference.
- Fix the writer and reader generically; never add fixture IDs, coordinates,
  schemas, or exception lists.
- Run focused tests after each phase and the optional strict fixture after each
  writer change.
- Review staged changes with Prism before each commit.

## Phase 1: Native reproduction and golden reference — complete

- Use the checked-in KiCad-authored sensor-node hierarchy as the structural
  reference and reproduce the failure with the generated generic hierarchy.
- Run `kicad-cli sch upgrade` on the native reference and generated output.
- Record structural diffs for `sheet_instances`, child `symbol_instances`,
  and library-symbol ownership.

Acceptance: the generated fixture fails on baseline for the same reason as the
recorded generic hierarchy output; the KiCad-authored fixture succeeds.

## Phase 2: Correct hierarchy emission — complete

- Update `internal/kicadfiles/designapi` hierarchy writing to emit all required
  sheet and symbol-instance paths.
- Ensure UUID/path construction is deterministic and parent-relative.
- Correct cross-sheet label/bus ownership while retaining existing flat output.

Acceptance: generated root and every child load through `kicad-cli sch upgrade`.

## Phase 3: Reader and round-trip coverage — complete

- Make project round-trip discovery enumerate all child sheets recursively.
- Normalize and compare root plus children; report exact file/path failures.
- Add root/child zero-diff tests and malformed hierarchy fail-closed tests.

Acceptance: no hierarchy warning can mask an unchecked child sheet.

## Phase 4: Generic regulated-power promotion — complete

- Re-enable `hierarchy.mode: auto` for the recorded generic protected
  USB-C/BMP280 regulated-power graph or a new catalog-resolved generic graph
  only if it introduces necessary coverage.
- Require `linear_regulator_ideal_v1` simulation evidence and add promotion
  metadata for the simulation artifact/stage.
- Pass the fixture's declared readiness through the promotion run and replay;
  assert declared and achieved `pass` plus `matches_expectation: true`.
- Run strict ERC, DRC, routing, connectivity, writer correctness, and all
  round-trip checks.

Acceptance: promotion is `pass`, matches metadata, and has zero unexpected
findings.

## Phase 5: Delivery — complete

- Run full Go tests, lint, and strict affected fixtures.
- Review staged changes with Prism.
- Commit focused phases, push, and verify GitHub Actions.
