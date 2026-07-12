# Generic Catalog-Resolved Circuit Graph IR Implementation Plan

Date: 2026-07-12
Status: Complete; Phases 1-10 implemented and reviewed

## Completion Evidence

- Phase commits: `d45b7dfd`, `5935a5fe`, `cf6c9fe9`, `7f2b7afd`,
  `053714e7`, `7dc30cea`, `320f2454`, `cbbee8fe`, `98a24b81`,
  `9b29f6d3`, and `610a808c`.
- `go test ./...` passes on the KiCad-equipped development host.
- `make lint` passes with zero findings.
- The optional KiCad-backed provider table passes for bounded BMP280, bounded
  protected LED, generic RC, and the expected candidate generic protected LED.
- A live OpenAI generic RC CLI run strict-decoded, catalog-resolved, routed,
  wrote KiCad files, and produced promotion `pass` under strict ERC, DRC,
  route-completion, writer, and round-trip options.
- The opt-in live semantic-equivalence test is checked in. Completion-audit
  retries after the successful CLI run were blocked by TCP dial timeouts to the
  provider endpoint; default and recorded tests remain deterministic and green.
- Prism reviews reported no unresolved high findings. Findings based on
  pointer nilability or missing enums were rejected against the Go signatures
  and staged enum constraints; `anyOf` remains required by OpenAI Structured
  Outputs for the disjoint parameter-value union.

## Delivery Rules

- Implement phases in order.
- Keep existing BMP280 and protected USB-C LED provider lanes passing.
- Do not add circuit-specific block families for proof fixtures.
- Default tests remain offline and credential-free.
- Run focused tests after each phase and broad tests when shared workflow code
  changes.
- Review staged changes with Prism before every commit.
- Commit each completed phase separately when practical.
- Revert experiments that regress connectivity, ERC/DRC, writer correctness,
  round-trip, or existing pass fixtures.

## Phase 1: Specification, Threat Model, And Architecture

### Objective

Define the unresolved/resolved graph contract, safety boundary, lowering model,
provider integration, proof fixtures, and completion gates.

### Files

- `specs/circuit-graph-ir/SPEC.md`
- `specs/circuit-graph-ir/PLAN.md`

### Work

1. Document graph shape and resource limits.
2. Separate provider claims from trusted resolver evidence.
3. Define component, endpoint, layout, PCB, and policy semantics.
4. Define schematic IR and explicit design workflow lowering.
5. Define diagnostics, artifacts, threat model, tests, and limitations.
6. Identify representative compatibility and non-block fixtures.

### Tests

- `git diff --check`
- Prism specification review

### Acceptance

- The spec does not promise arbitrary electrical correctness from graph syntax.
- The plan has explicit files, tests, acceptance criteria, and rollback risks
  for every phase.
- Existing architecture compatibility and cycle avoidance are addressed.

### Rollback risk

Low. Documentation only.

### Suggested commit

`Specify generic catalog-resolved circuit graph IR`

## Phase 2: Go Model, Strict Parser, Schema, And Validation

### Objective

Create the versioned unresolved graph package and deterministic structural
boundary.

### Likely files

- `internal/circuitgraph/model.go`
- `internal/circuitgraph/parse.go`
- `internal/circuitgraph/normalize.go`
- `internal/circuitgraph/validate.go`
- `internal/circuitgraph/schema.go`
- `internal/circuitgraph/*_test.go`
- `examples/circuit-graph/*.json`

### Work

1. Implement all v1 model types with strict JSON tags.
2. Add bounded strict decoding with trailing-value rejection.
3. Add deterministic defaults and normalization without mutating callers.
4. Validate IDs, component selection union, values, finite dimensions, counts,
   endpoints, nets, buses, layout references, PCB constraints, and policy.
5. Expose a fresh strict provider JSON Schema and schema-name constant.
6. Add LED, BMP280, RC, and multi-stage unresolved examples.

### Tests

- strict unknown field and trailing JSON tests;
- document and collection size limits;
- deterministic normalization and permutation tests;
- invalid identity/query/value/net/layout/PCB/policy tables;
- schema strictness and Go-field subset tests;
- `go test ./internal/circuitgraph -count=1`.

