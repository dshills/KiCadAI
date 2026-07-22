# External Review Mitigation Implementation Plan

Date: 2026-07-22
Status: Proposed
Specification: [`SPEC.md`](SPEC.md)

## 1. Objective

Implement the external-review mitigation specification in independently
reviewable phases. Fix the two confirmed P1 blockers first, then close the CLI,
discoverability, artifact, and promotion gaps without adding fixture-specific
production behavior or weakening any existing correctness gate.

## 2. Implementation Rules

- Start from the current committed repository state.
- Preserve user-owned and unrelated changes.
- Add a failing regression before each behavioral fix.
- Prefer existing normalized group, mobility, resolver, report, artifact, and
  transactional writer abstractions.
- Do not add fixture IDs, paths, coordinates, net names, library allowlists,
  topology checks, or block-family dispatch to production code.
- Do not add a public schema field unless existing semantics are proven
  insufficient and the specification is amended first.
- Keep referenced library defects fail-closed.
- Keep default tests offline and independent of sibling repositories.
- Keep all ordering, diagnostic sampling, candidate search, serialization, and
  hashes deterministic.
- Run focused tests after every edit-sized fix.
- Run the optional affected KiCad-backed fixtures after every phase that
  changes generated output.
- Preserve the existing USB-C I2C protected, LED, power-tree, interface,
  writer, and round-trip pass evidence.
- Review staged changes with Prism before each phase commit and resolve material
  findings. The required command is `prism review staged`; record the Prism
  version in final audit evidence so the review is reproducible.
- Commit each completed phase separately with a focused message.
- Push completed phase commits and verify GitHub Actions before advancing to a
  dependent phase.

## 3. Phase 0: Freeze Reproductions And Baseline Evidence

### Goal

Turn every confirmed finding into a stable, local reproduction and distinguish
the unconfirmed portion of F4 from the confirmed output-volume problem.

### Work

- Record the implementation baseline commit and configured KiCad version.
- Add or identify test fixtures for:
  - checked-in AP2112K regulator intent creation;
  - compact 40 mm by 25 mm LED composition;
  - simple RC circuit graph against stock KiCad roots;
  - function-level circuit creation;
  - JSON-mode success and failure;
  - circuit-lane artifact inventory.
- Copy only identity-neutral request data needed for durable tests into the
  repository. Sanitize absolute paths, user/author names, machine identifiers,
  proprietary project labels, and unrelated metadata; replace names and net
  labels only where doing so does not change the reproduced semantics. Do not
  depend on the sibling `test-kicadai` directory.
- Confirm the external report and sibling copy are identical, but treat the
  checked-in report as the repository evidence source.
- Add failing tests that demonstrate:
  - member rejection occurs before group legalization;
  - unrelated resolver diagnostics block the RC design;
  - `circuit --help` exits nonzero;
  - circuit creation omits common evidence artifacts.
- Include current successful rigid-group repair cases in the frozen baseline so
  the atomic path must preserve their legal outcomes and relative geometry.
- Add a stdout/stderr separation test. If non-JSON stdout cannot be reproduced,
  record F4 parsing as already mitigated and retain a regression test.
- Stress the separation test with repeated and concurrent diagnostic/log
  production, cancellation, and external-check callbacks so intermittent
  stdout contamination is covered.
- Measure representative JSON payload size and diagnostic counts for the stock
  library failure.

### Likely files

- `cmd/kicadai/main_test.go`
- `cmd/kicadai/*_test.go`
- `internal/placement/*_test.go`
- `internal/libraryresolver/*_test.go`
- focused fixtures under existing `testdata` or `examples` conventions

### Tests

- Run each new regression individually and confirm it fails for the intended
  root cause.
- Run `go test ./... -count=1` to record unrelated baseline failures, if any.

### Acceptance

- Every confirmed report finding has a local deterministic test.
- F4 is split into parse hygiene and output volume rather than treated as one
  ambiguous defect.
- No test requires a private path, network call, or installed KiCad unless
  explicitly environment-gated.

### Commit message

```text
Add external review regression coverage
```

## 4. Phase 1: Place Rigid Groups Atomically

### Goal

