# KiCadAI Code Review — July 29, 2026

**Reviewer:** Claude (Fable 5), multi-agent review — 13 parallel subsystem reviewers + 1 repository-health pass
**Scope:** All non-generated Go source (~250K lines across `internal/`, `cmd/`, root), catalog data, CI configuration, repo hygiene. Generated protobuf (`internal/kiapi/gen/`) excluded from line-level review.
**Method:** Each subsystem was reviewed by a dedicated agent instructed to verify every finding against surrounding code before reporting. Findings below were kept only when verified with concrete evidence. No code was modified.

---

## Executive summary

The codebase is in unusually good shape for its size and ambition. Build, vet, and lint are clean; the test-to-source ratio is nearly 1:1 (491 test files / 516 source files); determinism is engineered deliberately almost everywhere (sorted map keys, stable comparators, canonical hashing); fail-closed discipline is real and layered; and independently verified electronics math (Sallen-Key, Butterworth ENBW, buck ripple, crystal loading, Ebers–Moll, class-B dissipation, all 47 BJT and 18 MOSFET catalog pinouts) checked out correct.

The serious defects cluster in five areas:

1. **Encoded circuit knowledge that produces non-functional hardware** — the class AB output stage installs its bias diodes backwards (Critical); the AC-coupled single-supply op-amp stage saturates at the rail; a bias-network port is never connected. All three pass CI because block tests count operations instead of asserting pin→net topology.
2. **Lossy imported-project rewrites** — the PCB/schematic readers parse a subset of KiCad's format and the writers re-render from that lossy model, silently resetting board stackup/thickness, deleting keepout zones, 3D models, silkscreen text, and title blocks on the read→modify→write path.
3. **Fail-closed / gating gaps** — a DRC tool crash still counts as ERC/DRC acceptance; `routing_retry.drc_policy: "required"` is accepted but wired to dead code; `place`/`route` CLI commands exit 0 on blocking failures; `check erc|drc` exits 0 when KiCad is missing; repair claims `StatusRepaired` without re-detecting the original issue.
4. **Data-safety windows in transactions and repair** — imported-project writes have a crash window where user files are absent; the stale-lock recovery race can admit two concurrent applies; persisted repair has no rollback for partial multi-file replacement.
5. **Residual nondeterminism** — a handful of map-iteration-order dependencies survive in an otherwise rigorously deterministic codebase, including one that changes lowered-circuit hashes (`compositionlowering`) and one that randomly rejects valid input (`"ohms"` unit parsing).

**Finding counts:** 1 Critical · 17 High · ~50 Medium · ~60 Low across ~128 deduplicated findings.

---

## Severity index — Critical & High

| # | Sev | Subsystem | Finding |
|---|-----|-----------|---------|
| C1 | Critical | blocks | Class AB output-stage bias diodes installed backwards |
| H1 | High | blocks | AC-coupled single-supply op-amp stage saturates at rail (DC gain not unity) |
| H2 | High | blocks | `amplifier_bias_network` DRIVER_OUT port never connected (net-name key mismatch) |
| H3 | High | kicadfiles | PCB read→write resets `general`/`setup`/`title_block` to defaults |
| H4 | High | kicadfiles | Footprint re-render discards fp_text, 3D models, descr/tags/locked/margins |
| H5 | High | kicadfiles | Keepout zones silently converted to copper zones on rewrite |
| H6 | High | transactions | Crash window in imported-project group write leaves user files missing |
| H7 | High | transactions | Stale-lock removal race admits two concurrent applies |
| H8 | High | repair | Footprint-assignment repair appended after `write_project` — never takes effect |
| H9 | High | repair | `StatusRepaired` claimed without re-running the validation that found the issues |
| H10 | High | repair | No rollback for partial multi-file replacement; crash marker deleted on error path |
| H11 | High | designworkflow | DRC tool crash (no output) still counts as ERC/DRC acceptance achieved |
| H12 | High | designworkflow | `routing_retry.drc_policy: "required"` accepted but enforcement is dead code |
| H13 | High | routing | Board-edge clearance ignores trace half-width + grid rounding; no backstop check |
| H14 | High | routing | `ViaClearanceMM` accepted, defaulted, plumbed — never enforced |
| H15 | High | components | Exact float equality breaks value matching across SI-prefix spellings |
| H16 | High | compositionlowering | Map-order iteration makes lowered net metadata (and hashes) nondeterministic |
| H17 | High | cmd | `place request` / `route request` exit 0 despite blocking issues |

---

## Critical findings

### C1. Class AB output-stage bias diodes are installed backwards
- **Location:** `internal/blocks/class_ab_output.go:61-63`, `class_ab_output.go:472-477`
- **Issue:** In the `diode_string` topology, `bias_upper` pin 1 is wired to the upper (NPN base) node and pin 2 to the driver midpoint; `bias_lower` pin 1 to the midpoint and pin 2 to the lower (PNP base) node. The catalog record for `diode.onsemi.1n4148w.sod_123` (`data/components/alternatives.json`) declares pin 1 = CATHODE, pin 2 = ANODE, so both diodes are reverse-biased for the VCC→VEE bias-string current. The two sibling blocks using the same part prove the intended orientation: `amplifier_bias_network.go:50-52` and `class_ab_speaker_power_stage.go:114-119` both chain anode-to-cathode correctly — `class_ab_output.go` is the outlier.
- **Impact:** The generated stage has no controlled quiescent bias: the driver midpoint is isolated behind two reverse-biased diodes, while the 10 kΩ feeds drive both output devices on simultaneously (uncontrolled shoot-through, ~tens of mA at default 9 V rails). Any board built from this block is non-functional. `class_ab_output_test.go` only counts operations and never checks diode polarity, so this passes CI.

---

## High findings

### H1. AC-coupled single-supply op-amp stage amplifies its own DC bias into rail saturation
- **Location:** `internal/blocks/opamp.go:99-100`, `opamp.go:154-181`
- **Issue:** In `input_coupling: "ac"` mode, IN+ is biased to VCC/2 by the 100k/100k divider (correct), but Rg remains DC-connected to ground with no DC-blocking capacitor in the Rg leg. DC gain therefore equals the full closed-loop gain, so output DC = (VCC/2)·(1+Rf/Rg) ≥ VCC for any gain ≥ 2. (The LMV321 pin map itself is correct.)
- **Impact:** Every AC-coupled single-supply instance saturates at the positive rail and passes no signal. The standard fix — a series capacitor between Rg and ground, making DC gain unity — is absent.

