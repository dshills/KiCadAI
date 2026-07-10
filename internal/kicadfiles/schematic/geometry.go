package schematic

import (
	"math"
	"regexp"
	"strconv"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

var embeddedUnitBodyStylePattern = regexp.MustCompile(`_(\d+)_(\d+)$`)

// TransformConnectionAnchor maps a symbol-local physical pin offset to its
// schematic-space position. It follows KiCad's mirror-then-rotate ordering
// used while parsing and writing a symbol instance.
func TransformConnectionAnchor(offset kicadfiles.Point, rotation kicadfiles.Angle, mirror SymbolMirror) kicadfiles.Point {
	switch mirror {
	case SymbolMirrorX:
		offset.Y = -offset.Y
	case SymbolMirrorY:
		offset.X = -offset.X
	}
	if rotation == 0 {
		return offset
	}
	theta := float64(rotation) * math.Pi / 180
	sin, cos := math.Sincos(theta)
	x := float64(offset.X)
	y := float64(offset.Y)
	return kicadfiles.Point{
		X: kicadfiles.IU(math.Round(x*cos - y*sin)),
		Y: kicadfiles.IU(math.Round(x*sin + y*cos)),
	}
}

// CanonicalSymbolTransform selects the representation KiCad itself emits for
// equivalent quarter-turn mirrored transforms. Keeping generated instances in
// this form avoids save-time churn while preserving the physical transform.
func CanonicalSymbolTransform(rotation kicadfiles.Angle, mirror SymbolMirror) (kicadfiles.Angle, SymbolMirror) {
	switch mirror {
	case SymbolMirrorX:
		if rotation == 180 {
			return 0, SymbolMirrorY
		}
	case SymbolMirrorY:
		switch rotation {
		case 90:
			return 270, SymbolMirrorX
		case 180:
			return 0, SymbolMirrorX
		case 270:
			return 90, SymbolMirrorX
		}
	}
	return rotation, mirror
}

type embeddedPinGeometry struct {
	Number    string
	Offset    kicadfiles.Point
	Unit      int
	BodyStyle int
}

type symbolBoundsAccumulator struct {
	initialized bool
	min         kicadfiles.Point
	max         kicadfiles.Point
}

func (bounds *symbolBoundsAccumulator) include(point kicadfiles.Point) {
	if !bounds.initialized {
		bounds.min = point
		bounds.max = point
		bounds.initialized = true
		return
	}
	if point.X < bounds.min.X {
		bounds.min.X = point.X
	}
	if point.Y < bounds.min.Y {
		bounds.min.Y = point.Y
	}
	if point.X > bounds.max.X {
		bounds.max.X = point.X
	}
	if point.Y > bounds.max.Y {
		bounds.max.Y = point.Y
	}
}

func (bounds symbolBoundsAccumulator) result() (SymbolBodyBounds, bool) {
	if !bounds.initialized {
		return SymbolBodyBounds{}, false
	}
	// A line-only symbol has a useful location but no area. Preserve it as a
	// small obstacle so readability checks do not silently discard its geometry.
	if bounds.max.X == bounds.min.X {
		bounds.min.X -= kicadfiles.MM(0.5)
		bounds.max.X += kicadfiles.MM(0.5)
	}
	if bounds.max.Y == bounds.min.Y {
		bounds.min.Y -= kicadfiles.MM(0.5)
		bounds.max.Y += kicadfiles.MM(0.5)
	}
	return SymbolBodyBounds{Min: bounds.min, Max: bounds.max}, true
}

// embeddedSymbolGeometry extracts the actual pin anchors and visible graphics
// bounds from a KiCad embedded library symbol. Unit zero is common geometry;
// the requested unit and body style provide the unit-specific geometry.
func embeddedSymbolGeometry(root sexpr.ParsedNode, targetUnit, targetBodyStyle int) ([]embeddedPinGeometry, SymbolBodyBounds, bool) {
	if targetUnit <= 0 {
		targetUnit = 1
	}
	if targetBodyStyle <= 0 {
		targetBodyStyle = 1
	}
	var pins []embeddedPinGeometry
	var bounds symbolBoundsAccumulator
	var visit func(sexpr.ParsedNode, int, int)
	visit = func(node sexpr.ParsedNode, unit, bodyStyle int) {
		if node.Head() == "symbol" && len(node.Children) > 1 {
			if parsedUnit, parsedStyle, ok := embeddedUnitBodyStyle(node.ListValue(1)); ok {
				unit, bodyStyle = parsedUnit, parsedStyle
			}
		}
		selected := unit == 0 || (unit == targetUnit && (bodyStyle == 0 || bodyStyle == targetBodyStyle))
		if selected {
			switch node.Head() {
			case "pin":
				if number, ok := namedValue(node, "number", 1); ok {
					if offset, ok := namedPoint(node, "at"); ok {
						pins = append(pins, embeddedPinGeometry{Number: number, Offset: offset, Unit: unit, BodyStyle: bodyStyle})
					}
				}
			case "rectangle":
				includeNamedPoint(&bounds, node, "start")
				includeNamedPoint(&bounds, node, "end")
			case "circle":
				if center, centerOK := namedPoint(node, "center"); centerOK {
					if radius, radiusOK := namedFloat(node, "radius", 1); radiusOK {
						bounds.include(kicadfiles.Point{X: center.X - kicadfiles.MM(radius), Y: center.Y - kicadfiles.MM(radius)})
						bounds.include(kicadfiles.Point{X: center.X + kicadfiles.MM(radius), Y: center.Y + kicadfiles.MM(radius)})
					}
				}
			case "polyline":
				if points, ok := namedList(node, "pts"); ok {
					for _, point := range points.ChildrenByHead("xy") {
						if value, ok := pointXY(point); ok {
							bounds.include(value)
						}
					}
				}
			case "arc":
				start, startOK := namedPoint(node, "start")
				mid, midOK := namedPoint(node, "mid")
				end, endOK := namedPoint(node, "end")
				if startOK && midOK && endOK {
					includeArcBounds(&bounds, start, mid, end)
				}
			}
		}
		for _, child := range node.Children {
			if child.IsList {
				visit(child, unit, bodyStyle)
			}
		}
	}
	visit(root, 0, targetBodyStyle)
	bodyBounds, bodyOK := bounds.result()
	return pins, bodyBounds, bodyOK
}

// schematicEmbeddedSymbolGeometry applies the same Y conversion KiCad uses
// while parsing symbol-library coordinates. The raw helper remains available
// for verified built-in templates whose writer contract is intentionally kept
// in their existing coordinate frame.
func schematicEmbeddedSymbolGeometry(root sexpr.ParsedNode, targetUnit, targetBodyStyle int) ([]embeddedPinGeometry, SymbolBodyBounds, bool) {
	pins, bounds, ok := embeddedSymbolGeometry(root, targetUnit, targetBodyStyle)
	for index := range pins {
		pins[index].Offset = schematicLibraryPoint(pins[index].Offset)
	}
	if ok {
		bounds = schematicLibraryBounds(bounds)
	}
	return pins, bounds, ok
}

func schematicLibraryBounds(bounds SymbolBodyBounds) SymbolBodyBounds {
	minPoint, maxPoint := kicadfiles.SchematicLibraryBounds(bounds.Min, bounds.Max)
	return SymbolBodyBounds{
		Min: minPoint,
		Max: maxPoint,
	}
}

// schematicLibraryPoint mirrors KiCad's parseXY(true) behavior for embedded
// library geometry. Schematic coordinates stay in the same frame as wires,
// no-connect markers, and symbol instance positions.
func schematicLibraryPoint(point kicadfiles.Point) kicadfiles.Point {
	return kicadfiles.SchematicLibraryPoint(point)
}

func includeArcBounds(bounds *symbolBoundsAccumulator, start, mid, end kicadfiles.Point) {
	bounds.include(start)
	bounds.include(mid)
	bounds.include(end)
	center, ok := circleCenter(start, mid, end)
	if !ok {
		return
	}
	radius := math.Hypot(float64(start.X-center.X), float64(start.Y-center.Y))
	startAngle := normalizedAngle(start, center)
	midAngle := normalizedAngle(mid, center)
	endAngle := normalizedAngle(end, center)
	ccw := angleOnCCWArc(startAngle, endAngle, midAngle)
	for _, angle := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
		onArc := angleOnCCWArc(startAngle, endAngle, angle)
		if !ccw {
			onArc = angleOnCCWArc(endAngle, startAngle, angle)
		}
		if onArc {
			bounds.include(kicadfiles.Point{
				X: center.X + kicadfiles.IU(math.Round(math.Cos(angle)*radius)),
				Y: center.Y + kicadfiles.IU(math.Round(math.Sin(angle)*radius)),
			})
		}
	}
}

