# Generic Multi-Unit Components

## Status

Required by the `generic_lm358_buffered_signal_conditioner` promotion milestone.

## Problem

KiCadAI already carries numeric symbol units through resolved endpoints,
schematic IR, transactions, and the KiCad writer. That support is implicit:

- a circuit component cannot declare the logical units it intends to use;
- provider capability data collapses all unit functions into one flat list;
- catalog symbol bindings do not name a unit or distinguish functional and
  power units;
- schematic placement is package-scoped, so individual units cannot express
  distinct stage roles;
- duplicate footprint-assignment operations may be emitted for units sharing a
  physical reference;
- required-unit and shared-package invariants are not validated as a coherent
  contract.

This is sufficient for narrow writer tests, but not for an untrusted AI provider
to describe one physical dual op-amp accurately. Modeling the LM358 as two
independent components would create two footprints and two BOM items. Omitting
its power unit would create an incomplete schematic. Treating all unit functions
as interchangeable would make endpoint resolution ambiguous.

## Goals

- Represent one physical catalog component with multiple logical schematic
  units.
- Keep one component ID, reference, footprint, placement, and BOM identity per
  package.
- Give logical units stable provider-facing IDs independent of fixture names.
- Map each logical unit to one verified KiCad symbol unit.
- Require all catalog-declared mandatory units, including a separate power unit.
- Resolve unit-qualified endpoints deterministically and fail closed on
  ambiguity or conflict.
- Place logical units independently in the schematic while retaining package
  ownership.
- Emit one footprint assignment and one PCB component for the package.
- Preserve deterministic normalized graphs, schematic IR, transactions, and
  written KiCad files.
- Promote one LM358 circuit through recorded and live provider lanes and all
  existing KiCad-backed evidence gates.

## Non-Goals

- Arbitrary interchangeable-unit optimization or gate swapping.
- Automatic decomposition of multiple packages from one component.
- Hidden power-unit synthesis when the catalog or graph omits required data.
- Multi-package symbols, heterogeneous packages, stacked pins, or jumper-pad
  semantics beyond current verified support.
- Analog simulation or claims about stability, bandwidth, noise, distortion,
  output drive, output swing, common-mode range, or load compatibility.
- New topology-specific provider schemas, circuit blocks, dispatch rules, or
  coordinate tables.
- Changes to unrelated PCB placement, routing, or block families.

## Verified LM358 Model

The first verified record is a Texas Instruments LM358 in the standard SOIC-8
package, using:

- symbol `Amplifier_Operational:LM358`;
- footprint `Package_SO:SOIC-8_3.9x4.9mm_P1.27mm`;
- unit `A`, KiCad unit 1: `OUT` pad 1, `IN_MINUS` pad 2,
  `IN_PLUS` pad 3;
- unit `B`, KiCad unit 2: `IN_PLUS` pad 5, `IN_MINUS` pad 6,
  `OUT` pad 7;
- unit `P`, KiCad unit 3: `V_MINUS` pad 4, `V_PLUS` pad 8.

The pin mapping is verified against the project's checked-out KiCad 10.0.3
source/library baseline, the checked-out KiCad footprint library, and the Texas
Instruments LM358 data sheet:

- `kicad-symbols/Amplifier_Operational.kicad_symdir/LM358.kicad_sym`
- `kicad-symbols/Amplifier_Operational.kicad_symdir/LM2904.kicad_sym`
- `kicad-footprints/Package_SO.pretty/SOIC-8_3.9x4.9mm_P1.27mm.kicad_mod`
- <https://www.ti.com/lit/ds/symlink/lm358.pdf>

The catalog record must select a concrete orderable SOIC-8 MPN and carry the
same evidence and review-status discipline as existing verified op-amp records.

## Data Model

### Catalog Unit Metadata

`components.SymbolBinding` gains package-unit metadata:

```go
type SymbolBinding struct {
    SymbolID     string
    Unit         int
    UnitID       string
    UnitType     SymbolUnitType
    RequiredUnit bool
    FunctionPins []FunctionPin
    // existing fields
}

type SymbolUnitType string

const (
    SymbolUnitFunctional SymbolUnitType = "functional"
    SymbolUnitPower      SymbolUnitType = "power"
)
```