### H2. `amplifier_bias_network` never connects its DRIVER_OUT port in the schematic
- **Location:** `internal/blocks/amplifier_bias_network.go:301-331`
- **Issue:** `appendAmplifierBiasNetworkConnections` looks up ports via `portsByNet[net.NameTemplate]`, but the map key is `"driver"` while the net's `NameTemplate` is `"driver_out"`. The lookup returns `""` and the port-to-diode connect operation is skipped. The PCB realization *does* route the port, making schematic and PCB inconsistent.
- **Impact:** Realized standalone (or composed without a `net_alias`), the driver input to the bias string is an open circuit in the schematic netlist — the parent's connection lands on `<inst>_DRIVER_OUT` while the diode midpoint stays on `<inst>_driver_out`. ERC or bring-up failure. Tests only check net names and ref counts.

### H3. PCB read→write silently resets `general`, `setup`, and `title_block` to defaults
- **Location:** `internal/kicadfiles/pcb/read.go:36,72`; render at `pcb/render.go:62-68`
- **Issue:** `Read` initializes the board with `DefaultGeneral()`/`DefaultSetup()` and explicitly *skips* the file's own `general`, `setup`, `title_block`, `paper`, `layers` nodes — and because they are in the skip list, they are also excluded from the `Preserved` raw-node fallback. `render` then writes the defaults.
- **Impact:** Any read→modify→write cycle on an existing board — the production imported-mutation path (`transactions.applyImported` → `writeImportedProject` → `pcb.Write`) — silently replaces board thickness (e.g. 1.0 mm → default 1.6 mm), stackup, mask clearances, and all `pcbplotparams`, and deletes the title block. These are physically meaningful manufacturing parameters, and the preservation report cannot catch data that is *substituted* rather than dropped.

### H4. Footprint re-render discards unmodeled footprint children
- **Location:** `internal/kicadfiles/pcb/read.go:92-138`; `pcb/render.go:302-372`
- **Issue:** `readFootprint` parses only `at`, `layer`, `path`, `attr`, `property`, `pad`, and `fp_*` graphics. It captures `fp.Raw`, but `renderFootprint` never consults it (only `pad.Raw` is used for preservation). `fp_text` user text, `model` (3D models), `descr`, `tags`, `locked`, `solder_mask_margin`, `zone_connect`, etc. are absent from the rewrite. Pad-level unknown children *are* carefully preserved, which makes the missing footprint-level equivalent look like an oversight rather than policy.
- **Impact:** Rewriting an imported board strips silkscreen user text, all 3D models, and footprint-level clearance/mask overrides from every footprint.

### H5. Keepout zones are converted to copper zones on read→write
- **Location:** `internal/kicadfiles/pcb/read.go:265-313`; `pcb/render.go:818-860`
- **Issue:** `readZone` never parses the `keepout` node (or `attr`, `filled_areas_thickness`, fill `mode`), so `zone.Keepout` is always nil for parsed zones. `renderZone` with nil Keepout emits an ordinary net-0 copper zone with defaulted `hatch`/`min_thickness`/`fill`. `zone.Raw` is captured but never used.
- **Impact:** A rule-area/keepout zone in an imported board comes back as an ordinary copper zone — the keepout restriction is silently deleted, which can directly change DRC results and the manufactured board.

### H6. Crash window in imported-project write leaves user files missing
- **Location:** `internal/transactions/apply.go:1153-1221` (`writeImportedProjectFilesAtomic`, `backupImportedProjectTarget`)
- **Issue:** The group write first renames every target aside into hidden `.name.backup-*` files, and only then renames temps into place. Between those renames — and across the whole multi-file loop — the user's `.kicad_sch`/`.kicad_pcb` do not exist at their paths. Nothing on a subsequent run scans for or restores the backups. Renaming temp→target directly (rename-over is atomic on both Unix and Windows) would eliminate the window.
- **Impact:** A crash/SIGKILL/power loss mid-apply leaves the project with its files apparently deleted, content stranded in hidden dot-files. This is precisely the data-loss class the code exists to prevent.

### H7. Stale-lock removal race lets two applies run concurrently
- **Location:** `internal/transactions/apply.go:1434-1485` (`AcquireProjectApplyLock`, `removeStaleApplyLock`)
- **Issue:** On `O_EXCL` failure, the code reads the lock, checks the recorded PID, then unconditionally `os.Remove(lockPath)`. Two processes can both read the same dead-PID lock; A removes and recreates it with its own PID; B then removes **A's live lock** (the remove is not conditioned on the inspected file still being present) and creates its own. `releaseLock` also removes unconditionally, so the loser deletes the winner's lock on exit.
- **Impact:** Two concurrent applies against the same project can both pass the lock and interleave staged renames/manifest writes. Low probability (requires a crashed prior holder plus simultaneous starts), but this is exactly the case the lock exists for. `flock` or recreate-via-unique-temp-rename closes the race.

### H8. Footprint-assignment repair appends its operation after `write_project` — silently no-op
- **Location:** `internal/repair/executor.go:130`
- **Issue:** When no existing `assign_footprint` op matches, `assignFootprint` does a plain tail append, while every other repair path routes through `insertBeforeWrite`/`upsertSingletonOperation` (executor.go:396-421) precisely to land before `OpWriteProject`. Generated transactions always end with `write_project`, and `transactions.Apply` writes files at that op. `transactions.Validate` has no ordering rule, so revalidation passes and the attempt reports `StatusRepaired`.
- **Impact:** The repaired transaction replays with the footprint assigned *after* the project files are written — the written PCB never receives it. The repair reports success and burns loop-budget attempts on an action that can never converge. The only test for this path uses an empty transaction, so the ordering bug is uncovered.

