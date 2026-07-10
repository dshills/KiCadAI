# Schematic Design/Layout IR Implementation Plan

Date: 2026-07-09

## Objective

Implement v1 of a strict AI-facing schematic design/layout IR that separates
circuit intent, schematic layout intent, and validation/repair policy, then
maps supported IR documents into existing KiCadAI generation paths.

Scope remains schematic IR only. PCB routing, PCB placement, fabrication, DRC,
and new block families are out of scope unless existing integration requires a
small metadata handoff.

## Phase 1: Spec, Examples, And Go Data Model

### Work

- Add `specs/schematic-design-ir/SPEC.md`.
- Add `specs/schematic-design-ir/PLAN.md`.
- Add a new package, likely `internal/schematicir`.
- Define Go structs for:
  - `Document`;
  - `Metadata`;
  - `Circuit`;
  - `Component`;
  - `Pin`;
  - `Net`;
  - `Port`;
  - `Layout`;
  - `Group`;
  - `Lanes`;
  - `LayoutRules`;
  - `Placement`;
  - `Policy`;
  - `ValidationPolicy`;
  - `RepairPolicy`.
- Add JSON tags and documented constants for schema ID, version, and supported
  enums.

### Likely Files

- `internal/schematicir/model.go`
- `internal/schematicir/model_test.go`
- `specs/schematic-design-ir/SPEC.md`
- `specs/schematic-design-ir/PLAN.md`

### Tests

- Model enum/default tests.
- Compile-only tests for JSON tags and zero-value expectations.

### Acceptance Criteria

- Spec and plan are checked in.
- Data model compiles.
- No behavior changes outside the new package.

### Rollback Risk

Low. New docs and new package only.

## Phase 2: Strict Parser And Validator

### Work

- Implement `ParseJSON` or `DecodeJSON` with unknown-field rejection.
- Use `json.Decoder.DisallowUnknownFields()` rather than plain
  `json.Unmarshal`, because the default Go decoder silently ignores unknown
  fields.
- Allow only the documented top-level and scoped `extensions` maps as
  forward-compatible escape hatches; all other unknown fields remain errors.
- Implement `Validate(Document) Result`.
- Add issue model compatible with existing `reports.Issue`.
- Define a `LibraryProvider` interface or equivalent validation context so the
  validator can resolve symbol pin maps without hard-coding KiCad library
  filesystem paths.
- Validate:
  - schema ID and version agreement;
  - safe metadata name using `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`;
  - duplicate component IDs;
  - duplicate refs unless each shared-ref component has a unique non-empty
    `unit`;
  - shared-ref components with differing `symbol` or non-empty `footprint`
    values;
  - duplicate net names with conflicting roles, labels, or `use_label` values;
  - component ID regex and dot exclusion;
  - case-sensitive references for component IDs, net names, ports, pins, and
    groups;
  - supported component role vocabulary;
  - `Library:Name` syntax for symbols and footprints;
  - `no_connect` nets limited to one endpoint;
  - `pins[].no_connect` rejected if the pin appears in any net endpoint;
  - non-`no_connect` nets require at least two unique component endpoints plus
    ports referencing that net;
  - endpoint references, target pin selectors, and malformed
    `<component_id>.<pin_selector>` strings;
  - port net references;
  - electrical values against the v1 value grammar, with bare numeric values
    rejected when a unit-bearing value is required by component role;
  - port `direction` and `electrical_type` conflicts;
  - closed port `electrical_type` enum values;
  - supported acceptance values: `structural`, `erc_clean`, and `readable`;
  - layout `rank` and `side` conflicts;
  - `power_neg` nets without an explicit `lanes.power_negative` declaration;
  - contradictory `layout.origin` and `rules.center_on_page` values;
  - group members;
  - enum values;
  - policy values;
  - missing refs when `policy.repair.allow_ref_assignment` is false;
  - unsupported repair behavior.
- Normalize duplicate-name nets by merging endpoint lists before adapter
  generation.
- Normalize unnamed `no_connect` net records with deterministic internal names.
- Add default normalization that does not mutate caller input unexpectedly.

### Likely Files

- `internal/schematicir/parse.go`
- `internal/schematicir/validate.go`
- `internal/schematicir/normalize.go`
- `internal/schematicir/parse_test.go`
- `internal/schematicir/validate_test.go`

### Tests

- Valid LED example parses cleanly.
- Unknown field fails.
- Duplicate component ID fails.
- Unknown endpoint fails.
- Unknown group member fails.
- Unsupported layout enum fails.
- Unsafe repair policy fails.

### Acceptance Criteria