Move `translate_as_unit` legalization ahead of individual member rejection and
establish one deterministic atomic placement path.

### Work

- Inspect normalization and group validation for conditional members and
  contradictory ownership.
- Add an internal placement work-item model that distinguishes:
  - board-absolute hard components;
  - atomic translatable groups;
  - ordinary components.
- Derive the distinction from existing `TranslateAsUnit`, group membership,
  fixed state, and mobility data.
- Compute rigid-group local bounds from final footprint geometry.
- Include group-owned keepouts in the same transform.
- Place board-absolute hard objects first.
- Place each rigid group as one candidate using a shared translation.
- Commit member placements and translated owned keepouts only after the whole
  candidate passes.
- Place ordinary components afterward.
- Remove or narrow the current post-placement repair path so there is only one
  authoritative group legalization behavior.
- Preserve deterministic group order and exhaustive finite-board search.
- Keep authored rotation and side unchanged unless the existing group contract
  explicitly permits other transforms.
- Attribute scoring and rejection evidence to both the group and affected
  members without duplicating root errors.

### Likely files

- `internal/placement/placer.go`
- `internal/placement/rigid_groups.go`
- `internal/placement/geometry.go`
- `internal/placement/occupancy.go`
- `internal/placement/model.go` only for internal result/work-item types
- corresponding placement tests

### Tests

- A group whose authored coordinates start outside the board translates inside
  while preserving relative geometry.
- A group overlapping a hard keepout finds another legal transform.
- A group-owned keepout moves with its anchor.
- A fixed member inside a translatable group is group-relative.
- An ungrouped fixed member remains board-absolute.
- A group with conditional members removed remains valid.
- A conditionally omitted declared anchor deterministically reanchors to the
  first surviving normalized member and records evidence.
- An omitted semantic proximity anchor is not silently retargeted.
- Group legalization never changes board side; a future explicit side-changing
  transform must mirror all owned geometry atomically.
- A truly oversized group fails with board-fit evidence.
- A fully occupied board fails only after deterministic candidate exhaustion.
- Shuffled input produces identical normalized placements and diagnostics.
- Existing rigid-group, mobility, geometry, and scoring tests pass.

### Acceptance

- The original early-rejection test passes through atomic legalization.
- No fixture-specific branch exists in production code.
- No partial group placement is committed.
- Repeated placement runs produce identical output.

### KiCad-backed checks

- Run affected LED and USB-C I2C promotion fixtures.
- Require clean ERC, strict DRC, connectivity, route completion, writer
  correctness, and zero normalized round-trip differences.

### Commit message

```text
Place translatable groups atomically
```

## 5. Phase 2: Close Intent Composition And Readiness Gaps

### Goal

Prove the atomic placement behavior through real intent composition and prevent
known deterministic placement failure from being presented as unconditional
readiness.

### Work

- Replace the current regulator evidence test that expects creation failure
  with successful end-to-end assertions.
- Promote `examples/intent/regulator_ap2112k_sensor.json` through intent create.
- Promote the compact LED composition without changing its board dimensions as
  the mitigation.
- Ensure conditional component omission occurs before groups, owned keepouts,
  proximity rules, and net placement dependencies are finalized.
- Review intent plan/preflight status construction.
- Run deterministic geometry feasibility before emitting overall ready status
  when the normalized design request is available.
- If the CLI exposes semantic planning readiness separately, document and test
  the distinction; do not silently reinterpret a stable field.
- Collapse derivative missing-anchor messages under the placement root cause.
- Persist placement candidate and rejection summaries in workflow evidence.

### Likely files

- `cmd/kicadai/main.go`
- `cmd/kicadai/main_test.go`
- intent planning/preflight adapters
- block composition normalization code
- placement diagnostic adapters
- checked-in intent examples only when an existing example is incomplete

### Tests

- AP2112K plan and create success.
- Compact LED create success at 40 mm by 25 mm.
- Infeasible board returns a root group-fit issue, not only missing proximity
  anchors.
- Semantic-only planning status remains distinguishable where external
  evidence is unavailable.
- Existing intent planner and regulator-family tests pass.

### Acceptance

- A checked-in intent reported as creation-ready does not fail a deterministic
  placement check in the same environment.
