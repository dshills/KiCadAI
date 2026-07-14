# Generic Multi-Unit Components Implementation Plan

## Phase 1: Contract And Fail-First Tests

Files likely touched:

- `internal/components/model.go`
- `internal/components/catalog_test.go`
- `internal/circuitgraph/model.go`
- `internal/circuitgraph/schema.go`
- `internal/circuitgraph/parse_validate_test.go`
- `internal/circuitgraph/capability_test.go`

Work:

- Add catalog unit identity/role/required metadata.
- Add graph component unit declarations and unit-qualified schematic placement.
- Add structured unit qualifiers for schematic `near`, `above`, and `right_of`
  targets.
- Extend the strict provider schema and deterministic normalization.
- Add fail-first tests for named units, duplicate declarations, missing required
  units, invalid placement identity, and deterministic capability output.

Tests:

- `go test ./internal/components ./internal/circuitgraph -count=1`

Acceptance:

- The data contract is strict, additive, deterministic, and covered by failing
  semantic tests before resolver behavior is added.
- Legacy single-unit documents still decode and normalize unchanged.

Rollback risk: medium. The provider schema changes, but only through additive
fields and capability data.

## Phase 2: Catalog Validation And LM358 Evidence

Files likely touched:

- `internal/components/catalog.go`
- `internal/components/catalog_test.go`
- `data/components/verified_active.json`
- component catalog golden/hash fixtures as required

Work:

- Validate named unit uniqueness, injective positive numeric mapping, unit roles,
  required-unit metadata, and reject any record that mixes named and anonymous
  symbol bindings.
- Add exactly one verified TI LM358 SOIC-8 record with A/B/P unit bindings,
  package pad functions, ratings, source evidence, and review-required analog
  evidence.
- Verify the record against checked-out KiCad symbol/footprint libraries and the
  TI data sheet.

Tests:

- `go test ./internal/components -count=1`
- catalog load/validation commands used by the repository

Acceptance:

- The catalog loads without diagnostics.
- LM358 A/B/P functions map to pads 1-8 exactly as verified.
- Omitting or corrupting required power-unit metadata fails catalog validation.

Rollback risk: low to medium. One trusted record is added and catalog validation
becomes stricter only for records opting into named units.

## Phase 3: Named-Unit Resolution

Files likely touched:

- `internal/circuitgraph/resolved.go`
- `internal/circuitgraph/resolve.go`
- `internal/circuitgraph/resolve_test.go`
- `internal/circuitgraph/parse.go`
- `internal/circuitgraph/validate.go`

Work:

- Resolve graph unit IDs through catalog metadata.
- Retain selected unit identity in resolved functions/components and hashes.
- Require declared mandatory units and reject undeclared/nonexistent units.
- Enforce unit-qualified endpoint selection and physical pad ownership across
  units.
- Preserve anonymous legacy unit behavior.

Tests:

- valid A/B/P package;
- nonexistent and duplicate unit IDs;
- missing power unit;
- omitted ambiguous endpoint unit;
- inconsistent shared-pad nets;
- deterministic normalization and resolution hashes.

Acceptance:

- One graph component resolves to one physical package with three selected
  symbol units.
- All required failure cases produce stable, fail-closed diagnostics.

Rollback risk: medium to high. Resolution and hash semantics are shared by every
generic provider fixture.

## Phase 4: Schematic Lowering And Package Identity

Files likely touched:

- `internal/circuitgraph/schematic.go`
- `internal/circuitgraph/schematic_test.go`
- `internal/circuitgraph/design_request.go`
- `internal/circuitgraph/design_request_test.go`
- `internal/schematicir/adapter.go`
- `internal/schematicir/adapter_test.go`
- `internal/transactions/apply_test.go`

Work:

- Lower declared units using stable unit-derived IDs and shared references.
- Apply unit-qualified schematic placement and role hints.
- Emit one footprint assignment per physical reference and reject conflicts.
- Prove one explicit PCB component, deduplicated physical pads, one footprint,
  and one BOM identity.
- Verify resolver-backed KiCad write/read behavior for units A/B/P.

Tests:

- deterministic schematic IR and transaction output;
- exact unit/pin subsets and shared reference;
- duplicate footprint-assignment rejection;
- one PCB component and one package identity;
- resolver-backed LM358 write/read test.

Acceptance:

- KiCad receives three schematic units and one physical LM358 package.
- Repeated lowering emits byte-equivalent normalized transactions.

