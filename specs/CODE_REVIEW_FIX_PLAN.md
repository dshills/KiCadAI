# Code Review Fix Plan

Source review: `specs/CODE_REVIEW.md`

Date: 2026-06-14

## Goal

Close the findings from `specs/CODE_REVIEW.md` in a safe order, with regression
tests proving each bug before or alongside the fix. The primary objective is to
make imported KiCad projects safe to read, edit, and write without parse
failures, validation failures, or silent data loss.

## Fix Strategy

The review identifies one dominant root cause: partial readers feed strict
writers. The fix direction is:

1. Fully model the KiCad nodes that the writer validates.
2. Preserve raw nodes for anything not fully modeled.
3. Add round-trip tests for every repaired reader/writer path.
4. Only then address broader atomicity, IPC, collision, placement, and cleanup
   findings.

Do not make broad refactors while repairing correctness bugs. Split large files
only after the data-loss and transaction-safety issues are closed.

## Phase 1 - Fix S-Expression Numeric and String Round-Trip

Findings: `C1`, `H2`

### Scope

Repair the S-expression parser/renderer asymmetries that make ordinary KiCad
schematic content fail or corrupt on read/write.

### Implementation Steps

1. Update `sexpr.ParsedNode.Node()` so unquoted numeric tokens are converted
   into renderable numeric node types instead of `sexpr.Atom`.
2. Preserve exact numeric spelling where practical:
   - prefer `sexpr.Fixed` or raw-preserving output for KiCad coordinates;
   - avoid lossy float reformatting in preserved embedded symbol bodies.
3. Add `\r` handling to `parseString`.
4. Add focused parser tests:
   - unquoted `0`, `1.27`, `-5.08` round-trip through `ParsedNode.Node()`;
   - embedded list containing numeric children renders without
     `ErrInvalidAtom`;
   - `"\r"` parses as carriage return and writes back as `\r`.
5. Add a schematic-level test with a minimal `lib_symbols` cache that can be
   read and written.

### Acceptance Criteria

- A parsed embedded `lib_symbols` body containing numeric coordinates can be
  rendered without atom validation errors.
- String escaping is symmetric for `\n`, `\r`, `\t`, quotes, backslashes, and
  hex escapes.
- `go test ./internal/kicadfiles/sexpr ./internal/kicadfiles/schematic` passes.

### Commit

`Fix S-expression round-trip parsing`

## Phase 2 - Read Schematic Sheets Losslessly Enough to Write

Findings: `C2`, `M5`

### Scope

Make hierarchical schematic sheets and raw schematic items survive read/write
without validation failure or disruptive ordering changes.

### Implementation Steps

1. Extend `readSheet` to parse:
   - `(size x y)`;
   - sheet pins if currently modeled by the writer;
   - sheet instances if needed for writer validation.
2. Preserve sheet properties and UUID behavior as-is.
3. Fix raw item ordering:
   - map raw item order into the same order domain as typed schematic items; or
   - store enough source order to render raw nodes near their original location.
4. Add `case "text"` in schematic read if text is modeled by the writer; if not
   fully modeled, ensure text stays as a raw item with stable order.
5. Add tests:
   - read/write a schematic with one child sheet and positive size;
   - sheet pins remain valid if parsed;
   - unmodeled raw `text` does not move ahead of all typed content.

### Acceptance Criteria

- Reading a hierarchical schematic and writing it back no longer fails with
  `sheets[...].size: positive size required`.
- Raw schematic items do not reorder into the file header area.
- Existing generated schematic tests remain green.

### Commit

`Fix schematic sheet round-trip reading`

## Phase 3 - Stop PCB Reader Data Loss

Findings: `H1`, `M6`, `M7`

### Scope

Repair PCB read/write paths that currently drop geometry and make read PCBs
unwritable or electrically misleading.

### Implementation Steps

1. Extend `readVia` to parse:
   - `size`;
   - `drill`;
   - `layers`;
   - net code/name consistently with writer expectations.
2. Extend `readZone` to parse modeled zone fields required by validation and
   rendering, or preserve the original raw zone node and skip modeled rendering
   when the zone is incomplete.
3. Extend `readDrawing` to parse every drawing kind the writer models:
   - line;
   - rect;
   - circle;
   - arc;
   - poly;
   - text.
