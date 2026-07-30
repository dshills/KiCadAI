# Fable Code Review Remediation Plan

Date: 2026-07-29
Status: In progress (Phases 0–8 complete)
Baseline commit: `7c7fd9c0`
Source review: [`../FABLE_CODE_REVIEW_07_29_26.md`](../FABLE_CODE_REVIEW_07_29_26.md)

## Implementation Status

| Phase | Status | Evidence |
| --- | --- | --- |
| 0. Reproductions and evidence boundaries | Complete locally | [`FINDINGS.json`](FINDINGS.json) records every `C1`–`H17` owner, reproduction, capability impact, disposition, implementation phase, and closing receipt. [`TRANSACTION_SNAPSHOTS.json`](TRANSACTION_SNAPSHOTS.json) freezes three normalized block-operation digests and four protected design-transaction digests. [`BASELINE.md`](BASELINE.md) records tool versions and the local validation ladder. Closed Phase 1–2 findings retain their historical invalidation; pending findings have deterministic characterization or fault-injection tests and no closing commit. |
| 1. Electrical topology correctness | Complete locally | Catalog-aware topology projection covers diode/BJT functions, supply/output ports, and exported port contact; AC op-amp feedback references midpoint bias and preserves supported gain within 2%; block verification, design workflow, writer/round-trip checks, and the installed-KiCad amplifier/ESP32/USB-C fixture matrix pass. The structurally verified diode-string blocks retain their explicit unsupported quiescent-current/fabrication claims. |
| 2. Honest acceptance, repair, and CLI results | Complete locally | Transactions reject mutation after `write_project` and late footprint assignment; footprint repair inserts before placement/write; persisted repairs retain originating issues unless supported post-mutation validation proves them absent; required KiCad evidence fails closed; unsupported required retry DRC policy is rejected; blocking place/route/repair/check reports return errors. Focused suites, full `go test -short ./...`, lint, writer/round-trip tests, and the installed-KiCad amplifier/ESP32/USB-C fixture matrix pass. |
| 3. Imported KiCad preservation | Complete locally | PCB and schematic readers inventory fully modeled, safely raw-preserved, and mutation-unsupported constructs. Imported board metadata, custom paper, setup/plot data, footprint metadata/text/models/unknown children, schematic presentation nodes, and keepouts survive read-modify-write. Duplicate singleton constructs fail closed. Staged output is internally re-read and compared with source preservation evidence before any live replacement. Focused reader, round-trip, transaction, and unchanged-source refusal tests pass. |
| 4. Recoverable project mutation | Complete locally | Imported apply and persisted repair share a same-directory, hash-verified group commit with retained rollback data, a synced state journal, deterministic startup recovery, ownership-token locks, advisory locking where portable, and Windows replacement via `MoveFileExW`. Transition fault tests prove recovery to all-old or all-new, and worsened repair validation restores the prior project. |
| 5. Routing and DFM geometry | Complete locally | One effective object-pair clearance policy now drives trace/via search and validation. Trace half-width and via radius constrain inward-quantized board-edge eligibility; final validation independently checks copper-to-edge and full-shape obstacle clearance. `ViaClearanceMM`, conservative net-class pairing, KiCad pad rotation, side-aware courtyards, and arbitrary copper-layer adaptation have focused boundary coverage. Protected USB-C fixtures retain clean installed-KiCad evidence. |
| 6. Component values and lowering determinism | Complete locally | Canonical value encoding, op-amp clamp comparison, deterministic condition events, stable explicit-net ordering, and order-independent lowering are closed by commit `1f8e34b7`. |
| 7. Workflow, capability, architecture, and connectivity | Complete locally | [`PHASE7_AUDIT.md`](PHASE7_AUDIT.md) records the five focused closing commits, repository-wide local validation, installed-KiCad evidence, and Prism disposition. |
| 8. Simulation and protection physics | Complete locally | [`PHASE8_AUDIT.md`](PHASE8_AUDIT.md) records transactional fuse I²t state, corrected SOA timing/evidence, deterministic tolerance identity, analytic/refinement tests, local validation, installed-KiCad amplifier evidence, and the bounded promotion-fixture routing limitation. |
| 9. Durability tooling and repository health | Proposed | Not started. |

## 1. Goal

Close the correctness, evidence-integrity, imported-project safety, durability,
determinism, and engineering-model defects identified by the July 29 Fable
review. The work must restore trustworthy claims before expanding arbitrary
circuit generation:

- generated circuit blocks are electrically correct, not merely structurally
  complete;
