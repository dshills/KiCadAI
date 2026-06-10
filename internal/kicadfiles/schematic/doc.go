// Package schematic writes KiCad .kicad_sch files for generated designs.
//
// The writer targets the KiCad 10 schematic file shape used by this project:
// modern headers with generator_version, always-present lib_symbols, nested
// symbol instances, root sheet_instances, and source-derived top-level item
// ordering. Supported first-class objects include symbols, symbol properties,
// symbol pins, wires, buses, bus entries, polylines, labels, no-connects,
// junctions, text, hierarchical sheets, sheet pins, and sheet instances.
//
// Generated designs should build SchematicFile values with deterministic UUIDs,
// call Validate or ValidateGeneratedConnectivity, then call Write. Constructors
// such as NewSymbol, NewWire, NewLabel, NewNoConnect, NewSheet, and NewSheetPin
// cover the common object shapes but do not replace validation.
//
// RawSchematicItem is an escape hatch for unsupported top-level schematic
// objects and future read-modify-write work. Raw items are preserved as
// comment-free S-expression fragments and sorted with known KiCad item kinds
// when possible. This is not a full lossless schematic parser: unsupported
// nested fields are opaque, and callers are responsible for supplying valid raw
// bodies with matching UUID metadata.
package schematic