- Both reproduced composition failures pass without fixture-only coordinates or
  enlarged boards.

### KiCad-backed checks

- Run optional AP2112K and compact LED create workflows.
- Require clean ERC, strict DRC, connectivity, route completion, writer
  correctness, and zero normalized round-trip differences.
- Re-run the USB-C I2C protected fixture.

### Commit message

```text
Promote composed intent placement
```

## 6. Phase 3: Scope Library Diagnostics To The Design Closure

### Goal

Allow stock-library users to create designs despite unrelated installed-library
lint while preserving strict validation of referenced objects.

### Work

- Extend resolver diagnostics with stable source and object identity if needed.
- Separate index construction from the decision to block a design.
- Retain partially useful index entries when unrelated files produce
  object-level diagnostics and safe continuation is possible. Treat file-level
  syntax, decode, truncation, and structural-integrity failures as invalidating
  every entry sourced from that file; never resolve a partially parsed object.
- Resolve direct symbols, footprints, units, pins, pads, inherited bases, and
  selected variants into a deterministic design closure.
- Filter or classify resolver issues against that closure.
- Promote closure diagnostics and closure-unknown failures to blockers.
- Report unrelated inventory findings as bounded nonblocking summary evidence,
  or omit them from create output when the explicit audit command is the
  authoritative location.
- Keep `component validate` as the comprehensive catalog/library audit path.
- Include roots, KiCad version, cache schema, and closure identity in relevant
  cache keys.
- Ensure local generated libraries and configured nickname mappings participate
  in closure resolution.

### Likely files

- `internal/libraryresolver/*`
- `cmd/kicadai/circuit_preflight.go`
- component validation adapters
- resolver and CLI tests
- `docs/libraries-and-components.md`

### Tests

- Stock KiCad 10 roots plus a simple RC graph succeed despite unrelated lint.
- The same unrelated lint remains visible in explicit full validation.
- A referenced malformed symbol fails.
- A referenced malformed footprint fails.
- A missing inherited base needed by a referenced symbol fails.
- An unrelated malformed inherited symbol does not block.
- Missing and ambiguous nickname resolution remain blocking.
- Closure and diagnostics are stable under filesystem enumeration changes.
- Cache reuse cannot cross incompatible roots or closure identities.

### Acceptance

- The documented RC quickstart works with unmodified stock KiCad 10 roots.
- No referenced library correctness gate is weakened.
- Create output no longer contains the entire unrelated installed-library
  failure set.

### KiCad-backed checks

- Run the RC quickstart using the installed stock KiCad roots.
- Run a referenced-error negative fixture against the same roots.
- Re-run existing library-resolved promotion fixtures.

### Commit message

```text
Scope library blockers to design closure
```

## 7. Phase 4: Bound Diagnostics And Complete The CLI Help Contract

### Goal

Make command discovery and JSON transport reliable for agents and shell users.

### Work

- Route `--help` before circuit subcommand validation.
- Add successful help for `circuit`, `circuit preflight`, and `circuit create`.
- Reuse the command's real flag definitions rather than duplicating usage text.
- Capture stdout and stderr independently in CLI integration tests.
- Refactor any direct `os.Stdout` use below the CLI dispatcher to injected
  report/log writers, and capture subprocess output rather than inheriting the
  JSON stream.
- Enforce one JSON document on stdout for success and failure.
- Group repeated issues by stable stage, code, and source identity.
- Include total, emitted, and omitted counts with deterministic samples.
- Persist complete diagnostics in an attempt/output artifact when available.
- Add a regression bound for the representative stock-library failure.
- Ensure human mode remains readable and includes the next actionable command.
- Confirm cancellation and timeout paths obey the same output contract.

### Likely files

- `cmd/kicadai/main.go`
- `cmd/kicadai/circuit_preflight.go`
- CLI report writers
- `internal/reports/*`
- CLI tests
- `docs/cli-reference.md`

### Tests

- Each circuit help command exits zero.
- JSON success stdout parses as exactly one document.
- JSON failure stdout parses as exactly one document.
- Stderr may contain logs without changing stdout validity.
- Repeated issue groups retain exact counts and deterministic samples.
- Full persisted diagnostics contain entries omitted from stdout.
- Representative output stays under the recorded regression bound.
- Human output remains actionable.