### Acceptance

- Valid examples strict-decode and normalize identically across repeated runs.
- Invalid input yields stable path-specific diagnostics.
- The schema cannot contain paths, commands, URLs, or writer geometry.
- No component, pin, or footprint is treated as resolved yet.

### Rollback risk

Low to medium. New isolated package and fixtures.

### Suggested commit

`Add strict generic circuit graph model`

## Phase 3: Catalog, Library, Pin, And Pad Resolution

### Objective

Convert unresolved provider claims into trusted component and endpoint evidence.

### Likely files

- `internal/circuitgraph/resolved.go`
- `internal/circuitgraph/resolve.go`
- `internal/circuitgraph/resolve_test.go`
- focused helpers in `internal/components` only when existing public APIs are
  insufficient
- resolver/pinmap test fixtures

### Work

1. Define immutable resolved graph/result and evidence types.
2. Resolve explicit IDs with `components.ResolveBinding`.
3. Resolve constrained queries with `components.Select` and reject ambiguity.
4. Verify manufacturer, MPN, symbol, footprint, ratings, and functions.
5. Build canonical logical-function to symbol-pin and footprint-pad maps.
6. Reconcile catalog bindings, package pad functions, pinmap evidence, and
   library resolver records.
7. Resolve every endpoint and enforce required/unused-pin policy.
8. Record catalog, source, library, and graph hashes.

### Tests

- explicit and query selection success;
- unknown, ambiguous, unsafe, and rating-deficient selection;
- symbol and footprint mismatch;
- valid aliases and invalid/fabricated pins;
- missing pads and pinmap conflicts;
- required pin/no-connect completeness;
- deterministic provenance;
- `go test ./internal/circuitgraph ./internal/components ./internal/pinmap -count=1`.

### Acceptance

- Writers cannot consume an unresolved graph through exported APIs.
- Every resolved endpoint has symbol-pin and footprint-pad evidence.
- Provider constraints can narrow but never override trusted evidence.
- Resolution is deterministic and fail-closed.

### Rollback risk

Medium. Shared component APIs may need small additions; preserve existing
selection behavior and tests.

### Suggested commit

`Resolve circuit graph components and endpoints`

## Phase 4: Graph-To-Schematic IR Adapter

### Objective

Generate readable, electrically validated schematic IR from resolved graphs.

### Likely files

- `internal/circuitgraph/schematic.go`
- `internal/circuitgraph/schematic_test.go`
- `internal/schematicir` only for general gaps proven by adapter tests
- schematic golden fixtures

### Work

1. Map metadata, components, resolved pins, nets, no-connects, buses, and policy.
2. Map groups, lanes, relative placements, and readability rules.
3. Infer missing layout from roles and topology using existing generic layout
   behavior.
4. Carry component/provenance properties into schematic symbols.
5. Validate, normalize, translate, write, and read back generated schematics.

### Tests

- golden graph-to-IR projections for all four examples;
- no fixture-name/path dependence;
- deterministic repeated and renamed-project output;
- strict schematic IR validation;
- transaction determinism;
- write/read electrical and readability checks;
- `go test ./internal/circuitgraph ./internal/schematicir -count=1`.

### Acceptance

- RC and multi-stage graphs produce schematics without dedicated blocks.
- Every net/no-connect maps to resolved symbol pins.
- Layout follows left-to-right, power-above, ground-below, spacing, and local
  support rules.
- Invalid geometry or missing anchors fail rather than guess.

### Rollback risk

Medium. Schematic IR is shared; keep changes generic and preserve its corpus.

### Suggested commit

`Lower resolved circuit graphs to schematic IR`

## Phase 5: Explicit-Component PCB Workflow

### Objective

Add a non-block explicit-component path to the design workflow and PCB writer.

### Likely files

- `internal/designworkflow/request.go`
- `internal/designworkflow/explicit_circuit.go`
- `internal/designworkflow/*_test.go`
- `internal/circuitgraph/design_request.go`
- `internal/kicadfiles/designapi` only for proven generic gaps