- acceptance levels are backed by the checks they name;
- repair status reflects observed post-repair validation;
- imported KiCad projects are either preserved losslessly or rejected before
  mutation;
- multi-file writes are recoverable and do not admit concurrent writers;
- routing and component policies are actually enforced;
- identical inputs produce identical normalized outputs and hashes.

Critical and High findings are release blockers for the affected capability.
Medium and Low findings are handled by risk cluster after the trust boundary is
restored.

## 2. Non-goals

- Adding new circuit families, parts, or arbitrary-circuit breadth while
  Critical or High findings remain open.
- Adding fixture-specific coordinates, net names, block-family branches,
  library allowlists, or acceptance exceptions.
- Replacing KiCad's ERC or DRC with internal approximations.
- Claiming byte-for-byte preservation for constructs that the reader cannot
  model or safely raw-preserve.
- Large package splits solely to reduce file length.
- Weakening an existing gate to preserve a stale promotion result.
- Requiring network access, an AI provider, or GitHub Actions for remediation
  acceptance.

## 3. Implementation Rules

1. Begin each behavioral correction with a focused regression that fails for
   the cited root cause.
2. Prefer a shared invariant or policy implementation over a one-call-site
   patch.
3. Keep production behavior independent of fixture IDs, paths, coordinates,
   reference designators, and board names.
4. When support is incomplete, fail before mutation and explain the unsupported
   construct or missing evidence.
5. Derive success from observed validation results. Never remove an issue from
   a report merely because a repair operation was attempted.
6. Use one rule calculation in route search and final validation so the two
   cannot drift.
7. Preserve deterministic ordering in reports, artifacts, serialized
   transactions, hashes, and candidate selection.
8. Keep default tests offline. Run installed-KiCad checks locally when a phase
   changes generated electrical content, PCB content, writer behavior, routing,
   or acceptance evidence.
9. Preserve the existing protected USB-C LED, protected USB-C I2C sensor, ESP32,
   and unaffected amplifier evidence unless stricter validation proves that the
   evidence was invalid. Record an honest downgrade when it does.
10. Run focused tests after every edit-sized fix and the phase test matrix
    before staging.
11. Review each staged phase with `prism review staged`; resolve material
    findings before committing.
12. Commit completed phases separately. Required validation is local; do not
    initiate or wait on GitHub Actions as part of this plan.

## 4. Finding-to-Phase Ledger

| Finding | Disposition | Phase |
| --- | --- | --- |
| `C1` reversed Class AB bias diodes | Fix and add polarity-aware topology proof | 1 |
| `H1` AC-coupled op-amp DC saturation | Fix DC bias topology and add operating-point proof | 1 |
| `H2` disconnected `DRIVER_OUT` | Remove string-key mismatch and add port-contact proof | 1 |
| `H3` board setup/title/general loss | Preserve or reject before mutation | 3 |
| `H4` footprint child loss | Preserve or reject before mutation | 3 |
| `H5` keepout conversion | Model/preserve keepout semantics or reject | 3 |
| `H6` missing-file crash window | Replace with recoverable group commit | 4 |
| `H7` stale-lock race | Add ownership-safe locking | 4 |
| `H8` repair operation after write | Enforce transaction phase ordering | 2 |
| `H9` unverified repaired status | Re-run originating validators | 2 |
| `H10` partial repair replacement | Reuse recoverable group commit and rollback | 4 |
| `H11` DRC crash counts as acceptance | Make required evidence fail closed | 2 |
| `H12` dead required retry DRC policy | Enforce it or reject the policy | 2 |
| `H13` copper-to-edge under-clearance | Share width-aware edge geometry | 5 |
| `H14` unenforced via clearance | Enforce in search and validation | 5 |
| `H15` exact-float component matching | Canonicalize engineering quantities | 6 |
| `H16` nondeterministic lowering metadata | Use stable member order | 6 |
| `H17` successful exit on blocking route/place | Align process exit with report status | 2 |

Medium and Low findings are retained in the source review. Phases 3 through 8
identify the subsystem cluster responsible for each; Phase 0 creates the
machine-readable ledger used to prevent omissions.

### Execution order and stop conditions

