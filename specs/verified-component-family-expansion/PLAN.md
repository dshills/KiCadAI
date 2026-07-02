# Verified Component Family Expansion Implementation Plan

## Goal

Implement the next Priority 1 roadmap slice: expand verified component
families so AI-generated designs can choose concrete, catalog-backed parts for
common passives, connectors, protection parts, and amplifier-relevant seed
parts.

Each phase should be reviewed with Prism and committed before moving to the
next phase.

## Phase 1 — Catalog Audit And Targets

### Scope

Create a deterministic audit of current checked-in catalog coverage and define
the first expansion target matrix.

### Implementation Steps

1. Add a coverage audit document under this spec directory listing current
   records by family, concrete count, generic fallback count, equivalence
   groups, missing MPNs, and known gaps.
2. Add or update a small test helper that asserts the required expansion target
   families are present in the checked-in catalog coverage.
3. Define target value/package sets in tests or package-local data so future
   catalog edits are measured against explicit expectations.
4. Keep the first target matrix intentionally bounded:
   - resistor 0603/0805 common values;
   - capacitor 0603/0805 common values and voltage bands;
   - 2.54 mm headers;
   - LED/diode/protection second-source families;
   - BJT/op-amp amplifier seed records.

### Acceptance Criteria

- The audit exists and matches current catalog behavior.
- Tests fail when a target family disappears.
- The audit does not require live supplier data.

### Validation

- `go test ./internal/components`
- `go test ./cmd/kicadai`

### Commit

`Document component expansion targets`

## Phase 2 — Passive Family Expansion

### Scope

Add concrete resistor and capacitor alternatives for the first target value and
package matrix.

### Implementation Steps

1. Add concrete resistor records with manufacturer, MPN, value, package,
   rating, footprint, and equivalence metadata.
2. Add concrete ceramic capacitor records with value, voltage, package,
   dielectric/derating notes, footprint, and equivalence metadata.
3. Add conservative polarized capacitor records only where pin mapping,
   footprint, polarity, and voltage evidence are clear.
4. Add representative source snapshot records for new concrete passives when
   local evidence is available.
5. Update golden coverage output.
6. Add selection tests for:
   - preferred concrete resistor selection;
   - alternate resistor evidence;
   - capacitor voltage-rating rejection;
   - capacitor package/value selection;
   - generic fallback rejection at acceptance levels that require concrete
     evidence.

### Acceptance Criteria

- `component validate` passes for checked-in catalog.
- Representative resistor and capacitor queries return concrete records.
- Underrated passives are rejected with stable issue codes.
- Coverage reports no missing/duplicate preferred equivalence groups for the
  new passive groups.

### Validation

- `go test ./internal/components`
- `go test ./cmd/kicadai`

### Commit

`Expand verified passive component families`

## Phase 3 — Connector And Protection Expansion

### Scope

Add verified connector, LED, diode, Schottky, and protection alternatives that
are useful for generated breakouts and small boards.

### Implementation Steps

1. Add verified 2.54 mm header records for 1x02 through 1x06 packages.
2. Add connector pin-function metadata for power, ground, signal, shield, and
   mechanical pins where appropriate.
3. Add LED alternatives across common sizes/colors with polarity and footprint
   evidence.
4. Add small-signal diode and Schottky alternatives with polarity evidence.
5. Add TVS/ESD protection records only where symbol, footprint, and pin mapping
   are verified.
6. Add source snapshots where local evidence exists.
7. Add selection tests for connector pin-count/package, LED color/package,
   diode polarity evidence, and protection-part required functions.

### Acceptance Criteria

- Connector and protection records select deterministically.
- Pinout-sensitive records have verified confidence and pinmap evidence.
- Coverage reports expanded concrete/equivalence counts.

### Validation

- `go test ./internal/components`
- `go test ./internal/blocks`
- `go test ./cmd/kicadai`

### Commit

`Expand verified connector and protection families`

## Phase 4 — Amplifier-Relevant Seed Parts

### Scope

Add a narrow set of concrete amplifier-relevant parts without claiming full
audio-design correctness.

### Implementation Steps

1. Add small-signal BJT NPN/PNP records with symbol, footprint, pin functions,
   package, polarity/type, ratings, and pinmap evidence.
2. Add or strengthen op-amp concrete records with supply-range, package,
   output-current/review metadata, and required function pins.
3. Keep power-output transistor and high-power amplifier parts blocked unless
   pinout, package, thermal, and SOA evidence are modeled.
4. Add selection tests for:
   - NPN and PNP package/function selection;
   - op-amp supply-range acceptance/rejection;
   - blocked high-power placeholder behavior where applicable.
5. Update amplifier example or workflow tests only if they can consume the new
   concrete records without weakening validation.

### Acceptance Criteria

- Class A/Class AB example paths can select concrete low-risk signal parts
  where supported.
- Unsupported power parts fail closed with explicit review/blocking evidence.
- No analog-performance claims are introduced.

### Validation

- `go test ./internal/components`
- `go test ./internal/amplifiers`
- `go test ./cmd/kicadai`

### Commit

`Add amplifier seed component coverage`

## Phase 5 — Coverage Gates And Workflow Propagation

### Scope

Make expanded family coverage visible and enforceable in CLI/workflow evidence.

### Implementation Steps

1. Add target-threshold coverage checks for the expanded first-slice families.
2. Update golden `component coverage` fixtures.
3. Ensure `design create` selected-component evidence includes the new concrete
   part identity fields without schema regressions.
4. Add at least one workflow test that proves expanded concrete parts propagate
   into generated schematic symbol properties and BOM/fabrication identity
   extraction.
5. Update docs to describe the expanded coverage and the local-snapshot
   limitation.

### Acceptance Criteria

- Coverage output clearly shows expanded concrete records and equivalence
  groups.
- Workflow evidence and generated schematic properties carry selected
  component identity.
- Documentation does not imply live procurement data.

### Validation

- `go test ./internal/components ./internal/designworkflow ./internal/fabrication`
- `go test ./cmd/kicadai`
- `go test ./...`

### Commit

`Gate expanded component coverage`

## Phase 6 — Final Review And Roadmap Update

### Scope

Close the spec slice, update project status, and ensure the next roadmap item
is clear.

### Implementation Steps

1. Update `specs/ROADMAP.md` Priority 1 foundation and remaining work.
2. Update README or docs component section with the current coverage summary.
3. Run full tests.
4. Run Prism on staged changes.
5. Commit final documentation updates.

### Acceptance Criteria

- Roadmap accurately reflects completed component-family expansion.
- README/docs mention the expanded families and limitations.
- Full test suite passes.
- Prism has no unresolved high/medium findings.

### Validation

- `go test ./...`
- `prism review staged`

### Commit

`Update component expansion roadmap`

