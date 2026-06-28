# Amplifier Example And Test Corpus Implementation Plan

Date: 2026-06-28

## Objective

Add amplifier-focused examples and tests that make KiCadAI better at reading,
evaluating, and eventually generating Class A and Class AB headphone and power
amplifier designs.

The plan starts with schematic fixtures and semantic checks, then moves into
structured intent, circuit blocks, PCB constraints, and optional KiCad-backed
promotion evidence.

## Phase 1: Audit Existing Amplifier Coverage

### Tasks

- Inspect `examples/06_class_ab_headphone_amp`.
- Inspect `examples/intent/amplifier_module.json`.
- Inspect `examples/intent_text/headphone_amplifier_unverified.txt`.
- Add an audit note documenting:
  - existing amplifier examples;
  - expected schematic landmarks;
  - current parsing/round-trip status;
  - missing test coverage;
  - unsupported generated-design gaps.
- Add a lightweight test, if missing, that proves the existing checked-in
  Class AB headphone amp fixture is discoverable and parseable by current
  readers.

### Acceptance

- Existing amplifier assets are documented.
- Existing `06_class_ab_headphone_amp` has at least smoke-level regression
  coverage.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Audit amplifier example coverage`

## Phase 2: Amplifier Schematic Semantic Test Harness

### Tasks

- Add an amplifier semantic test helper or package.
- Define fixture metadata for required landmarks:
  - input connector/net;
  - output connector/net;
  - power rails;
  - ground/reference;
  - feedback network;
  - bias network;
  - decoupling;
  - output stage components.
- Add tests for the existing `06_class_ab_headphone_amp` fixture.
- Add negative tests using minimal in-memory or testdata schematics that miss:
  - feedback;
  - output connector;
  - bias/reference;
  - decoupling.

### Acceptance

- Tests detect common amplifier schematic omissions.
- Existing Class AB headphone amp fixture passes declared semantic checks.
- Failure messages name missing amplifier landmarks clearly.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add amplifier schematic semantic checks`

## Phase 3: Additional Checked-In Amplifier Examples

### Tasks

- Add at least two new schematic fixture projects:
  - `09_class_a_headphone_amp`;
  - `10_opamp_buffer_headphone_amp`.
- Use the current writer conventions:
  - `.kicad_pro`;
  - `.kicad_sch`;
  - `.kicad_prl`;
  - `sym-lib-table`;
  - KiCad-current schematic format and symbol usage consistent with existing
    examples.
- Add README entries for each example.
- Add parse and semantic tests for the new examples.
- Avoid claiming production analog correctness or fabrication readiness.

### Acceptance

- New examples open as modern KiCad schematic projects.
- Tests verify required amplifier landmarks.
- Examples are documented in `examples/README.md`.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add amplifier schematic examples`

## Phase 4: Amplifier Intent Fixtures

### Tasks

- Add or expand structured intent examples for:
  - Class A headphone amplifier;
  - Class AB headphone amplifier;
  - low-voltage Class AB power amplifier skeleton.
- Add natural-language text fixtures for the same families.
- Update intent/planner tests to assert:
  - supported requests produce structured assumptions and known gaps;
  - unsupported topologies fail closed;
  - no fixture silently claims fabrication-ready output.
- Add rationale expectations for amplifier-specific blockers such as unverified
  output stage, thermal constraints, or missing layout proof.

### Acceptance

- Amplifier intent examples are checked in.
- Planner behavior is deterministic and explainable.
- Unsupported amplifier requests produce actionable known gaps.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add amplifier intent fixtures`

## Phase 5: Amplifier Circuit Block Inventory

### Tasks

- Add block inventory entries or explicit gaps for:
  - amplifier input stage;
  - op-amp gain stage;
  - Class A output stage;
  - Class AB output stage;
  - headphone output connector;
  - speaker output connector;
  - stability network;
  - amplifier power entry.
- Mark unsupported blocks as unsupported with tested gaps rather than hidden
  omissions.
- Add metadata for ports, required roles, electrical rules, PCB constraints,
  and local-route expectations where supported.
- Add block inventory and verification corpus tests.

### Acceptance

- `block inventory` reports amplifier block support/gaps.
- Unsupported amplifier blocks are explicit and tested.
- Supported structural blocks expose ports and rule metadata.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add amplifier block inventory`

## Phase 6: First Generated Amplifier Design Fixture

### Tasks

- Add the first generated amplifier design request using only supported blocks.
- Prefer an op-amp gain/headphone-buffer style fixture if it can reuse existing
  verified op-amp and connector foundations.
- If a fully generated schematic/PCB is not yet possible, add an expected-fail
  fixture with precise blockers.
- Ensure generated artifacts include:
  - component identity where available;
  - schematic landmarks;
  - placement/routing summaries;
  - explicit known gaps.
- Add regression tests so the fixture cannot silently regress or overclaim
  readiness.

### Acceptance

- A generated amplifier workflow fixture exists.
- It either completes to the supported acceptance level or fails closed with
  explicit amplifier-specific blockers.
- Tests cover parseability, artifact presence, and rationale/known-gap output.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add generated amplifier design fixture`

## Phase 7: PCB Constraint And Routing Evidence

### Tasks

- Add amplifier PCB constraint metadata for supported blocks:
  - input/output separation;
  - feedback proximity;
  - decoupling proximity;
  - output-device pairing;
  - high-current net width policy;
  - thermal region or edge preference.
- Add placement quality tests for amplifier-specific constraints.
- Add routing evidence tests for:
  - high-current output/supply net classification;
  - analog input net classification;
  - feedback route intent;
  - partial/contact-miss evidence when routing cannot complete.
- Avoid promoting to DRC-clean unless optional KiCad evidence supports it.

### Acceptance

- Amplifier PCB constraints are visible in placement/routing summaries.
- Tests catch missing or ignored amplifier layout intent.
- Remaining layout blockers are explicit.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add amplifier PCB constraint evidence`

## Phase 8: Optional KiCad-Backed Amplifier Fixtures

### Tasks

- Add optional KiCad-backed metadata for generated amplifier fixtures where
  useful.
- Mark each fixture as `expected_fail`, `candidate`, or `pass`.
- Ensure optional tests skip cleanly without `KICADAI_KICAD_CLI`.
- When KiCad is configured, capture:
  - writer correctness;
  - board validation;
  - ERC/DRC status;
  - artifact paths;
  - known blocker text.

### Acceptance

- Optional amplifier fixtures are documented and metadata-backed.
- KiCad-independent tests remain default.
- KiCad-backed tests provide useful evidence when configured.
- `go test ./...` passes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Add KiCad-backed amplifier fixture evidence`

## Phase 9: Documentation And Roadmap Update

### Tasks

- Update `README.md` with amplifier corpus status.
- Update `examples/README.md`.
- Add or update amplifier-focused docs if needed.
- Update `specs/ROADMAP.md` with:
  - implemented amplifier example/test coverage;
  - remaining amplifier block/component/layout gaps;
  - next recommended amplifier milestone.
- Ensure all tests pass.

### Acceptance

- Documentation reflects which amplifier examples are schematic-only,
  generated, optional KiCad-backed, expected-fail, candidate, or pass.
- ROADMAP names the next amplifier-specific blocker.
- `go test ./...` passes.
- Prism review is clean or any accepted findings are documented in the final
  phase notes.

### Review And Commit

- Review staged changes with `prism review staged`.
- Commit message: `Document amplifier example corpus`