4. Extend `readPad` where needed for modeled pad geometry, drill, layers,
   roundrect ratio, net, and UUID fields.
5. Harden `validateZone`:
   - non-zero `NetCode` must resolve to a non-empty net name;
   - explicit `NetName` must match the resolved name.
6. Review `anchorsTouch` connectivity behavior:
   - require actual shared endpoint/segment geometry for route connectivity;
   - avoid radius-sum-only connectivity masking open nets.
7. Add tests:
   - read/write PCB containing a via with size/drill/layers;
   - read/write zones without losing net name/code;
   - read/write non-line drawings;
   - connectivity does not treat nearby but disconnected same-net copper as
     connected.

### Acceptance Criteria

- A PCB containing modeled vias writes after read without zero-size/drill/layer
  validation errors.
- Modeled PCB geometry is either parsed or preserved raw; no matched node is
  silently discarded.
- Connectivity validation remains strict enough to catch open nets.

### Commit

`Fix PCB reader geometry preservation`

## Phase 4 - Make Imported Writes Transaction-Safe

Findings: `H3`

### Scope

Prevent partial on-disk updates when applying transactions to imported or
generated projects.

### Implementation Steps

1. Refactor imported writes to render every dirty file to temporary paths first.
2. Validate all rendered outputs before replacing any real project file.
3. Rename files into place only after every render succeeds.
4. On failure:
   - remove all temporary files;
   - leave existing project files unchanged.
5. Apply the same all-or-nothing discipline to generated project writes where
   manifest write failure can currently leave a partially written project.
6. Preserve existing per-file atomic write behavior internally, but orchestrate
   it at project level.
7. Add tests:
   - schematic render succeeds but PCB render fails leaves neither file changed;
   - manifest failure does not leave a misleading generated project state;
   - permissions and fsync behavior remain intact for successful writes.

### Acceptance Criteria

- Transaction apply is atomic at project-file set level for all files touched by
  one operation batch.
- Failed apply does not produce mixed schematic/PCB revisions.

### Commit

`Make transaction project writes atomic`

## Phase 5 - Fix IPC Send/Cancellation Semantics

Findings: `H4`

### Scope

Avoid discarding a KiCad IPC reply after the request has already been sent.

### Implementation Steps

1. Remove or narrow the post-`socket.Send(payload)` `ctx.Err()` check.
2. Let receive deadline/cancellation govern the post-send state.
3. Document that requests are not safely cancellable once transmitted unless
   the protocol supports correlation and drain semantics.
4. Add fake socket tests:
   - context cancellation before send still prevents send;
   - cancellation after send still attempts receive or drains according to the
     new contract;
   - send failure still closes/replaces the socket as today.

### Acceptance Criteria

- A successfully sent request is not reported as cancelled before the receive
  path has a chance to process the reply.
- REQ/REP socket state cannot be desynchronized by a narrow cancellation race.

### Commit

`Fix IPC post-send cancellation handling`

## Phase 6 - Prevent Block Reference Collisions

Findings: `H5`

### Scope

Make circuit-block instance reference generation collision-resistant and detect
any remaining collisions.

### Implementation Steps

1. Replace 16-bit truncated CRC token with a wider token:
   - full 32-bit hex token; or
   - compact base36/base32 token with at least 32 bits of entropy.
2. Add duplicate reference validation across composed block instances before
   returning operations.
3. Emit blocking `reports.Issue` values on collisions, including both instance
   IDs and conflicting references.
4. Add deterministic tests:
   - two crafted instance IDs that collide under the old 16-bit token no longer
     collide;
   - forced duplicate refs are caught by composition validation;
   - existing block examples retain stable references unless the token widening
     intentionally changes them.

### Acceptance Criteria

- Silent cross-instance reference merges are impossible in block composition.
- Any duplicate generated reference is reported before file generation.

### Commit

`Prevent circuit block reference collisions`

## Phase 7 - Harden Placement and Transaction Edge Cases

Findings: `M1`, `M2`, `M3`, `M4`

### Scope

Fix medium-severity bugs that produce misleading placement results, panics, or
stale PCB pads.

### Implementation Steps

1. Fixed placement conflicts:
   - check fixed components against occupancy and keepouts before accepting;
   - do not increment `PlacedCount` for blocked fixed parts;
   - report structured collision/outside-board issues.