| Order | Phase | Dependency | Stop condition |
| --- | --- | --- | --- |
| 1 | Phase 0 | Current committed baseline | Any Critical/High claim lacks a stable reproduction or explicit evidence disposition |
| 2 | Phase 1 | Phase 0 topology captures | Any affected amplifier block lacks topology and behavioral proof |
| 3 | Phase 2 | Phase 0 acceptance reproductions | Any required check can still fail open or a blocking CLI result exits zero |
| 4 | Phase 3 | Phase 0 imported corpus | Any unsupported imported construct can reach a live write |
| 5 | Phase 4 | Phase 3 preservation guard | Fault injection can expose missing/mixed files or concurrent ownership |
| 6 | Phases 5 and 6 | Phases 1–4 complete | Routing policy remains unenforced or normalized output remains nondeterministic |
| 7 | Phases 7 and 8 | All Critical/High findings closed | A Medium safety/model finding remains open without documented disposition |
| 8 | Phase 9 and closeout | Behavioral work complete | Local validation ladder or Prism review is not clean |

Do not resume arbitrary-circuit breadth work until Phases 1 through 6 are
complete. Phases 7 through 9 may be split into focused follow-up milestones,
but none may leave a known fail-open safety or fabrication claim.

## 5. Phase 0 — Freeze Reproductions, Evidence, and Capability Boundaries

### Objective

Turn each Critical and High finding into a deterministic local regression and
record which existing capability claims are affected before production behavior
changes.

Phase 0 was completed after Phases 1 and 2. The ledger therefore records the
review baseline separately from the implementation base and preserves the
historical invalidation for already-closed findings.

### Work

1. Record the baseline commit, Go version, KiCad CLI version, Prism version, and
   existing local test commands.
2. Create a finding ledger containing:
   - finding ID;
   - owning package;
   - regression test or fixture;
   - current disposition;
   - affected capability/promotion evidence;
   - implementation phase;
   - closing commit and verification receipt fields.
3. Add failing reproductions for all Critical and High findings. Tests may use
   fault-injection seams where process crash or file-system failure cannot be
   reproduced portably.
4. Capture current normalized transactions for:
   - `class_ab_output_stage`;
   - AC-coupled single-supply op-amp;
   - `amplifier_bias_network`;
   - protected Class AB amplifier fixtures;
   - protected USB-C LED and I2C fixtures.
5. Record the imported-PCB constructs needed for preservation tests:
   non-default `general`, `setup`, `title_block`, footprint text/model/metadata,
   and keepout/rule-area zones.
6. Mark affected amplifier block evidence as invalid or pending until Phase 1
   re-verifies topology. Do not delete the old evidence; retain it as historical
   evidence with an explicit invalidation reason.
7. Confirm the current suite passes despite each reproduced defect. This proves
   the new regressions exercise missing invariants rather than unrelated
   baseline failures.

### Tests and evidence

- Individual regression commands for `C1` through `H17`.
- `go test ./internal/blocks ./internal/kicadfiles/pcb`
- `go test ./internal/transactions ./internal/repair`
- `go test ./internal/designworkflow ./internal/routing`
- `go test ./internal/components ./internal/compositionlowering`
- CLI-level tests under `./cmd/kicadai`.

### Acceptance

- Every Critical and High finding has a stable reproduction or fault-injection
  test.
- Every affected promotion claim is listed and is not silently treated as
  current proof.
- Reproductions use generic inputs and no fixture-specific production logic.
- No production behavior changes in this phase except an evidence downgrade
  required to stop a known-invalid claim.

### Commit

`Add Fable review regression ledger`

## 6. Phase 1 — Restore Electrical Topology Correctness

Findings: `C1`, `H1`, `H2`, and the related block-testing Low findings.

### Objective

Prevent structurally valid operations from representing an electrically wrong
circuit and re-establish amplifier evidence from topology and behavior.

### Work

1. Add a reusable topology projection helper for block tests that:
   - decodes symbol, port, net, and connect operations;
   - resolves component role and pin function from the catalog;
   - produces a stable `role.pin_function -> net_role` view;
   - detects disconnected ports, conflicting nets, reversed polar devices, and
     unexpected aliases;
   - sorts all projected evidence deterministically.
2. Fix the Class AB diode string using pin-function semantics:
   - each diode anode/cathode orientation must support the intended quiescent
     current path;
   - tests must assert catalog functions, not literal pin numbers alone;
   - apply the helper to sibling Class AB and bias-network blocks to prevent
     future copy/paste inversion.
3. Fix AC-coupled single-supply op-amp bias:
   - make the feedback network's DC gain relative to the midpoint bias equal to
     one;
   - choose a generic DC-consistent topology, such as referencing the gain leg
     to the midpoint or AC-isolating the ground leg;
   - preserve requested small-signal AC gain;
   - do not special-case a component MPN or gain value.
