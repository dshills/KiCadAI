# Component Intelligence Implementation Plan

## 1. Objective

Implement the component intelligence layer described in `SPEC.md`.

The end state is a deterministic component catalog and selection system that can
validate real component choices against KiCad symbols, footprints, pinmaps,
ratings, package variants, and requested acceptance levels. This layer should
feed circuit blocks and the AI design workflow before schematic or PCB files are
written.

## 2. Implementation Rules

- Keep phases independently reviewable and commit-sized.
- Prefer curated, explicit data over broad inference.
- Use stable issue codes for every validation and selection failure.
- Do not claim fabrication readiness from placeholder or inferred records.
- Default tests must not require external KiCad repositories.
- Resolver-backed tests must use small local fixtures unless explicitly marked
  as external integration tests.
- All CLI output must be JSON-compatible and deterministic.
- Run `gofmt` on edited Go files.
- Run focused package tests after each phase.
- Run `go test ./...` before phase commits where practical.
- Run `prism review staged` before each phase commit and address material
  findings.

## 3. Phase 1: Package Skeleton And Core Model

### Goal

Create the component intelligence package and stable data model without loading
catalog files yet.

### Work

- Add package:

```text
internal/components
```

- Define core types:
  - `Catalog`;
  - `FamilyDefinition`;
  - `ComponentRecord`;
  - `PackageVariant`;
  - `SymbolBinding`;
  - `FunctionPin`;
  - `PadFunction`;
  - `ValueConstraint`;
  - `RatingConstraint`;
  - `SelectionRule`;
  - `VerificationRecord`;
  - `ConfidenceLevel`;
  - `AcceptanceLevel`;
  - `IssueCode` constants.
- Add helpers for:
  - confidence validation;
  - acceptance-level ordering;
  - deterministic sorting;
  - issue construction;
  - JSON marshal stability.

### Tests

- Model marshals to expected JSON field names.
- Confidence levels validate.
- Acceptance-level ordering is deterministic.
- Catalog sorting is stable.
- Unknown confidence produces a blocking issue.

### Acceptance Criteria

- `internal/components` compiles.
- No CLI or workflow changes yet.
- Unit tests pass without external repositories.

### Commit Message

```text
Add component intelligence model
```

## 4. Phase 2: Catalog Loader And Validation

### Goal

Load curated component catalog JSON files and validate their internal
consistency.

### Work

- Add default catalog path support:

```text
data/components
```

- Implement `LoadCatalog(ctx, opts)`.
- Support loading all top-level `*.json` files in a catalog directory. Recursive
  catalog loading is out of scope until catalog layout conventions are defined.
- Read records deterministically by file path then ID. Duplicate component IDs
  across files are validation errors, not overrides.
- Validate:
  - duplicate component IDs;
  - unknown families;
  - invalid confidence levels;
  - records without symbols;
  - records without package variants;
  - package variants without footprints;
  - malformed or empty function pin names;
  - malformed or empty rating/value units.
- Return structured diagnostics rather than panics.

### Tests

- Empty catalog directory produces a diagnostic.
- Multiple files merge deterministically.
- Duplicate IDs block validation.
- Unknown family blocks validation.
- Missing footprint blocks validation.
- Invalid confidence blocks validation.

### Acceptance Criteria

- Catalog loading is deterministic.
- Catalog validation can run without a resolver.

### Commit Message

```text
Load and validate component catalogs
```

## 5. Phase 3: Initial Curated Catalog

### Goal

Add a small checked-in component catalog that supports current blocks and common
examples.

### Work

- Add files under:

```text
data/components/
```

- Include initial family definitions for:
  - resistor;
  - capacitor;
  - LED;
  - diode;
  - connector;
  - voltage regulator;
  - op-amp;
  - MCU;
  - sensor.
- Add initial records for:
  - generic SMD resistors such as 0603 and 0805;
  - generic ceramic capacitors;
  - polarized electrolytic capacitor;
  - generic indicator LED;
  - generic signal diode and Schottky diode;
  - 1x02 and 1x04 pin headers;
  - a fixed 3.3 V regulator placeholder or verified record if resolver/pinmap
    evidence exists;
  - op-amp gain-stage placeholder record;
  - MCU minimal-system placeholder matching existing block needs;
  - I2C sensor placeholder matching existing block needs.
