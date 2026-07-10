# Vector-Bus Implementation Plan

## Phase 1: IR Model And Validation

Add `Circuit.Buses`, `Bus`, `BusMember`, `Layout.Buses`, `BusLayout`, and
`BusEntryLayout` with explicit scalar `endpoint` ownership. Implement strict
validation and deterministic normalization.

Files: `internal/schematicir/model.go`, `validate.go`, `normalize.go`, tests,
`SPEC.md`.

Acceptance: malformed membership, geometry, and hierarchy combinations fail with
structured blocking issues; valid bus IR normalizes deterministically.

## Phase 2: Transaction And Design API

Add `add_bus`, `add_bus_entry`, and explicit schematic wire operations. Add
builder methods that append native `schematic.Bus`, `schematic.BusEntry`, and
wire segments with deterministic UUIDs. Extend transaction validation, planning,
apply, cloning, and supported-operation reporting.

Files: `internal/transactions/model.go`, `validate.go`, `plan.go`, `apply.go`,
`internal/kicadfiles/designapi/builder.go`, tests.

Acceptance: a hand-built transaction writes native bus nodes and is rejected if
its geometry is malformed.

## Phase 3: IR Adapter And Readable Geometry

Map validated bus layout intent to transaction operations. Emit native bus and
entry graphics plus paired short scalar label stubs at each component endpoint
and KiCad entry connection point. Keep hierarchy conversion fail closed when
buses are present.

Files: `internal/schematicir/adapter.go`, tests, targeted hierarchy checks.

Acceptance: bus IR produces a readable transaction with all scalar members still
electrically represented and without crossing member wires.

## Phase 4: Golden Fixture And Readback

Add a small four-bit bus fixture. Write it through the public CLI and direct Go
path. Read back bus/entry/wire/label counts, run schematic validation and
readability checks, and add optional KiCad ERC/round-trip commands.

Files: `examples/schematic-ir/vector_bus.json`, `internal/schematicir`
goldens/tests, CLI tests, docs.

Acceptance: checked-in fixture passes all available gates and reports missing
KiCad evidence explicitly rather than passing silently. Readback preserves the
native bus, every entry, both scalar stubs per entry, and member label counts.

## Phase 5: Documentation And Verification

Update the schematic IR spec, roadmap, CLI reference, run Prism on staged code,
run focused and full tests, and commit each phase separately.

Rollback risk: medium to high because transaction operation dispatch is shared;
retain scalar-net behavior and keep bus support additive.