- Parser is strict by default.
- Validator returns structured issues without panics.
- Valid examples produce no blocking issues.

### Rollback Risk

Low. Still isolated to new package.

## Phase 3: IR To Existing Request/Transaction Adapter

### Work

- Add adapter package functions:
  - `ToDesignRequest(Document)`;
  - `ToSchematicTransaction(Document)`.
- Use design request mapping only for explicitly supported known patterns.
- Provide schematic-only transaction fallback for component/net IR.
- Convert:
  - components to `add_symbol`;
  - footprints to `assign_footprint`;
  - nets to `connect` or label-friendly operations where supported;
  - policy acceptance to existing acceptance strings.
- Preserve layout metadata for the next phase.

### Likely Files

- `internal/schematicir/adapter.go`
- `internal/schematicir/adapter_test.go`
- Possible small additions to `internal/designworkflow/request.go` only if a
  metadata field is needed.

### Tests

- LED IR maps to a schematic transaction.
- USB-C LED IR either maps to a supported design request or fails with a clear
  unsupported-block issue.
- I2C 3.3 V IR either maps to a supported design request or fails with clear
  unsupported-block issue.
- Adapter does not guess pins when policy forbids it.

### Acceptance Criteria

- At least one valid IR can be converted to an existing generation path.
- Unsupported conversions fail closed with actionable issues.
- Existing design workflow tests still pass.

### Rollback Risk

Medium. Adapter touches existing transaction/design request types. Keep changes
additive.

## Phase 4: Layout-Rule Normalization And Readable Placement Hints

### Work

- Normalize layout defaults:
  - `flow = left_to_right`;
  - `origin = centered`;
  - lanes: power top, signals middle, ground bottom;
  - spacing defaults.
- Create layout hint summaries consumable by schematic generation.
- Map group rank/order to deterministic symbol positions in the transaction
  fallback path.
- Prefer labels for long or cross-group nets when policy allows label insertion.
- Preserve enough evidence for readability reports.

### Likely Files

- `internal/schematicir/layout.go`
- `internal/schematicir/layout_test.go`
- Possible targeted integration in `internal/transactions` or
  `internal/kicadfiles/designapi` only for schematic placement metadata.

### Tests

- Groups sort by rank then ID.
- Components without groups get deterministic fallback groups.
- Power/ground lanes normalize correctly.
- Centered origin yields coordinates around page center.
- Label preference is emitted for cross-group nets.

### Acceptance Criteria

- Layout intent affects schematic transaction coordinates/hints.
- Generated schematic-only LED fixture is centered and readable enough for the
  existing readability validator to avoid new blockers.

### Rollback Risk

Medium. This begins to affect generated schematic geometry. Keep fallback path
behind the IR adapter, not global design generation.

## Phase 5: Example Fixtures And Golden Tests

### Work

- Add example IR fixtures for:
  - LED indicator;
  - USB-C powered LED indicator;
  - I2C sensor breakout with 3.3 V regulator.
- Add golden tests for parse, validate, normalize, and adapter summaries.
- Do not require KiCad-backed ERC/DRC for all IR examples in v1.

### Likely Files

- `examples/schematic-ir/led_indicator.json`
- `examples/schematic-ir/usb_c_led_indicator.json`
- `examples/schematic-ir/i2c_sensor_3v3.json`
- `internal/schematicir/examples_test.go`

### Tests

- Every example parses and validates.
- LED example maps to transaction and writes a schematic project in temp dir.
- USB-C and I2C examples report supported/unsupported mapping status
  deterministically.

### Acceptance Criteria

- All required examples are checked in.
- Golden tests protect the AI-facing contract.

### Rollback Risk

Low to medium. Fixtures are additive.

## Phase 6: CLI Entry Point Or Integration Command

### Work

- Add a CLI command, likely:

```text
kicadai schematic-ir validate --request examples/schematic-ir/led_indicator.json
kicadai schematic-ir create --request examples/schematic-ir/led_indicator.json --output ./out/led --overwrite
```

- `validate` parses, validates, normalizes, and prints JSON report.
- `create` runs adapter and writes through existing generation path.
- Keep JSON output default.
- Do not change existing `design create` behavior.

### Likely Files

- `cmd/kicadai/main.go`
- `cmd/kicadai/main_test.go`
- `docs/cli-reference.md`

### Tests

- CLI validate success.
- CLI validate unknown field failure.
- CLI create LED writes project to temp dir.
- CLI reports unsupported mapping for richer examples without panics.

### Acceptance Criteria

- Users and agents can validate IR without writing KiCad files.
- At least LED IR can produce a generated schematic project.

### Rollback Risk