- Mark every record with confidence accurately.
- Prefer `placeholder` over pretending a part is verified.

### Tests

- Checked-in catalog loads.
- Checked-in catalog validates.
- Current built-in block families have at least one matching component record.
- Placeholder records are visible in diagnostics.

### Acceptance Criteria

- `data/components` provides useful seed data.
- Catalog can support simple passive and connector selection.

### Commit Message

```text
Add initial component catalog
```

## 6. Phase 4: Selection Engine

### Goal

Implement deterministic component lookup and selection with confidence gating.

### Work

- Add:
  - `Query`;
  - `SelectionRequest`;
  - `Candidate`;
  - `Selection`;
  - `ResolvedComponent`.
- Implement:
  - `Find(ctx, catalog, query)`;
  - `Select(ctx, catalog, request)`;
  - `ResolveBinding(ctx, catalog, id, variant)`.
- Match on:
  - family;
  - package;
  - value;
  - voltage/current/power rating;
  - pin count for connectors;
  - required function pins;
  - minimum confidence;
  - acceptance level.
- Sort candidates deterministically by:
  - confidence;
  - exactness;
  - record ID;
  - variant ID.
- Block ambiguous equal-score selections unless the request allows alternatives.

### Tests

- Select generic resistor by package and value.
- Reject capacitor below requested voltage rating.
- Select connector by pin count.
- Reject placeholder for `connectivity` acceptance.
- Allow placeholder for `draft` acceptance with warning.
- Report ambiguity for equal candidate scores.

### Acceptance Criteria

- The selection engine can make safe passive choices.
- Low-confidence active choices block unless draft output is requested.

### Commit Message

```text
Select components with confidence gating
```

## 7. Phase 5: Resolver And Pinmap Validation

### Goal

Connect component records to KiCad resolver and pinmap evidence.

### Work

- Add catalog validation that accepts optional:
  - `libraryresolver.LibraryIndex`;
  - pinmap registry/checker.
- Validate:
  - symbol IDs resolve when resolver data is available;
  - footprint IDs resolve when resolver data is available;
  - verified records have verified pinmaps when required;
  - function pins map to symbol pins;
  - pad functions map to footprint pads;
  - passive rule-inferred mappings are limited to safe symmetric cases.
- Add diagnostics for:
  - `component_symbol_unresolved`;
  - `component_footprint_unresolved`;
  - `component_pinmap_missing`;
  - `component_function_pin_unmapped`;
  - `component_pad_function_unmapped`.

### Tests

- Tiny resolver fixture validates a resistor record.
- Missing symbol produces blocking issue for verified acceptance.
- Missing footprint produces blocking issue for verified acceptance.
- Missing pinmap blocks verified active part.
- Passive two-pad rule inference can pass when explicitly allowed.

### Acceptance Criteria

- Component readiness can be proven against resolver fixtures.
- Unsafe verified claims are rejected.

### Commit Message

```text
Validate components against resolver evidence
```

## 8. Phase 6: Component CLI

### Goal

Expose component intelligence to AI agents through the CLI.

### Work

- Add `component` command family:

```sh
kicadai --json component list
kicadai --json component show <component-id>
kicadai --json component find --family resistor --package 0805
kicadai --json --request request.json component select
kicadai --json component validate
```

- Reuse existing resolver flags where possible.
- Add `--catalog-dir` for custom catalogs.
- Return stable JSON:
  - catalog summary;
  - candidates;
  - selected record;
  - selected variant;
  - diagnostics;
  - confidence and acceptance-level decisions.
- Keep non-JSON mode minimal or unsupported if consistent with existing CLI
  behavior.

### Tests

- `component list` returns catalog records.
- `component show` returns one record.
- `component find` filters by family/package.
- `component select` blocks unsafe placeholder for connectivity.
- `component validate` reports catalog diagnostics.
- Unknown component exits with structured error.

