// Package designapi provides a higher-level Go API for building KiCad designs
// from intent-level operations such as adding symbols, connecting pins,
// assigning footprints, placing footprints, routing tracks, adding zones, and
// writing project directories.
//
// The package is intentionally layered above the lower-level project,
// schematic, PCB, and design writers. It produces the same Design model used by
// those writers, so callers can drop down to the raw structures when they need
// KiCad features that are not yet represented by the intent API.
package designapi
