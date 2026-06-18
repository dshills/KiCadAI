# Verified Component And Part Intelligence Implementation Plan

## 1. Objective

Implement `specs/verified-component-part-intelligence/SPEC.md`.

The outcome is a stricter and broader component intelligence system that can
select verified parts for common generated designs, block unsafe placeholder
active parts, report catalog coverage, and feed selected-part evidence into
blocks and design workflow outputs.

## 2. Implementation Rules

- Keep each phase independently reviewable and commit-sized.
- Preserve the existing `internal/components` public concepts unless the spec
  requires an extension.
- Prefer explicit curated data over inference.
- Do not raise a record to `verified` without resolver and pinmap evidence.
- Do not let placeholder active parts pass connectivity or stronger acceptance.
- Default tests must not require external KiCad repositories.
- External KiCad library checks must be opt-in integration tests or guarded by
  environment configuration.
- Every new validation failure needs a stable issue code.
- JSON output must be deterministic.
- Run `gofmt` for Go changes.
- Run focused tests after each phase and `go test ./...` before committing when
  practical.
- Run `prism review staged` before each commit and address material findings.

## 3. Phase 1: Catalog Coverage Report

### Goal

Make current catalog gaps visible before adding more records.

### Work

- Add a coverage model in `internal/components`.
- Report coverage by:
  - family;
  - confidence;
  - verified records;
  - rule-inferred records;
  - placeholder records;
  - blocked records;
  - records missing resolver evidence;
  - records missing pinmap evidence;
  - roadmap-required family coverage.
- Add a deterministic `CoverageReport` function.
- Add CLI support for a component coverage command if a component CLI command
  already exists; otherwise add the smallest command path consistent with the
  current CLI structure.
- Include machine-readable JSON output.

### Tests

- Coverage report counts current seed catalog records.
- Placeholder active records are reported separately from verified records.
- Missing required family coverage is reported as actionable issues.
- Output is deterministic.

### Acceptance Criteria

- A developer can run a command or package function to see which families are
  verified, placeholder-only, or missing.
- Coverage output can be consumed by AI workflow code later.

### Commit Message

```text
Report component catalog coverage
```

## 4. Phase 2: Metadata Model Extensions

### Goal

Represent the metadata needed to make part choices explainable and safe.

### Work

- Extend component records or package variants with fields for:
  - manufacturer part number at package/ordering-code level;
  - lifecycle;
  - tolerance;
  - temperature range;
  - required companion components;
  - derating rules;
  - placement hints;
  - routing/net-class hints;
  - schematic property emission.
- Add validation for:
  - malformed lifecycle values;
  - malformed temperature/rating/tolerance units;
  - duplicate companion requirements;
  - invalid placement or routing hint kinds;
  - package-level MPN conflicts.
- Keep JSON field names stable and explicit.

### Tests

- Extended model marshals deterministically.
- Valid metadata records pass validation.
- Invalid lifecycle, tolerance, and companion metadata produce stable issues.
- Existing catalog files remain loadable.

### Acceptance Criteria

- The model can describe real part records without stuffing metadata into notes.
- Existing records remain backward-compatible.

### Commit Message

```text
Extend component metadata model
```

## 5. Phase 3: Verified Passive, LED, Diode, And Connector Records

### Goal

Raise common simple families to useful verified or explicitly allowed
rule-inferred coverage.

### Work

- Add or refine records for:
  - 0603 and 0805 resistors;
  - 0603 and 0805 nonpolar ceramic capacitors;
  - polarized bulk capacitor with explicit polarity policy;
  - indicator LED;
  - signal diode;
  - Schottky diode;
  - TVS diode;
  - 1x02, 1x03, 1x04, and 1x05 headers.
- Add pinmaps where needed.
- Verify symbol pins and footprint pads with checked-in fixtures or built-ins.
- Ensure connectors remain blocked above structural unless explicit connector
  pinmap policy is implemented.