### Acceptance Criteria

- AI agents can inspect and select components without reading internal files.

### Commit Message

```text
Expose component catalog CLI
```

## 9. Phase 7: Circuit Block Integration

### Goal

Let circuit blocks declare component requirements and receive selected component
bindings.

### Work

- Extend block component definitions with optional:
  - `component_id`;
  - `component_query`;
  - `component_variant`;
  - `minimum_confidence`;
  - `acceptance`.
- Keep existing symbol/footprint fields for compatibility.
- During block instantiation:
  - load or receive a component catalog;
  - select required components;
  - attach resolved symbol, footprint, and pinmap evidence;
  - surface warnings for draft/placeholder selections;
  - block connectivity-level generation when selections are unsafe.
- Update built-in blocks gradually:
  - LED indicator;
  - connector breakout;
  - voltage regulator;
  - I2C sensor;
  - op-amp gain stage;
  - MCU minimal system.

### Tests

- LED block selects resistor and LED components.
- Connector breakout selects connector by pin count.
- Placeholder active block emits warning at draft acceptance.
- Placeholder active block blocks at connectivity acceptance.
- Existing block tests remain compatible.

### Acceptance Criteria

- Blocks can be backed by component records without breaking current callers.

### Commit Message

```text
Use component selections in circuit blocks
```

## 10. Phase 8: Design Workflow Integration

### Goal

Run component selection and validation as a first-class design workflow stage
before writing files.

### Work

- Add design workflow stage:

```text
component_selection
```

- Extend design request schema to allow:
  - catalog directory;
  - minimum confidence;
  - component overrides;
  - package preferences;
  - acceptance-level component policy.
- Validate selected components before:
  - schematic transaction generation;
  - footprint assignment;
  - PCB realization;
  - writer correctness.
- Include component diagnostics in workflow result feedback.
- Ensure low-confidence component choices stop the workflow before file writes
  unless draft output is requested.

### Tests

- Design create includes `component_selection` stage.
- Component selection failure blocks before project write.
- Draft acceptance can proceed with placeholder warning.
- Connectivity acceptance rejects placeholder active components.
- Successful workflow carries selected component metadata to later stages.

### Acceptance Criteria

- AI design workflow no longer silently uses ad hoc component choices when a
  component policy is requested.

### Commit Message

```text
Run component selection in design workflow
```

## 11. Phase 9: Documentation And Examples

### Goal

Document component intelligence and add examples useful to humans and AI agents.

### Work

- Update `README.md` with:
  - component CLI overview;
  - confidence levels;
  - catalog location;
  - limitations.
- Add component catalog guide:

```text
docs/component-intelligence.md
```

- Add request examples:

```text
examples/components/
  select_resistor.json
  select_connector.json
  reject_capacitor_voltage.json
  draft_placeholder_active.json
```

- Update `specs/ROADMAP_GAP.md` if this work changes gap status.

### Tests

- Run full unit suite.
- Run representative CLI examples where practical.

### Acceptance Criteria

- Users can understand how to inspect, add, validate, and select components.
- Current limitations are explicit.

### Commit Message

```text
Document component intelligence workflow
```

## 12. Phase 10: Hardening And Golden Corpus

### Goal

Add regression coverage that prevents component intelligence from becoming a
source of unsafe autonomous choices.

### Work

- Add golden catalog validation fixtures:
  - valid passive catalog;
  - duplicate IDs;
  - missing symbol;
  - missing footprint;
  - missing pinmap;
  - unsafe placeholder selection;
  - ambiguous selection.
- Add deterministic JSON golden outputs for:
  - list;
  - find;
  - select success;
  - select blocked.
- Add component readiness checks to relevant CI/test paths.

### Tests

- Golden output tests.
- Full `go test ./...`.
- Optional external resolver integration test if local roots are available and
  explicitly enabled.

### Acceptance Criteria

- Component selection regressions are caught by deterministic tests.
- Unsafe choices remain blocked by default.

### Commit Message

```text
Add component intelligence golden tests
```