2. Edge clearance:
   - separate board containment bounds from component-spacing/courtyard bounds;
   - use uninflated physical component bounds for board-edge checks;
   - use inflated bounds only for component-to-component occupancy.
3. Breakout transaction:
   - guard `req.Connectors[0]` behind `len(req.Connectors) > 0`;
   - add direct exported-function test for empty connectors with
     `GroundZone=true`.
4. Footprint replacement:
   - reconcile explicit pad payload as a full desired pad set;
   - remove pads absent from the new payload;
   - preserve stable UUID behavior for retained pads.
5. Add tests for every case above.

### Acceptance Criteria

- Fixed components cannot silently overlap or violate keepouts.
- Tight-board placement failures are not caused by double-counted edge spacing.
- `BreakoutTransaction` never panics on invalid exported input.
- Replacing a footprint with fewer pads removes stale pads.

### Commit

`Harden placement and transaction edge cases`

## Phase 8 - Clean Up Validation and CLI Inconsistencies

Findings: `M8`, `M9`, `M10`, selected Low findings

### Scope

Address maintainability issues that are low risk but make future fixes harder.

### Implementation Steps

1. Replace reflection/rune-obfuscated config field setting with direct
   assignment.
2. Extract duplicated library resolver worker-pool logic into a generic helper.
3. Replace the hand-rolled raw schematic UUID scanner with a parser walk, or
   isolate it behind tests if parser reuse is too invasive.
4. Remove confirmed dead code:
   - `samePlacementLayer`;
   - `writeOutputDir`;
   - legacy inspect scanner helpers if unused;
   - `formatVersionNumber`;
   - `schematicSymbolsByReference`.
5. Consolidate duplicated `IssueFromError` helpers where practical.
6. Make MCU missing-role issue order deterministic.
7. Replace trivial lint findings:
   - `math.Pow(x, 2)` to `x*x`;
   - `WriteString(fmt.Sprintf(...))` to `fmt.Fprintf`;
   - avoid nil context in tests.
8. Add or update tests only where behavior can regress.

### Acceptance Criteria

- `make lint` passes.
- Removed helpers are not referenced.
- Validation output order is deterministic.

### Commit

`Clean up code review maintainability findings`

## Phase 9 - Architecture Follow-Up Specs

Findings: cross-cutting architecture themes

### Scope

Do not split god files or redesign block emitters in the same commits as data
loss fixes. Create follow-up specs/plans for larger refactors after correctness
is stable.

### Deliverables

1. PCB package split plan:
   - `model.go`;
   - `read.go`;
   - `render.go`;
   - `validate.go`;
   - `preserved.go`;
   - `connectivity.go`.
2. CLI package split plan:
   - command parser;
   - per-command handlers;
   - shared JSON error envelope;
   - DTO packages.
3. Block emitter consolidation plan:
   - typed parameter validators;
   - shared component emitter;
   - shared finalization and issue handling.
4. Placement model support plan:
   - either implement or explicitly mark unsupported fields:
     `Group.KeepTogether`, `Group.MaxSpreadMM`, `Request.Seed`,
     `ExistingPlacementPolicy.PreserveFixed`, `Metrics.EstimatedBoundsCount`.

### Acceptance Criteria

- Each large refactor has its own plan and can be executed without blocking the
  data-loss fixes.

### Commit

`Plan codebase refactor follow-ups`

## Phase 10 - Final Verification

### Required Commands

Run after every implementation phase:

```sh
make lint
make test
```

Run after all phases:

```sh
make build
make coverage
go test ./...
go vet ./...
```

If `golangci-lint` is available locally, also run:

```sh
golangci-lint run ./...
```

If KiCad CLI is available, run the gated KiCad checks/round-trip tests:

```sh
KICADAI_RUN_KICAD_CLI=1 go test ./internal/kicadfiles/roundtrip ./internal/kicadfiles/checks
```

### Review Gate

For each phase:

1. Stage only files for that phase.
2. Run `prism review staged`.
3. Fix high findings and actionable medium findings.
4. Commit.

### Tracking

After fixes land, update `specs/CODE_REVIEW.md` with a short resolution table:

| Finding | Status | Fix Commit | Regression Test |
|---|---|---|---|
| C1 | fixed | `<sha>` | `<test name>` |

Do not delete the original review; keep it as audit history.