func circleCenter(first, second, third kicadfiles.Point) (kicadfiles.Point, bool) {
	ax, ay := float64(first.X), float64(first.Y)
	bx, by := float64(second.X), float64(second.Y)
	cx, cy := float64(third.X), float64(third.Y)
	determinant := 2 * (ax*(by-cy) + bx*(cy-ay) + cx*(ay-by))
	if math.Abs(determinant) < 1e-9 {
		return kicadfiles.Point{}, false
	}
	firstSquared := ax*ax + ay*ay
	secondSquared := bx*bx + by*by
	thirdSquared := cx*cx + cy*cy
	centerX := (firstSquared*(by-cy) + secondSquared*(cy-ay) + thirdSquared*(ay-by)) / determinant
	centerY := (firstSquared*(cx-bx) + secondSquared*(ax-cx) + thirdSquared*(bx-ax)) / determinant
	return kicadfiles.Point{X: kicadfiles.IU(math.Round(centerX)), Y: kicadfiles.IU(math.Round(centerY))}, true
}

func normalizedAngle(point, center kicadfiles.Point) float64 {
	angle := math.Atan2(float64(point.Y-center.Y), float64(point.X-center.X))
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

func angleOnCCWArc(start, end, target float64) bool {
	const epsilon = 1e-9
	difference := end - start
	if difference < 0 {
		difference += 2 * math.Pi
	}
	targetDifference := target - start
	targetDifference = math.Mod(targetDifference, 2*math.Pi)
	if targetDifference < 0 {
		targetDifference += 2 * math.Pi
	}
	return targetDifference <= difference+epsilon || targetDifference <= epsilon
}

func parsedGeometryNode(node sexpr.Node) sexpr.ParsedNode {
	switch value := node.(type) {
	case sexpr.List:
		children := make([]sexpr.ParsedNode, 0, len(value))
		for _, child := range value {
			children = append(children, parsedGeometryNode(child))
		}
		return sexpr.ParsedNode{Children: children, IsList: true}
	case sexpr.Atom:
		return sexpr.ParsedNode{Atom: string(value)}
	case sexpr.String:
		return sexpr.ParsedNode{String: string(value), Quoted: true}
	case sexpr.Int:
		return sexpr.ParsedNode{Atom: strconv.FormatInt(int64(value), 10)}
	case sexpr.Float:
		return sexpr.ParsedNode{Atom: strconv.FormatFloat(float64(value), 'g', -1, 64)}
	case sexpr.Fixed:
		return sexpr.ParsedNode{Atom: string(value)}
	case sexpr.Number:
		return sexpr.ParsedNode{Atom: string(value)}
	case sexpr.Raw:
		return sexpr.ParsedNode{Atom: string(value)}
	default:
		return sexpr.ParsedNode{}
	}
}

func embeddedUnitBodyStyle(name string) (int, int, bool) {
	matches := embeddedUnitBodyStylePattern.FindStringSubmatch(name)
	if len(matches) != 3 {
		return 0, 0, false
	}
	unit, unitErr := strconv.Atoi(matches[1])
	bodyStyle, bodyErr := strconv.Atoi(matches[2])
	if unitErr != nil || bodyErr != nil {
		return 0, 0, false
	}
	return unit, bodyStyle, true
}

func namedList(node sexpr.ParsedNode, name string) (sexpr.ParsedNode, bool) {
	child, ok := node.Child(name)
	return child, ok && child.IsList
}

func namedValue(node sexpr.ParsedNode, name string, index int) (string, bool) {
	child, ok := node.Child(name)
	if !ok || child.ListValue(index) == "" {
		return "", false
	}
	return child.ListValue(index), true
}

func namedFloat(node sexpr.ParsedNode, name string, index int) (float64, bool) {
	child, ok := node.Child(name)
	if !ok {
		return 0, false
	}
	return child.FloatValue(index)
}

func namedPoint(node sexpr.ParsedNode, name string) (kicadfiles.Point, bool) {
	child, ok := node.Child(name)
	if !ok {
		return kicadfiles.Point{}, false
	}
	x, xOK := child.FloatValue(1)
	y, yOK := child.FloatValue(2)
	if !xOK || !yOK {
		return kicadfiles.Point{}, false
	}
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}, true
}

func pointXY(node sexpr.ParsedNode) (kicadfiles.Point, bool) {
	x, xOK := node.FloatValue(1)
	y, yOK := node.FloatValue(2)
	if !xOK || !yOK {
		return kicadfiles.Point{}, false
	}
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}, true
}

func includeNamedPoint(bounds *symbolBoundsAccumulator, node sexpr.ParsedNode, name string) {
	if point, ok := namedPoint(node, name); ok {
		bounds.include(point)
	}
}
