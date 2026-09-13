package schematiclayout

import (
	"cmp"
	"maps"
	"slices"

	"kicadai/internal/kicadfiles"
)

// functionalSupportPositions compacts explicit support families within a block.
// A fragment decoupler with no support-parent record is assigned only when its
// block has exactly one active component. Multiple-active ambiguity is retained.
// Body envelopes include pin-label clearance supplied by the IR adapter. The
// native writer remains responsible for final wire/label/field collision checks.
func functionalSupportPositions(components []Component, original map[string]kicadfiles.Point, rules Rules) map[string]kicadfiles.Point {
	return functionalSupportPositionsWithPins(components, nil, original, rules, false)
}

func functionalSupportPositionsWithPins(components []Component, nets []Net, original map[string]kicadfiles.Point, rules Rules, pinAware bool) map[string]kicadfiles.Point {
	return functionalSupportPositionsJoint(components, nets, original, rules, pinAware, false)
}

func functionalSupportPositionsJoint(components []Component, nets []Net, original map[string]kicadfiles.Point, rules Rules, pinAware, joint bool) map[string]kicadfiles.Point {
	return functionalSupportPositionsPower(components, nets, original, rules, pinAware, joint, false)
}

func functionalSupportPositionsPower(components []Component, nets []Net, original map[string]kicadfiles.Point, rules Rules, pinAware, joint, powerLocality bool) map[string]kicadfiles.Point {
	byRef := map[string]Component{}
	active, activeCount := "", 0
	for _, c := range components {
		byRef[c.Ref] = c
		if containsNormalizedRole(c.Role, "ic", "regulator", "sensor") {
			active, activeCount = c.Ref, activeCount+1
		}
	}
	parents := map[string]string{}
	for _, c := range components {
		parent := c.SupportParent
		if parent == "" && activeCount == 1 && containsNormalizedRole(c.Role, "decoupling_capacitor", "bulk_capacitor", "decoupling") {
			parent = active
		}
		if _, known := byRef[parent]; parent != "" && known && parent != c.Ref {
			parents[c.Ref] = parent
		}
	}
	if len(parents) == 0 {
		return maps.Clone(original)
	}
	positions := maps.Clone(original)
	placed := map[string]bool{}
	for _, c := range components {
		placed[c.Ref] = parents[c.Ref] == ""
	}
	refs := slices.Sorted(maps.Keys(parents))
	// Candidate offsets are a bounded, deterministic radial search. No example
	// IDs, absolute coordinates, source ordering, or physical-board data enter it.
	step := max(rules.Grid, kicadfiles.MM(2.54))
	var offsets []kicadfiles.Point
	for y := -80; y <= 80; y++ {
		for x := -80; x <= 80; x++ {
			offsets = append(offsets, kicadfiles.Point{X: kicadfiles.IU(x) * step, Y: kicadfiles.IU(y) * step})
		}
	}
	slices.SortFunc(offsets, func(a, b kicadfiles.Point) int {
		distance := func(p kicadfiles.Point) float64 { return float64(p.X)*float64(p.X) + float64(p.Y)*float64(p.Y) }
		if order := cmp.Compare(distance(a), distance(b)); order != 0 {
			return order
		}
		if a.Y != b.Y {
			return cmp.Compare(a.Y, b.Y)
		}
		return cmp.Compare(a.X, b.X)
	})
	gap := max(rules.MinComponentSpacing, kicadfiles.MM(17.78)) / 2
	if powerLocality {
		// Honor the declared block spacing without V5's extra radial padding.
		// Native label/field and body clearance checks remain mandatory.
		gap = max(rules.MinComponentSpacing, kicadfiles.MM(15.24)) / 2
	}
	for remaining := len(refs); remaining > 0; {
		progress := false
		for _, ref := range refs {
			if placed[ref] || !placed[parents[ref]] {
				continue
			}
			origin, found := positions[parents[ref]], false
			side := ""
			if pinAware {
				side = supportAttachmentSide(ref, byRef[parents[ref]], nets)
			}
			ownerBounds := componentBoundsAt(byRef[parents[ref]], origin)
			var pinCorridors []Rect
			if pinAware {
				pinCorridors = supportOwnerPinCorridors(byRef[parents[ref]], nets, origin)
			}
			candidates := offsets
			if powerLocality {
				candidates = powerSupportOffsets(byRef[ref], byRef[parents[ref]], nets, offsets)
			}
			for _, offset := range candidates {
				p := SnapPoint(kicadfiles.Point{X: origin.X + offset.X, Y: origin.Y + offset.Y}, rules.Grid)
				bounds := componentBoundsAt(byRef[ref], p).Inflate(gap)
				if !supportOnAttachmentSide(bounds, ownerBounds, side) {
					continue
				}
				if joint && !jointSupportClearPower(byRef[ref], p, byRef, positions, placed, nets, gap, powerLocality) {
					continue
				}
				clear := true
				for _, corridor := range pinCorridors {
					if bounds.Intersects(corridor) {
						clear = false
						break
					}
				}
				for other, done := range placed {
					if done && bounds.Intersects(componentBoundsAt(byRef[other], positions[other]).Inflate(gap)) {
						clear = false
						break
					}
				}
				if clear {
					positions[ref], placed[ref], found, progress = p, true, true, true
					remaining--
					break
				}
			}
			if !found {
				return maps.Clone(original)
			}
		}
		if !progress { // Cycles supplied directly to the layout engine: no partial move.
			return maps.Clone(original)
		}
	}
	return positions
}