### H9. Persisted repair claims `StatusRepaired` without re-running the validation that found the issues
- **Location:** `internal/repair/persisted.go:260-271`, `persisted.go:421-424`, `internal/repair/runner.go:97-107`
- **Issue:** `persistedRepairValidator.ValidateAttempt` runs only structural `transactions.Validate`, then `removeAttemptedIssue` — it *assumes* the executed action fixed the attempted issue. `PostValidationOptions` is all-false by default, so nothing re-detects the original stage issues, and `statusFromValidationDelta` returns `StatusRepaired`.
- **Impact:** With default options, `ApplyPersistedBundle` overwrites the user's project and reports "repaired" even when the executed action did not fix the design problem (e.g., H8 above). Headline-status correctness depends entirely on callers opting into post-validators.

### H10. No rollback after in-place replacement; partial-failure path deletes the crash marker
- **Location:** `internal/repair/persisted.go:521-566` (esp. 530-534)
- **Issue:** `replaceGeneratedOutput` copies staged files into the live project one at a time; an error midway (disk full, permissions) returns with some files replaced and others not — no restore of prior contents exists anywhere in the package. The `.kicadai/repair-in-progress` marker is removed by `defer os.Remove(marker)` on *every* return path, including errors. Post-apply validators also run only after replacement, so a `StatusBlocked` delta leaves mutated files in place.
- **Impact:** A mid-replacement I/O failure corrupts the project into a mixed old/new state that carries no evidence of interruption; a repair that post-validates *worse* still permanently overwrites previously working output. Individual writes are atomic; the multi-file transaction is not.

### H11. Required DRC that crashes without output still counts as ERC/DRC acceptance achieved
- **Location:** `internal/designworkflow/kicad_checks.go:247-257`; `internal/designworkflow/result.go:190-232`
- **Issue:** `workflowCheckToolErrorSeverity` downgrades a DRC tool failure to `SeverityWarning` when `ToolErrorKind == ToolErrorNoOutputCrash` (a deliberate carve-out for macOS AppKit crashes). Warnings are non-blocking, so the stage completes and `AchievedAcceptance` returns `AcceptanceERCDRC` — despite no DRC report ever being produced.
- **Impact:** A request with `RequireDRC` and acceptance `erc_drc` reports `Achieved: erc_drc` with zero DRC evidence whenever kicad-cli aborts before writing a report — on the exact platform where the crash is known to occur. Acceptance claims become unreliable.

### H12. `routing_retry.drc_policy: "required"` is accepted but never enforced — evidence plumbing is dead code
- **Location:** `internal/designworkflow/retry_drc_evidence.go:58-92`; `placement_routing_retry.go:436-450, 737-746`; `request.go:1035`
- **Issue:** `retryDRCEvidenceForAttempt`, `applyRetryDRCEvidenceToAttempt`, and `kicadRetryDRCEvidenceAdapter` have no callers outside their own file and tests. Every retry attempt summary is created with `DRCStatus: retryEvidenceSkipped` and never updated, while `validateRoutingRetryPolicy` accepts `RetryDRCPolicyRequired` as valid config and the attempt comparator's `RetryDRCPolicyRequired` branch compares constants.
- **Impact:** A user configuring required per-attempt DRC gating gets no gating, no `fewer_drc_blockers` ranking, no `drc_regression` flags, and no `CorrectionStopDRCRegression` stop — silently. Fail-open relative to the declared policy contract.

### H13. Board-edge clearance ignores trace half-width and grid rounding; nothing backstops it
- **Location:** `internal/routing/geometry.go:143` (`UsableBoardRect`); `internal/routing/occupancy.go:205` (`blockOutsideUsable`); `internal/routing/validation.go:366`
- **Issue:** The routable region margin is `board.MarginMM + rules.EdgeClearanceMM` with no `TraceWidthMM/2` (or via-radius) term — unlike every other obstacle, which is inflated by `clearance + movingCopperRadius`. `ToGrid` rounds to nearest, so a cell center up to `GridMM/2` *outside* the usable rect stays routable. `ValidateResult` only checks centerline containment.
- **Impact:** With defaults (edge clearance 0.25, trace width 0.25, grid 0.25), a routed centerline can sit 0.125 mm from the outline — copper edge at 0 mm from Edge.Cuts, a hard DFM violation. The `physicalrules` copper-to-edge check covers polygons, not these tracks.

### H14. `ViaClearanceMM` is accepted, defaulted, and plumbed everywhere but never enforced
- **Location:** `internal/routing/occupancy.go:40-44` (`BuildViaOccupancy`); `internal/routing/validation.go:151,184,192`
- **Issue:** `ViaClearanceMM` exists in `Rules`, `NetClass`, `EffectiveRule`, is defaulted to 0.20 (`model.go:408`) and applied by `applyEffectiveRule` (`route.go:1066`) — but `BuildViaOccupancy` inflates with `Rules.ClearanceMM + ViaDiameterMM/2`, and every via validation check compares against `Rules.ClearanceMM`. The only functional read is a heuristic escape margin.
- **Impact:** A net class declaring `via_clearance_mm: 0.4` gets vias placed and validated at 0.2 mm with no warning — the rule silently does nothing, defeating the "explicit net class" suggestion the router itself emits for power nets.

### H15. Exact float equality breaks component value matching across SI-prefix spellings
- **Location:** `internal/components/selection.go:969-971` (`recordHasValue`)
- **Issue:** When a record has a `Typ` value, matching is `got == want` on parsed float64s. Equivalent engineering spellings are not bit-identical: verified `0.1u ≠ 100n`, `0.47u ≠ 470n`, `6.8u ≠ 6800n`, `1.5n ≠ 1500p` (while `2.2u == 2200n` happens to match). Catalog values use spellings like `4.7u`, `10k`.
- **Impact:** A query spelling the same value with a different prefix intermittently fails: `Find` returns no candidates and `Select` escalates to blocked `COMPONENT_NOT_FOUND` for a part that exists — a silent, spelling-dependent failure feeding `circuitgraph.Synthesize`. Needs relative-epsilon comparison.

### H16. Nondeterministic net metadata from map-order iteration in composition lowering
- **Location:** `internal/compositionlowering/lower.go:822-824, 1131-1143, 1225-1233`
- **Issue:** `lowerConnections` merges per-node metadata with `for node := range union.members(root)` — a `map[string]bool` — and `combineMetadata` is order-sensitive (first non-empty domain wins; equal-rank roles resolve by encounter order). When one composed net carries nodes with two different non-empty domain labels, `FunctionConnection.VoltageDomain` varies between runs.
- **Impact:** The lowered `circuitgraph.Document` — and everything hashed from it (resolution hash, replay/promotion evidence) — can differ across identical runs, undermining deterministic replay and producing intermittent hash-mismatch failures. Fix: iterate the already-sorted `nodes` slice.