### Acceptance

- An agent can discover the circuit workflow without triggering an error.
- JSON consumers never need to strip log lines from stdout.
- Bounded output does not conceal the existence or count of findings.

### Commit message

```text
Stabilize circuit CLI diagnostics
```

## 8. Phase 5: Publish Function-Level Capabilities And Example

### Goal

Make the successful function-level lane usable without grepping internal
testdata.

### Work

- Identify the authoritative function registry and its parameter metadata.
- Expose a deterministic capability listing through an existing capability
  command or a focused circuit subcommand.
- Generate documentation tables from the same metadata when practical.
- Add one minimal, useful, identity-neutral function-level example under
  `examples/circuit-graph/`.
- Document preflight, create, evidence inspection, and optional KiCad checks.
- Include function name, required parameters, endpoint roles, unit conventions,
  readiness limits, and unsupported claims.
- Improve structural validation so independent request mistakes are reported in
  one pass where safe.

### Likely files

- `internal/circuitgraph/*`
- function registry/corpus packages
- `cmd/kicadai/*`
- `examples/circuit-graph/*`
- `docs/cli-reference.md`
- `docs/ai-generation.md`
- `docs/kicadai-agent-skill.md`

### Tests

- Capability output matches the registry and is deterministically ordered.
- Every published function name validates.
- Published parameter requirements match validation behavior.
- The public example preflights and creates offline.
- Documentation commands run unchanged.
- Independent structural mistakes are aggregated without misleading cascades.

### Acceptance

- A new agent can choose and invoke a function-level operation using only
  public CLI/docs/examples.
- No hand-maintained capability list can drift from validation silently.

### KiCad-backed checks

- Run the public function-level example through installed-KiCad ERC/DRC,
  connectivity, route completion, writer correctness, and round-trip checks.

### Commit message

```text
Publish function-level circuit workflow
```

## 9. Phase 6: Unify Core Evidence Artifacts

### Goal

Give provider, intent, and circuit creation lanes one shared automation
contract for core result evidence.

### Work

- Inventory current artifact writers and lane-specific result adapters.
- Define one internal core evidence bundle model.
- Reuse existing artifact schemas where they already represent the required
  data; do not create circuit-specific duplicates.
- Route provider, intent, and circuit lanes through shared writers for:
  - normalized design request;
  - transaction;
  - workflow result;
  - validation summary;
  - design promotion;
  - manifest.
- Represent inapplicable gates explicitly with status and rationale.
- Include artifact kind, path, hash, schema version, and generation stage in the
  manifest.
- Require a documented `schema_version` in every shared core artifact and test
  version changes when an incompatible representation is introduced.
- Keep lane-specific artifacts additive.
- Use the existing transactional writer so failed artifact creation cannot
  replace a complete project.
- Define deterministic normalization for paths and optional external evidence.

### Likely files

- `cmd/kicadai/ai_design.go`
- `cmd/kicadai/ai_graph_design.go`
- `cmd/kicadai/circuit_preflight.go`
- artifact helpers in `cmd/kicadai/main.go` or a focused internal package
- manifest/report models
- CLI and golden tests

### Tests

- Provider, intent, and circuit lanes emit the shared core artifact set.
- Shared files decode through the same Go types.
- Inapplicable gates are explicit.
- Manifest hashes match file contents.
- Repeated generation is byte-stable after normalization.
- Failed artifact write preserves a previous complete output.
- Existing consumers and promotion tests continue to pass.

### Acceptance

- Agent workflows can inspect the same core paths regardless of entry lane.
- Circuit creation no longer stops at transaction and manifest evidence.
- Artifact parity does not overstate checks that did not run.

### KiCad-backed checks

- Run provider, intent, and circuit representatives.
- Compare promotion and validation semantics, not merely file presence.
- Re-run overwrite preservation and writer correctness coverage.

### Commit message

```text
Unify creation evidence artifacts
```

## 10. Phase 7: Promote The Independent Review Ladder

### Goal

Prove the combined mitigations from a clean checkout and make regressions
release-blocking.

### Work

