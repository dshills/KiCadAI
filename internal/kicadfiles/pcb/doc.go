// Package pcb writes KiCad .kicad_pcb files from Go data structures.
//
// The package targets the KiCad 10 file shape observed in KiCad-saved demo
// boards: modern file metadata, full layer tables, structured footprint
// properties, embedded footprint geometry, routes, vias, zones, board graphics,
// and optional preservation of unsupported raw S-expression nodes.
//
// Most callers should build a PCBFile with DefaultGeneral, DefaultTwoLayerStack,
// DefaultSetup, and NewNetRegistry, then call Write. Validate can be used before
// writing when callers want diagnostics without producing output.
//
// File ownership is intentionally being tightened so future KiCad object support
// lands in narrow diffs:
//   - pcb.go is the current legacy home for the model, renderer, validation, and
//     preservation helpers until those sections are moved.
//   - read.go owns S-expression parsing and node-to-model conversion.
//   - connectivity.go owns generated-board electrical connectivity checks.
//   - corpus.go owns golden corpus loading helpers.
//   - correctness_fixture.go and led.go own runtime fixture constructors used by
//     examples, tests, and higher-level generators.
//
// Planned follow-up files in this same package are model.go, render.go,
// validate.go, and preserved.go. The reorganization is a same-package file split,
// not a package rename or sub-package extraction.
package pcb
