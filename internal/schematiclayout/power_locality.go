package schematiclayout

import (
	"cmp"
	"slices"

	"kicadai/internal/kicadfiles"
)

type powerCorridor struct {
	net       string
	box       Rect
	shareable bool
}

func powerLocalityGroup(group string, components []Component) bool {
	if group == "" {
		return false
	}
	var local []Component
	for _, component := range components {
		if component.GroupID == group {
			local = append(local, component)
		}
	}
	_, pure := pureDecouplingBlock(local)
	return pure
}

// Tag reservations so V6 can share same-net power/return access corridors.
// Bodies, foreign nets, signal labels and singleton interfaces remain excluded.
func powerTaggedCorridors(component Component, nets []Net, at kicadfiles.Point) []powerCorridor {
	var result []powerCorridor
	for _, net := range nets {
		shareable := net.LocalWiring && len(net.Endpoints) > 1 && (net.Role == "power" || net.Role == "ground")
		for _, box := range supportOwnerPinCorridors(component, []Net{net}, at) {
			result = append(result, powerCorridor{net: net.Name, box: box, shareable: shareable})
		}
	}
	return result
}

// Prefer the active device as a local tree root; do not force a dirty route or
// change the N-1 logical tree. This uses functional roles, never reference IDs.
func powerRootPriority(ref string, components []Component) int {
	for _, component := range components {
		if component.Ref == ref && containsNormalizedRole(component.Role, "ic", "regulator", "sensor") {
			return 0
		}
	}
	return 1
}

func functionalPowerNet(name string, nets []Net) bool {
	for _, net := range nets {
		if net.Name == name {
			return net.LocalWiring && containsNormalizedRole(net.Role, "power", "power_pos", "power_neg", "ground", "return")
		}
	}
	return false
}

// Resolver-derived body envelopes include pin-label padding. One grid step
// from a real pin may still be inside that envelope, trapping every dogleg and
// the bounded grid search. Escape the complete envelope along the real pin
// direction first; the unchanged scorer validates the full endpoint segment.
func powerPinAccess(anchor, direction kicadfiles.Point, body Rect, grid kicadfiles.IU) kicadfiles.Point {
	access := kicadfiles.Point{X: anchor.X + direction.X, Y: anchor.Y + direction.Y}
	if body.Empty() {
		return access
	}
	switch {
	case direction.X < 0:
		access.X = min(access.X, SnapIU(body.MinX-grid, grid))
	case direction.X > 0:
		access.X = max(access.X, SnapIU(body.MaxX+grid, grid))
	case direction.Y < 0:
		access.Y = min(access.Y, SnapIU(body.MinY-grid, grid))
	case direction.Y > 0:
		access.Y = max(access.Y, SnapIU(body.MaxY+grid, grid))
	}
	return access
}

// Prefer a capacitor's actual rail pin near the owner's rail-pin axis. The
// bounded candidate set is unchanged, and existing body/annotation clearance
// and attachment-side checks still accept or reject each candidate. Ambiguous
// multi-pin rails and non-capacitor support retain the V5 radial ordering.
func powerSupportOffsets(support, owner Component, nets []Net, offsets []kicadfiles.Point) []kicadfiles.Point {
	if !containsNormalizedRole(support.Role, "decoupling_capacitor", "bulk_capacitor", "decoupling") {
		return offsets
	}
	var ownerPin, supportPin Pin
	count := 0
	for _, net := range nets {
		if net.Role != "power" || !net.LocalWiring {
			continue
		}
		var a, b []Pin
		for _, endpoint := range net.Endpoints {
			for _, pin := range owner.Pins {
				if endpoint.Ref == owner.Ref && endpoint.Pin == pin.Number {
					a = append(a, pin)
				}
			}
			for _, pin := range support.Pins {
				if endpoint.Ref == support.Ref && endpoint.Pin == pin.Number {
					b = append(b, pin)
				}
			}
		}
		if len(a) != 1 || len(b) != 1 {
			if len(a) > 0 && len(b) > 0 {
				return offsets
			}
			continue
		}
		ownerPin, supportPin, count = a[0], b[0], count+1
	}
	if count != 1 {
		return offsets
	}
	direction := TransformPoint(ownerPin.Direction, owner.Rotation, owner.Mirror)
	if (direction.X == 0) == (direction.Y == 0) {
		return offsets
	}
	a := TransformPoint(ownerPin.At, owner.Rotation, owner.Mirror)
	b := TransformPoint(supportPin.At, support.Rotation, support.Mirror)
	cost := func(p kicadfiles.Point) kicadfiles.IU {
		alignment := absIU(p.Y + b.Y - a.Y)
		if direction.X == 0 {
			alignment = absIU(p.X + b.X - a.X)
		}
		return absIU(p.X) + absIU(p.Y) + 4*alignment
	}
	result := slices.Clone(offsets)
	slices.SortStableFunc(result, func(a, b kicadfiles.Point) int { return cmp.Compare(cost(a), cost(b)) })
	return result
}
