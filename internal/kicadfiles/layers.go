package kicadfiles

import (
	"math"
	"strings"
)

// DefaultFootprintPropertyPosition returns side-independent footprint-local positions for KiCad's generated properties.
func DefaultFootprintPropertyPosition(name string) Point {
	switch name {
	case "Reference":
		return Point{Y: MM(-1.5)}
	case "Value":
		return Point{Y: MM(1.5)}
	default:
		return Point{}
	}
}

// BoardLayerForPlacement returns the equivalent board layer for a footprint placement side.
func BoardLayerForPlacement(layer BoardLayer, placementLayer BoardLayer) BoardLayer {
	if placementLayer != LayerBCu {
		return layer
	}
	switch layer {
	case LayerFCu:
		return LayerBCu
	case LayerFMask:
		return LayerBMask
	case LayerFPaste:
		return LayerBPaste
	case LayerFAdhes:
		return LayerBAdhes
	case LayerFSilkS:
		return LayerBSilkS
	case LayerFFab:
		return LayerBFab
	case LayerFCrtYd:
		return LayerBCrtYd
	default:
		return layer
	}
}

// BoardTextJustifyForPlacement applies KiCad's side transform to footprint
// text justification. Flipping a footprint toggles the mirror token; applying
// the function a second time restores canonical library justification.
func BoardTextJustifyForPlacement(justify []string, placementLayer BoardLayer) []string {
	result := append([]string(nil), justify...)
	if placementLayer != LayerBCu {
		return result
	}
	filtered := result[:0]
	foundMirror := false
	for _, value := range result {
		if strings.EqualFold(strings.TrimSpace(value), "mirror") {
			foundMirror = true
			continue
		}
		filtered = append(filtered, value)
	}
	if foundMirror {
		return filtered
	}
	return append(filtered, "mirror")
}

// BoardLocalPointForPlacement applies KiCad's top-to-bottom footprint flip to
// a footprint-local point. KiCad's TOP_BOTTOM flip reflects the local Y axis
// while retaining the footprint instance rotation.
func BoardLocalPointForPlacement(point Point, placementLayer BoardLayer) Point {
	if placementLayer == LayerBCu {
		point.Y = -point.Y
	}
	return point
}

// BoardLocalXYForPlacement is the floating-point form of
// BoardLocalPointForPlacement used by placement and routing models.
func BoardLocalXYForPlacement(x, y float64, placementLayer BoardLayer) (float64, float64) {
	if placementLayer == LayerBCu {
		return x, -y
	}
	return x, y
}

// BoardLocalAngleForPlacement reflects a footprint-local geometric angle for
// a bottom-side placement. Applying it twice restores the canonical angle.
func BoardLocalAngleForPlacement(angle Angle, placementLayer BoardLayer) Angle {
	if placementLayer != LayerBCu {
		return angle
	}
	return normalizeBoardAngle(-angle)
}

// BoardTextAngleForPlacement mirrors a text angle using KiCad's upright text
// convention for bottom-side footprint fields and text.
func BoardTextAngleForPlacement(angle Angle, placementLayer BoardLayer) Angle {
	if placementLayer != LayerBCu {
		return angle
	}
	return normalizeBoardAngle(180 - angle)
}

func normalizeBoardAngle(angle Angle) Angle {
	value := math.Mod(float64(angle), 360)
	if value < 0 {
		value += 360
	}
	if math.Abs(value) < boardRotationEpsilonDegrees || math.Abs(value-360) < boardRotationEpsilonDegrees {
		return 0
	}
	return Angle(value)
}
