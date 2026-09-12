package schematiclayout

import (
	"slices"

	"kicadai/internal/kicadfiles"
)

// Interpret the IR's supported roles only in the versioned drawing projection.
// Names, connectivity and caller-owned net metadata remain unchanged.
func jointCanonicalNetRoles(nets []Net) []Net {
	result := slices.Clone(nets)
	for i := range result {
		switch result[i].Role {
		case "power_pos", "power_neg":
			result[i].Role = "power"
		case "return":
			result[i].Role = "ground"
		}
	}
	return result
}

// A support body alone is insufficient: its own pin stubs and long outward
// labels can cross the owner's annotation corridor even when bodies are clear.
// Reserve both component envelopes jointly, before routing. The native writer
// still makes the final decision with actual wires and glyph bounds.
func jointSupportClear(support Component, at kicadfiles.Point, byRef map[string]Component, positions map[string]kicadfiles.Point, placed map[string]bool, nets []Net, gap kicadfiles.IU) bool {
	return jointSupportClearPower(support, at, byRef, positions, placed, nets, gap, false)
}

func jointSupportClearPower(support Component, at kicadfiles.Point, byRef map[string]Component, positions map[string]kicadfiles.Point, placed map[string]bool, nets []Net, gap kicadfiles.IU, powerLocality bool) bool {
	body := componentBoundsAt(support, at).Inflate(gap)
	corridors := powerTaggedCorridors(support, nets, at)
	for ref, done := range placed {
		if !done {
			continue
		}
		other := componentBoundsAt(byRef[ref], positions[ref]).Inflate(gap)
		otherCorridors := powerTaggedCorridors(byRef[ref], nets, positions[ref])
		for _, a := range corridors {
			if a.box.Intersects(other) {
				return false
			}
			for _, b := range otherCorridors {
				if powerLocality && a.net == b.net && a.shareable && b.shareable {
					continue // A same-net power corridor can become a shared rail.
				}
				if a.box.Intersects(b.box) {
					return false
				}
			}
		}
		for _, b := range otherCorridors {
			if body.Intersects(b.box) {
				return false
			}
		}
	}
	return true
}