### H17. `place request` and `route request` exit 0 even with blocking issues
- **Location:** `cmd/kicadai/main.go:2422-2425` (place), `main.go:2485-2490` (route)
- **Issue:** Both handlers end with `return writeReportJSON(...)` and never check `result.OK`, unlike every sibling command (`block`, `library`, `pinmap validate`, `check`, etc., which follow with `if !result.OK { return errors.New(...) }`). A failed run prints `"ok": false` and exits 0. Tests assert only the success path.
- **Impact:** Any script or CI stage gating on the exit code of `kicadai place request` / `route request` silently passes failed placement/routing.

---

## Medium findings by subsystem

### designworkflow
- **Inter-block contact validation emits issues/proofs in map-iteration order** (`interblock_contact.go:175`): appends to `evidence.Proofs`/`Issues` while ranging a map; issue order in workflow-result JSON varies run to run. Sibling functions sort net names first. Output-determinism defect only (repair hints are deduped/sorted downstream).
- **30-second wall-clock budget makes local-route rebuild machine-speed dependent** (`routing.go:50, 2810, 3094-3096`): `rebuildMovedLocalRouteOperations` uses `context.WithTimeout(30s)` — the only wall-clock dependence in the pipeline; the same request can route fully on a fast machine and block on a slow one. Fails closed but breaks "identical input → identical output."
- **Block-verification evidence root resolves via `runtime.Caller` source path** (`block_evidence.go:209-218`): absent `KICADAI_BLOCK_VERIFICATION_ROOT`, evidence loads from the build machine's source-tree path baked into the binary. Deployed binaries silently lose all built-in block verification evidence — every block reports `missing`, capability classifications degrade, fabrication-candidate acceptance becomes unreachable, with no issue naming the cause. Embed via `go:embed` or fail loudly.
- **Explicit-circuit pipeline diverges from the block pipeline** (`explicit_circuit.go:16-114` vs `create.go:158-193`): hand-rolled stage sequence never runs fabrication or repair stages and ignores `opts.Fabrication/Repair/PostRepair`; an explicit circuit requesting `fabrication_candidate` silently caps at `erc_drc` with no diagnostic.

### cmd/
- **`repair plan`/`repair apply` can report `ok: false` yet exit 0** (`main.go:5218-5226`; `internal/repair/planner.go:112-131`): the non-`--target` path gates only on `StatusBlocked`; `StatusPartial` (some attempts blocked) still emits blocking issues but returns nil. The `--target` variants get this right.
- **Default catalog dir resolves to embedded catalog in some commands, cwd-relative path in others** (`main.go:1701`, `behavioral_intent.go:57`, `ai_graph_design.go:43`, `requirement_create.go:51` vs `generation_capability.go:36-39`, `circuit_preflight.go:676-679`): `component list`, `intent compile`, `requirement create`, and generic-profile `design create` fail file-not-found outside the repo root despite the catalog being embedded exactly for that case.
- **`check erc|drc|project` exits 0 when KiCad CLI is missing; `--require-erc/--require-drc` ignored by `check`** (`main.go:2556-2572, 4308-4323`; same for `roundtrip`): discovery failure yields a warning skip and exit 0; CI passes silently on machines without KiCad.
- **Interrupt handling wired but unused by several long commands; Ctrl-C swallowed during `roundtrip`** (`main.go:357-360, 400-413, 4492`): `runRoundTrip`/`runPlace`/`runRoute`/`runRepairCommand`/`runSchematicIR` never receive the NotifyContext ctx; SIGINT cancels a context nobody reads while the handler suppresses default termination.

### transactions / infrastructure
- **Imported readback failure occurs after backups are deleted** (`apply.go:430-436, 1190-1196`): readback validation runs after commit and backup deletion; a serializer bug that produces unparseable output overwrites the user's project irreversibly. Validate against staged temps instead.
- **Overwrite/preservation gating is TOCTOU-racy, skips locking for "fresh" targets, keys solely on `.kicad_pro`** (`apply.go:84-128, 858-860`; `plan.go:372-386`): manifest-current decision made before lock acquisition; no lock at all when target lacks `.kicad_pro`; a directory containing loose `.kicad_sch`/`.kicad_pcb` but no project file is treated as fresh and `payload.Overwrite` bypasses the preservation gate.
- **IPC `Request` ignores context cancellation while blocked** (`internal/ipc/mangos.go:94-199`; also surfaced from the kiapi side at `client.go:194`): only `ctx.Deadline()` is consulted; `ctx.Done()` is never selected on. With a cancelable-but-deadline-less context (normal SIGINT shape), cancellation is ignored until the socket deadline — up to the 24h `blockingDeadline` with zero-value config. The existing `closeSocketAfterInterruptedRequest` machinery could be triggered from a `ctx.Done()` watcher.
- **`net_overrides[].role` silently ignored during rule resolution** (`internal/pcbrules/rules.go:198-249`; populated at `internal/routing/model.go:763`): `resolve` derives role exclusively from the `NetDescriptor`; `override.Role` is never read. `net_overrides: {"VCC": {"role": "power"}}` never gets power-role defaults (0.35 mm width, via cap) — user-authored electrical constraints silently dropped.
- **Four divergent atomic-write implementations; three skip the directory fsync** (`internal/atomicfile/write.go:14-54`; `apply.go:518-550, 1396-1432`; `provenance.go:204-236`): only `atomicfile.Write` fsyncs the parent dir after rename; one ignores Chmod errors; one truncates temp names mid-UTF-8. Two independent writers of `.kicadai/transaction.json` can drift. Consolidate on `atomicfile.Write` + `provenance.Write`.
- **`internal/atomicfile` has zero tests** (also flagged by the repo-health pass): the durability primitive that `manifest`, `creationevidence`, and repair persistence rely on has no coverage for cleanup-on-error, mode preservation, replace-over-existing, or the Windows retry loop.