Supported roles in v1:

- `functional`: a usable logical function such as op-amp A or B;
- `power`: a separate symbol unit containing shared package supply pins.

The catalog field is named `unit_type` to distinguish package structure from the
graph declaration's circuit-specific `role` such as `reference_buffer`.

`unit_id` is the provider-facing stable identifier. It must be unique within one
catalog record, use the existing safe identifier grammar, and map to exactly one
positive KiCad unit number. Unit IDs are canonicalized to uppercase during
normalization and before comparison or hashing; uniqueness is checked after that
normalization. Positive KiCad unit numbers must also be unique within the record,
so two unit IDs cannot select the same symbol unit. Named multi-unit records must
not mix named and anonymous symbol bindings; any partial mixture is a catalog
validation error. `required_unit` states that every graph instance must declare
and lower that unit. Functional units are optional at the package level; the
target fixture explicitly selects both A and B. A separate package power unit is
mandatory. A binding with a non-empty `unit_id` must have `unit > 0`; zero is
never valid for a named unit. The existing signed Go field is retained for JSON
compatibility and catalog validation rejects negative values.

Existing single-unit records remain compatible: an omitted `unit_id` and unit
number zero retain current behavior. Zero is reserved for this legacy
null/default representation; named units and all resolved KiCad units use
KiCad's 1-based numbering. Existing anonymous multi-unit test records remain
supported during migration, but they are not exposed as named multi-unit
provider capabilities.

### Circuit Graph Unit Declarations

`circuitgraph.Component` gains:

```json
"units": [
  {"id": "A", "role": "reference_buffer"},
  {"id": "B", "role": "gain_stage"},
  {"id": "P", "role": "power"}
]
```

Each declaration contains:

- `id`: a catalog-defined `unit_id`;
- `role`: a bounded layout/design role string used for explanation and
  schematic layout, not for physical pin resolution.

Rules:

- unit IDs are normalized to uppercase first and must then be unique within a
  component;
- a named multi-unit catalog component requires explicit declarations;
- every declared unit must exist in the selected catalog record;
- every catalog `required_unit` must be declared exactly once;
- an endpoint `unit` must refer to a declared unit;
- graph unit IDs resolve through catalog metadata, not alphabet arithmetic;
- legacy numeric/A-to-Z endpoint parsing remains only for anonymous legacy
  catalog bindings;
- units cannot select a second component, package, footprint, MPN, or reference.

The provider schema remains `generic-circuit-v1`. This is an additive component
field, not a topology-specific contract. KiCadAI supplies the current strict
schema with each provider request and decodes provider output with the matching
implementation. Existing v1 documents omit the new optional fields and remain
valid. An older strict KiCadAI binary is not expected to consume newly generated
named-unit documents; cross-version interchange of provider responses is not a
supported contract. A version bump is reserved for a required-field or semantic
change that invalidates existing v1 documents.

### Unit-Qualified Schematic Placement

`SchematicPlacement` gains optional `unit`:

```json
{"component":"amplifier","unit":"A","group":"vref","orientation":"normal"}
```

Relative relationships gain corresponding optional unit qualifiers:

```json
{"component":"feedback","near":"amplifier","near_unit":"B"}
```

`near_unit`, `above_unit`, and `right_of_unit` are valid only when their base
relationship field is present. The named target component must declare the
selected unit. This keeps targets structured without encoding component/unit
pairs in an ad hoc string.

Placement identity becomes `(component, unit)` for a named multi-unit package.
For single-unit components, `unit` remains empty. For named multi-unit
components:

- a package-only placement may supply defaults shared by all units;
- a unit-qualified placement overrides the package default for that unit on a
  per-field basis; omitted fields inherit the package-level value;
- duplicate placements for the same `(component, unit)` fail validation;
- placement references remain component IDs; layout groups may contain the
  physical component once, while unit roles and unit-qualified placements
  determine each symbol unit's lane and local stage position;
