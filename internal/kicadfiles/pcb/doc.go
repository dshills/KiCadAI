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
package pcb