- Add rating and polarity metadata.

### Tests

- Select resistor and capacitor by value, package, and rating.
- Reject polarized capacitor when polarity evidence is missing.
- Select LED with anode/cathode evidence.
- Detect diode polarity mismatch fixture.
- Select pin headers by pin count.
- Block connector connectivity readiness when evidence policy is insufficient.

### Acceptance Criteria

- Basic passives, LED indicators, diodes, and headers have honest coverage.
- Polarity mistakes are caught in tests.

### Commit Message

```text
Verify common passive and connector components
```

## 6. Phase 4: Verified Regulator And Power-Path Records

### Goal

Replace regulator placeholders with at least one concrete verified power part
path.

### Work

- Choose one concrete 3.3 V linear regulator available in KiCad symbols and
  footprints.
- Add manufacturer, MPN, lifecycle, voltage/current ratings, dropout notes,
  thermal notes, and capacitor requirements.
- Add verified symbol, footprint, and pinmap evidence.
- Add selection rules for:
  - input voltage range;
  - output voltage;
  - output current;
  - required input/output capacitors;
  - package thermal notes.
- Add or refine Schottky/TVS records needed by power-protection blocks.

### Tests

- Select regulator for a valid 5 V to 3.3 V request.
- Reject regulator when input voltage exceeds rating.
- Reject regulator when requested output current exceeds rating.
- Block fabrication candidate when required companion components are missing.
- Validate regulator symbol-footprint pinmap evidence.

### Acceptance Criteria

- At least one regulator can be selected at connectivity level with verified
  evidence.
- Power block generation can report concrete regulator evidence.

### Commit Message

```text
Add verified regulator component records
```

## 7. Phase 5: Verified Op-Amp, MCU, Sensor, Crystal, And USB-C Records

### Goal

Add enough active-part coverage for the current near-term circuit blocks.

### Work

- Add at least one verified record each for:
  - low-voltage single op-amp;
  - MCU minimal-system package;
  - I2C sensor;
  - crystal or resonator;
  - USB-C receptacle suitable for power-only use.
- Add function pins, electrical roles, ratings, package variants, and pinmaps.
- Add required companion components:
  - op-amp decoupling;
  - MCU decoupling, reset, programming, and optional oscillator requirements;
  - I2C pull-ups;
  - crystal load capacitors;
  - USB-C CC resistors and ESD/protection notes.
- Add explicit limitations for unsupported peripheral mapping or high-speed USB.

### Tests

- Select op-amp by supply range.
- Reject op-amp outside supply range.
- Select MCU minimal-system part and verify required power/reset/programming
  functions.
- Select I2C sensor and verify VCC/GND/SDA/SCL functions.
- Select crystal and verify load capacitance metadata.
- Select USB-C power receptacle while blocking unsupported USB data mode.

### Acceptance Criteria

- Current active block families have at least one concrete verified path.
- Placeholder active records are no longer the only available choice for common
  generated designs.

### Commit Message

```text
Add verified active component records
```

## 8. Phase 6: Rating And Rule-Aware Selection

### Goal

Make component selection use ratings, companion requirements, and family rules
instead of simple family/package matching.

### Work

- Extend selection requests to express:
  - voltage, current, power, temperature, tolerance, and supply requirements;
  - required functions;
  - concrete versus generic preference;
  - required companion policy;
  - fabrication candidate strictness.
- Improve scoring using:
  - exact value/package matches;
  - confidence;
  - rating margin;
  - concrete-part preference;
  - deterministic IDs.
- Return rejected candidate reasons.
- Add stable issue codes for:
  - missing rating;
  - insufficient rating;
  - missing required function;
  - missing companion component;
  - concrete part required;
  - unsupported use case.

### Tests

- Select highest-confidence exact package candidate.
- Reject active part with missing required rating.
- Reject underspecified fabrication candidate.
- Return rejected candidate diagnostics.
- Block ambiguous equal-score choices.

### Acceptance Criteria