Medium. CLI dispatch is shared; keep new command family isolated.

## Phase 7: Documentation And Roadmap Update

### Work

- Update README or docs to explain:
  - why AI should emit IR instead of prose or KiCad S-expressions;
  - supported v1 schema;
  - validation and repair policy;
  - CLI usage;
  - current gaps.
- Update `specs/ROADMAP.md` with schematic IR status and next steps.
- Cross-link to natural-language intent adapter and schematic readability docs.

### Likely Files

- `README.md`
- `docs/intent-planning.md`
- `docs/cli-reference.md`
- `docs/kicadai-agent-skill.md`
- `specs/ROADMAP.md`

### Tests

- Documentation-only phase normally requires no Go tests unless examples are
  copied into tests.
- Run focused CLI tests if command examples changed.

### Acceptance Criteria

- Docs describe current functionality accurately.
- Roadmap shows what remains after v1 IR.

### Rollback Risk

Low.

## Phase 8: Bounded Readability Repair And Pin-Safe Routing

### Work

- Preserve the distinction between default label policy and an explicit direct
  routing request.
- Retry one readable layout with deterministic label insertion only when the
  direct attempt reports a crossing, body conflict, or unrelated pin-anchor
  conflict and repair policy allows labels.
- Treat unrelated pin anchors, including no-connect pins, as hard route
  obstacles and expose `wire_pin_overlap` in validation and rule metadata.
- Keep repair score-based and fail closed; do not introduce a general search
  optimizer.

### Likely Files

- `internal/schematiclayout/model.go`
- `internal/schematiclayout/route.go`
- `internal/schematiclayout/validate.go`
- `internal/schematiclayout/rules_metadata.go`
- `internal/schematicir/adapter.go`
- focused layout and adapter tests

### Tests

- Explicitly disabled label fallback remains disabled for direct routing.
- A route cannot pass through an unrelated pin anchor.
- Resolver-backed multi-unit/no-connect fixture remains electrically clean.
- Full Go suite remains green.

### Acceptance Criteria

- Direct routing is not silently converted to label fallback.
- No generated wire crosses an unrelated or no-connect pin anchor.
- Readable acceptance can perform at most one deterministic label-fallback
  retry and retains it only when diagnostics improve.

### Rollback Risk

Medium. Routing and validation changes affect all schematic layout consumers;
retain the existing default behavior and keep the retry scoped to IR readable
acceptance.

## Phase 9: Repair Stress Fixtures And Completion Evidence

### Work

- Add deterministic stress fixtures for high fanout, feedback, long labels,
  mixed orientations, disconnected islands, and no-connect pin corridors.
- Exercise the complete IR -> normalize -> layout -> transaction -> KiCad
  schematic path for representative fixtures.
- Run optional KiCad parsing/ERC and round-trip checks when the KiCad CLI is
  available.
- Record supported cases, fail-closed cases, and remaining limitations in the
  specification and roadmap.

### Likely Files

- `internal/schematicir/project_write_test.go`
- `internal/schematicir/adapter_test.go`
- `internal/kicadfiles/roundtrip/schematic_ir_integration_test.go`
- `specs/schematic-design-ir/SPEC.md`
- `specs/ROADMAP.md`

### Tests

- Deterministic repeated generation produces identical layout evidence.
- Stress fixtures pass internal readability/electrical checks.
- KiCad-backed tests remain optional and pass when enabled.

### Acceptance Criteria

- The repair behavior is proven on more than one hand-authored topology.
- Unsupported cases produce explicit diagnostics rather than silently ugly
  output.
- Documentation states that arbitrary circuits are accepted structurally but
  readability is guaranteed only within the tested layout envelope.

### Rollback Risk

Low to medium. Mostly additive tests and documentation, with no new PCB scope.

## Prism And Commit Protocol

For each phase:

1. Implement the smallest phase slice.
2. Run focused tests.
3. Stage only phase-relevant files.
4. Run `prism review staged`.
5. Address high and medium findings or explicitly justify deferring low-only
   findings.
6. Commit with a focused message.
7. Report:
   - current phase completed;
   - tests run;
   - Prism status;
   - next phase;
   - whether scope stayed limited to schematic IR.

## Milestone Completion Criteria

- Spec and plan are committed.
- IR model, parser, validator, examples, adapter, layout normalization, and CLI
  are implemented or explicitly deferred by phase outcome.
- Required examples exist.
- Existing KiCad-backed fixture `usb_c_i2c_sensor_3v3_protected` remains
  passing when run with KiCad CLI.
- No PCB routing, placement, fabrication, or block-family work was added beyond
  narrow integration needs.
