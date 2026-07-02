# Code Review — 2026-07-02

Scope: full-repository review of non-generated Go code (~101k production LOC, ~66k test LOC across `cmd/kicadai` and ~47 internal packages). Generated protobuf code under `internal/kiapi/gen` and vendored snapshots under `third_party/` were excluded. The review was performed by ten parallel review passes, each covering a coherent package cluster (kicadfiles, designworkflow, blocks, fabrication/validation, placement/routing, repair/transactions, intent/AI, components/library, CLI + small packages, and a cross-cutting architecture pass). Every finding below was verified against the surrounding code; several were verified empirically with throwaway tests.

Note: at review time the working tree contained staged, uncommitted changes (`internal/designworkflow/routing.go`, `internal/designworkflow/routing_test.go`, and the new `specs/vcc-route-tree-path-completion/` spec). The review covers the tree as it stood, including those changes.

## Verification Run

- `go build ./...` — passed.
- `go test ./cmd/... ./internal/...` — all tests pass.
- `go vet ./cmd/... ./internal/...` — 2 findings (copylocks in `internal/components/model_test.go:45,144`: `Catalog` embeds `sync.RWMutex` and is copied by value).
- `golangci-lint run ./cmd/... ./internal/...` — 82 issues: errcheck 20, unused 37, staticcheck 17, ineffassign 6, govet 2. No committed `.golangci.yml`; `make lint` runs only gofmt + go vet, so lint results are not reproducible across machines.

Coverage highlights (`go test -cover`): most packages sit in the 75–90% band. Extremes:

| Lowest | | Highest | |
|---|---|---|---|
| `internal/inspect` | 61.9% | `internal/config` | 100.0% |
| `internal/kicadfiles/roundtrip` | 64.1% | `internal/amplifiers` | 97.9% |
| `internal/boardvalidation` | 68.7% | `internal/componentprops` | 96.6% |
| `internal/ipc` | 68.9% | `internal/kicadfiles` | 94.0% |
| `internal/pcbrules` | 71.0% | `internal/schematic` | 93.5% |
| `cmd/kicadai` | 71.8% | `internal/schematicrules` | 92.7% |

## Executive Summary

The codebase is disciplined in the large: deterministic output everywhere (stable sorts, tie-breakers, UUIDv5 identity), defensive cloning, issue-based error reporting instead of panics, atomic single-file writes (temp + fsync + rename), bounded searches, and race-free bounded concurrency. Tests are extensive (0.65 test:prod LOC ratio) and all pass.

The serious defects cluster in five areas:

1. **Electronics data correctness** (`internal/blocks`) — a role-binding heuristic routes the MCU AREF net to the wrong capacitor (shorting VCC to AREF on the board), and two copy-pasted power-LED subcircuits are reverse-biased.
2. **Read→write round-trip data loss** (`internal/kicadfiles`) — the readers model only a subset of what the writers emit, so rewriting an imported KiCad project silently strips silkscreen/courtyard/3D models, converts slotted holes to round holes, clears DNP flags, collapses multi-unit symbols, and wipes DRC/ERC settings from `.kicad_pro`. The `Raw` preservation fields exist but no writer consumes them.
3. **Grid/continuous boundary bugs in routing validation** (`internal/routing`, `internal/designworkflow`) — the connectivity validator requires pad centers to lie exactly on the routing grid, so realistic boards are falsely reported `blocked`; conversely the clearance validator has no segment-intersection test, so crossed traces (dead shorts) pass.
4. **Repair pipeline integrity** (`internal/repair`, `internal/transactions`) — the copy-back into the live project is not transactional, the repair loop's revalidator cannot observe the issues being repaired (so only one repair per invocation ever applies), and zone refill invalidates the provenance manifest that gates future repairs.
5. **Real-library compatibility** (`internal/libraryresolver`) — symbol metadata is parsed only in a legacy format that no KiCad 6–9 library uses, degrading compatibility scoring and producing false KLC errors against stock libraries.

Architecturally, `internal/designworkflow` has grown into a 26.8k-LOC god package (fan-out 21 internal packages, fan-in from the intent layer — an inverted dependency), `cmd/kicadai/main.go` is a 4,867-line monolith with one shared ~80-flag namespace, and only ~20% of `fmt.Errorf` calls use `%w` despite 86 `errors.Is/As` call sites.

A recurring meta-pattern across findings: **options that are parsed, validated, and documented but never enforced** (retry DRC policy, `MinRoutingScoreDelta`, `StopOnRepeatedSignature`, `RequireCourtyard`, `WarningOnlyFields`, `AllowPassiveRules`, `ModeValidateOnly`). Users setting these get silently different behavior than requested. A schema-to-consumer audit would be worthwhile beyond the specific instances listed below.

---

## Critical Findings

### C1 — Wrong-net PCB route: AREF decoupling cap binds to a VCC cap