4. Fix `amplifier_bias_network` port binding:
   - replace duplicated free-form string keys with one normalized semantic net
     identity;
   - make missing declared-port contact a blocking block-validation error;
   - prove schematic and PCB aliases resolve to the same logical net.
5. Extend topology assertions to every applicable polar block component in the
   affected amplifier family: diodes, BJTs/MOSFETs, electrolytics, and supply
   ports. Do not invent polarity assertions for nonpolar components or blocks
   outside the three affected topology paths.
6. Add electrical behavior checks where the affected block contract makes the
   corresponding behavioral claim:
   - op-amp DC output remains near the midpoint bias across supported gain
     settings;
   - small-signal closed-loop gain remains within the block contract;
   - a Class AB topology with simulation-backed quiescent-current claims must
     bound that current and exclude uncontrolled simultaneous conduction;
   - a structural-only Class AB diode-string topology must retain explicit
     unsupported quiescent-current and fabrication claims rather than promote
     operation-count evidence;
   - the driver port has a continuous path to the bias midpoint.
7. Regenerate block verification and promotion evidence only after all topology,
   simulation, and installed-KiCad gates pass.

### Tests

- `go test ./internal/blocks -count=1`
- Focused graph/simulation tests for the three blocks.
- Relevant component pin-map and catalog validation tests.
- Optional installed-KiCad amplifier fixture matrix:
  `class_a_bjt_line_preamplifier`, `class_ab_headphone_driver`,
  `class_ab_headphone_protected`, and `class_ab_speaker_10w_protected`.
- Preserve local pass evidence for `usb_c_led_indicator_protected` and
  `usb_c_i2c_sensor_3v3_protected`.

### Acceptance

- The three original broken topologies fail under the new helper.
- Corrected blocks pass topology, operating-point, gain, ERC, connectivity, and
  applicable strict DRC checks.
- No amplifier evidence is promoted from operation counts alone.
- Repeated generation produces identical normalized transactions and evidence.

### Commit

`Fix amplifier electrical topology`

## 7. Phase 2 — Make Acceptance, Repair, and CLI Results Honest

Findings: `H8`, `H9`, `H11`, `H12`, `H17`; related `cmd`,
`designworkflow`, and repair Medium findings.

### Objective

Make every success status, acceptance level, and process exit code reflect
observed evidence.

### Work

1. Define and validate transaction operation phases:
   - construction and mutation operations precede `write_project`;
   - post-write checks follow it;
   - an operation in an invalid phase is rejected rather than silently ignored.
2. Change footprint-assignment repair to use the shared pre-write insertion
   path and add a replay test proving the written PCB contains the assignment.
3. Replace attempted-issue removal with validator-driven repair status:
   - retain the originating validator identity and parameters;
   - re-run it after each attempted repair;
   - compare stable issue identities before and after;
   - report `repaired` only when the original issue is absent and no equal-or-
     worse blocking regression appears;
   - report `partial` or `blocked` from the observed delta.
4. Make required KiCad checks strict:
   - a DRC crash, missing output, parse failure, or unavailable CLI is blocking
     when DRC is required;
   - optional checks may report skipped/warning but cannot raise achieved
     acceptance;
   - `erc_drc` requires readable ERC and DRC evidence for the same artifact
     revision.
5. Wire retry DRC evidence into production attempt construction, ranking, and
   stopping. If a supported per-attempt snapshot cannot be checked, reject
   `drc_policy: required` during validation until enforcement exists.
6. Align JSON and process contracts:
   - `place request`, `route request`, partial/blocked repair, and required
     `check` failures return non-zero;
   - successful reports return zero;
   - pretty and compact JSON behave identically;
   - stdout remains machine-readable and diagnostics remain on stderr.
7. Add cancellation tests for affected long-running commands while touching
   their command boundaries.

### Tests

- `go test ./internal/repair ./internal/designworkflow -count=1`
- `go test ./cmd/kicadai -count=1`
- CLI subprocess tests asserting both JSON `ok` and exit status.
- Fake KiCad-runner tests for crash, no output, malformed output, missing tool,
  clean result, and DRC regression.
- One local installed-KiCad retry fixture when retry DRC is enabled.

### Acceptance

- No operation appended after `write_project` can claim to affect written
  output.
- `StatusRepaired` always has post-repair evidence from the validator that
  raised the issue.
