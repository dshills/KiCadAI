# Component Intelligence Specification

## 1. Purpose

Build a component intelligence layer that lets KiCadAI reason about real
electronic parts, not just KiCad symbol IDs and footprint strings.

The goal is to make AI-generated projects safer and more autonomous by giving
the design workflow typed knowledge about component families, symbol-footprint
compatibility, pin functions, electrical constraints, package variants,
selection confidence, and validation expectations.

This is the next gap after Writer Correctness Closeout. The file writers,
library resolver, block library, schematic-to-PCB transfer, placement, routing,
validation, and AI design workflow foundations exist. The missing layer is the
canonical part knowledge needed to choose and validate components before the
writer emits a project.

## 2. Current Foundation

KiCadAI already has:

- `internal/libraryresolver` for local KiCad symbol and footprint discovery,
  parsed metadata, compatibility checks, and resolver-backed footprint
  hydration.
- `internal/pinmap` for symbol-footprint pinmap records and validation.
- `internal/blocks` for reusable circuit blocks and PCB realization metadata.
- `internal/designworkflow` for deterministic AI-facing design generation.
- `internal/schematicpcb` for schematic-to-PCB transfer.
- `internal/writercorrectness` for generated writer correctness gates.
- `internal/boardvalidation` and `internal/kicadfiles/checks` for board and
  optional KiCad validation.

The component intelligence layer must reuse these systems. It must not become a
parallel writer, resolver, or block framework.

## 3. Goals

The component intelligence layer must:

- define typed component families and use cases;
- represent concrete component choices and package variants;
- connect components to KiCad symbols, footprints, and pinmaps;
- expose pin functions, polarity, electrical roles, and package-specific pad
  expectations;
- encode common value/rating constraints for passives and active parts;
- distinguish verified data from inferred or placeholder data;
- provide deterministic component selection for explicit design requirements;
- block unsafe autonomous choices when confidence is insufficient;
- feed circuit blocks, design workflow, footprint assignment, validation, and
  future repair planning;
- provide machine-readable CLI output suitable for AI agents.

## 4. Non-Goals

Initial implementation does not need to:

- query distributors or live availability APIs;
- guarantee manufacturer compliance or safety certification;
- model every KiCad library part;
- perform SPICE simulation;
- solve thermal or high-speed design completely;
- replace KiCad ERC/DRC;
- infer unknown manufacturer pinouts without explicit verified data;
- choose production alternates without a human-approved data source.

## 5. Definitions

### 5.1 Component Family

A broad class of parts with shared behavior and selection rules.

Examples:

- resistor;
- capacitor;
- LED;
- diode;
- transistor;
- connector;
- voltage regulator;
- op-amp;
- MCU;
- sensor;
- crystal/oscillator;
- USB interface/protection;
- power protection.

### 5.2 Component Record

A reusable part definition. A record may be generic, such as an 0805 resistor,
or specific, such as a manufacturer regulator with an exact MPN.

### 5.3 Package Variant

A physical and pinout variant for a component record. One logical component may
have several package variants with different footprints, pad names, thermal
constraints, or symbol mappings.

### 5.4 Function Pin

A semantic pin role, independent of the KiCad symbol pin number or footprint pad
name.

Examples:

- `VIN`
- `VOUT`
- `GND`
- `EN`
- `SDA`
- `SCL`
- `LED_ANODE`
- `LED_CATHODE`
- `OPAMP_OUT`
- `MCU_RESET`

### 5.5 Confidence Level

Every mapping and selection must declare confidence:

- `verified`: backed by a curated record, explicit pinmap, and resolver evidence.
- `library_derived`: derived from KiCad library metadata but not manually
  verified.
- `rule_inferred`: inferred from conservative local rules such as passive pad
  count and symmetric pins.
- `placeholder`: usable only for draft structural output.
- `blocked`: not safe to select or generate.

Autonomous generation may only claim connectivity readiness for `verified` or
explicitly allowed `rule_inferred` passive cases.
`blocked` records are rejected at every acceptance level, including `draft`.

The initial explicit allowlist for connectivity-level `rule_inferred` records is
limited to symmetric passive families: nonpolar capacitors and resistors.
Connectors and active components require verified evidence for connectivity
acceptance.

