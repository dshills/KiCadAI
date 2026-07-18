package kicadfiles

import "math"

const boardRotationEpsilonDegrees = 1e-9

// RotateBoardLocalXY applies KiCad's footprint rotation convention to a local
// board-space vector. Positive footprint angles are counter-clockwise on the
// canvas, whose serialized Y coordinates increase downward; therefore +90
// degrees maps local +X to board -Y.
func RotateBoardLocalXY(x, y, degrees float64) (float64, float64) {
	normalized := math.Mod(degrees, 360)
	if normalized < 0 {
		normalized += 360
	}
	switch {
	case math.Abs(normalized) < boardRotationEpsilonDegrees || math.Abs(normalized-360) < boardRotationEpsilonDegrees:
		return x, y
	case math.Abs(normalized-90) < boardRotationEpsilonDegrees:
		return y, -x
	case math.Abs(normalized-180) < boardRotationEpsilonDegrees:
		return -x, -y
	case math.Abs(normalized-270) < boardRotationEpsilonDegrees:
		return -y, x
	default:
		sine, cosine := math.Sincos(normalized * math.Pi / 180)
		return x*cosine + y*sine, -x*sine + y*cosine
	}
}