- Required ERC/DRC acceptance cannot be achieved from missing or crashed checks.
- Every CLI report with blocking issues exits non-zero.
- Required retry DRC is either demonstrably enforced or rejected as unsupported.

### Commit

`Make workflow acceptance fail closed`

## 8. Phase 3 — Guard and Preserve Imported KiCad Mutation

Findings: `H3`, `H4`, `H5`; related PCB/schematic round-trip Medium findings.

### Objective

Ensure imported projects are never silently rewritten through a lossy model.

### Work

1. Add a preservation-capability inventory to PCB and schematic readers:
   - fully modeled;
   - safely raw-preserved;
   - unsupported for mutation.
2. Before any imported mutation, traverse the parsed project and block if an
   unsupported construct would be discarded, reordered unsafely, or converted
   to a different semantic object.
3. Build KiCad-authored round-trip fixtures covering:
   - non-default board thickness and `general`;
   - stackup, mask, plot, and other `setup` content;
   - `title_block` and custom paper dimensions;
   - footprint text, description, tags, lock state, properties with effects and
     rotation, models, mask/paste margins, and zone-connect behavior;
   - keepout/rule-area flags and zone fill modes;
   - schematic title, junction appearance, sheet BOM/on-board/DNP flags, and
     text effects.
4. For each node family, choose one explicit strategy:
   - model and render every behavior-bearing field; or
   - splice preserved unknown children around modeled fields without duplicate
     emission; or
   - reject mutation with a structured unsupported-preservation issue.
5. Parse keepout zones as keepouts. Never default an unrecognized zone into a
   net-0 copper zone.
6. Make preservation reports compare the source AST and staged output AST,
   including substituted defaults, not only missing nodes.
7. Validate staged output through the KiCad reader and internal reader before
   permitting commit.
8. Expand support incrementally; retain fail-closed rejection for constructs
   not yet covered.

### Tests

- `go test ./internal/kicadfiles/... -count=1`
- `go test ./internal/transactions -count=1`
- KiCad-authored PCB and schematic round-trip corpus.
- Supported constructs: zero normalized semantic differences.
- Unsupported constructs: deterministic pre-mutation refusal and unchanged
  source hashes.

### Acceptance

- `general`, `setup`, `title_block`, footprint metadata/models/text, and
  keepout semantics survive supported mutation.
- A construct not proven safe prevents mutation before any live file changes.
- Preservation evidence detects both deletion and default substitution.
- Supported fixtures have zero normalized round-trip differences.

### Commit

`Guard imported KiCad preservation`

## 9. Phase 4 — Unify Durable Multi-file Commit and Locking

Findings: `H6`, `H7`, `H10`; related transaction/infrastructure Medium and Low
findings.

Status: Complete locally. The shared `internal/atomicfile` group commit stages
and syncs every member, validates staged content, records prior and replacement
hashes in a synced journal, replaces through the platform abstraction, verifies
the committed state, and retains rollback data until application validation
succeeds. Startup recovery rolls incomplete states back and completes cleanup
for validated commits. Imported apply and persisted repair use this primitive;
repair holds the project lock through post-apply validation and restores the
prior project when evidence worsens. Token-checked locks prevent stale takeover
and release races, with process-start identity on Linux and advisory locking on
Unix. Deterministic fault tests cover every required transition, and the
Windows implementation cross-compiles with an overwrite-replacement test.

### Objective

Make imported apply and persisted repair recoverable across process crashes,
partial I/O failure, and concurrent invocations.

### Work

1. Consolidate the four atomic-write implementations behind one tested
   durability package.
2. Define a recoverable group-commit protocol:
   - render all target files to same-directory staging files;
   - sync staged file contents;
   - validate staged readback and cross-file consistency;
   - write and sync a journal describing target, staged file, prior identity,
     replacement identity, and commit state;
   - replace targets using the platform-safe atomic replacement primitive;
   - sync parent directories;
   - validate committed hashes;
   - mark the journal complete before cleaning recoverable artifacts.
3. Add startup recovery:
   - detect incomplete journals before another apply;
   - deterministically finish or roll back based on recorded state and hashes;
   - never guess from filename order alone.
4. Replace PID-only stale locking with ownership-safe locking:
   - unique nonce/token per holder;
   - PID and process-start identity where available;
   - removal only when the on-disk token still matches the inspected token;
   - release only by the current owner;
   - OS advisory locking where portable, with a token protocol as the tested
     cross-platform contract.
5. Route imported apply and persisted repair through the same group-commit
   primitive.
