# KiCadAI Code Review

**Date:** 2026-06-13
**Scope:** All hand-written Go source under `cmd/` and `internal/` (~29K LOC, excluding generated protobuf in `internal/kiapi/gen` and tests).
**Method:** Subsystem-by-subsystem read, plus `go build`, `go vet`, and `golangci-lint`. Every **Critical** and **High** finding below was read and confirmed directly against the current source. One reported finding was discarded as inaccurate (see [Notes](#notes-on-verification)).

---

## Executive summary

The codebase is generally well-organized at the package level, has strong test coverage (~19K lines of tests), and builds/vets cleanly. The most serious problems are concentrated in one place and share **one root cause**:

> **The KiCad file readers are intentionally partial/lossy, but the writers validate strictly. This makes read → modify → write (the documented "imported project" workflow and the transactions apply path) fail outright or silently corrupt data on real KiCad files.**

Two of these round-trip failures are **Critical** (they make `Write` return an error after `Read` of an ordinary KiCad schematic). Several **High** issues compound them. Beyond correctness, the codebase carries a handful of 1,000–2,700-line "god files" and a large amount of copy-paste boilerplate that should be consolidated.

| Severity | Count | Theme |
|---|---|---|
| Critical | 2 | Schematic round-trip is broken |
| High | 5 | More round-trip data loss, non-atomic writes, IPC reply loss, ref-collision risk |
| Medium | ~10 | Placement geometry, transaction edge cases, validation gaps, complexity |
| Low / cleanup | ~12 | Dead code, lint findings, magic numbers |

**Top 5 to fix first:** #1, #2 (Critical round-trips), #3 (PCB read data loss), #5 (non-atomic transaction write), #6 (IPC reply discarded).

---

## Resolution tracking

| Finding | Status | Fix Commit | Regression Test |
|---|---|---|---|
| C1 | fixed | `a2417d0` | S-expression numeric/lib-symbol round-trip tests |
| C2 | fixed | `964e2af` | schematic sheet read/write tests |
| H1 | fixed | `3484530` | PCB reader geometry preservation tests |
| H2 | fixed | `a2417d0` | string escape round-trip tests |
| H3 | fixed | `7b4eaa3` | imported project atomic write failure test |
| H4 | fixed | `0a2a09d` | IPC post-send cancellation tests |
| H5 | fixed | `0f61ddb` | block reference collision tests |
| M1 | fixed | `288892c` | fixed placement collision tests |
| M2 | fixed | `288892c` | physical board-edge placement tests |
| M3 | fixed | `288892c` | empty breakout connector ground-zone test |
| M4 | fixed | `288892c` | footprint stale-pad removal test |
| M5 | fixed | `964e2af` | raw schematic item ordering tests |
| M6 | fixed | `3484530` | zone net name/code preservation tests |
| M7 | fixed | `3484530` | PCB connectivity false-positive regression tests |
| M8 | fixed | `5491737` | CLI/config tests and `make lint` |
| M9 | fixed | `5491737` | library resolver tests |
| M10 | planned | `041e24e` | schematic/package refactor follow-up plan |
| Low cleanup subset | fixed | `5491737` | `make lint` |
| Large package/CLI/block refactors | planned | `041e24e` | follow-up plans under `specs/refactor-followups/` |

---

## Critical

### C1 — Embedded `lib_symbols` cache cannot round-trip; `Write` fails after `Read`
**Files:** `internal/kicadfiles/schematic/read.go:104`, `internal/kicadfiles/schematic/schematic.go:1367`, `internal/kicadfiles/sexpr/parse.go:85`, `internal/kicadfiles/sexpr/sexpr.go:242,257`

`readLibSymbols` stores each embedded symbol body via `child.Node()`. `ParsedNode.Node()` (`parse.go:85`) classifies **every** unquoted token as `sexpr.Atom` — including numeric coordinates like `0`, `1.27`, `-5.08` that pervade symbol graphics (`(at 0 0 0)`, `(length 2.54)`, …). On write, `renderLibSymbols` re-renders that list, and `writeAtom` rejects it:

```go
func ValidAtom(value string) bool {
    return atomPattern.MatchString(value) && !numericPattern.MatchString(value)  // numerics are NOT valid atoms
}
func (r renderer) writeAtom(value string) error {
    if !ValidAtom(value) { return fmt.Errorf("%w: %q", ErrInvalidAtom, value) }  // "0" -> ErrInvalidAtom
    ...
}
```

**Impact:** Reading any real KiCad schematic that contains a populated `lib_symbols` cache (i.e. essentially all of them) and writing it back returns `ErrInvalidAtom`. This breaks the read→modify→write path that `internal/transactions/apply.go` relies on for imported projects.
**Fix:** In `ParsedNode.Node()`, classify unquoted tokens matching the numeric pattern as `Int`/`Float`/`Fixed` (or carry the original `Raw` fragment through as `sexpr.Raw`) so they re-render verbatim instead of going through atom validation.

### C2 — Sub-sheet `size` is never read; `Write` then fails sheet validation
**Files:** `internal/kicadfiles/schematic/read.go:83`, `internal/kicadfiles/schematic/schematic.go:1009`

`readSheet` reads only UUID, position, and properties — never the `(size …)` node — so every round-tripped `Sheet` has `Size = {0,0}`. The writer's validator rejects that:

```go
// schematic.go:1009
if sheet.Size.X <= 0 || sheet.Size.Y <= 0 {
    errs = append(errs, fieldError(prefix("size"), "positive size required"))
}
```

**Impact:** Reading any hierarchical schematic (one containing sub-sheets) and writing it back fails validation with "positive size required". Hierarchical designs cannot be round-tripped at all.
**Fix:** Parse the `size` child in `readSheet` (mirror `readAtPoint`); also read sheet pins/instances if full fidelity is intended.

---

## High

### H1 — PCB readers silently drop geometry → data loss and write-validation failure
**File:** `internal/kicadfiles/pcb/read.go:136` (`readVia`), plus `readZone`, `readPad`, `readDrawing`

`readVia` reads only UUID/position/net — it drops `size`, `drill`, and `layers`. `readZone` drops everything but UUID/net/name. `readDrawing` handles only `line` (rect/circle/arc/poly/text are lost). These objects are *matched* during `Read`, so they are **not** preserved as raw passthrough nodes either — the data is gone.

**Impact:** A read-modify-write cycle on a real `.kicad_pcb` silently loses geometry. Worse, a via read back has `Size==0`/`Drill==0`/empty layers, so `validateVia` then rejects it ("size must be positive", "at least two copper layers required") and the file cannot be written. The same gap makes vias electrically invisible to `ValidateGeneratedConnectivity` (empty layer list → `anchorsShareCopperLayer` always false).
**Fix:** Fully parse via/zone/pad/drawing geometry, or route incompletely-modeled top-level objects through the `Preserved`/raw-node path so they survive untouched.

### H2 — Carriage return in strings is corrupted on round-trip
**Files:** `internal/kicadfiles/sexpr/sexpr.go:282`, `internal/kicadfiles/sexpr/parse.go:159`

`quoteString` emits `\r` for `'\r'`, but `parseString` has no `case 'r'` — it falls through `default` and decodes `\r` as the literal byte `'r'`:

```go
// write: case '\r': b.WriteString(`\r`)
// parse: cases handle n, t, ", \, x ... default: builder.WriteByte(esc)  // 'r' -> "r"
```

**Impact:** Any string value containing a carriage return is silently mangled (`"\r"` → written `\r` → read back as `"r"`). Asymmetric escape table.
**Fix:** Add `case 'r': builder.WriteByte('\r')` to `parseString`.

### H3 — Imported-project write is not atomic across files
**File:** `internal/transactions/apply.go:639` (`writeImportedProject`)

The function writes the schematic first, then the PCB, each "atomically" on its own:

```go
if schematicDirty { ... writeSchematicAtomic(path, *design.Schematic) ... }  // committed to disk
if pcbDirty       { ... writePCBAtomic(path, *design.PCB) ... }              // may fail here
```

**Impact:** If the schematic write succeeds and the PCB write fails, the on-disk schematic is updated while the PCB is not — leaving the imported project internally inconsistent (footprints referencing changed symbols, etc.). Atomicity is per-file, not per-transaction. The same shape exists in the generated path (`apply.go:74`): files are committed before `writeManifestForApply`, which on failure leaves a written-but-unmanifested project that changes behavior on re-run.
**Fix:** Render all files to temp paths first; rename them only after every render succeeds; on any error, remove all temps and write nothing.

### H4 — IPC reply discarded after a successful send on context cancellation
**File:** `internal/ipc/mangos.go:139`

```go
if err := socket.Send(payload); err != nil { ... return ... }  // request already delivered to KiCad
if err := ctx.Err(); err != nil {                              // if cancelled here:
    t.closeSocketAfterInterruptedRequest(socket)               // close socket, drop the reply
    return nil, err
}
```

**Impact:** A request that has already been transmitted (and possibly mutated KiCad state) is reported to the caller as cancelled/failed. Because this is a REQ/REP socket, dropping the reply and reconnecting also risks desynchronizing request/reply pairing on the next call.
**Fix:** Remove the post-`Send` `ctx.Err()` check and let the receive deadline govern cancellation, or explicitly document that a request is not safely cancellable once sent.

### H5 — Per-instance reference token is only 16 bits with no collision check
**File:** `internal/blocks/emitter.go:39`

```go
token := fmt.Sprintf("%04x", uint16(crc32.ChecksumIEEE([]byte(instanceID))))
```

The full CRC32 is truncated to 16 bits, and neither `ComposeBlocks` nor `project.go` checks for cross-instance reference collisions.

**Impact:** With a 16-bit token, two instances can produce identical reference designators (e.g. `U1a2b001`). Two colliding refs across instances in one project yield an invalid schematic/netlist (components silently merged). Collision probability is low at a handful of instances but rises sharply with scale (~50% near ~300 instances) and the failure is silent.
**Fix:** Use the full 32-bit token (or a base36 encoding) **and** add a cross-instance duplicate-ref check that emits a blocking issue.

---

## Medium

### Bugs

- **M1 — Fixed components bypass overlap checking at placement time.** `internal/placement/placer.go:44,97`. `placeComponent` returns immediately for `Fixed` components without consulting occupancy, and the loop unconditionally increments `PlacedCount` and adds them to occupancy. A fixed component overlapping another component or a keepout is reported as "placed"; the overlap is only surfaced afterward by `ValidateGeometry` (which then downgrades status to `Partial`). Fixed-vs-fixed overlaps are never prevented, and `PlacedCount` is misleading. *Fix:* check `occupancy.FirstConflict` for fixed components too and mark them blocked on conflict.

- **M2 — Edge clearance is conflated with component spacing/courtyard.** `internal/placement/geometry.go:45`, used at `placer.go:139`. `ComponentPlacementBounds` inflates bounds by `ComponentSpacingMM/2 + CourtyardMM`, and these inflated bounds are also tested against `BoardUsableRect`, which **already** subtracts `BoardEdgeClearanceMM`. Net effect: a component's effective edge clearance is `BoardEdgeClearanceMM + ComponentSpacingMM/2 + CourtyardMM`, shrinking usable area and causing spurious "no legal placement found" on tight boards. *Fix:* use un-inflated bounds for the board-containment test and inflated bounds only for component-to-component conflicts (or document the combined semantics deliberately).

- **M3 — `BreakoutTransaction` panics on empty connectors.** `internal/generate/breakout.go:143`. The exported function indexes `req.Connectors[0].Pins` when `req.GroundZone` is true, guarded only by `hasPin(...)` — no `len(req.Connectors) > 0` check. Safe only because the in-package caller validates first; a direct caller with `GroundZone=true` and no connectors panics. *Fix:* `if req.GroundZone && len(req.Connectors) > 0 && hasPin(...)`.

- **M4 — Footprint re-placement is additive-only.** `internal/transactions/apply.go:545` (`updateImportedFootprint`). Pads are upserted by name but pads absent from the new payload are never removed, so re-placing a footprint with fewer pads leaves stale pads (old nets/positions → phantom connections). *Fix:* reconcile the full pad set, deleting unreferenced pads.

- **M5 — Raw schematic items are reordered on round-trip.** `internal/kicadfiles/schematic/read.go:141`. `rawItem` sets `Order` to the small parse index (6–25), while typed items use `int(kind)*1000` (10000+). On render, raw items therefore sort before all typed items. Anything not modeled by a typed slice — including `text` items, which `Read` has no case for — is hoisted to the top of the file. *Fix:* map raw items into the same ordering space as typed items; add a `case "text"` in `Read`.

- **M6 — Zone net code/name consistency not validated on write.** `internal/kicadfiles/pcb/pcb.go:1406`,`2416`. `renderZone` emits `(net <code>)` and `(net_name <name>)` where the name is looked up from `netNames[zone.NetCode]`, but `validateZone` only checks an *explicit* mismatched `NetName`; a non-zero code that maps to `""` renders `(net_name "")`, which KiCad treats as corrupt. *Fix:* require non-zero `NetCode` to resolve to a non-empty name.

- **M7 — Over-eager connectivity via radius-sum fallback.** `internal/kicadfiles/pcb/connectivity.go:469`. `anchorsTouch` connects any two anchors with `radius>0` when centers are within `a.radius + b.radius + tolerance` (track endpoints use `width/2`). Two unrelated same-net endpoints that merely come within the sum of half-widths are treated as electrically connected, which can mask genuine open nets. *Fix:* require point coincidence / actual segment touch for endpoints rather than the inflated radius sum.

### Architecture / complexity

- **M8 — Reflection-with-rune-encoded-strings to set one config field.** `cmd/kicadai/main.go:~2205` (`setConfigField`/`credentialFieldName`). Uses reflection plus literals built from rune slices (`string([]rune{84,111,107,101,110})` = `"Token"`) to assign a single known struct field. A rename becomes a silent runtime error instead of a compile error. *Fix:* `explicit.Token = opts.apiCredential`.

- **M9 — Duplicated parser worker pools.** `internal/libraryresolver/footprint_parser.go:60` and `symbol_parser.go:57` are near-identical GOMAXPROCS worker pools differing only in callback/result type — and they've already drifted (one presizes its map, the other doesn't). The concurrency logic (the most bug-prone code here) is copy-pasted. *Fix:* extract `parseFiles[T any](ctx, files, fn) []T`.

- **M10 — Hand-rolled S-expression scanner duplicating the real parser.** `internal/kicadfiles/schematic/schematic.go:1262` (`rawSchematicItemUUIDs`) re-implements string/escape scanning by byte offset (`i+5`, magic `(uuid` matching) instead of using `sexpr.Parse`. Fragile and redundant. *Fix:* parse the raw body once and walk `uuid` children.

---

## Low / cleanup

Confirmed by `golangci-lint` (`go build` and `go vet` are clean):

- `internal/blocks/composition.go:270` — ineffectual assignment to `parent` (ineffassign).
- `internal/blocks/regulator.go:190` — `math.Pow(x, 2)` should be `x*x` (staticcheck QF1005).
- `internal/placement/geometry.go:240` — `samePlacementLayer` is unused (dead code).
- `internal/transactions/apply.go:295` — `writeOutputDir` is unused (dead code).
- `internal/transactions/apply.go:809` — `WriteString(fmt.Sprintf(...))` → `fmt.Fprintf` (staticcheck QF1012).
- `internal/blocks/registry_test.go:301` — passing `nil` Context (staticcheck SA1012).

Additional dead/duplicate code found by reading:

- `internal/inspect/inspect.go:227,351-526` — `copyCounts` and the entire legacy S-expr scanner (`scanSchematic`, `discardLine`, `readAtom`, …, ~180 lines) are unused; inspection uses the structured reader now.
- `internal/kicadfiles/project/project.go:320` — `formatVersionNumber` unused (and the format version is never actually written into project `meta`).
- `internal/kicadfiles/design/design.go:438` — `schematicSymbolsByReference` unused (duplicates an inline map built elsewhere).
- Two different `IssueFromError` functions (`internal/reports/issue.go:53` vs `internal/evaluate/evaluate.go:243`) with different signatures — consolidate.
- Duplicated multiplier tables in `internal/blocks` (`voltageMultipliers`/`currentMultipliers` are byte-identical and rebuilt per call) — make package-level vars.
- `renderSymbolInstances`/`renderSheetInstances` (`schematic.go:1557`,`1764`) are near-identical — extract a shared grouping helper.

Other low-severity observations:

- **Nondeterministic validation output:** `internal/blocks/mcu.go:337` iterates a `map[string]string` literal to emit missing-role issues; map order is randomized, breaking reproducible/diffable output. Iterate an ordered slice.
- **Latent int64 overflow / inconsistency:** `internal/kicadfiles/pcb/connectivity.go:759` `orientationSign` computes the cross product in raw `int64`, while `collinear` (`pcb.go:2342`) does the analogous triple product in `math/big`. Overflow is not reachable at realistic board sizes (it needs coordinate spans beyond ~2 m of nanometer IU), but the inconsistency is a robustness smell — use `big.Int` (or float + epsilon) in both.
- **Inconsistent params ownership in blocks:** `led/regulator/opamp/connector` assign `output.Instance.Params = params` directly (and some mutate `params` in place) while `mcu/i2c_sensor` defensively clone. Currently safe only because `normalizeRequest` allocates fresh maps; standardize on cloning to avoid a future aliasing bug.
- **`--json` error envelope is inconsistent across CLI commands:** several handlers (e.g. `runConfig`, the `runDocuments` document-type parse path) return bare errors even under `--json`, unlike the rest. Route all command failures through the structured-error helper.
- **Non-standard net encoding:** the PCB writer emits `(net "<name>")` instead of KiCad's `(net <code> "<name>")`. Self-consistent within this repo, but a divergence from the on-disk KiCad format worth flagging if interop with stock files is a goal.

---

## Cross-cutting architecture themes

1. **Partial readers + strict writers = fragile round-trips (root cause of C1, C2, H1).** The single most valuable structural fix is to decide on one contract: either (a) readers fully model every field the writers validate, or (b) anything not fully modeled is preserved as raw passthrough so it survives a round-trip untouched. Right now the code does neither consistently, which is why ordinary KiCad files fail to round-trip. Add round-trip tests that read real KiCad 9/10 sample files and re-write them.

2. **God files.** Several files mix data model, validation, serialization, and geometry in one place and should be split along those seams (the package already splits some concerns, so the precedent exists):
   - `internal/kicadfiles/pcb/pcb.go` — **2,735 lines** → `model.go` / `render.go` / `validate.go` / `order.go` / `preserved.go`.
   - `cmd/kicadai/main.go` — **2,265 lines** mixing ~20 command handlers, result DTOs, validation, and reflection helpers → per-command files or an `internal/cli` package; move DTOs out of `main`.
   - `internal/kicadfiles/schematic/schematic.go` — **1,864 lines** → `model.go` / `validate.go` / `render.go` / `connectivity.go` / `rawitems.go`.
   - `internal/kicadfiles/designapi/builder.go` (1,354), `internal/kicadfiles/pcb/connectivity.go` (1,040), `internal/transactions/apply.go` (925) are also large but more cohesive.

3. **Copy-paste boilerplate.** The per-block instantiators (`led.go`, `regulator.go`, `mcu.go`, `usb_c_power.go`, `i2c_sensor.go`, `opamp.go`, `connector.go`, ~1,700 lines total) repeat the same scaffold (blocking-issue gate → per-param `parseUnit` validation → allocator → operation/issue/ref accumulation → finalize), and already diverge in small ways (`issues` vs `issuesOut`, clone vs no-clone). Extract a typed parameter validator, a `componentEmitter`, and a finalize helper. The duplicated CLI command scaffolding (`runEvaluate`/`runInspect`/`runCheckCommand`) and duplicated artifact-workspace path logic (`checks/artifacts.go` vs `roundtrip/artifacts.go`, which also disagree on `EvalSymlinks`) are smaller instances of the same problem.

4. **Leaky abstraction in the placement model.** `internal/placement/model.go` validates and exposes `Group.KeepTogether`, `Group.MaxSpreadMM`, `Request.Seed`, and `ExistingPlacementPolicy.PreserveFixed`, but `placer.go` never consumes them (and `Metrics.EstimatedBoundsCount` is never incremented). Either implement these or remove/clearly mark them as unsupported so callers aren't misled.

---

## Notes on verification

- All **Critical** and **High** findings were confirmed by reading the cited code directly (escape tables, `ValidAtom`/`writeAtom`, `validateSheet`, the read functions, `writeImportedProject`, `mangos.go` send path, `emitter.go`).
- One subsystem review claimed the placement metrics are derived by `strings.Contains` on issue messages. This is **incorrect** for the current code — `placer.go:58-62` counts on structured `reports.CodePlacementCollision` / `reports.CodePlacementOutsideBoard` codes — so it was excluded from this report.
- `go build ./...` and `go vet ./internal/...` both pass with no output. The Low-severity lint items above come from `golangci-lint`.