- Create a matrix manifest for the six review scenarios or identity-neutral
  equivalents.
- Record each fixture's required lane, board dimensions, expected status,
  artifacts, internal gates, and optional KiCad gates.
- Use stock KiCad roots for the quickstart case.
- Run each fixture twice and compare normalized project and evidence outputs.
- Add explicit negative cases for:
  - infeasible rigid-group fit;
  - referenced malformed library object;
  - invalid function parameter;
  - artifact write failure.
- Add or update CI jobs for the offline matrix.
- Keep installed-KiCad checks optional locally and required in the configured
  promotion environment.
- Review public documentation against the exact passing commands.
- Record a completion audit with commit, tool versions, commands, artifact
  hashes, and remaining limitations.

### Likely files

- a focused regression corpus under existing testdata conventions
- matrix/golden tests
- optional KiCad-backed test scripts
- CI workflow files only if current jobs cannot express the matrix
- `specs/external-review-mitigation/AUDIT.md`
- relevant project-status and roadmap documentation

### Default tests

- focused regression packages;
- `go test ./... -count=1`;
- repository lint target;
- offline review matrix;
- deterministic repeated-run comparison;
- JSON parsing and artifact contract tests.

### Installed-KiCad tests

For every applicable promoted fixture require:

- clean ERC;
- strict DRC with zero real findings;
- connectivity success;
- required-route completion;
- writer correctness;
- zero normalized round-trip differences.

At minimum run:

- AP2112K regulator composition;
- compact LED composition;
- stock-library RC quickstart;
- public function-level example;
- existing USB-C I2C protected fixture;
- affected power-tree and interface-conditioning fixtures.

### Acceptance

- All specification completion criteria are evidenced in `AUDIT.md`.
- No fixture-specific mitigation exists in production code.
- Prism has no unresolved high-severity findings.
- CI passes on the pushed final phase commit.
- The final worktree is clean.

### Commit message

```text
Promote external review regression ladder
```

## 11. Phase Dependency Order

The required order is:

1. Phase 0 freezes evidence.
2. Phase 1 fixes the shared placement root cause.
3. Phase 2 proves intent composition and readiness behavior.
4. Phase 3 fixes stock-install library scope independently of placement.
5. Phase 4 stabilizes the CLI transport and help contract.
6. Phase 5 publishes the function-level workflow.
7. Phase 6 unifies evidence artifacts.
8. Phase 7 promotes the complete ladder.

Phase 3 may be developed in parallel with Phases 1 and 2 if separate worktrees
and commits are used, but it must merge before output-volume assertions in
Phase 4 are finalized. Phase 6 should follow CLI report stabilization so common
artifacts do not encode a transient result shape.

## 12. Review Checklist For Every Phase

Before staging:

- inspect the diff for fixture-specific logic and unrelated changes;
- run focused tests;
- run affected optional KiCad-backed fixtures when generated output changed;
- compare deterministic artifacts across two runs;
- verify no unexpected round-trip differences.

After staging:

- run `prism review staged`;
- address all material findings;
- rerun affected tests after review changes;
- inspect the staged diff and file list;
- commit only the completed phase.

After committing:

- push the current branch;
- verify GitHub Actions;
- record the exact commit and evidence before starting a dependent phase.

## 13. Stop Conditions

Pause the phase and amend the specification rather than improvising if:

- atomic placement requires changing the public request schema;
- a stock-library defect cannot be associated with an object or source closure;
- bounding diagnostics would remove the only durable copy of an error;
- artifact parity would falsely claim a gate ran;
- a generated design has a real ERC, DRC, connectivity, route-completion,
  writer, or round-trip failure unrelated to the intended change;
- the only apparent mitigation is a fixture coordinate, allowlist, board-size,
  topology, or block-family exception.

## 14. Final Completion Report

The final audit must report:

- each finding F1 through F6 and its disposition;
- root cause and generic correction;
- focused and full test commands;
- installed KiCad version and library roots used;
- ERC and strict DRC results;
- connectivity and route-completion results;
- writer-correctness and round-trip results;
- deterministic artifact comparisons;
- Prism findings and resolutions;
- phase commit hashes;
- GitHub Actions result;
- known remaining limitations and the recommended next goal.