6. Preserve the repair marker or journal on failed commit or failed rollback;
   remove it only after a fully resolved outcome.
7. Run post-apply validation before discarding rollback data. If validation is
   worse, restore the prior project and report the failed repair.
8. Surface rollback and recovery failures as blocking issues; do not swallow
   them during cleanup.
9. Add deterministic fault injection at every transition:
   staging, file sync, journal sync, replacement, directory sync, validation,
   rollback, and cleanup.

### Tests

- `go test ./internal/atomicfile ./internal/transactions ./internal/repair -count=1`
- Concurrent stale-lock takeover tests.
- Fault-injection table tests proving either all-old or all-new observable
  project state after recovery.
- Windows-specific replacement tests in the existing platform abstraction;
  do not assume Unix rename-over semantics are portable.

### Acceptance

- No crash point leaves the project unrecoverably missing or mixed.
- A second writer cannot remove or release a live writer's lock.
- Imported apply and repair share one durability contract.
- Backups/recovery data survive until post-apply validation succeeds.
- All rollback failures are visible and blocking.

### Commit

`Make project mutation recoverable`

## 10. Phase 5 — Enforce Routing and DFM Geometry

Findings: `H13`, `H14`; routing/fabrication Medium findings.

### Objective

Use physically correct copper geometry and the declared effective rule set in
both route search and final validation.

### Work

1. Centralize effective clearance calculations by moving object pair:
   trace-to-edge, via-to-edge, trace-to-obstacle, via-to-obstacle,
   trace-to-trace, trace-to-via, and via-to-via.
2. Include trace half-width or via radius in board-edge eligibility.
3. Quantize inward conservatively so grid rounding cannot reduce the required
   physical clearance.
4. Add a final copper-shape-to-board-edge validation backstop independent of
   search occupancy.
5. Enforce `ViaClearanceMM` in:
   - via occupancy;
   - via-to-pad/obstacle checks;
   - via-to-trace and via-to-via checks;
   - final result validation;
   - effective net-class and override resolution.
6. Define asymmetric net-class pairing conservatively, normally using the
   greater applicable clearance unless the rule model specifies otherwise.
7. Make obstacle validation use full trace geometry rather than centerline
   intersection.
8. Fix the related pad-rotation convention, board-side-aware courtyard
   comparison, and non-2/non-4-layer adapter behavior.
9. Add property and boundary tests around exact-clearance, one-grid-step-inside,
   and one-grid-step-outside cases.
10. Run unchanged protected USB-C and amplifier fixture matrices after every
    geometry correction.

### Tests

- `go test ./internal/routing ./internal/fabrication/... -count=1`
- Routing property tests across grid, width, diameter, and clearance values.
- Strict KiCad DRC on all generated fixtures whose routes change.
- Route completion, writer correctness, and zero normalized round-trip
  differences for the protected USB-C LED and I2C fixtures.

### Acceptance

- No accepted trace or via violates copper-to-edge clearance after considering
  copper radius and grid quantization.
- Changing `ViaClearanceMM` changes both search legality and validation.
- Search and final validation agree at rule boundaries.
- Existing pass fixtures remain pass or receive an evidence-backed,
  fail-closed regression report.

### Commit

`Enforce physical routing clearances`

## 11. Phase 6 — Canonicalize Component Values and Lowering Determinism

Findings: `H15`, `H16`; component, catalog, architecture-search, and
determinism Medium/Low findings.

### Objective

Remove spelling-dependent selection and map-order-dependent circuit identity.

### Work

1. Replace exact binary-float equality for component values with canonical
   engineering quantities:
   - parse recognized SI prefixes and unit aliases;
   - reject unknown suffixes rather than dropping a multiplier;
   - normalize to a dimension plus exact decimal/rational magnitude where
     practical;
   - compare canonical quantities, not original spelling.
2. Continue through all constraints of a kind rather than returning from the
   first malformed or nonmatching entry.
3. Correct min-only rating handling and distinguish invalid requested values
   from genuinely insufficient ratings.
4. Select complementary transistor pairs jointly, then rank valid pairs;
   validate complementary-group symmetry at catalog load.
5. Include thermal paths and all behavior-bearing evidence in catalog hashes.
6. Iterate composition members in the already sorted node order before
   combining metadata.
7. Define deterministic conflict behavior for two non-empty voltage domains
   instead of relying on first encounter.
8. Fix `ohm`/`ohms` normalization by longest-token or explicit alias lookup,
   never map iteration.