### Work

1. Extend workflow requests with a resolved explicit circuit payload or neutral
   explicit component/net specs without creating an import cycle.
2. Make block and explicit modes mutually exclusive in v1.
3. Realize resolved symbols, footprints, pads, net assignments, and board
   outline through existing builder APIs.
4. Preserve component identity and resolution provenance.
5. Feed explicit components into existing placement, routing, writer, and
   validation stages.
6. Preserve all existing block-mode behavior.

### Tests

- strict request validation for mode ownership;
- explicit footprint/pad/net realization;
- schematic-to-PCB connectivity projection;
- project write and parse;
- writer correctness;
- existing design workflow suite;
- `go test ./internal/designworkflow ./internal/kicadfiles/designapi -count=1`.

### Acceptance

- Explicit graph components reach PCB pads and nets without synthetic blocks.
- Existing block requests serialize and execute unchanged.
- Required endpoint and pad mappings remain fail-closed.

### Rollback risk

High. This touches shared orchestration. Keep the path additive and guarded by
explicit request mode.

### Suggested commit

`Add explicit circuit graph design workflow`

## Phase 6: Deterministic Placement And Routing Constraints

### Objective

Translate graph PCB intent into deterministic placements, net classes, routes,
and route-completion evidence.

### Likely files

- `internal/circuitgraph/pcb.go`
- `internal/designworkflow/explicit_placement.go`
- `internal/designworkflow/explicit_routing.go`
- focused placement/routing adapters and tests

### Work

1. Map regions, relative placement, proximity, edge-facing, and keepouts.
2. Apply catalog placement/routing hints only when evidence matches the selected
   component.
3. Map per-net current, width, clearance, priority, and layer policy.
4. Route required nets with existing engines and canonical endpoint ordering.
5. Validate board bounds, collisions, pad contact, graph completeness, and
   zones.
6. Add bounded retry without changing electrical intent.

### Tests

- deterministic placements across input permutations and project names;
- proximity, region, edge, keepout, and board-bound tests;
- net-class and width preservation;
- route contact and completion evidence;
- impossible route fail-closed tests;
- RC and multi-stage PCB goldens;
- focused placement/routing suites.

### Acceptance

- Required graph nets are graph-complete or block promotion with precise
  endpoint evidence.
- Provider input cannot directly inject copper coordinates.
- Existing fixture routes remain unchanged.

### Rollback risk

High. Routing is sensitive; use additive adapters and fixture-specific evidence
without fixture-specific production branches.

### Suggested commit

`Place and route explicit circuit graphs`

## Phase 7: Recorded Generic Provider Lane

### Objective

Connect the explicit generic provider schema to parse, resolve, lower, and
workflow execution with deterministic offline evidence.

### Likely files

- `cmd/kicadai/ai_design.go`
- CLI option/parser files and tests
- `internal/aiprovider` profile/schema integration
- `examples/ai/generic_*/`
- provider evidence artifacts

### Work

1. Add `--ai-profile generic-circuit-v1` validation.
2. Select the generic schema explicitly before provider calls.
3. Decode provider envelopes into circuit graphs, not intent-planner requests.
4. Run preflight parse, resolve, schematic, and design-request lowering before
   output creation.
5. Persist sanitized graph/resolution/action artifacts.
6. Add recorded responses and semantic critical projections.
7. Preserve automatic BMP280/LED profile dispatch when the flag is absent.

### Tests

- unknown profile and conflicting-option failures before provider/write;
- malformed, ambiguous, and unresolved recorded responses;
- bounded correction attempts;
- credential-free provider-to-workflow end-to-end tests;
- repeated-run artifact determinism;
- prompt/path/project-name independence;
- existing provider tests.

### Acceptance

- Recorded generic commands reach planner/workflow status appropriate to their
  evidence.
- Unsupported graphs fail before project writes.
- Existing profile behavior and evidence formats remain compatible.

### Rollback risk

Medium to high. The CLI provider lane is shared; isolate generic branching
behind the explicit profile.