### kicadfiles
- **Schematic read drops the title block and degrades junctions, sheets, text effects** (`schematic/read.go:72, 82, 192-226`): `title_block` skipped and not preserved; junction diameter/color dropped; sheet `exclude_from_sim`/`in_bom`/`on_board`/`dnp` ignored (BOM-affecting); label/property effects re-rendered at fixed defaults.
- **Footprint properties lose rotation/effects on read; Reference/Value text resets orientation** (`pcb/read.go:112-125`; `pcb/render.go:468-484`): every rewritten board's Ref/Value silkscreen snaps to rotation 0, default size, visible.
- **Custom paper dimensions lost in both readers** (`pcb/read.go:46-48`; `schematic/read.go:49-51`): `(paper "User" 200 150)` round-trips as `(paper "User")` with no dimensions.

### capability / promotion / aiprovider
- **Bundle verification accepts promotion evidence full validation would reject** (`promotionrunner/bundle_build.go:304-312`, `bundle_verify.go:395-403`): `validatePromotionEvidence` collapses gates into a last-wins map and never calls `PromotionReport.Validate()` (which rejects duplicate gate IDs and enforces `RequiredFor`). A bundle whose report lists `{id:"connectivity",status:"fail",...}` followed by `{id:"connectivity",status:"pass"}` verifies clean. The live-run path validates properly; the reviewer-facing `kicadai-promotion verify` path does not.
- **Rule-inferred component evidence silently upgraded to "verified", keeping fabrication-ready eligibility** (`capabilitygate/architecture.go:126-157`): `EvidenceRuleInferred` becomes requirement-linked `EvidenceVerified` plus an advisory record + risk; advisory evidence is excluded from classification, so the design classifies `supported` with `FabricationReadyEligible = true`. A one-way valve around the "inferred ⇒ experimental" rule applied everywhere else.
- **2 MiB response cap too small for streaming responses at the profile's own token limits** (`aiprovider/openai.go:328-338`; `reference_profiles.go:59-74`): SSE mode reads the entire stream through `MaxResponseBytes`; at `DefaultGenericOutputTokens = 32768` (max 65536) the per-token delta framing alone plausibly exceeds 2 MiB → deterministic `ErrorMalformed`, retried once with the same result. Only `KICADAI_AI_BACKGROUND=true` avoids it.
- **Background polling: no overall deadline, fixed 1 s interval, no backoff; 429 never retried** (`aiprovider/openai.go:229-268`): a stuck `in_progress` response polls forever at 1 rps until Ctrl-C; a single transient 429/5xx during polling discards an otherwise-successful background generation.

### closedloopsynthesis / intent
- **`Run` overwrites canceled/budget-exhausted stop reasons with `StopPassed`** (`closedloopsynthesis/runner.go:56-66, 110-112`; `behavioralintent/closed_loop.go:22`): a canceled run can emit `status:"pass", stop_reason:"passed"`; `ApplyClosedLoopEvidence` checks neither `Diagnostics` nor `Consumption`, so any caller gating on report fields alone would qualify an incompletely evaluated run. Current CLI wiring is saved only because `ValidatePromotionReport` runs first.
- **Gain regex captures board dimensions as amplifier gain** (`intentdraft/parse.go:30`, `extract.go:170-176`): the bare `x` alternative in `gainPattern` matches the dimension separator — "headphone amplifier, 30 x 30 mm board, gain of 5" extracts gain = 30 at 0.85 confidence, silently overriding the stated gain of 5. Exactly the silent misinterpretation the clarify layer exists to prevent.
- **One signal connector wired to LED, amplifier, and ESD consumers under conflicting net aliases** (`intentplanner/map.go:656-679`): three independent index counters all resolve connector 0; downstream rejects with a confusing "conflicting PCB fragment net aliases" block instead of the planner allocating distinct connectors or emitting a gap.
- **`cloneSimulationAnalysis` swallows marshal errors and returns a zero value** (`closedloopsynthesis/planned_simulation_resolver.go:2059-2064`): `json.Marshal` fails on NaN/Inf; the clone silently becomes an empty `simmodel.Analysis{}`, diagnosed far from the cause. Every sibling clone helper fails closed.

### architecturesearch
- **Partially reported power/area metrics summed and ranked as if complete** (`search.go:1011-1024, 1786-1794`): a candidate where 1 of 5 fragments declares 1 mW outranks a fully-accounted 2 mW candidate; the board-area gate sums only known areas, so under-reporting providers make the envelope check fail-open.
- **O(objectives² × bindings²) full-document Normalize+Validate in power-role inference** (`search.go:529-556` → `contracts.go:286-290`): `ContractFromBinding` clones and re-validates the whole requirement inside a four-deep loop — up to ~4M clone+validate passes at schema maxima.
- **Frontier re-sorted every iteration with uncached SHA-256-of-JSON keys** (`search.go:90-92, 1719-1723`): the comparator serializes and hashes the *entire state* per comparison, per iteration; makes an intended O(256)-expansion search quadratic-plus with a huge constant. Memoize the key.
- **Map-literal iteration makes translator rejection diagnostics nondeterministic** (`catalog_provider.go:966`): which missing-constraint error is returned varies run to run; the error string lands in `SearchResult.Rejections`. The only unsorted map iteration found in the package.

### placement / routing / fabrication
- **Pad-rotation sign inverted in physicalrules geometry** (`fabrication/physicalrules/evaluate.go:2689-2699, 2712-2724, 1326-1336, 2983-2994`): four functions apply the standard math rotation matrix while the codebase convention (and KiCad) maps +90° to `(x,y)→(y,−x)`. Containment, edge-plating distance, and mask-web bounds use a mirrored rectangle for non-90° rotations; 90°-symmetric geometry (the common case) is unaffected, which is why tests pass.
- **Courtyard overlap check ignores board side** (`evaluate.go:2902-2913, 2935-2947`): F.CrtYd and B.CrtYd merge into one AABB and pairs are compared without side discrimination — back-to-back double-sided mounting (explicitly supported by placement) is falsely `StatusBlocked`.
- **Placement→routing adapter drops inner layers for any copper count except 2 or 4** (`routingadapters/placement.go:118-127`): `count == 6` yields a 2-layer routing board with no diagnostic.
- **`ValidateResult` obstacle check tests trace centerline only** (`routing/validation.go:369-373, 405-417`): the broad-phase inflates by `WidthMM/2`, the narrow phase never does — externally composed/repaired routes can encroach half a trace width into keepouts undetected. Related: `buildOccupancy` uses `obstacle.Clearance + width/2` while `filterPhysicalEndpointAccess` uses `max(Rules.ClearanceMM, obstacle.Clearance)` — the access filter is stricter than the grid the search actually uses, so routable nets can be rejected.
- **Quadratic per-pair work in the dense-board routing path** (`route.go:166, 896-966`; `validation.go:30-88`): full via-occupancy grid rebuilt per crowded pair; `filterPhysicalEndpointAccess` scans all components × pads per access point per pair; `validatePhysicalClearance` is O(segments × pads) with no spatial index (one already exists in `clearanceIssues`).