9. Sweep remaining report-order nondeterminism in design workflow,
   architecture search, manifests, placement, library resolver, and block alias
   resolution.
10. Add repeated-process determinism tests, not only repeated calls in one Go
    process.

### Tests

- `go test ./internal/components ./internal/compositionlowering ./internal/blocks -count=1`
- Equivalence matrix: `0.1u == 100n`, `0.47u == 470n`,
  `1.5n == 1500p`, and invalid suffix rejection.
- Catalog symmetry and hash-change tests.
- Repeated lowering and evidence hashing under shuffled input.

### Acceptance

- Equivalent engineering values select the same candidates.
- Invalid suffixes fail closed with an invalid-value issue.
- Lowered documents, metadata, hashes, and diagnostics are identical across
  repeated processes.
- Complementary-pair search finds an available valid pair rather than blocking
  on independently selected mismatches.

### Commit

`Canonicalize component and lowering behavior`

## 12. Phase 7 — Close Remaining Workflow, Capability, and Schematic Medium Risk

Status: complete on 2026-07-30. Implementation and local acceptance evidence are
recorded in `PHASE7_AUDIT.md`.

### Objective

Resolve the Medium findings that can misstate capability, drop required work,
or create incorrect connectivity.

### Work streams

1. **Design workflow**
   - sort inter-block proof and issue emission;
   - replace machine-speed-dependent routing timeout outcomes with deterministic
     work budgets, retaining an outer cancellation deadline;
   - embed block verification evidence or fail loudly when unavailable;
   - unify explicit-circuit and block-planned stage contracts.
2. **CLI and provider**
   - use the embedded catalog consistently;
   - propagate cancellation through long commands and IPC;
   - bound AI responses consistently with configured token limits;
   - add background polling deadline, backoff, and transient retry policy.
3. **Capability and promotion**
   - validate promotion reports through one authoritative validator;
   - prevent inferred evidence from becoming fabrication-eligible verified
     evidence;
   - enforce generated-case expectations or remove the dead field.
4. **Architecture search**
   - distinguish complete from partial power/area accounting;
   - cache state keys;
   - move whole-document validation out of inner loops;
   - sort all rejection diagnostics.
5. **Schematic connectivity**
   - deduplicate validation diagnostics;
   - validate duplicate net conflicts before normalization destroys evidence;
   - treat different-net endpoint contact and collinear overlap as shorts;
   - union junction-less wire T-joints consistently with KiCad.

### Tests

- Focused package tests for each work stream.
- Characterization tests proving explicit and block-planned pipelines reach the
  same applicable stages.
- CLI cancellation and provider fake-server tests.
- KiCad ERC checks for schematic connectivity changes.

### Acceptance

- No Medium finding in these clusters remains `open` without a documented
  deferral, owner, and safety justification.
- Capability and promotion results cannot become stronger from missing or
  inferred evidence.
- Schematic connectivity semantics agree with KiCad for the covered cases.

### Commit strategy

Use one focused commit per work stream rather than one cross-package commit.

## 13. Phase 8 — Correct Simulation and Protection Physics

Status: complete on 2026-07-30. Implementation and local acceptance evidence are
recorded in `PHASE8_AUDIT.md`.

### Objective

Make protection and electrothermal decisions conservative and consistent with
the declared datasheet quantities.

### Work

1. Define one fuse I²t state model shared by coarse and predictor trajectories.
2. Ensure rejected substeps cannot mutate accepted accumulated state.
3. Preserve pre-existing surge history when entering predictor evaluation.
4. Choose and document one recovery/reset rule for below-rated intervals.
5. Compare datasheet melting I²t against `integral(I^2 dt)`, not an
   excess-current integral, unless a separately named model explicitly requires
   the latter.
6. Fix the one-step SOA excursion clock lag and fail closed when SOA evidence is
   absent.
7. Include assertion bounds in assertion identity and remove duplicate corner
   rows.
8. Add analytic pulse cases and convergence tests across timestep refinement.

### Tests

- `go test ./internal/simmodel -count=1`
- Analytic constant-current fuse pulses above, at, and below rating.
- Split-step versus unsplit-step equivalence.
- Rejected-predictor rollback and accumulated-history tests.
- Existing amplifier protection and electrothermal promotion fixtures.

### Acceptance

- Fuse trip time converges toward the analytic I²t result as timestep shrinks.
- Predictor fallback cannot change state from a rejected path.
- Protection assertions never pass from absent physical evidence.

### Commit

`Correct protection simulation state`

## 14. Phase 9 — Durability Tooling and Repository Health