Rollback risk: high. This phase changes shared schematic lowering and transaction
emission, so existing multi-unit and provider fixtures require regression runs.

## Phase 5: Recorded Provider Fixture

Files likely touched:

- `examples/ai/generic_lm358_buffered_signal_conditioner/prompt.txt`
- `examples/ai/generic_lm358_buffered_signal_conditioner/recorded-response.json`
- `examples/ai/generic_lm358_buffered_signal_conditioner/metadata.json`
- `internal/aiprovider/generic_lm358_provider_test.go`
- shared projection helpers only if the change remains generic

Work:

- Add the natural-language prompt, strict recorded graph, promotion metadata,
  and generic provider tests.
- Assert one package, A/B/P units, one footprint/BOM identity, required analog
  review evidence, topology-derived stage ordering, and no fixture dispatch.
- Add strict failure tests for ambiguous or invalid provider unit output.

Tests:

- focused `internal/aiprovider` recorded tests;
- provider schema/capability tests;
- offline provider-to-planner workflow.

Acceptance:

- Recorded output strict-decodes, resolves only through trusted catalog evidence,
  and produces the expected critical graph projection.
- Default tests remain deterministic and credential-free.

Rollback risk: low. The fixture is additive; shared test helpers may carry medium
risk if generalized.

## Phase 6: Deterministic Placement, Routing, And Recorded Promotion

Files likely touched:

- shared circuit-graph layout or design-workflow files only when current evidence
  identifies a generic blocker
- focused tests next to each changed implementation
- fixture metadata when all real gates pass

Work:

- Run the recorded fixture through the full workflow.
- Fix blockers one at a time in gate order.
- Prefer generic unit-aware topology, placement, endpoint-access, and route-tree
  corrections.
- Preserve one PCB package while routing shared supply, ground, raw/buffered
  VREF, feedback, input, and output nets.
- Run KiCad ERC/strict DRC, writer correctness, route completion, and normalized
  round-trip evidence outside the restricted sandbox where required.

Tests:

- focused tests for every blocker;
- optional fixture promotion lane;
- existing provider-backed pass fixtures.

Acceptance:

- Recorded promotion is `pass` with clean required evidence.
- No production path depends on project name, fixture path, reference, or fixed
  coordinate table.

Rollback risk: medium to high. Placement/routing changes must remain narrowly
generic and be reverted if existing pass fixtures regress.

## Phase 7: Live Provider Equivalence And Promotion

Files likely touched:

- live provider tests and sanitized recorded response only if strict semantic
  equivalence requires a provider-contract correction
- no credential files

Work:

- Run the live OpenAI provider using the existing credential gate.
- Compare live and recorded critical graph projections.
- Run the live CLI lane through the same KiCad-backed gates.
- Stabilize provider instructions only when output reveals a generic ambiguity;
  preserve bounded correction attempts and fail-closed behavior.

Tests:

- credential-gated live provider test;
- live end-to-end CLI generation and promotion;
- recorded replay after any provider-contract change.

Acceptance:

- Live output strict-decodes and is semantically equivalent to recorded output.
- Live generation reaches AI `ready` and KiCad-backed `pass`.

Rollback risk: medium. Provider variability may expose contract ambiguity; no
live artifact may weaken offline determinism.

## Phase 8: Documentation And Closeout

Files likely touched:

- `docs/ai-generation.md`
- catalog or circuit-graph documentation as appropriate
- `specs/ROADMAP.md`
- `README.md` only for a concise link or capability statement
- this plan's status notes

Work:

- Document the multi-unit model, LM358 command, evidence paths, provider gates,
  and analog review limitations.
- Record exact reproducible recorded and optional live commands.
- Run all focused tests, `go test ./...`, `make lint`, and optional KiCad-backed
  provider fixtures.
- Review every staged change with Prism before commit.

Acceptance:

- Documentation distinguishes logical schematic units from physical package
  identity.
- All existing pass fixtures remain clean.
- Prism has no unresolved high findings.
- The roadmap reflects the delivered capability and the worktree is clean.

Rollback risk: low for documentation; final regression results determine whether
the milestone can be declared complete.

## Commit And Reporting Discipline

- Commit each completed phase or independently completed blocker separately.
- Stage only files belonging to that phase.
- Run Prism on staged changes before every commit.
- After each commit report the current phase/gate, blocker fixed, tests and KiCad
  evidence run, Prism status, next blocker, whether generic multi-unit support
  advanced, whether existing fixtures remained clean, and whether the change
  remained generic.