### Suggested commit

`Add recorded generic circuit provider lane`

## Phase 8: KiCad-Backed Generic Promotion Fixtures

### Objective

Promote representative explicit graphs through real KiCad evidence.

### Likely files

- `examples/circuit-graph/`
- `examples/ai/generic_*/metadata.json`
- optional promotion harnesses
- blocker fixes limited to generic adapters

### Work

1. Run LED and BMP280 generic reproductions first.
2. Compare critical projections with promoted block fixtures.
3. Promote RC and one multi-stage catalog-only graph.
4. Resolve gates in order: parse, resolve, schematic, connectivity, placement,
   routing, writer, round-trip, ERC, DRC.
5. Keep KiCad execution optional and examples-workspace based.

### Tests

- optional environment-gated KiCad promotion table;
- strict ERC/DRC severity;
- required-net route completion;
- writer correctness;
- zero unexpected schematic and PCB round-trip diffs;
- existing BMP280 and LED promotions.

### Acceptance

- At least RC plus one compatibility fixture reaches KiCad-backed pass.
- Remaining fixtures have explicit candidate/blocker evidence only if a real
  catalog or engine limitation prevents pass; completion otherwise requires all
  declared promoted fixtures to pass.
- No DRC/ERC suppression or acceptance weakening.

### Rollback risk

Medium. Primarily fixtures and adapter corrections; KiCad CLI behavior is
environment-sensitive.

### Suggested commits

- blocker-specific commits;
- `Promote generic circuit graph fixtures`.

## Phase 9: Optional Live Provider Equivalence

### Objective

Prove live OpenAI can emit the generic graph contract and match recorded
critical design projections.

### Likely files

- `internal/aiprovider/openai_live_test.go`
- generic provider prompt/record fixtures
- CLI live evidence tests or documented commands

### Work

1. Add opt-in live tests for RC and one promoted compatibility graph.
2. Compare canonical typed projections excluding prose and provider metadata.
3. Run full live CLI generation with KiCad and strict round-trip.
4. Harden only schema fields proven variable and safety-critical.
5. Re-run both existing live profile tests.

### Tests

- `KICADAI_OPENAI_LIVE_TEST=1` live projection tests;
- full live CLI command with existing credential;
- recorded/live semantic equivalence;
- existing live BMP280 and LED tests.

### Acceptance

- Live graph strict-decodes, resolves, and lowers without hidden fixture logic.
- Live critical projection matches recorded semantics.
- At least one live generic command reaches KiCad-backed pass.
- Default tests remain credential-free.

### Rollback risk

Medium. Live model variability must not weaken strict schemas.

### Suggested commit

`Harden live generic circuit provider contract`

## Phase 10: Documentation, Migration, And Completion Audit

### Objective

Document the generic lane honestly and close the milestone with direct evidence.

### Likely files

- `README.md`
- `docs/ai-generation.md`
- `docs/intent-planning.md`
- `docs/kicadai-agent-skill.md`
- `docs/cli-reference.md`
- `docs/project-status.md`
- `specs/ROADMAP.md`
- this plan status

### Work

1. Document graph schema, CLI, artifacts, diagnostics, and examples.
2. Explain bounded profiles versus explicit generic mode.
3. Explain migration of blocks into optional graph macros.
4. State structural, ERC/DRC, and fabrication boundaries.
5. Run all focused, full, lint, optional KiCad, and live tests.
6. Run Prism and verify a clean worktree.

### Tests

- `go test ./...`;
- `make lint`;
- all generic optional KiCad promotions;
- existing BMP280 and protected LED optional promotions;
- opt-in live tests with existing credentials;
- `git diff --check` and clean status.

### Acceptance

- Documentation has exact commands and evidence paths.
- Every specification completion criterion has direct test or artifact evidence.
- Prism has no unresolved high findings.
- Worktree is clean after final commit.

### Rollback risk

Low for documentation; any audit-discovered code fix inherits its subsystem
risk and receives a separate reviewed commit.

### Suggested commit

`Complete generic circuit graph audit`