### circuitgraph / components / libraryresolver
- **Min-only ratings can never satisfy a required rating** (`components/selection.go:1206-1241`): the loop returns true only from `Max`/`Typ` branches; a `Min`-only rating (present in catalog: `mcu.espressif.esp32_wroom_32e` `supply_current_minimum`) falls through to `false` → spurious `COMPONENT_RATING_TOO_LOW`. Unparseable *required* values are also misreported as "too low" rather than invalid-request.
- **Amplifier output pair selection cannot recover from group mismatch** (`components/amplifier_output.go:63-72, 191-205`): top NPN and PNP are picked independently, then pair-validated — if ratings reject one side's alphabetically-first device, the result is a blocked cross-group pair even when a valid same-group pair exists. No pair-wise search.
- **Dangling/one-sided `complementary_group` data in BJT catalogs** (`data/components/npn_transistors.json`, `pnp_transistors.json`): groups referencing absent parts (`st_bd139_bd140` with no BD139 record, `onsemi_ss8050_ss8550` with no SS8050) and one-sided pairs (S9012↔S9013, BC327↔BC337, 2N3906↔2N3904, 2SA1015↔2SC1815, 2SA1358↔2SC3421, 2SD882↔KSB772). `speaker_validation.go:233` requires equal non-empty groups, so these can never form validated pairs. `ValidateCatalog` checks equivalence-group symmetry but not complementary-group symmetry.
- **Catalog hash excludes ThermalPaths** (`circuitgraph/resolve.go:1020-1034`): behavior-bearing thermal paths (consumed by architecturesearch) can change without changing the "immutable catalog snapshot" hash — promoted results can silently mix snapshots.
- **Value/tolerance matching stops at the first constraint of a kind** (`components/selection.go:909-978`): `return` instead of `continue` inside the loop — one malformed or narrower first constraint makes an otherwise-matching record unselectable, with no diagnostic distinguishing parse failure from mismatch.

### simmodel
- **Substep-predictor fallback corrupts the fuse I²t accumulator** (`mna_transient.go:259-273, 1821`): the rejected coarse step's accumulation/reset persists in the shared map while the accepted predictor trajectory's accumulation is discarded; the predictor also starts from an empty accumulator, ignoring prior surge history. Fuse-clearing decisions near the I²t boundary can be wrong in either direction.
- **Closed-state vs clearing-model fuse accumulation semantics contradict each other** (`mna_transient.go:2608-2624` vs `2640-2646`): the clearing model resets on sub-rated intervals (documented rationale); the closed-state model accumulates forever — repeated small surges separated by long recovery eventually trip a false melting diagnostic. One of the two is wrong by the package's own definition.
- **"Excess-current" integral compared against datasheet melting I²t** (`mna_transient.go:2617, 2648`): both models accumulate `(I² − Irated²)·dt` but compare to `nominal_melting_i2t_a2s`, which is plain `∫I²dt`. At I = 1.2·Irated the accumulated value is ~30% of true `∫I²dt` → ~3× overprediction of survivable pulse duration. Anti-conservative for low-multiple overloads (short-circuit cases are fine).

