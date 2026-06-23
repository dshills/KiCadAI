# Verified Coverage Expansion Implementation Plan

## Objective

Implement the next roadmap item: expand verified component and block coverage
with new evidence-backed families that can be used by structured AI design
requests.

## Implementation Rules

- Commit each phase independently after Prism review.
- Keep tests hermetic by default.
- Do not introduce live distributor or datasheet network calls.
- Do not promote uncertain records to fabrication readiness.
- Add small verified records before broad placeholder catalogs.
- Prefer existing `internal/components`, `internal/blocks`,
  `internal/pinmap`, and workflow evidence patterns.
- Run focused tests after each phase and `go test ./...` before the final
  documentation commit.

## Phase 1: Coverage Baseline And Gap Fixtures

### Goal

Create a checked baseline for the new target families before adding records.

### Work

- Add or extend coverage tests that identify the target expansion families:
  - crystal/oscillator;
  - reset/programming;
  - ESD protection;
  - reverse-polarity/input protection.
- Add explicit unsupported-gap expectations where current block inventory lacks
  these families.
- Add a small helper or test fixture if needed to make family coverage
  assertions readable.
- Keep current behavior unchanged except for clearer gap reporting.

### Tests

- Component coverage reports current gaps deterministically.
- Block inventory reports current unsupported target families explicitly.

### Acceptance

- The repository has executable tests documenting the current missing coverage.

### Commit

```text
Add verified coverage expansion gap fixtures
```

## Phase 2: Crystal/Oscillator Component Records

### Goal

Add verified component records and pinmap evidence for the first clock-source
family.

### Work

- Add one concrete crystal or resonator component record.
- Add required passive companion policy for load capacitors.
- Add symbol/footprint/pinmap evidence.
- Add ratings/metadata for frequency, load capacitance, package, and lifecycle
  where known.
- Add selection and validation behavior for frequency and package constraints.

### Tests

- Catalog validation accepts the new record.
- Component coverage includes oscillator family coverage.
- Selection succeeds for a supported frequency/package request.
- Selection blocks unsupported or under-evidenced oscillator requests.
- Pinmap/resolver evidence is checked.

### Acceptance

- KiCadAI can select a concrete oscillator record with enough evidence to feed
  a block.

### Commit

```text
Add verified oscillator component coverage
```

## Phase 3: Crystal/Oscillator Block

### Goal

Add a reusable oscillator block that can connect to MCU clock pins.

### Work

- Add `crystal_oscillator` block metadata.
- Define parameters for frequency, load capacitance, and package.
- Define ports for `XTAL1`, `XTAL2`, and `GND`.
- Emit schematic symbols/nets for crystal/resonator and load capacitors.
- Add PCB placement hints near the target MCU.
- Add local-route expectations for the crystal loop.
- Add known-gap notes for startup simulation and high-quality oscillator
  layout.

### Tests

- Block inventory includes `crystal_oscillator`.
- Block realization emits expected components, ports, nets, constraints, and
  route metadata.
- Block verification corpus covers the new block.
- Unsupported frequency/package combinations produce structured issues.

### Acceptance

- Structured requests can instantiate an oscillator block with deterministic
  schematic and PCB fragment evidence.

### Commit

```text
Add crystal oscillator circuit block
```

## Phase 4: Reset And Programming Support

### Goal

Add verified reset/programming component records and a block variant usable by
MCU designs.

### Work

- Add or verify records for reset switch, pull-up resistor policy, and one
  programming/debug connector.
- Add connector pin-number and orientation evidence.
- Add `reset_programming` block metadata and realization.
- Support at least one concrete variant, such as AVR ISP or UART programming.
- Mark SWD or other unsupported variants as explicit gaps if not implemented.
- Add placement hint for accessible connector edge placement.

### Tests

- Catalog validation and coverage include reset/programming components.
- Component selection rejects unsupported programming variants.
- Block inventory and realization cover supported reset/programming variant.
- Workflow evidence exposes selected programming block and unsupported gaps.

### Acceptance

- An MCU design can request reset/programming support and receive deterministic
  components, nets, ports, and placement constraints.

### Commit

```text
Add reset programming coverage
```

## Phase 5: ESD Protection Support

### Goal

Add connector-facing ESD protection component and block coverage.

### Work

- Add one concrete ESD/TVS diode or diode-array record with package and pinmap
  evidence.
- Add voltage/channel metadata.
- Add `esd_protection` block metadata and realization.
- Define protected/unprotected signal ports and GND.
- Add proximity and local-route constraints from connector to protection to
  protected net.
- Document unsupported high-speed impedance and compliance claims.

### Tests

- Catalog validation and coverage include ESD protection.
- Selection checks working voltage and channel count when requested.
- Block realization emits expected ports, components, constraints, and route
  metadata.
- Unsupported high-speed/compliance requests produce structured gaps.

### Acceptance

- Connector-facing blocks can request ESD protection and receive deterministic
  local protection fragments.

### Commit

```text
Add ESD protection block coverage
```

## Phase 6: Reverse-Polarity/Input Protection Support

### Goal

Add simple power-input protection coverage.

### Work

- Add a concrete Schottky diode or MOSFET-based protection record only if
  package/pinmap/rating evidence is available.
- Add `reverse_polarity_protection` block metadata and realization.
- Define ports for `VIN_RAW`, `VIN_PROTECTED`, and `GND`.
- Add topology metadata and route-width/current policy.
- Add placement hint near the power input connector.
- Add rating checks against requested input voltage/current where existing
  selection model supports them.

### Tests

- Catalog validation and coverage include input protection.
- Selection blocks over-current or over-voltage requests.
- Block realization emits deterministic schematic/PCB fragment evidence.
- Unsupported ideal-diode-controller requests remain blocked unless a verified
  record is added.

### Acceptance

- Power-input designs can add a simple verified protection block without
  claiming unsupported ideal-diode or high-reliability behavior.

### Commit

```text
Add input protection block coverage
```

## Phase 7: Design Workflow Integration

### Goal

Expose the new blocks and records through existing structured design workflows.

### Work

- Ensure `design create` can accept the new block IDs in structured requests.
- Ensure selected component evidence appears in workflow output.
- Ensure unsupported variants produce stage issues rather than panics or silent
  omissions.
- Add one representative generated design fixture that combines at least two
  new blocks with an existing MCU or connector block.

### Tests

- Design workflow test for one supported combined design.
- Design workflow test for one unsupported variant.
- CLI or golden output updates where needed.

### Acceptance

- AI callers can discover and use the new verified coverage through the same
  workflow surface as existing blocks.

### Commit

```text
Integrate expanded coverage into design workflow
```

## Phase 8: Documentation And Roadmap Update

### Goal

Document the new verified coverage and update the roadmap status.

### Work

- Update `README.md` with the new blocks and caveats.
- Update `specs/ROADMAP.md` to reflect implemented coverage.
- Add example request documentation if a representative fixture is added.
- Run `go test ./...`.

### Tests

- Full `go test ./...`.
- Prism review staged documentation and code.

### Acceptance

- Documentation accurately describes supported families, gaps, and verification
  boundaries.

### Commit

```text
Document verified coverage expansion
```