- no PCB placement accepts a unit because PCB placement is package-scoped.

Unqualified relative constraints continue to target physical component IDs.
Qualified constraints target one declared schematic unit. Unit ordering is
otherwise derived from unit role, group rank, endpoint topology, and stable unit
ID. PCB placement relationships remain package-only.

All placement fields are strings or identifiers for which the empty string is
not a valid explicit override. Empty therefore means omitted; no pointer-field or
numeric-zero ambiguity is introduced by the inheritance rule.

## Provider Capability Contract

The provider capability entry for a named multi-unit component includes:

```json
"units": [
  {"id":"A","role":"functional","required":false,
   "functions":["IN_PLUS","IN_MINUS","OUT"]},
  {"id":"B","role":"functional","required":false,
   "functions":["IN_PLUS","IN_MINUS","OUT"]},
  {"id":"P","role":"power","required":true,
   "functions":["V_PLUS","V_MINUS"]}
]
```

The existing flat `functions` list remains for compatibility. The capability
rules explicitly require `components[].units` and endpoint `unit` qualifiers for
named multi-unit components. Provider output remains untrusted and must pass the
strict JSON schema, graph validation, catalog resolution, and physical binding
checks before planning.

## Resolution Semantics

Resolution performs these steps in order:

1. resolve one component record and package variant for the physical component;
2. build a case-insensitive map from catalog `unit_id` to KiCad unit number;
3. validate graph unit declarations against that map;
4. require all mandatory catalog units;
5. retain only functions belonging to declared units;
6. resolve every endpoint through its declared unit and logical function;
7. map the symbol pin to exactly one verified package pad;
8. register physical pad ownership across all units;
9. reject one physical pad assigned to different nets, even through different
   unit views;
10. require every required function to be connected or explicitly no-connected;
11. for a required power unit, require every physical pad represented by its
    required functions to be connected to the corresponding resolved net.

Resolved functions retain both `UnitID` and numeric `Unit`. Resolved components
retain the selected unit declarations and package-level identity. Resolution
hashes include unit metadata and declarations.

## Lowering And Identity

### Schematic

One schematic IR component is emitted per declared unit. Every emitted unit:

- shares the physical reference and component identity;
- has ID `mu_` plus the leading 24 characters of the lowercase hexadecimal
  SHA-256 sum over
  `kicadai:schematic-unit:v1\x00<component-id>\x00<uppercase-unit-id>`;
- registers that ID globally and fails lowering on any collision rather than
  relying on ambiguous string concatenation;
- carries the verified numeric KiCad unit;
- contains only that unit's symbol pins;
- uses unit-qualified layout intent when present;
- receives a deterministic UUID through existing transaction rules.

The required power unit is emitted as a normal KiCad symbol unit. It is placed
in the power/ground context near package decoupling and cannot be silently
omitted.

### Footprint, PCB, And BOM

The physical graph component remains the ownership boundary:

- `ToDesignRequest` emits one explicit component and one pad set;
- physical pads are deduplicated by pad number across units;
- lowering verifies that every declared unit resolves through the same catalog
  record, package variant, and footprint ID before creating the physical
  component;
- the schematic adapter emits at most one `assign_footprint` operation per
  physical reference and rejects conflicting footprint IDs;
- PCB placement emits one footprint for the reference;
- BOM/component evidence counts the package once;
- unit IDs never appear as PCB references or independent BOM lines.

## Failure Policy

The implementation must fail closed with stable diagnostics for:

- duplicate or invalid graph unit IDs;
- a declared unit absent from catalog evidence;
- a required catalog unit absent from the graph;
- an endpoint that omits a unit when its function exists on multiple declared
  units;
- an endpoint naming a unit not declared by the component;
- one unit function resolving to multiple pads;
- different logical symbol pins collapsing to one physical pad; v1 permits only
  repeated unit views with the exact same symbol pin and pad identity;