## 6. Data Model

The implementation should add a package:

```text
internal/components
```

The concrete Go shape may evolve, but the public JSON concepts must remain
stable.

### 6.1 Catalog

```go
type Catalog struct {
    Version     string
    GeneratedAt time.Time
    Records     []ComponentRecord
    Families    []FamilyDefinition
    Diagnostics []reports.Issue
}
```

### 6.2 ComponentRecord

```go
type ComponentRecord struct {
    ID              string
    Family          string
    Name            string
    Description     string
    Generic         bool
    Manufacturer    string
    MPN             string
    Lifecycle       string
    Tags            []string
    Values          []ValueConstraint
    Ratings         []RatingConstraint
    ElectricalRoles []ElectricalRole
    Symbols         []SymbolBinding
    Packages        []PackageVariant
    PinMappings     []ComponentPinMapping
    SelectionRules  []SelectionRule
    Verification    VerificationRecord
}
```

### 6.3 PackageVariant

```go
type PackageVariant struct {
    ID            string
    Name          string
    FootprintID   string
    PackageType   string
    MPN           string
    PinMapID      string
    PadFunctions  []PadFunction
    DimensionsMM  Bounds
    Constraints   []PhysicalConstraint
    Verification  VerificationRecord
}
```

`ComponentRecord.PinMappings` is the preferred place to prove valid
symbol-footprint-pinmap triplets. `PackageVariant.PinMapID` is retained as
footprint-side evidence for simple records and import compatibility. If a
selection combines a `SymbolBinding.PinMapID`, `PackageVariant.PinMapID`, and a
matching `ComponentPinMapping`, all IDs must resolve to the same compatible
symbol-footprint pinmap. Conflicts are blocking validation failures.

`ComponentRecord.MPN` is a base/default part number only. `PackageVariant.MPN`
overrides it when the manufacturer part number changes by package, suffix, or
ordering code.

### 6.4 SymbolBinding

```go
type SymbolBinding struct {
    SymbolID      string
    Unit          int
    FunctionPins  []FunctionPin
    PinMapID      string
    Verification  VerificationRecord
}
```

`SymbolBinding.PinMapID` identifies symbol-side pinmap evidence. A record may
omit it for draft or rule-inferred passive records, but verified active records
must prove that the selected symbol pins and footprint pads share a compatible
pinmap.

`Unit` is 1-based when a KiCad symbol has multiple units. `0` means the binding
applies to the physical part as a whole and is the default for component records
that represent one package. Use a nonzero unit only for advanced catalogs that
intentionally bind a single gate/unit to a specific physical part. Negative unit
values are invalid. `Unit: 0` represents KiCad pins common to all units and
package-level bindings that are not specific to one gate/unit. When KiCad
documentation or UI labels units alphabetically, map `A` to `1`, `B` to `2`,
and so on. Multi-unit physical parts should use `Unit: 0` for shared power or
package-level pins and include one `SymbolBinding` per unit when function pins
differ by unit.
During schematic generation, Unit 0 pins must be realized exactly once per
physical component. The deterministic priority is: use the KiCad power/common
unit when the symbol defines one; otherwise attach Unit 0 pins to the first
instantiated unit for that physical component.

### 6.5 FunctionPin

```go
type FunctionPin struct {
    Function     string
    SymbolPin    string
    ElectricalType string
    Polarity     string
    Required     bool
    Aliases      []string
}
```

`SymbolPin` is the unique KiCad symbol pin number or identifier used by netlist
and footprint pin mapping, not the human-readable pin name.

### 6.6 Supporting Types

```go
type ElectricalRole struct {
    Role        string
    Description string
}

type PadFunction struct {
    Function string
    Pad      string
    Aliases  []string
}

type Bounds struct {
    SizeXMM  float64
    SizeYMM  float64
    HeightMM float64
}

type ComponentPinMapping struct {
    SymbolID    string
    PackageID   string
    PinMapID    string
    Verification VerificationRecord
}
```

Dimensions are measured in the footprint's zero-rotation orientation: `SizeXMM`
maps to the local X span, `SizeYMM` maps to the local Y span, and `HeightMM`
maps to the vertical Z clearance.