`internal/blocks/realize.go:581` — `roleRefsFromOutput` matches roles to emitted components with a first-unused heuristic on symbol+footprint only (`matchingRefForComponent`). For `mcu_minimal` with defaults, three identical VCC caps are emitted before the AREF cap, so role `aref_decoupling_capacitor` binds to a VCC decoupling cap. Verified empirically: realization emits route `mcu_aref_decoupling` (net `..._aref`) from a capacitor whose schematic pins are on VCC/GND to MCU pin 20 (AREF) — shorting VCC to AREF on the board — and the real AREF cap is never placed. The exact `Role` is already present in every `AddSymbolOperation` payload but `componentFactsFromOperations` (`realize.go:531`) discards it.

**Fix:** carry `Role` through `componentFactsFromOperations` and match on it directly.

### C2 — PCB read→write round-trip silently destroys imported board content

`internal/kicadfiles/pcb/read.go:90` — `readFootprint` parses only `at`/`layer`/`path`/`property`/`pad`, dropping `fp_text`, `fp_line`/`fp_rect`/`fp_circle`/`fp_arc`/`fp_poly`, `model`, `attr`, `descr`, `tags`; `readZone` (`read.go:227`) drops `keepout` and `attr`; `Read` (`read.go:70`) discards `general`, `setup`, `title_block`, which `render` replaces with `DefaultGeneral()`/`DefaultSetup()`. `Footprint.Raw`/`Pad.Raw`/`Zone.Raw` are populated but never consumed by any writer. Reading a KiCad-authored board and writing it back (the imported-project path in `internal/transactions/apply.go` `writeImportedProject`) silently strips all silkscreen, courtyard, fab outlines, 3D models, and keepout rules, and resets board thickness/plot/DRC setup to tool defaults.

**Fix:** wire `Raw` preservation through the writers for modeled nodes, or extend the readers to model what the writers emit. Add a byte-level round-trip regression test on a KiCad-authored fixture.

### C3 — Routing connectivity validation falsely blocks realistic boards

`internal/routing/validation.go:161` — `routeEndpointsConnected` matches pad access points against route points via `nearestKey` with tolerance `distanceEpsilonSquared` = (1e-12)² mm; route endpoints are grid-snapped (0.25mm default) while access points are exact pad centers. Any pad whose center is off-grid (0805 offsets of ±0.95mm, 1.27mm-pitch parts, any placement + footprint-offset combination) mismatches, producing a blocking `DISCONNECTED_PAD` issue, and `RouteRequestContext` (`route.go:164`) flips a fully routed result from `routed` to `blocked`. Verified empirically: a 2-net board with ±0.95mm pad offsets routes successfully yet returns overall `status=blocked`. The same root cause falsely fails nets whose two endpoints snap to the same grid cell (empty segment list → empty union-find → "disconnected").

**Fix:** match endpoints with a tolerance of at least half the grid pitch (or track the pad→grid-snap mapping explicitly).

---

## High Findings

### Blocks (circuit definitions)

