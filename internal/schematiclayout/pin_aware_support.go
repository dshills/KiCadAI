package schematiclayout

import "kicadai/internal/kicadfiles"

// supportAttachmentSide follows actual shared owner pins, never a reference or
// component-ID substring. Signal/bias attachments outrank a shared supply rail;
// ground cannot choose a side. Conflicting pin sides remain explicitly unknown.
func supportAttachmentSide(support string, owner Component, nets []Net) string {
	bestPriority, side := -1, ""
	for _, net := range nets {
		if net.Role == "ground" || net.Role == "no_connect" {
			continue
		}
		connected := false
		for _, e := range net.Endpoints {
			connected = connected || e.Ref == support
		}
		if !connected {
			continue
		}
		priority := 1
		if net.Role == "power" {
			priority = 0
		}
		for _, e := range net.Endpoints {
			if e.Ref != owner.Ref || priority < bestPriority {
				continue
			}
			candidate := ""
			for _, pin := range owner.Pins {
				if pin.Number == e.Pin {
					p := TransformPoint(pin.At, owner.Rotation, owner.Mirror)
					body := componentBoundsAt(owner, kicadfiles.Point{})
					x, y := p.X-(body.MinX+body.MaxX)/2, p.Y-(body.MinY+body.MaxY)/2
					// Resolver pin direction identifies the actual body side even
					// for a left-facing pin near the top of a tall IC.
					if pin.Direction != (kicadfiles.Point{}) {
						direction := TransformPoint(pin.Direction, owner.Rotation, owner.Mirror)
						x, y = direction.X, direction.Y
					}
					switch {
					case absIU(x) > absIU(y) && x < 0:
						candidate = "left"
					case absIU(x) > absIU(y):
						candidate = "right"
					case absIU(y) > absIU(x) && y < 0:
						candidate = "top"
					case absIU(y) > absIU(x):
						candidate = "bottom"
					}
				}
			}
			if priority > bestPriority {
				bestPriority, side = priority, candidate
			} else if side != candidate {
				side = "ambiguous"
			}
		}
	}
	return side
}

func supportOnAttachmentSide(support, owner Rect, side string) bool {
	switch side {
	case "left":
		return support.MaxX < owner.MinX
	case "right":
		return support.MinX > owner.MaxX
	case "top":
		return support.MaxY < owner.MinY
	case "bottom":
		return support.MinY > owner.MaxY
	default:
		return true
	}
}

// Reserve the owner's outward label corridors using actual net names and pin
// direction. This is drawing clearance, not an inferred electrical function.
func supportOwnerPinCorridors(owner Component, nets []Net, origin kicadfiles.Point) []Rect {
	var corridors []Rect
	for _, net := range nets {
		if net.Role == "no_connect" || net.Name == "" {
			continue
		}
		for _, endpoint := range net.Endpoints {
			if endpoint.Ref != owner.Ref {
				continue
			}
			for _, pin := range owner.Pins {
				if pin.Number != endpoint.Pin || pin.Direction == (kicadfiles.Point{}) {
					continue
				}
				d := TransformPoint(pin.Direction, owner.Rotation, owner.Mirror)
				p := TransformPoint(pin.At, owner.Rotation, owner.Mirror)
				p.X += origin.X
				p.Y += origin.Y
				length := NativeFieldBounds(net.Name, kicadfiles.Point{}).Width() + kicadfiles.MM(10.16)
				q := p
				switch {
				case absIU(d.X) > absIU(d.Y) && d.X < 0:
					q.X -= length
				case absIU(d.X) > absIU(d.Y):
					q.X += length
				case d.Y < 0:
					q.Y -= length
				default:
					q.Y += length
				}
				corridors = append(corridors, Rect{MinX: min(p.X, q.X), MinY: min(p.Y, q.Y), MaxX: max(p.X, q.X), MaxY: max(p.Y, q.Y)}.Inflate(kicadfiles.MM(2.54)))
			}
		}
	}
	return corridors
}