Standard `ElectricalRole.Role` strings are:

- `passive`;
- `signal`;
- `power_input`;
- `power_output`;
- `ground`;
- `decoupling`;
- `protection`;
- `connector`;
- `clock`;
- `programming`.

### 6.7 Value And Rating Constraints

Examples:

- resistor resistance range, tolerance, power rating;
- capacitor capacitance, voltage rating, dielectric, polarity;
- LED forward voltage/current/color;
- regulator input/output voltage and current;
- op-amp supply range, bandwidth, output current;
- connector pitch, pin count, orientation;
- MCU voltage, package, programming interface;
- sensor bus type, supply voltage, address.

Constraints must be explicit and unit-aware. Query values and required rating
values are engineering strings such as `10k`, `10uF`, `2pins`, or `100V`, not
JSON numbers. Catalog constraints keep value and unit fields separate for
normalization, but selection requests use strings so agents can preserve common
electronics notation. Parsing must be deterministic and use the implementation's
engineering parser: SI prefixes are case-sensitive (`M` mega, `m` milli), ASCII
`u` and Unicode `µ` both mean micro, optional spaces before units are allowed,
`k` is canonical for kilo but `K` may be normalized with a warning when
unambiguous, and malformed units produce blocking validation issues. The
canonical pin-count unit is `pins`.

For standard capability ratings such as voltage, current, and power, catalog
rating values represent the component's maximum supported capability. Selection
must require `catalog_rating >= requested_requirement`. Value constraints such
as resistance or capacitance describe allowed requested values and must contain
the requested value inside the catalog range.

### 6.8 VerificationRecord

```go
type VerificationRecord struct {
    Confidence      string
    Sources         []string
    ResolverChecked bool
    PinMapChecked   bool
    Tests           []string
    Notes           []string
}
```

## 7. Catalog Sources

Initial catalog data should be project-local and checked in.

Suggested layout:

```text
data/components/
  catalog.json
  families.json
  passives.json
  connectors.json
  power.json
  analog.json
  mcu.json
  sensors.json
```

The implementation should support loading one directory of JSON files and
merging them into a deterministic catalog. Later work may add YAML or generated
indexes, but JSON is enough for the first implementation.

Records must be small and curated at first. Coverage should favor correctness
over breadth.

## 8. Initial Required Families

The first usable catalog should cover:

- resistors: generic through-hole and SMD package variants;
- capacitors: nonpolar ceramic SMD, electrolytic polarized variants;
- LEDs: generic red/green/blue indicator LEDs;
- diodes: signal, Schottky, rectifier, TVS placeholder families;
- connectors: pin headers and basic screw terminals;
- regulators: linear fixed-output records for common 3.3 V and 5 V use cases;
- op-amps: generic single/dual op-amp records with conservative warnings;
- MCU minimal system: at least one verified MCU-family placeholder suitable for
  existing blocks;
- I2C sensors: structural placeholder records for block composition.

Every family may start with a small number of records, but each record must
state whether it is verified, inferred, or placeholder.

## 9. Selection API

The package should expose deterministic selection functions:

```go
func LoadCatalog(ctx context.Context, opts LoadOptions) (*Catalog, error)
func ValidateCatalog(ctx context.Context, catalog *Catalog, resolver *libraryresolver.LibraryIndex) reports.Result
func Find(ctx context.Context, catalog *Catalog, query Query) ([]Candidate, reports.Result)
func Select(ctx context.Context, catalog *Catalog, request SelectionRequest) (Selection, reports.Result)
func ResolveBinding(ctx context.Context, catalog *Catalog, id string, variant string) (ResolvedComponent, reports.Result)
```

Selection must return structured diagnostics when:

- no component matches;
- multiple components match with equal confidence;
- symbol is missing;
- footprint is missing;
- pinmap is missing;
- rating is insufficient;
- selected record is placeholder-only;
- confidence is lower than requested acceptance.

## 10. CLI Surface

Add a `component` command family:

```sh
kicadai --json component list
kicadai --json component show resistor.generic.0805
kicadai --json component find --family resistor --package 0805
kicadai --json --request request.json component select
kicadai --json component validate
```