- Selection decisions are explainable and conservative.
- Active fabrication candidate selection cannot proceed on incomplete ratings.

### Commit Message

```text
Select components with rating rules
```

## 9. Phase 7: Resolver And Pinmap Evidence Hardening

### Goal

Make evidence validation strong enough to trust selected active parts.

### Work

- Validate duplicate function pins and duplicate pad functions.
- Validate required function coverage across symbol and footprint.
- Validate symbol electrical type expectations where resolver data exists.
- Validate multi-unit symbol policy.
- Validate polarity consistency between symbol function pins and pad functions.
- Validate verified active records require explicit pinmap evidence.
- Add small local resolver fixtures where external KiCad libraries are not
  available.

### Tests

- Missing symbol pin blocks evidence.
- Missing footprint pad blocks evidence.
- Duplicate function mapping blocks evidence.
- Polarity mismatch blocks evidence.
- Multi-unit symbol without policy blocks evidence.
- Verified active record without pinmap blocks evidence.

### Acceptance Criteria

- Evidence validation catches common catalog mistakes before generation.
- Tests cover both valid and invalid records.

### Commit Message

```text
Harden component evidence validation
```

## 10. Phase 8: Block And Design Workflow Integration

### Goal

Expose selected component evidence in generated designs and block readiness.

### Work

- Update circuit blocks to request components with meaningful selection
  constraints instead of relying on placeholders where verified records exist.
- Include selected component IDs, variant IDs, confidence, symbol, footprint,
  pinmap, and rating summaries in block verification output.
- Update design workflow outputs to include selected-part evidence and rejected
  candidate diagnostics.
- Gate workflow readiness when:
  - active selected part is placeholder;
  - required pinmap is missing;
  - required rating is missing or insufficient;
  - resolver evidence fails.
- Emit schematic properties from selected component metadata where currently
  supported by the writer.

### Tests

- Built-in block verification reports selected component evidence.
- Design workflow blocks unsafe active placeholders for connectivity readiness.
- Design workflow allows draft placeholder paths with warnings.
- Schematic output includes catalog ID and MPN/value metadata for selected
  parts where available.

### Acceptance Criteria

- Generated projects carry enough part evidence for AI review.
- Workflow readiness reflects component confidence and evidence.

### Commit Message

```text
Integrate verified components into design workflow
```

## 11. Phase 9: Golden Corpus And Coverage Gates

### Goal

Prevent regression as the catalog expands.

### Work

- Add golden coverage reports for the built-in catalog.
- Add golden selection reports for:
  - valid resistor;
  - valid LED;
  - valid regulator;
  - valid op-amp;
  - valid MCU;
  - valid I2C sensor;
  - unsupported USB data request;
  - unsafe placeholder active selection;
  - missing pinmap;
  - insufficient rating.
- Add a coverage threshold test for roadmap-required families.
- Document how to add new verified component records.

### Tests

- Golden reports are deterministic.
- Coverage threshold fails when a required family loses verified coverage.
- Documentation examples load and validate.

### Acceptance Criteria

- Catalog growth is guarded by repeatable tests.
- Future contributors have clear rules for adding verified records.

### Commit Message

```text
Add component intelligence golden coverage
```

## 12. Final Verification

Before closing this project:

- Run focused component tests.
- Run block verification tests.
- Run design workflow tests.
- Run `go test ./...`.
- Run `prism review staged`.
- Confirm `specs/ROADMAP.md` can be updated to mark Priority 1 foundation as
  complete or partially complete with remaining explicit gaps.

## 13. Completion Definition

Priority 1 is complete when:

- verified or policy-allowed records exist for the required common families;
- active demo/block selections no longer depend on placeholders when concrete
  parts are required;
- resolver and pinmap evidence failures are blocking and test-covered;
- component selection explains accepted and rejected candidates;
- design workflow and block verification expose selected-part evidence;
- coverage reports make remaining catalog gaps explicit.