### Objective

Add static and concurrency checks appropriate to the repaired trust boundary
without making network or GitHub execution part of acceptance.

### Work

1. Add tests for the consolidated atomic-write package before enabling it
   everywhere.
2. Enable `staticcheck` and `errcheck` incrementally, with documented narrow
   exclusions only where intentional behavior is proven.
3. Add a local short-suite race target for concurrent IPC, locking, provider,
   and transaction packages.
4. Consolidate repository-local Go caches and document one optional cache
   location.
5. Remove ignored stale test binaries and empty temporary directories only
   after confirming they are generated artifacts.
6. Update stale indirect dependencies in a separate commit and run the full
   local compatibility matrix.
7. Add direct tests for the promotion CLI and its failure exit behavior.

### Acceptance

- Static analysis adds signal beyond `go vet`.
- The targeted local race suite passes.
- Atomic-write and promotion tools have direct regression coverage.
- Repository cleanup does not change tracked generated evidence.

### Commit strategy

Separate tooling, dependency, and cleanup commits.

## 15. Local Validation Ladder

Run the smallest applicable rung after each edit and every lower rung before
advancing.

### Rung A — focused

- Exact regression test for the finding.
- Owning package tests with `-count=1`.
- `gofmt` and `git diff --check`.

### Rung B — subsystem

- All packages named in the phase.
- Related writer, round-trip, promotion, or simulation suites.
- Repeated deterministic tests where ordering or hashes changed.

### Rung C — repository

```sh
make lint
make GO_TEST_FLAGS=-count=1 test
make coverage-check
make review-matrix
go test -count=1 ./internal/writercorrectness ./internal/kicadfiles/roundtrip
git diff --check
```

### Rung D — installed KiCad

When generated output, routing, writer behavior, or acceptance evidence changes:

```sh
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
GOCACHE=/tmp/kicadai-go-cache \
go test -v ./internal/designworkflow \
  -run 'TestDesignExamplesOptionalKiCadBackedTier/(class_a_bjt_line_preamplifier|class_ab_headphone_driver|class_ab_headphone_protected|class_ab_speaker_10w_protected|esp32_wroom_32e_minimal_pass|usb_c_led_indicator_protected|usb_c_i2c_sensor_3v3_protected)$' \
  -count=1 -timeout 300s
```

Required per applicable fixture:

- clean ERC;
- strict DRC with an actual readable report;
- internal and KiCad connectivity;
- route completion;
- writer correctness;
- zero normalized round-trip differences;
- evidence and manifest hashes bound to the checked artifact revision.

### Rung E — staged review

1. Stage only the completed phase.
2. Run `prism review staged`.
3. Resolve material findings.
4. Re-run affected local tests after review changes.
5. Commit with the phase message.

GitHub Actions is not a required rung and must not be initiated or polled by
this remediation workflow.

## 16. Rollback and Compatibility Policy

- Every phase is independently revertible.
- Schema changes require an explicit migration and backward-compatibility test.
- Existing accepted-but-ignored policy fields should first fail closed, then be
  implemented; they must not remain silently accepted.
- Imported projects blocked by preservation guards remain byte-for-byte
  unchanged.
- A failed repair or apply must recover to the all-old state unless the journal
  proves the all-new state was fully committed and validated.
- Promotion evidence invalidated by a stricter check remains historical; never
  rewrite old evidence as though it had passed the new gate.

## 17. Final Definition of Done

The remediation milestone is complete only when:

1. `C1` and all `H1`–`H17` ledger entries are closed by tests and commits.
2. The three circuit defects have catalog-aware topology and behavioral proof.
3. Required acceptance levels cannot be reached without the named evidence.
4. Repair status is derived from post-repair validation.
5. Imported mutation is lossless for supported constructs and refuses all
   unsupported destructive rewrites before mutation.
6. Multi-file mutation survives fault injection and stale-lock concurrency
   tests without unrecoverable mixed state.
7. Routing search and validation enforce the same edge and via rules.
8. Component matching and lowering are spelling-independent and deterministic.
9. Fuse/protection simulation passes analytic and timestep-convergence tests.
10. The complete local validation ladder passes.
11. Applicable installed-KiCad fixtures pass clean ERC, strict DRC,
    connectivity, route completion, writer correctness, and zero normalized
    round-trip differences.
12. Prism has reviewed every staged phase and material findings are resolved.
13. The finding ledger and project documentation record final disposition,
    evidence, commit, and any explicitly deferred Medium/Low item.
