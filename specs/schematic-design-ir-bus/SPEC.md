# Schematic IR Vector-Bus Support

Date: 2026-07-09

## Goal

Add deterministic, KiCad-native vector-bus support to the schematic design IR
so an AI can describe grouped digital nets without reducing them to unrelated
labels. The feature must preserve the existing readable scalar-net behavior and
must fail closed when the requested bus geometry cannot be represented safely.

## Scope

V1 supports:

- named buses with explicit ordered members;
- member nets that remain ordinary electrical scalar nets;
- explicit bus spine points and bus-entry geometry in layout intent;
- scalar member wires and labels attached to KiCad bus entries;
- strict parse/validation, transaction emission, writer output, readback, and
  readability evidence;
- root-sheet generation.

V1 does not support:

- guessing bus membership from net names;
- expanding ranges or aliases implicitly;
- bus-aware automatic hierarchy partitioning;
- cross-sheet bus propagation;
- changing electrical net membership during repair;
- PCB bus semantics.

If a readable document requires hierarchy partitioning while it also contains a
vector bus, conversion must return a blocking diagnostic rather than dropping
the bus or converting it to scalar labels.

## IR Contract

`circuit.buses` is separate from `circuit.nets`:

```json
{
  "circuit": {
    "buses": [
      {
        "id": "data_bus",
        "name": "DATA[0..3]",
        "members": [
          {"net": "DATA0", "label": "DATA0"},
          {"net": "DATA1", "label": "DATA1"},
          {"net": "DATA2", "label": "DATA2"},
          {"net": "DATA3", "label": "DATA3"}
        ]
      }
    ]
  },
  "layout": {
    "buses": [
      {
        "bus": "data_bus",
        "points": [
          {"x_mm": 75.0, "y_mm": 80.0},
          {"x_mm": 150.0, "y_mm": 80.0}
        ],
        "entries": [
          {"member": "DATA0", "at": {"x_mm": 82.5, "y_mm": 80.0}, "size": {"x_mm": 2.54, "y_mm": 2.54}},
          {"member": "DATA1", "at": {"x_mm": 100.0, "y_mm": 80.0}, "size": {"x_mm": 2.54, "y_mm": 2.54}}
        ]
      }
    ]
  }
}
```

The `member` value identifies the member net, not a component pin. Every bus
member must reference exactly one existing non-`no_connect` net. A member
label is explicit because KiCad bus-member naming has syntax beyond ordinary
net-name equality. Multiple entries may use the same member label when the
member is intentionally connected at multiple locations.

Coordinates are millimetres in the IR and are converted to KiCad internal
units by the adapter. Points must be orthogonalized only when the requested
segments are already horizontal or vertical; the adapter must not silently
reshape arbitrary user geometry.

## Validation

Blocking issues include:

- duplicate bus IDs;
- empty or duplicate bus member nets;
- bus members referencing unknown nets or `no_connect` nets;
- a net assigned to conflicting bus declarations;
- empty bus names or member labels;
- fewer than two bus spine points;
- non-orthogonal bus segments;
- entries referencing unknown members;
- zero-size entries;
- entries not lying on a bus spine point/segment;
- duplicate entry geometry that cannot be distinguished deterministically;
- readable hierarchy partition requested for a document containing buses.

The existing scalar `net.role = "bus"` remains valid for a logical bus member
such as SDA/SCL. It does not create a vector bus and must not be auto-promoted.

## KiCad Mapping

The adapter emits:

1. `add_bus` operations for each spine segment;
2. `add_bus_entry` operations for each declared entry;
3. member wire operations from each entry connection point to the referenced
   component pin endpoint;
4. local member labels using the declared `label`.

The entry connection point is `entry.at + entry.size`, matching KiCad’s
`bus_entry (at ...) (size ...)` convention. Member wires are ordinary KiCad
schematic wires and therefore remain visible to existing scalar connectivity
checks. The bus graphics do not replace scalar net identity.

## Readability Policy

- Bus spine segments must be on the requested page and on the configured grid.
- Entries must be on the spine and use a consistent size within one bus.
- Member wires must be orthogonal and must not terminate inside a symbol body.
- Member labels are placed at the entry-connected wire endpoint, rotated only
  when required by the entry direction.
- Bus diagnostics are blocking under `acceptance = readable`; no warning is
  suppressed merely because KiCad can open the file.

## Repair Policy

Allowed:

- deterministic ordering of buses, members, and entries;
- conversion of equivalent integer-unit coordinates to the configured grid;
- insertion of a missing member label only when the IR explicitly supplies a
  unique member label elsewhere in the same bus.

Forbidden:

- guessing bus ranges or membership;
- moving a bus or entry to resolve a collision without an explicit repair
  policy and a reported changed coordinate;
- converting a vector bus to independent scalar labels;
- dropping an entry or member.

## Evidence

The feature is complete only when a checked-in bus fixture proves:

- strict IR validation;
- deterministic normalized bus ordering;
- transaction validation and plan support;
- KiCad schematic readback with bus and entry counts preserved;
- scalar member wires/labels preserved;
- internal schematic validation and readability pass;
- zero round-trip diff when KiCad CLI is available;
- explicit environment-gated evidence when KiCad CLI is unavailable.
