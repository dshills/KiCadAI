# Code Review Fix Plan — 2026-07-02

Source review: `specs/CODE_REVIEW_07_02_2026.md`

Date: 2026-07-02

## Goal

Close the highest-risk findings from the July 2 code review in an order that
improves generated-design correctness first, then imported-project safety, then
repair reliability and maintainability. Every phase starts by proving the cited
bug with a focused regression test or fixture check before changing behavior.

## Strategy

- Prefer small correctness fixes before broad refactors.
- Keep generated-design correctness and KiCad writer preservation separate.
- Avoid mixing behavior fixes with command or package restructuring.
- Re-run the full Go suite and `prism review staged` before each commit.
- If a finding is stale, update this plan with the verification result instead
  of forcing an unnecessary code change.

## Phase 1 — Block Electrical Correctness

Findings: `C1`, `H1`, `H2`, related low duplication notes.

### Scope

Fix bugs that can generate electrically wrong circuit blocks before the PCB
writer or router sees them.

### Implementation Steps

1. Add failing block-realization tests for `mcu_minimal` proving the AREF
   decoupling route binds to the component whose role is
   `aref_decoupling_capacitor`.
2. Carry the component `Role` from `AddSymbolOperation` payloads through
   `componentFactsFromOperations`.
3. Update role matching to prefer exact role identity before falling back to
   symbol/footprint matching for legacy operations.
4. Add polarity tests for regulator and USB-C power LEDs:
   - supply net reaches resistor pin 1;
   - resistor pin 2 reaches LED anode pin 2;
   - LED cathode pin 1 reaches ground.
5. Fix `instantiateRegulatorPowerLED` and `instantiateUSBPowerLED` polarity.
6. Extract a small helper for power LED series wiring only if it removes the
   copy-paste without broadening the phase.

### Acceptance Criteria

- MCU minimal realization cannot bind AREF routes to VCC decoupling capacitors.
- Regulator and USB-C power LEDs are forward-biased.
- `go test ./internal/blocks` passes.

### Commit

`Fix block electrical correctness`

## Phase 2 — Routing Validation Correctness

Findings: `C3`, `H6`, routing medium/low items related to endpoint snapping and
failed route operations.

### Scope

Make route validation catch real shorts and stop false-blocking routed boards
solely because pad centers are off the routing grid.

### Implementation Steps

1. Add a regression test where an off-grid two-pad net routes successfully and
   validates as connected.
2. Replace exact route endpoint matching with explicit pad-to-snap mapping or
   a bounded tolerance derived from grid pitch and access geometry.
3. Add a regression test where two different-net segments cross at their
   interiors.
4. Update `segmentDistance` or clearance validation to return zero for segment
   intersections before endpoint-distance checks.
5. Add a test that failed/partial routes do not get applied as successful board
   route operations.
6. Review route endpoint snapping around via-split fragments and mark only
   true pad-access endpoints as snap candidates.

### Acceptance Criteria

- Off-grid but physically connected routes are not falsely marked blocked.
- Crossed traces on the same copper layer emit a blocking clearance/short
  issue.
- Failed routes do not produce board mutations that can poison later nets.
- `go test ./internal/routing ./internal/designworkflow` passes.

### Commit

`Fix routing validation correctness`

## Phase 3 — Library Metadata Compatibility

Findings: `H13`, `H14`, related KLC duplicate-pin concern.

### Scope

Make stock KiCad symbol libraries parse with useful metadata and accurate KLC
diagnostics.

### Implementation Steps

1. Add parser tests using KiCad 6+ property-form symbol metadata:
   - `Description`;
   - `ki_keywords`;
   - `ki_fp_filters`;
   - inherited symbols.
2. Fall back to property-form metadata when legacy bare nodes are absent.
3. Preserve existing `ki_datasheet` fallback behavior.
4. Add a cyclic inheritance fixture that must produce diagnostics on all
   affected symbols.
5. Fix `resolveInheritedSymbols` so nested diagnostics are not overwritten by
   outer record assignment.
6. Audit KLC duplicate-pin checks so unit/body-style scoping matches parsed
   symbol validation.

### Acceptance Criteria

