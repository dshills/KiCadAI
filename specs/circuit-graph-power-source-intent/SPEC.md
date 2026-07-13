# Circuit Graph Power-Source Intent Specification

## Purpose

`generic-circuit-v1` must represent an externally driven supply without inventing
a physical component or relying on KiCadAI to infer electrical drive from a
connector role. This addendum introduces explicit schematic power-flag intent
for catalog-resolved circuit graphs.

## Goals

- Let an untrusted provider explicitly identify externally driven power and
  ground nets.
- Validate the assertion before graph resolution and lowering.
- Emit a visible KiCad `power:PWR_FLAG` symbol on each declared net.
- Keep power flags out of PCB placement, pad assignment, routing, BOM, and
  component-count evidence.
- Preserve deterministic normalization, hashing, transactions, and round trips.

## Non-Goals

- Inferring power sources from project names, component references, connector
  roles, or fixture paths.
- Treating a power flag as proof that voltage, current, polarity, or protection
  requirements are correct.
- Applying KiCad ERC suppression markers or exclusions instead of correcting
  the modeled source evidence.
- Adding virtual footprints or catalog component records for schematic-only
  symbols.
- Changing PCB routing or placement.

## IR Contract

The circuit graph gains an optional top-level `power_flags` array:

```json
{
  "power_flags": [
    {"net": "VBUS_RAW"},
    {"net": "GND"}
  ]
}
```

Each entry contains exactly one required `net` field. The declaration means the
design author asserts that the named net is driven by a source outside the
modeled circuit and requests a schematic `PWR_FLAG` on that net.

Circuit-graph net roles are explicit input fields validated before resolution;
they are never inferred from a power-flag declaration or its connections.

The provider schema remains strict. Unknown fields fail decoding.

## Validation

KiCadAI must fail closed when:

- `net` is empty;
- the named net does not exist;
- the same net is declared more than once;
- the referenced net role is not `power`, `power_pos`, `power_neg`, `ground`, or
  `return`;
- trusted resolved pin metadata shows that the net already has an internal
  `power_out` driver; or
- the number of declarations exceeds the dedicated limit of 64 flags.

Power flags do not change endpoint, voltage-domain, current, rating, or catalog
validation. A declaration on a structurally invalid net remains invalid. The
internal-driver check runs after catalog and pin-function resolution, before
schematic lowering. It does not guess electrical types from component roles.
Multi-source and OR-ed supplies are outside this v1 declaration: they must model
their switching, ideal-diode, jumper, or OR-ing components explicitly so each
source-side net has unambiguous drive intent.

## Normalization And Provenance

Net-name matching is case-sensitive, consistent with graph net identity.
Normalization sorts power flags by exact net name. The normalized
declarations participate in the graph hash and recorded/live semantic
projection. Repeated parsing and resolution must produce identical ordering,
synthetic IDs, references, layout, and transactions.

The 64-entry limit is a provider-input safety bound dedicated to schematic-only
flags. It caps synthetic-symbol and net-attachment work independently of the
larger physical-component and endpoint limits and does not consume the physical
component limit. Designs requiring more than 64 separately driven domains
require a later schema revision.

## Schematic Lowering

For each normalized declaration, graph-to-schematic lowering must:

1. Generate a deterministic component ID in the reserved internal namespace
   `kicadai_pwr_flag_<hash>`, where `<hash>` is the first 16 lowercase hex
   characters of SHA-256 over the exact net name; fail closed if an input
   component somehow collides with that ID.
2. Add a schematic-only component using `power:PWR_FLAG`.
3. Assign unique hidden-style references as `#FLG01`, `#FLG02`, and so on in
   normalized exact-net-name order; reject input component references using the
   reserved `#FLG` prefix and check generated references for collision.
4. Expose pin `1` as a power pin and preserve the KiCad library symbol's
   `power_out` electrical type in the written schematic.
5. Append that pin to the existing named schematic net.
6. Mark the component role as `power_symbol` so layout keeps it in the power or
   ground lane associated with the net.

The synthetic component must not be added to the resolved catalog component
list or explicit PCB component list. It therefore cannot receive a footprint,
placement, pad net, route endpoint, BOM identity, or catalog confidence claim.

## Layout And Readability

Power flags are support symbols, not primary flow anchors. Automatic schematic
layout places flags on positive/power nets above the primary schematic bounding
box and flags on ground/return nets below it, aligned near the first endpoint of
their declared net after endpoints are sorted by exact component ID, unit, and
resolved pin number. Page centering and label-based long-net rules still apply.
If existing generic support-role inference cannot do this readably, only a
role-based reusable correction is permitted.

## Repair Policy

KiCadAI may normalize ordering and generated IDs. It may not insert a power flag
unless the input graph declares it. Missing drive evidence remains an ERC
finding. A provider correction attempt may add a declaration only by returning a
new graph that passes strict validation.

## Testing

Unit coverage must prove:

- strict JSON-schema acceptance and rejection;
- unknown, duplicate, empty, and non-power net failures;
- deterministic normalization and graph hashing;
- schematic-only lowering to `power:PWR_FLAG`;
- connection of each flag to the declared net;
- no change to explicit PCB component, placement, or routing inputs;
- fail-closed behavior when a flag collides with an existing generated ID.

Integration coverage must exercise the generic USB-C BMP280 recorded fixture,
verify that KiCad ERC power-driver errors are removed, and preserve all existing
generic and bounded pass fixtures.

## Acceptance Criteria

- `generic-circuit-v1` strict-decodes explicit power flags.
- Invalid declarations fail before planning.
- Valid declarations create deterministic schematic-only PWR_FLAG symbols.
- No virtual footprint or topology-specific production branch exists.
- The generic USB-C BMP280 fixture clears its power-driver ERC findings.
- Existing tests and KiCad-backed pass fixtures do not regress.