- the same physical pad assigned to multiple nets through any unit;
- duplicate schematic placement for one `(component, unit)`;
- multiple or conflicting footprint assignments for one physical reference;
- a package power unit or its required supply pins left unconnected;
- any unit producing a second PCB component or BOM identity.

No validation may infer missing power connections or suppress ERC/DRC findings.

## Target Fixture

`generic_lm358_buffered_signal_conditioner` contains:

- one 5 V/GND input connector;
- a resistor divider producing raw 2.5 V reference;
- LM358 unit A as a unity-gain VREF buffer;
- bypass capacitance on the buffered VREF;
- LM358 unit B as an AC-coupled non-inverting gain stage;
- feedback and gain resistors adjacent to unit B;
- one package supply decoupling capacitor adjacent to the LM358;
- signal input and output connectors;
- one LM358 SOIC-8 footprint and BOM identity.

Layout requirements:

- input on the left, gain stage in the center, output on the right;
- reference divider and unit A near the analog stage;
- positive supply above and ground below;
- feedback adjacent to unit B;
- decoupling adjacent to package supply pins;
- bounded, centered, deterministic schematic;
- one deterministic PCB package placement with graph-complete routing for
  supply, ground, raw/buffered VREF, feedback, input, and output nets.

## Analog Evidence Policy

ERC and DRC prove structural/electrical connectivity and board geometry only.
The fixture and promotion report must retain explicit `review_required` evidence
for:

- stability;
- gain-bandwidth;
- output swing;
- input common-mode range;
- output drive;
- noise;
- distortion;
- load compatibility.

These findings do not become implicit pass claims. They are documented review
obligations at the declared acceptance level.

## Testing

### Catalog And Schema

- LM358 record validation, unit uniqueness, required power unit, function/pad
  mapping, symbol/footprint evidence, and op-amp evidence.
- Provider schema accepts valid unit declarations and rejects unknown fields,
  duplicate units, and invalid roles.
- Capability output exposes deterministic unit/function metadata.

### Graph Resolution

- named A/B/P resolution and endpoint binding;
- nonexistent, omitted, duplicated, and conflicting units;
- required power-unit omission;
- shared physical pad net conflict across units;
- deterministic normalization and resolution hashes.

### Lowering And Writing

- three schematic units with one reference;
- correct numeric units and pin subsets;
- one footprint assignment, one physical PCB component, and one BOM identity;
- deterministic unit UUIDs and transaction bytes;
- write/read validation against resolver-backed LM358 geometry;
- normalized schematic and PCB round-trip stability.

### Fixture And Promotion

- recorded provider strict decode and semantic projection;
- recorded provider-to-KiCad offline workflow;
- optional environment-gated KiCad ERC/DRC promotion;
- optional credential-gated live OpenAI generation;
- live/recorded critical graph equivalence;
- regression coverage for existing provider-backed pass fixtures;
- `go test ./...`, `make lint`, and Prism review.

## Compatibility And Migration

- The graph version remains 1 because fields are additive and strict providers
  receive the updated schema and capability in one release.
- Existing single-unit graphs and catalogs require no changes.
- Existing anonymous multi-unit internal tests continue to use numeric unit
  behavior until migrated.
- Named multi-unit behavior is enabled only when catalog `unit_id` metadata is
  present; it never guesses package units from a project or fixture name.
- Recorded provider fixtures are regenerated only when their capability/schema
  projection changes materially.

## Completion Criteria

- The graph models one LM358 package with A, B, and required power units.
- Both functional units and the power unit lower into valid KiCad symbols.
- Exactly one LM358 footprint and BOM identity are emitted.
- Shared package pins and physical pad nets are correct.
- Recorded and live provider outputs strict-decode and are semantically
  equivalent.
- Schematic and PCB output are deterministic, readable, connected, and routed.
- KiCad ERC and strict DRC are clean.
- Writer correctness and normalized round-trip checks pass.
- Promotion metadata and the optional lane classify the fixture as `pass`.
- Existing provider-backed fixtures remain clean.
- Full tests and lint pass, Prism has no unresolved high findings, documentation
  is current, and the worktree is clean.