- Stock-library style metadata contributes to compatibility scoring and KLC
  checks.
- Cyclic inheritance reports deterministic diagnostics.
- Multi-unit/body-style symbols do not get false duplicate-pin errors.
- `go test ./internal/libraryresolver` passes.

### Commit

`Fix KiCad symbol metadata parsing`

## Phase 4 — Request And Option Enforcement

Findings: `H9`, `H15`, `H16`, `H17`, `H18`, medium dead-knob findings.

### Scope

Close user/AI input paths where accepted options are ignored, contradictory, or
unsafe.

### Implementation Steps

1. Add an upper bound for function/interface quantities with clear validation
   issues and tests for large values.
2. Normalize intentdraft clock family output so drafted `"crystal"` maps to the
   planner-supported family.
3. Tighten CAN-bus clarification detection so ordinary English "can" does not
   trigger a blocking unsupported-interface question.
4. Fix ISP/UART programming power alias selection so shared power ports use the
   same source-derived net alias as the MCU supply.
5. Wire retry DRC evidence into production retry attempts when a KiCad runner
   is configured.
6. Add tests for `MinRoutingScoreDelta`, `StopOnRepeatedSignature`, and other
   validated retry options that currently drift from their documented behavior.

### Acceptance Criteria

- AI-facing requests cannot cause unbounded allocation through quantity.
- Natural-language clock and CAN phrases plan consistently.
- MCU programming-header flows do not split VCC aliases.
- Documented retry knobs influence retry selection or validation rejects them
  as unsupported.
- `go test ./internal/intentdraft ./internal/intentplanner ./internal/designworkflow` passes.

### Commit

`Enforce AI request options`

## Phase 5 — PCB And Schematic Round-Trip Preservation

Findings: `C2`, `H4`, `H5`, project preservation medium finding, PCB arc
visibility low finding.

### Scope

Prevent read/write operations from silently damaging KiCad-authored projects.

### Implementation Steps

1. Add KiCad-authored PCB and schematic fixtures covering:
   - footprint graphics, attributes, descriptions, tags, models;
   - slotted/oval drills;
   - zones with keepout/attributes;
   - board `general`, `setup`, and `title_block`;
   - schematic DNP, unit, instances, label shapes, and rotations;
   - `.kicad_pro` DRC/ERC/pcbnew sections.
2. Decide per node family whether to fully model or raw-preserve.
3. Ensure modeled nodes consume their raw preservation or render all parsed
   fields needed for byte-level semantic preservation.
4. Preserve unknown or unsupported project sections through `.kicad_pro`
   read/write.
5. Parse top-level PCB track arcs into connectivity-visible model data rather
   than invisible generic preservation.
6. Add round-trip tests that compare normalized semantic content and explicitly
   fail on known destructive rewrites.

### Acceptance Criteria

- Imported projects retain user-authored geometry, DNP/unit semantics, zone
  rules, board setup, and project settings after read/write.
- Slotted holes do not round-trip as circular holes.
- Track arcs remain visible to connectivity validation.
- `go test ./internal/kicadfiles/... ./internal/transactions` passes.

### Commit

`Preserve KiCad round-trip content`

## Phase 6 — Repair And Transaction Integrity

Findings: `H10`, `H11`, `H12`, transaction locking/overwrite findings.

### Scope

Make repair/apply operations all-or-nothing and ensure provenance remains valid
after post-processing.

### Implementation Steps

1. Refactor repair copy-back to stage all replacement files before touching the
   live project.
2. Add backup/rollback behavior matching the existing imported-write atomic
   pattern.
3. Keep the repair-in-progress marker until rollback/commit is fully resolved.
4. Replace the repair-loop revalidator with one that can observe remaining
   stage issues after each planned repair.
5. Recompute and rewrite generated-project manifest hashes after zone refill.
6. Apply `.kicadai.apply.lock` consistently to generated apply and repair
   persisted-apply paths.
7. Harden overwrite behavior so hand-authored projects cannot be destroyed by
   generated apply without explicit imported-mutation consent.

### Acceptance Criteria