All output must use structured JSON and stable issue codes.

The CLI should accept resolver flags already used elsewhere:

- `--symbols-root`
- `--footprints-root`
- `--klc-root`
- `--templates-root`
- `--library-cache`
- `--refresh-library-cache`

## 11. Integration Requirements

### 11.1 Circuit Blocks

Blocks should move from ad hoc symbol/footprint strings toward component IDs and
selection requirements.

Example:

```json
{
  "role": "current_limit_resistor",
  "component_query": {
    "family": "resistor",
    "value_kind": "resistance",
    "value": "330ohm",
    "package": "0603",
    "minimum_confidence": "rule_inferred"
  }
}
```

The block may still pin an explicit symbol and footprint, but the component
record should become the source of validation and confidence.

### 11.2 Design Workflow

`design create` should validate selected components before writing:

```text
intent/block -> component selection -> resolver validation -> pinmap validation
-> schematic/PCB generation -> writer correctness -> board validation
```

Low-confidence selections should fail before file writing unless the requested
acceptance level allows draft output.

### 11.3 Pinmap

Component records should reference `pinmap` IDs when a symbol-footprint pairing
is verified. The component layer should not duplicate pinmap logic; it should
point to the pinmap registry and require it for verified component readiness.

### 11.4 Library Resolver

Catalog validation must check that declared symbols and footprints exist in the
resolver index when roots are available. Missing external libraries should be
warnings for catalog unit tests but blocking for acceptance levels that require
verified output.

## 12. Validation Rules

Catalog validation must detect:

- duplicate component IDs;
- unknown families;
- invalid confidence levels;
- missing symbol bindings;
- missing package variants;
- package variants without footprints;
- verified records without pinmaps when pinmaps are required;
- unresolved symbol IDs when resolver data is available;
- unresolved footprint IDs when resolver data is available;
- symbol and footprint IDs that do not use KiCad `Library:Identifier` format;
- duplicate function pins within the same unit where not explicitly allowed;
- ratings that cannot be compared because units are malformed.

Selection validation must detect:

- requested voltage/current/power outside rating bounds;
- package variants that do not match the request;
- polarity-sensitive parts placed in symmetric-only contexts;
- placeholder choices requested for connectivity or stronger acceptance;
- conflicting block requirements.

## 13. Acceptance Levels

Component selection must support these acceptance levels:

- `draft`: placeholder and inferred choices allowed with warnings.
- `structural`: symbol and footprint must resolve or be locally generated with
  explicit warnings.
- `connectivity`: symbol, footprint, and pinmap must be verified or allowed
  passive rule-inferred.
- `erc_drc`: same as connectivity plus KiCad validation evidence where
  available.
- `fabrication_candidate`: verified component records, pinmaps, package
  variants, ratings, and no unresolved placeholders.

## 14. Test Strategy

Tests must not require the user's external KiCad repositories by default.

Required deterministic tests:

- catalog loading and merge order;
- duplicate ID diagnostics;
- family validation;
- confidence-level gating;
- simple resistor selection;
- capacitor voltage-rating rejection;
- LED polarity mapping;
- connector pin-count selection;
- missing pinmap blocking for verified acceptance;
- resolver-backed symbol/footprint checks using tiny fixtures;
- CLI JSON shape tests.

External integration tests may use:

```text
$KICAD_SYMBOLS_DIR
$KICAD_FOOTPRINTS_DIR
```

but they must be skipped unless explicitly enabled.

## 15. Documentation

Documentation must explain:

- what component intelligence does and does not guarantee;
- how confidence levels work;
- how to add a new component record;
- how component records relate to KiCad symbols, footprints, and pinmaps;
- which records are safe for autonomous generation;
- current catalog coverage and limitations.

## 16. Open Questions

- Should catalog files remain pure JSON, or should a later authoring format use
  YAML for comments and reviewability?
- Should manufacturer-specific data live in the same catalog or a separate
  sourcing catalog?
- How strict should passive rule inference be for autonomous connectivity-level
  generation?
- Should component IDs be KiCadAI-specific or align with an external ontology
  later?
- When should natural-language part selection be introduced, and should it live
  outside the deterministic core?