### schematicir / schematiclayout / schematicpcb
- **`Validate` runs 2-3× per layout, duplicating every diagnostic** (`schematiclayout/place.go:49`, `route.go:64, 635-641`): `Place`, `Route`, and `finalizeLayoutCandidate` each call `Validate`, which appends to the existing slice; `NormalizeDiagnostics` sorts and truncates but never dedupes. Counts inflate, duplicates can evict unique findings from the `MaxDiagnostics=100` list, and `schematicir` turns each duplicate into a duplicate blocking issue under `AcceptanceReadable`.
- **Duplicate-net conflict check vacuous on the `ToTransaction` path** (`schematicir/parse.go:145-170`, `validate.go:380-384`, `adapter.go:45-47`): normalization merges duplicate-named nets (first-wins) before validation runs, so conflicting `Role`/`Label` duplicates from programmatic callers are silently merged. Only `DecodeStrict` enforces the rule.
- **Route scoring/validation ignore different-net wires touching at endpoints or overlapping collinearly** (`schematiclayout/route.go:525-535, 407-415`; `validate.go:42-53`): the code's own comment acknowledges KiCad treats either case as an electrical connection, and `labelPlacementCollides` uses the stricter check — but `scoreRoute` and the `wire_crossing` validator use the looser one. A route ending exactly at another net's label-stub endpoint produces an electrical short only downstream ERC can catch.
- **`schematicpcb` net inference misses junction-less wire T-joints** (`transfer.go:355-396`): terminals are built only from junctions/labels/pin anchors; a wire endpoint landing mid-segment on another wire (connected in KiCad's model even without a junction dot) is never unioned — one electrical net splits into two components, producing wrong or missing pad-net hints.

### Repo health
- **~13.5 GB of build caches inside the working tree** (`.gocache/` 6.7G, `.cache/go-build/` 6.2G, `.tmp/gocache` 431M, `internal/compositionlowering/tmp/` 487M): tracked content is only ~27 MB. All git-ignored (no contamination risk), but three overlapping Go build caches waste disk and mask repo contents. The Makefile intentionally sets `GOCACHE=$(CURDIR)/.gocache`; `.cache/go-build` comes from an external env setting.
- **golangci-lint configured to run only `govet`** (`.golangci.yml`): `linters: default: none` + govet only — the "clean lint" signal duplicates `go vet`. `errcheck`, `staticcheck`, ineffassign, unused, etc. are all unchecked; for a codebase this size with heavy numerical/concurrency code, enabling at least `staticcheck` + `errcheck` would add real signal.
- **No race detector anywhere in the test pipeline** (Makefile, `.github/workflows/ci.yml`): no `-race` in any target or CI job, despite mutex-heavy concurrent transport code in `internal/ipc`. A `-race` variant of the `-short` tier would be cheap insurance.
- **Indirect deps frozen at 2021/2022 versions** (`go.mod`): `golang.org/x/sys v0.0.0-20210124...` (~5.5 years stale) and `go-winio v0.5.2`, minimums demanded by mangos v3.4.2. Builds today; a future toolchain or Windows port will trip. `go get golang.org/x/sys@latest` clears it.

---

## Selected Low findings

(Compressed; each verified. Full details available in the per-package review transcripts.)

- **kiapi:** mangos REQ auto-resend can execute a command twice when `TimeoutMS ≥ 60s` (`OptionRetryTime` never set — latent duplicate-mutation bug for future write commands; `client.go:112-119`, `ipc/mangos.go:36-54`). ~22,800 lines of generated `gen/board`/`gen/schematic` bindings are compiled but unreachable. `CapabilitySchematicRead` reported "supported" with no invocable API. Responses over mangos' default 1 MiB recv cap fail as opaque timeouts. Stale future-tense package doc.
- **designworkflow:** capability assessment drops block-evidence load issues when the gate refuses (`capability.go:52`); `PlaceFragments` overwrites its defensively cloned issue slice with the shared original (`placement.go:123,145`); fabrication block-readiness report silently omitted when any block lacks verified evidence (`create.go:227-253`); route-only correction attempts report an empty `retry_adjustment`; `routing.go` is a 6,200-line god file whose phase-ordering invariants are duplicated across two pipelines; `schematic_electrical.go` (silent wire-drop path) and `schematic_layout_inference.go` have no dedicated tests.
- **cmd:** staged-path rewriting in AI design output is byte-literal and breaks on JSON-escaped Windows paths (`ai_output_attempt.go:66,182-236`; evidence also deleted before flush errors are checked); `--json` silently overrides explicit `--format text`; `--experimental` undocumented in usage text; dead `runStructuredCommandSkeleton`; hand-rolled `circuit` sub-flag parsers diverge from the global flag system; allowlist/timeout loading duplicated 3× with different validation; `kicadai-promotion` has zero tests despite gating CI status.
- **transactions/infra:** rollback failures silently swallowed in imported-write rollback (`apply.go:1223-1241`); pcbrules resolver cache hands out a shared `AllowedLayers` slice (aliasing hazard); writercorrectness sheet-discovery containment check runs before symlink resolution (escape bypass); `TransferConfidenceVerified` is unreachable dead vocabulary; `manifest.Read` emits staleness issues in map order.
- **capability/promotion:** `runScenario` ignores the lane-lookup error (`runner.go:91`); capability-evaluation reports carry no self-integrity hash, so the held-out improvement gate trusts editable disk content; `GeneratedCase.ExpectedPass` is declared, hashed, and never enforced; `rationale.LoadFromTarget` reads unbounded JSON and its 949-line `build.go` is untested.
- **closedloopsynthesis:** trusted-diagnostic classification by substring matching on another package's message text (`simmodel_adapter.go:340-354`); `compositionlowering/closed_loop_simulation.go` (3,446 lines) and the two resolver files (2,064 + 1,805) are god files with copy-paste-prone family triples; intentdraft extraction is under-tested (~4:1 code-to-test) for a trust boundary; `mapProtection` early-return trap.
- **architecturesearch:** generated E192 table deviates from IEC 60063 (9.19 vs standard 9.20 at i=185; `preferred_values.go:76-85`); `searchStateKey` swallows marshal errors, silently collapsing distinct states; `rail_sequence_before` accepts equal startup times as zero-margin "proof" of ordering; MCU power-envelope diagnostics with empty measurement names and 0 Ω catalog evidence rejected despite `allowZero`; 4,880- and 3,805-line provider god files; core determinism utilities (`orderSearchObligationsBySignalFlow` cycle fallback, `firstPowerTreeCycle`) untested.
- **placement/routing/fabrication:** keepout-conflict evidence depends on map iteration order (diagnostics only; `placer.go:1087`); via at a two-point layer transition lands on an off-grid aligned endpoint (`vias.go:50-59`); `evaluateBoardContainment` flags edge-connector footprints whose anchor is legitimately outside the outline; mask-web/courtyard pair scans are O(n²) with weak pruning and pair-counted `unknown_geometry` inflation; A* heap growth is unbounded by `MaxSearchNodes` (budget counts pops, not pushes).
- **components/libraryresolver:** engineering-value parser silently drops the multiplier on unrecognized unit suffixes (`"100nX"` → 100, a 10⁹ error, passes validation; `model.go:1174-1207`); IRF3205 power dissipation understated vs its own cited datasheet (150 W vs 200 W; conservative, but claims verified provenance — 2N7000/FQD8P10 Vgs entries also merit re-verification); library index diagnostics ordered by map iteration (cached, too); `firstCandidates` is dead code.
- **simmodel:** transient SOA excursion clock lags one time step (anti-conservative by one dt); `sameOpAmpClamps` treats a key missing from one map as value 0 — the one place found that inverts the package's fail-closed posture; assertion identity excludes bounds so duplicate assertions alias in sensitivity/directed corners; duplicate corner rows for single-group plans; periodic zero-crossing warm-start decays with elapsed time (perf only); SOA-margin assertions on devices without SOA evidence report 0 instead of a diagnostic; electrothermal condition events override the base condition before their trigger; amplifiers `simulationInputAttenuation` can silently drop requested attenuation in the opt-in artifact path.
- **blocks:** `"ohms"`/`"Ohms"` unit spelling parses nondeterministically — the alias map iteration applies `"ohm"` before `"ohms"` on ~half of runs, producing a blocking parse error for valid input (`led.go:232-247`; effectively Medium — randomly rejects valid designs); `class_ab_output_stage` lacks the peak-current gate its sibling enforces (322 mA through a 200 mA-max MMBT3904 instantiates without warning); `headphone_output_protection` PCB realization keys nets by port names, breaking the package convention (PCB copper on phantom nets); stale `usb_c_power` warning claims no-connect ops are unsupported while the same file uses them; `ResolveCompositionNetAliases` returns nondeterministic order; `led_forward_voltage` accepts non-positive values; block tests validate structure, not electrical topology (root cause of C1/H1/H2 escaping CI).
- **repair:** zone refill treats any exit code 1 as success (`zone_refill.go:103-105` — argument/file errors report `Ran: true`); `regeneratePadNets` no-change path skips revalidation and drops decode-failure evidence; `Classify` text heuristics route "zone fill clearance" issues to rerouting (zone branch unreachable for messages that also mention clearance).
- **Repo hygiene:** empty leftover `internal/tmp_corpus_report/` and `.tmpdebug/` dirs (untracked, unignored); 51 MB of stale compiled test binaries at the repo root (ignored, deletable); `specs/tolerance-sensitivity/report_embed.go` participates in `go build ./...` from inside `specs/` (tiny, purposeful — staleness gate for the capability report — but unconventional).

---

## Cross-cutting themes

1. **Block tests assert structure, not topology.** The Critical and two of the High findings are wiring errors behind passing tests that count operations and check net-name presence but never assert which pin lands on which net. A shared helper decoding `ConnectOperation`s into a role/pin→net map, asserted against golden topology (including diode polarity from catalog `function_pins`), would have caught all three and converts this whole failure class into test failures.
2. **Read→write fidelity is one-directional.** The writers are excellent (deterministic, exact coordinate math, careful UUID preservation, journaled commits); the readers parse a subset and the renderers rebuild from the lossy model. Everything on the imported-mutation path inherits this. Either extend `Raw`-node preservation to footprint/zone/setup level (the pad-level pattern already exists) or gate imported mutation on detecting unsupported constructs.
3. **Policy surface without enforcement.** `ViaClearanceMM`, `routing_retry.drc_policy: required`, `net_overrides[].role`, `GeneratedCase.ExpectedPass`, `TransferConfidenceVerified`, `CapabilitySchematicRead` — configuration and vocabulary that is accepted, documented, or defaulted but wired to nothing. Each is a silent fail-open relative to its declared contract. An audit pass that greps every config field to its enforcement site would pay off.
4. **Exit codes vs JSON `ok` disagree in places.** The CLI's contract (non-zero exit whenever `ok: false`) is followed by most commands but broken by `place`, `route`, partial `repair`, and skipped `check` — precisely the commands CI most wants to gate on.
5. **Residual map-order nondeterminism.** ~8 sites survive in a codebase that otherwise sorts everything: `compositionlowering.lowerConnections` (changes hashes — High), blocks `normalizeUnitLiteral` (changes behavior), and diagnostic-ordering sites in designworkflow, architecturesearch, manifest, placement, libraryresolver, blocks. All are one-line sorted-key fixes.
6. **God files concentrate the risk.** `main.go` (5,952), `routing.go` (6,200), `catalog_provider_multifunction.go` (4,880), `catalog_provider.go` (3,805), `closed_loop_simulation.go` (3,446), `adapter.go` (2,478), `compositionlowering` (4,681 in 2 files). Several duplicated invariants (post-processing chains, stage sequences) already diverged across their copies (H12, explicit-circuit divergence, class-AB peak-current gate).
7. **Durability primitives deserve consolidation and tests.** Four atomic-write implementations with different fsync/chmod behavior, an untested `atomicfile`, backup-then-rename windows, and unconditional lock removal — the transaction layer's ideas are right but the implementations have drifted.

## What is notably strong

- Determinism engineering: sorted emission nearly everywhere, canonical hashing with normalization before sealing, deterministic v5 UUIDs with length-prefixed name components, corpus-freeze regression suites.
- Fail-closed layering: `Compile` nulls the requirement on blocking issues; promotion validation re-run independently in `designworkflow`; follow-up hash-binding at the provider boundary is a genuinely strong replay/injection defense.
- Numerical core: MNA solver with scaled partial pivoting and residual verification, iteration-capped Newton with continuation fallbacks, correct device physics and worst-case corner math; no rand/time/map-order nondeterminism found in `simmodel`.
- Catalog data quality: all 47 BJT and 18 MOSFET pinouts spot-checked correct against real datasheets; aggressive load-time validation.
- S-expression core: strict parser (depth cap, bounds-checked, trailing-input rejection), exact integer-IU coordinate formatting, journaled two-phase directory commit with crash recovery.
- CI: SHA-pinned actions, 75% coverage floor, 11 sharded frozen-corpus regression jobs, gofmt gating.
- API-key hygiene: the OpenAI key never reaches logs, evidence, or replay artifacts; prompts are size-bounded and hashed rather than stored.

## Prioritized recommendations

1. **Fix the three block wiring bugs (C1, H1, H2)** and add a topology-assertion test helper so the class can't recur.
2. **Close the gating gaps (H8–H12, H17, `check` skip-exit-0):** repair ordering + honest `StatusRepaired`, DRC crash carve-out, dead retry-DRC plumbing, CLI exit codes.
3. **Protect user data (H6, H7, H10, readback-after-delete):** rename-over instead of backup-then-rename, `flock`-based locking, rollback for persisted repair, readback against staged temps. Add tests to `internal/atomicfile` and consolidate the four atomic-write implementations.
4. **Guard the imported-mutation path (H3–H5):** either extend raw-preservation to setup/footprint/zone level or refuse to rewrite files containing constructs the reader doesn't model.
5. **Enforce or remove dead policy surface (H14, `net_overrides[].role`, `ExpectedPass`, etc.).**
6. **Fix the two behavioral nondeterminisms (H16, `normalizeUnitLiteral`)** and sweep the remaining diagnostic-order sites.
7. **Component selection correctness (H15, min-only ratings, pair search, catalog group symmetry + a `ValidateCatalog` check for complementary groups).**
8. **Routing/DFM correctness (H13, centerline-only keepout validation, pad-rotation sign, courtyard side-awareness, 6-layer adapter gap).**
9. **Fuse model physics (I²t accumulator integrity + excess-current vs melting-I²t comparison).**
10. **Tooling:** enable `staticcheck` + `errcheck` in `.golangci.yml`, add a `-race` CI tier, bump `golang.org/x/sys`, reclaim ~13 GB of redundant build caches, and add tests for `kicadai-promotion`.