- Failed repair apply leaves the live project unchanged.
- Multi-issue repair bundles can apply more than one planned repair in one run.
- Zone refill does not make the manifest stale.
- Concurrent apply/repair invocations cannot interleave writes.
- `go test ./internal/repair ./internal/transactions` passes.

### Commit

`Make repair apply transactional`

## Phase 7 — Fabrication And Board Validation Accuracy

Findings: `H19`, `H20`, boardvalidation and physical-rule medium findings.

### Scope

Prevent fabrication-readiness reports from passing geometry or BOM states that
are physically wrong.

### Implementation Steps

1. Add rotated-footprint DFM fixtures that compare pad/mask/courtyard
   coordinates against the same transform semantics used by board validation.
2. Normalize pad rotation handling across DFM and boardvalidation helpers.
3. Add multi-unit schematic BOM/CPL tests using a dual op-amp reference.
4. Deduplicate BOM references by reference/unit policy so multi-unit symbols do
   not inflate quantity or trigger false duplicate-reference blockers.
5. Cap or short-circuit filled-zone self-intersection checks before O(V²)
   work on large polygons.
6. Make route-completion validation prove same-net copper is one connected
   component, not just that every pad touches some copper.
7. Enforce or remove fabrication profile knobs that are currently validated but
   ignored.

### Acceptance Criteria

- Rotated pads are evaluated in the same coordinate frame everywhere.
- Multi-unit symbols produce correct BOM quantity and CPL consistency.
- Large-zone checks are bounded.
- Route-completion reports split same-net copper islands as incomplete.
- `go test ./internal/fabrication/... ./internal/boardvalidation` passes.

### Commit

`Fix fabrication validation accuracy`

## Phase 8 — CLI, Build, And Cross-Platform Hygiene

Findings: CLI findings, Windows build break, lint config, help drift.

### Scope

Make installed-binary behavior and developer validation reproducible.

### Implementation Steps

1. Add committed `.golangci.yml` with generated code exclusions and a scoped
   initial lint target.
2. Fix `go vet` copylock failures in component tests.
3. Fix Windows build break in transaction process detection with platform
   specific files or a portable abstraction.
4. Embed or explicitly package built-in block verification manifests so
   installed `kicadai block verify --builtins` works outside the source tree.
5. Update CLI help text so all active flags are documented and command-specific
   flag misuse is easier to spot.
6. Thread command contexts through long-running commands that currently ignore
   cancellation.

### Acceptance Criteria

- `make lint` has reproducible output across machines.
- `GOOS=windows go test ./internal/transactions` or at least `go test` compile
  path succeeds for the intended Windows support level.
- Installed binary workflows no longer depend on source-relative fixture paths.
- Ctrl+C reaches long-running command paths.

### Commit

`Harden CLI and build hygiene`

## Phase 9 — Structural Cleanup

Findings: `H21`, `H22`, error wrapping, package docs, observability, specs
sprawl.

### Scope

Improve maintainability after correctness fixes have landed.

### Implementation Steps

1. Split `cmd/kicadai/main.go` into per-command files with command-specific
   flag sets.
2. Extract shared request/report/type definitions needed by intent packages
   into leaf packages so intent no longer depends on `designworkflow`.
3. Move routing/check type conversions into a leaf package to remove the
   designapi/routing layering cycle-in-spirit.
4. Standardize `%w` wrapping on paths consumed by `errors.Is`/`errors.As`.
5. Add `doc.go` to large internal packages.
6. Add optional `*slog.Logger` or structured progress hooks at workflow and
   repair orchestration boundaries.
7. Generate `specs/INDEX.md` and move superseded review/roadmap files into a
   documented archive folder.

### Acceptance Criteria

- Package dependencies flow from high-level orchestration to leaf types, not
  from intent packages back into workflow orchestration.
- CLI command files are small enough to review independently.
- Specs root has an index and no ambiguous loose historical files.
- `go test ./...` passes.

### Commit

`Clean up project structure`

## Execution Notes

- Start each phase with tests that demonstrate the current failure.
- Use `prism review staged` before each commit.
- Keep unrelated review findings out of phase commits.
- Update `specs/ROADMAP.md` after phases 1, 2, 5, and 6 because those phases
  change AI-generation readiness materially.