- **H1** `internal/blocks/regulator.go:321` — Regulator power LED is reverse-biased and can never light: `instantiateRegulatorPowerLED` wires R.2 → LED pin 1 (cathode, per the package's own comment at `led.go:131`) and LED pin 2 (anode) → GND. The correct polarity exists in `instantiateLEDIndicator` (`led.go:100-106`).
- **H2** `internal/blocks/usb_c_power.go:262` — USB VBUS power LED has the same reversed polarity (copy of H1). The LED is reverse-biased across VBUS and never illuminates.
- **H3** `internal/blocks/builtin_realization.go:80` — `voltage_regulator` realization hardcodes the AMS1117 profile even though `regulatorProfileFor` (`regulator.go:186`) also supports AP2112K-3.3/SOT-23-5. With the AP2112K profile the symbol match fails, the regulator is never placed, and both required bypass routes fail. Even if matching were fixed, the realization routes reference pins "3"/"2", which on the AP2112K are EN and GND, not VIN and VOUT.

### KiCad file round-trip

- **H4** `internal/kicadfiles/pcb/read.go:321` — `readPadDrill` collapses oval/slot drills to a single circular diameter: `(drill oval 0.6 1.0)` reads as the larger dimension and `renderPad` (`render.go:411`) writes back `(drill 1)`. Round-tripping a board with slotted holes (common on connectors) silently converts slots into oversized round holes — a physical geometry change caught only at fabrication.
- **H5** `internal/kicadfiles/schematic/read.go:186` — Schematic round-trip clears DNP flags, collapses multi-unit symbols, and resets label shapes: `readSymbol` never reads `dnp`, `unit`, `exclude_from_sim`, or `instances`, but `renderSymbol` (`schematic.go:1449`) unconditionally writes `(unit 1)` and `(dnp no)`; `readLabel` (`read.go:293`) drops label `shape` and rotation. A Do-Not-Populate component in an imported schematic becomes populated after rewrite.

### Routing / designworkflow

- **H6** `internal/routing/validation.go:212` — `segmentDistance` has no segment-intersection test, so the clearance validator misses crossed traces (dead shorts). Two crossing segments of different nets return the minimum endpoint-to-segment distance, which can far exceed the clearance rule; `ValidateResult` — the exported independent DRC backstop — silently passes copper that is physically shorted.
- **H7** `internal/designworkflow/routing.go:977` — Endpoint snapping assumes every route operation spans pad-to-pad, but the router splits routes into per-layer/per-width fragments at each via (`internal/routing/operations.go:66`). Any two-layer inter-block route with a via produces a fragment ending mid-board → spurious `SeverityBlocked` snap issue that fails the routing stage; and if a mid-route fragment endpoint happens to fall within 0.75mm of a pad, it is silently relocated, disconnecting it from the adjacent fragment. Only access-pair branch routes are marked `SnapExempt`.
- **H8** `internal/designworkflow/route_endpoint_resolver.go:78` — Duplicate pad names on one footprint produce a blocking "duplicate normalized pad endpoint" issue. Footprints legitimately repeat pad numbers (thermal tabs); the package's own verified template `SOT-223-3_TabPin2` (`pad_hydration.go:171`) has two pads named "2", so any design hydrated from it fails the routing stage. The sibling path `assignPadNetsFromIndex` (`pad_hydration.go:267`) explicitly supports duplicate pad names.
- **H9** `internal/designworkflow/retry_drc_evidence.go:92` — The retry-DRC evidence pipeline is dead code in production: `retryDRCEvidenceForAttempt` / `applyRetryDRCEvidenceToAttempt` / `kicadRetryDRCEvidenceAdapter` are invoked only from tests, never from `maybeRetryPlacementRouting`. Every attempt has `DRCStatus=skipped`, so `routing_retry.drc_policy: "required"`, the `drc_regression` flag, and the DRC stop condition are silently inert while the request schema accepts them.

### Repair pipeline

- **H10** `internal/repair/persisted.go:420` — Repair apply's copy-back from the stage into the live project is not transactional: `replaceGeneratedOutput` copies per-file and `removeStaleGeneratedFiles` deletes old files mid-sequence with no backup/rollback. A failure partway (disk full, permissions) leaves the user's project as a mix of old and new files, and the `.kicadai/repair-in-progress` marker is removed even on the error path (`defer os.Remove(marker)`, line 428). Contrast `writeImportedProjectFilesAtomic` and `WriteProjectDirectory`, which both implement backup/rollback.
- **H11** `internal/repair/persisted.go:152` — The repair loop's revalidator is `transactions.Validate(tx)`, which is guaranteed to return zero issues for any bundle that reached this point (`ValidateBundle` rejects bundles whose transaction has any issue). After the first executed attempt, `Runner.RunContext` (`runner.go:92`) sees an empty validation, returns `StatusRepaired`, and silently skips every remaining planned repair — a bundle with N stage issues gets exactly one repair per invocation while claiming "repaired".
- **H12** `internal/repair/persisted.go:189` — Zone refill self-invalidates provenance: `RunZoneRefill` rewrites the live `.kicad_pcb` after `replaceGeneratedOutput` wrote the manifest, so the board no longer matches `manifest.FileHashes`, the manifest becomes `Stale`, `HydrateTarget` reports the project as non-generated, and all subsequent repair applies and bundle exports are blocked. No code path re-writes the manifest after refill.

### Library resolution / components

- **H13** `internal/libraryresolver/symbol_parser.go:163` — Symbol description/keywords/footprint-filters are never parsed from real KiCad 6–9 libraries: `firstSymbolText(node, "ki_description")` etc. only match legacy bare top-level nodes, but real libraries store these as `(property "Description" ...)` / `(property "ki_keywords" ...)` / `(property "ki_fp_filters" ...)` (verified against the installed KiCad 9 `Device.kicad_sym`: zero bare-form occurrences). Consequences: `footprintFilterMatches` never fires so real-library passives can never reach `CompatibilityCompatible`; `ValidateSymbolKLC` emits false S4.1/S4.2 warnings for every stock symbol; `allowsStackedSymbolPins` never applies, so stacked-pin symbols produce false duplicate-pin errors. The code already falls back to `Properties["ki_datasheet"]` for datasheet — the same fallback is missing for the other three fields.
- **H14** `internal/libraryresolver/symbol_parser.go:204` — Cyclic symbol-inheritance diagnostics are silently lost: in `resolveInheritedSymbols`, the cycle issue is appended inside the nested `resolve` call, but the outer frame overwrites `records[index]` with a pre-diagnostic copy and replaces `Diagnostics` entirely. Verified: an `A extends B / B extends A` library indexes both symbols with zero issues and bogus half-merged pin sets.

### Intent / NL adapter

- **H15** `internal/intentplanner/map.go:398` — No upper bound on `Quantity`: `{"kind":"indicator","quantity":1000000000}` passes `ValidateRequest` (only rejects `< 1`) and `mapFunctions`/`mapInterfaces` loop a billion times allocating per-iteration — OOM/hang from a tiny JSON request, reachable from user/AI input via `DecodeRequestStrict` → `Plan`.
- **H16** `internal/intentdraft/extract.go:120` — Drafted clock family `"crystal"` is rejected by the planner (`mapFunctions` accepts only `""`/`"crystal_oscillator"`/`"canned_oscillator"`, `map.go:387`), so every NL request mentioning a frequency — including the exact string in `extract_test.go:32` — drafts fine but is blocked in `Plan`. No test covers the draft→plan seam.
- **H17** `internal/intentdraft/clarify.go:19` — The common English word "can" triggers a blocking CAN-bus clarification (`containsAny(normalized, "can", ...)` uses word-boundary matching), so phrasing like "a board that can read a temperature sensor" prevents planning entirely with a misleading question about unsupported interfaces.
- **H18** `internal/intentplanner/map.go:697` — ISP/UART programming `power.vcc` net alias is derived from the target (consumer) ID, falling through to `"VCC_mcu"` (`map.go:937`), while the MCU's supply connection to the same port uses the source-derived alias (e.g. `"VCC_5v"`). Two aliases on one port later fail with SeverityError "conflicting PCB fragment net aliases" (`designworkflow/pcb_fragments.go:214`) on the flagship MCU+programming-header flow.

### Fabrication / DFM

- **H19** `internal/fabrication/physicalrules/dfm_observations.go:262` — Pad rotation is applied in the opposite direction from every other transform in the codebase: `dfmTransformPadOpeningPoints` rotates by −θ (and a test pins this as "the KiCad direction") while `transformedOffset`, `transformedPadBounds`, `padInside`, `silkscreenTextInsideBoard`, and `boardvalidation.absolutePadPosition` all rotate by +θ. Within `dfmPadMaskOpening` the pad rotation (−θ) composes with the footprint rotation (+φ), yielding R(φ−θ) instead of R(φ+θ). Rotated footprints on non-symmetric boards get containment/edge-clearance/courtyard/mask verdicts computed at mirrored positions.
- **H20** `internal/fabrication/reports.go:285` — Multi-unit symbols inflate BOM quantity and trigger a false blocking error: `BuildBOMRows` appends one row-reference per schematic symbol instance with no de-dup by reference/unit, so a dual op-amp (U1 units A/B) becomes `References: ["U1","U1"], Quantity: 2`, and `ValidateBOMCPLConsistency` (`reports.go:393`) emits SeverityError `CodeDuplicateReference` — blocking fabrication readiness for any design using multi-unit parts.

### Architecture

- **H21** `internal/designworkflow` — God package at the center of the dependency graph: 26,834 LOC across 40 production files, imports 21 internal packages, and is imported by `intentplanner`, `intentdraft`, `rationale`, and `cmd` — the intent layer depending on the workflow orchestrator inverts the expected direction. Extracting the shared types the intent packages need into a leaf package would break the inversion.
- **H22** `internal/kicadfiles/designapi` ↔ `internal/routing` — Layering cycle-in-spirit: `designapi/builder.go:16` (low-level file construction) imports the routing engine, while `routing/checks.go` imports `kicadfiles/checks` (a package that shells out to kicad-cli) purely for type conversion. Both edges exist only for type sharing; moving the shared types into `reports` or a leaf package restores clean layering.

---

## Medium Findings

### designworkflow

- `placement_routing_retry.go:378` — `boardValidationCountsFromRoutingStage` reads summary keys (`board_validation_issue_count`/`board_validation_blocking`) that no production code writes; the first-priority retry comparator criterion and the `board_validation_regression` stop condition always compare 0 == 0.
- `request.go:133` — `MinRoutingScoreDelta` and `StopOnRepeatedSignature` are normalized, validated, and documented but never consumed (`placement_routing_retry.go:556` uses exact float inequality; the repeated-state stop runs unconditionally).
- `planner.go:21` — Every request-validation failure is reported twice in the block-planning stage (`PlanBlocks` appends `ValidateRequest` issues, then `ToCompositionRequest` re-runs it and appends identical issues).
- `component_selection.go:288` — In `matchingWorkflowRefForComponent`, a role mismatch does not exclude a ref: when the role fast-path fails, a ref with a different recorded role still matches on symbol/footprint (e.g. two capacitors with roles `input_cap`/`output_cap`), so a selection for one role can rewrite another component's value/footprint — silently wrong BOM.
- `block_evidence.go:209` — `builtinVerificationRoot` locates verification manifests via `runtime.Caller(0)` + a source-relative path; installed binaries silently degrade to "no built-in verification evidence" and fabrication-candidate requests get spurious errors, gated only by the undocumented `KICADAI_BLOCK_VERIFICATION_ROOT` env var.
- `StageResult.Summary` (`map[string]any`) is used as a typed API between the routing stage and the retry loop — exactly how the dead evidence keys above drifted in undetected. Consider a typed summary struct.

### kicadfiles

- `project/read.go:70` — `project.Read` never populates `PreservedSections`, and `newDocument` (`project.go:276`) unconditionally overwrites `board`, `erc`, `pcbnew`, etc. Re-writing an existing `.kicad_pro` wipes DRC constraints, ERC exclusions, and pcbnew defaults even though the preservation machinery exists precisely to keep them.
- `checks/checks.go:115` — An ERC/DRC run whose report file was never produced can classify as `pass`: `parseReportIfPresent` returns nil issue-free when `os.Stat` fails, so kicad-cli exiting 0 without writing JSON yields a clean check instead of a tool error.
- `designapi/builder.go:473` — `mergeNet` reparents net names but never updates schematic labels already created by `addSchematicWire`; after merging an auto-named net into a named one, the schematic still carries the stale `NET_...` local label, so schematic-derived and PCB net names can disagree.

### components / libraryresolver

- `components/selection.go:716` — `recordSatisfiesRating` can never return true for a rating specifying only `Min` (falls through to `(false, true)`), producing a false `COMPONENT_RATING_TOO_LOW` blocking rejection. Latent only because all checked-in ratings pair min with max.
- `components/model.go:51` — The `Catalog` embedded `sync.RWMutex` cannot enforce anything: exported `Records`/`Families` are iterated without the lock in `ValidateCatalog`, `ValidateCatalogEvidence`, `ComponentCoverage`, and `cmd/kicadai/main.go:1772`, while `SortCatalog`/`RebuildCatalogIndexes` mutate under the write lock. `go vet` already fails on the package. Encapsulate access or drop the mutex and document single-goroutine use.
- `components/model.go:742` — The `"Ω"` case in `isElectricalUnitSuffix` is unreachable (input is lowercased first, mapping Ω→ω which is not in the case list), so `parseLeadingEngineeringNumber("10kΩ")` returns 10 instead of 10000 — a 1000× error feeding value matching, rating checks, and constraint sorting with no diagnostic.
- `components/evidence.go:73` — The `AllowPassiveRules` escape hatch is dead code: the outer condition requires `ConfidenceVerified` but `passiveRuleInferred` requires `ConfidenceRuleInferred`; the flag silently does nothing.
- `libraryresolver/klc.go:46` — `ValidateSymbolKLC` keys duplicate-pin detection on pin number alone, ignoring unit and body style (unlike `validateParsedSymbol`'s correct `(unit, bodyStyle, number)` keying); De Morgan alternate body styles produce false `SeverityError` duplicate-pin issues.

### intentplanner / evaluate

- `map.go:556` — LEDs, amplifiers, and ESD blocks each index the shared `signalConnectorIDs` list independently, so different consumer types collide on the same connector pin (one LED + one amplifier both take `signalConnectorAt(0)` — same physical port on two nets, second connector unused).
- `synthesis.go:862` — `parseFloatParam` uses `fmt.Sscanf("%f")`, which ignores trailing text: `"400kHz"` parses as 400 Hz (selects the wrong I²C pull-up policy), `"0.02A"` parses as 0.02 mA (1000× resistor error), with no diagnostic. The strict `parseFloatString` in the same file should be used.
- `map.go:298` — `battery` and `dc_jack` power inputs pass request validation and intentdraft even asks a battery-voltage clarification, but `mapPower` handles only `usb_c`/`header`/`external` — the user is steered into a guaranteed "unsupported power input kind" dead end.
- `evaluate/evaluate.go:508` — `finish()` sets `FabricationReady: true` when ERC/DRC checks were skipped (they register as `Required: false` + `CheckSkipped`, and only required-skipped checks veto readiness) — a structurally clean project reports fabrication-ready with zero ERC/DRC evidence.

### fabrication / boardvalidation

- `physicalrules/dfm_geometry.go:237` — Uncapped O(V²) self-intersection test on filled-zone polygons (real KiCad fills have 10⁴+ vertices → ~10⁸ segment-pair tests per polygon), run twice per polygon, before the 256-edge width guard. `dfmPolygonEdgeDistanceWithBoundsMM` has a 1M-pair cap; this path has none.
- `boardvalidation/validate.go:319` — `fully_routed` can be reported for a net whose copper is split into disconnected islands: `netStatuses` checks that every pad touches a track but never that tracks form a single connected component. A 4-pad net routed as two separate 2-pad clusters passes both `unrouted_net_validation` and `route_completion`.
- `physicalrules/model.go:185` — Several profile knobs can never fire: `Options.RequireCourtyard` is populated but never read (missing courtyards hardcode to `StatusWarning`); `MinCourtyardSpacingMM`, `MinSilkPadClearanceMM`, `MinSilkEdgeClearanceMM` are never referenced; `profiles.Metadata.WarningOnlyFields` is validated against a 60-entry table and hashed but no evaluation code consults it.

### routing / placement

- `routing/search.go:90` — Routed traces stop at the snapped grid cell, up to ~0.18mm (at defaults) from the pad center, with no tail segment to the pad. For fine-pitch pads narrower than the snap error (0.3mm QFN), the trace endpoint can fall entirely outside pad copper — a real open circuit reported as routed.
- `routing/operations.go:20` — `OperationsFromResultWithIssues` emits route operations for failed routes (no `route.Status` filter): partial segments of mid-route failures and full copper of length-policy failures are applied to the board; the failed net's stale copper was also already appended to `request.Existing`, blocking subsequent nets with no rip-up.
- `routing/route.go:202` — `ModeValidateOnly` silently performs full two-layer routing and emits mutation operations; no validate-only implementation exists anywhere in the package.
- `routing/occupancy.go:269` — Back-layer (B.Cu) components are modeled with unmirrored pad geometry in routing, placement, and routingadapters (verified: no mirror/flip handling exists), but KiCad mirrors flipped footprints — back-side endpoints and obstacles sit at wrong X. Latent because back-layer placement defaults to off.

### repair / transactions

- `repair/executor.go:130` — `assignFootprint` appends its operation after `write_project` instead of using `insertBeforeWrite` like every other repair path, so on replay the assignment mutates the builder after files are written — a silent no-op repair that consumes budget.
- `transactions/apply.go:1277` — `processAlive` calls `syscall.Kill`; `GOOS=windows go build ./internal/transactions/` fails (verified), propagating to every importer including `internal/repair`. The same file special-cases Windows elsewhere, so Windows is clearly intended to be supported.
- `transactions/apply.go:76` — `Overwrite` + first-op `create_project` routes an existing directory down the generated path, bypassing the `AllowImportedMutation` consent gate and all preservation checks, with no provenance check (unlike repair's `HydrateTarget`). `--overwrite` pointed at a hand-authored project silently destroys it and every non-KiCad file in it.
- `transactions/apply.go:154` — The `.kicadai.apply.lock` is acquired only in `applyImported`; the generated apply path and the entire repair persisted-apply pipeline run with no lock, so two concurrent invocations against one output directory can interleave replacement and deletion into a mixed-generation project.

### CLI

- `cmd/kicadai/main.go:310` — Ctrl+C is a silent no-op for roughly a third of commands: `run` installs `signal.NotifyContext` for every command (suppressing default SIGINT termination), but `inspect`, `evaluate`, `roundtrip`, `pinmap`, `place`, `route`, `repair`, `transaction`, `fabrication`, and `generate` never receive the context, and `runIntentPlan` explicitly discards it (`_ = ctx`, line 3084).
- `cmd/kicadai/main.go:973` — `block verify --builtins` probes `internal/blocks/testdata/verification` relative to the process CWD; for an installed binary it fails with a confusing suite-load error. Embed the manifests (`go:embed`) or explain the source-tree requirement.
- `cmd/kicadai/main.go` — 4,867-line single-file `main` wiring ~30 commands through one shared `flag.FlagSet` of ~80 flags: every flag is accepted by every command, so command-inappropriate flags are silently ignored and per-command boilerplate is duplicated. It also imports `kiapi/gen/common/types` directly (line 34), bypassing the `kiapi` wrapper.

### Cross-cutting

- **Error wrapping** — Only 82 of 409 non-test `fmt.Errorf` calls use `%w` (~20%), yet there are 86 `errors.Is/As` call sites; sentinel errors are concentrated in the kiapi/IPC generation of code (designworkflow has exactly 1). `errors.Is/As` over mostly-unwrapped chains silently fails to match — a latent correctness hazard.
- **Integration-test gating** — Two competing mechanisms: 2 files use `//go:build integration` while ~10 distinct env vars (`KICADAI_RUN_KICAD_CLI`, `KICADAI_REAL_KICAD_CLI`, `KICAD_VALIDATE_GENERATED_FILES`, ...) gate the rest; no single documented switch, and `make test` has no integration variant.
- **Context propagation** — 80 exported functions take `context.Context`, but 17 non-test `context.Background()` calls sit mid-stack inside internal packages (libraryresolver 4, inspect 3, routing 2, repair 2, evaluate 2, designworkflow 2, ...), severing cancellation on paths that ultimately drive kicad-cli subprocesses.
- **specs/ sprawl** — 95+ spec directories (87 SPEC.md) with no index; stale artifacts (`CODE_REVIEW.md`, old fix plans, `OLD_ROADMAP*.md`) sit loose at the specs root. A generated INDEX.md and an `archive/` subfolder would make the corpus navigable.

---

## Low Findings

- `cmd/kicadai/main.go:4501` — nil-pointer panic in `version --format text` if KiCad returns an OK `GetVersionResponse` with the version field unset (`GetVersion` returns `(nil, nil)`; the JSON path is safe, the text path dereferences).
- `cmd/kicadai/main.go:53` — Help-text drift: seven defined flags missing from the usage template (`--source-dir`, `--kicad-corpus`, `--kicad-corpus-tier`, `--max-repair-attempts`, `--require-kicad-roundtrip`, `--strict-diffs`, `--allow-unrouted`); `--source-dir` is load-bearing for `component` subcommands.
- `internal/inspect/inspect.go:536` — ~200 lines of dead code: `scanSchematic`, the byte-level s-expression helpers (lines 531–706), and `copyCounts` are referenced by nothing — leftover pre-structured-parser scanner.
- `internal/ipc/mangos.go:94` — `Request` honors deadlines but not deadline-less cancellation; a canceled context is never observed after entry, so with a zero `TimeoutConfig` the socket blocks on the 24h `blockingDeadline`. Latent (callers always configure timeouts).
- `internal/blocks/project.go:249` — `fmt.Errorf(issues[0].Message)` non-constant format string; a message containing `%` is mangled.
- `internal/blocks/mcu.go:269` — Reset/ISP/UART subcircuit duplicated nearly verbatim from `reset_programming.go:161`; combined with three separate power-LED implementations (one correct, two reversed — see H1/H2), duplication is what let the polarity bugs diverge. Extract shared sub-circuit builders.
- `internal/blocks/builtin_realization.go:96,287` — `power_led_resistor`/`power_led` and `vbus_fuse`/`vbus_tvs`/`bulk_capacitor` realization entries lack `When` gates matching their definition counterparts, producing phantom-role warnings and (for `include_fuse=false`) false validation failures. `builtin_realization.go:47` — `led_indicator` expectations require net `gnd` unconditionally, but `active_high=false` mode creates no gnd net.
- `internal/blocks/composition.go:118` — Voltage-domain compatibility is raw string equality ("5V" vs "5.0V" vs "3300mV" trigger blocking "conflicting voltage domains"); the package already has `parseUnit`.
- `internal/blocks/realize.go:632` — Role matching requires the emitted footprint to exactly equal the definition default, so overriding any documented footprint parameter silently breaks PCB realization (verified with `led_indicator` + `resistor_footprint` override).
- `internal/kicadfiles/pcb/read.go:56` — Top-level `arc` (track arcs) lands in `Preserved` instead of `TrackArcs`; bytes survive but arcs are invisible to net-reference resolution and connectivity validation — tracks terminating on an imported arc are falsely reported disconnected.
- `internal/kicadfiles/schematic/read.go:342` — `readProperty` can't distinguish the unquoted `private` atom from a property named "private" (`ParsedNode.Quoted` exists but isn't consulted); such a property is silently dropped.
- `internal/kicadfiles/schematic/led.go:69` — `SchematicFile.Instances` is populated by the LED fixture but never serialized by `render` (only per-symbol instances are written) — a footgun field.
- `internal/kicadfiles/designapi/builder.go:423` — `addSchematicWire` dedup is O(n²) linear scans per segment; thousands of nets will spend most of build time here. A point-keyed set (as `schematicConnectivityAnchors` uses) makes it O(1).
- `internal/routing/validation.go:191` — `clearanceIssues` ignores vias entirely (no via-to-track/via-to-via checks), includes failed routes' segments, and is O(total-segments²) across route pairs.
- `internal/routing/model.go:598` — Pad-layer validation is case-sensitive (`"f.cu"` rejected with a blocking issue) while the rest of the router normalizes case.
- `internal/schematiclayout/route.go:95` — The wire router has no obstacle avoidance but the package's own validator reports the resulting overlaps as `SeverityError`, so `Layout()` output routinely fails its own gate; the suppression exists only for parsed fixtures.
- `internal/designworkflow/placement_routing_retry.go:117` — `summary.Attempts` only counts attempts that reached routing; placement-blocked/repeated-state attempts are uncounted, understating retry evidence.
- `internal/designworkflow/placement.go:399` — `appendUniquePlacementEndpoints` builds a case-sensitive pin dedup key while the companion `placementEndpointKey` upper-cases; the two dedup paths disagree for pins differing only by case.
- `internal/components/catalog.go:87` — Multi-file catalog version reconciliation conflates "unset" with the default version; the mismatch warning depends on file iteration order.
- `internal/pinmap/pinmap.go:225` — `footprintPadIssues` diagnostic states the condition backwards (fires when a pinmap pad is missing from the footprint, but says the opposite).
- `internal/libraryresolver/model.go:120` / `cache.go:62` — Every symbol/footprint record retains full raw s-expression text and `writeCache` JSON-encodes all of it; against the stock KiCad libraries this roughly doubles resident memory and the cache decode can rival re-parsing.
- `internal/fabrication/physicalrules/evaluate.go:1746` — `evaluateTrackWidths`/`evaluateViaDimensions` gate pass/fail on the reporting slice (`len(low) == 0`, which drops empty-UUID objects) instead of the violation counter like every other check — violations without UUIDs report `StatusPass`.
- `internal/pcbrules/rules.go:199` — Net-override lookup uses the untrimmed name while the cache key trims; `"GND"` and `" GND"` share a cache entry but resolve differently — first resolution poisons the cache for both.
- `internal/repair/bundle.go:70` — `ValidateBundle` fails on any issue including warnings; should filter on `issue.Blocking()`. A successfully applied project can produce a bundle that export/load rejects.
- `internal/transactions/plan.go:459` — `touchesDesign` omits `OpAddNoConnect`, so a trailing `add_no_connect` isn't rejected up front and fails at the end with a misleading error.
- `internal/transactions/apply.go:1162` — Dead alternate write paths `writeSchematicAtomic`/`writePCBAtomic` (plus `statusFromPostValidation`, `blockingIssueCount`, `findingsFromMap` in repair) have no callers; in file-mutation code, unused write paths are a misuse hazard — remove them.
- `internal/intentplanner/map.go:15,128` — Two near-identically named voltage epsilons with different magnitudes (`voltageCompareEpsilon` 0.01 vs `voltageComparisonEpsilon` 0.001); consolidate or rename for intent. `i2cConnectorAt` (`map.go:978`) is dead code.
- `go.mod` — Stale transitive pins: `golang.org/x/sys` at a 2021-01-24 pseudo-version and `Microsoft/go-winio v0.5.2` (2022), both via mangos v3; `go get -u` + `go mod tidy` clears them with no code change.
- **Package docs** — `doc.go` exists in only 7 of ~40 packages, all on the older kiapi/IPC side; the eight largest packages (designworkflow, blocks, placement, repair, routing, intentplanner, fabrication, transactions) have none.
- **Observability** — Zero logging anywhere (`log`/`slog` unused); long-running operations (repair loops, routing, kicad-cli round-trips) have no progress/diagnostic channel until they return. A thin optional `*slog.Logger` at the designworkflow/repair level would help debug production runs.
- `internal/tmp_corpus_report/` — Empty, untracked local directory inside `internal/`; delete it.
- **Lint config** — No committed `.golangci.yml`; `make lint` runs only gofmt + go vet while the development workflow expects `golangci-lint`. Commit a config (excluding `internal/kiapi/gen/`) so results are reproducible.

---

## Statistics

- Production Go (non-generated): 101,392 LOC in 279 files; tests: 65,857 LOC in 271 files (ratio 0.65).
- Largest packages: designworkflow 26,834 / blocks 12,718 / placement 11,303 / cmd/kicadai 10,236 (main.go alone 4,867) / kicadfiles/pcb 8,368 / repair 7,270 LOC.
- Dependency hubs: `reports` (306 LOC) imported by 30 packages (as intended); `kicadfiles` root imported by 20; designworkflow fan-out 21, fan-in 4.
- Error handling: 409 non-test `fmt.Errorf` (82 with `%w`), 110 `errors.New`, 86 `errors.Is/As` call sites.
- Repo hygiene: clean — `bin/`, `.gocache/`, `.gomodcache/`, `.cache/`, `.tmp/`, `.coverage/` all gitignored with zero tracked files inside; hermetic Makefile with a 75% coverage gate excluding generated code.
- 741 commits since the previous review (2026-06-11).

## Recommended Priorities

1. **Fix the three Criticals** (C1 AREF mis-binding, C2 round-trip data loss, C3 false-blocked routing). C1 and C3 are small, well-localized fixes; C2 needs a preservation design decision (consume `Raw` on write vs. model more fields) plus a byte-level round-trip regression test against KiCad-authored fixtures.
2. **Fix the reversed power LEDs (H1/H2) and extract a shared LED/indicator sub-circuit builder** so polarity lives in one place; sweep the other copy-pasted subcircuits (reset/ISP/UART) while there.
3. **Repair-pipeline integrity (H10–H12)**: make copy-back transactional (reuse the existing staged backup/rollback pattern), give the repair loop a revalidator that can see stage issues, and re-write the manifest after zone refill.
4. **Do a schema-to-consumer audit of all request/profile options** — the dead-knob pattern (H9, retry deltas, courtyard/silk profile limits, `AllowPassiveRules`, `ModeValidateOnly`) appeared independently in four packages. A test that walks the request schema and asserts each field influences behavior would prevent recurrence.
5. **Library metadata parsing (H13)** — small fix (fall back to `(property ...)` nodes, as already done for datasheet) with outsized effect on real-library compatibility scoring and KLC accuracy.
6. **Structural**: split `cmd/kicadai/main.go` into per-command files with per-command flag sets; extract shared types to break the intent→designworkflow inversion and the designapi↔routing cycle; standardize `%w` wrapping on paths consumed by `errors.Is/As`; consolidate the triplicated 2D geometry code (root cause of H19) into one shared, tested package.
7. **Quick hygiene batch**: the 82 golangci-lint findings (unchecked `Close`/`Fprintf` errors, 37 unused symbols, ineffassign), the Windows build break (`syscall.Kill`), dead code deletion, and `internal/tmp_corpus_report/` removal are all low-risk cleanups suitable for a single PR.
