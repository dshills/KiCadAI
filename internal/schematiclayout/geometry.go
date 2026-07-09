package schematiclayout

import (
	"math"
	"strings"
	"unicode/utf8"

	"kicadai/internal/kicadfiles"
)

func (rect Rect) Empty() bool {
	return rect.MaxX <= rect.MinX || rect.MaxY <= rect.MinY
}

func (rect Rect) Width() kicadfiles.IU {
	return rect.MaxX - rect.MinX
}

func (rect Rect) Height() kicadfiles.IU {
	return rect.MaxY - rect.MinY
}

func (rect Rect) Translate(delta kicadfiles.Point) Rect {
	return Rect{
		MinX: rect.MinX + delta.X,
		MinY: rect.MinY + delta.Y,
		MaxX: rect.MaxX + delta.X,
		MaxY: rect.MaxY + delta.Y,
	}
}

func (rect Rect) Inflate(amount kicadfiles.IU) Rect {
	return Rect{
		MinX: rect.MinX - amount,
		MinY: rect.MinY - amount,
		MaxX: rect.MaxX + amount,
		MaxY: rect.MaxY + amount,
	}
}

func (rect Rect) Intersects(other Rect) bool {
	if rect.Empty() || other.Empty() {
		return false
	}
	return rect.MinX < other.MaxX && rect.MaxX > other.MinX && rect.MinY < other.MaxY && rect.MaxY > other.MinY
}

func (rect Rect) ContainsPoint(point kicadfiles.Point) bool {
	return point.X >= rect.MinX && point.X <= rect.MaxX && point.Y >= rect.MinY && point.Y <= rect.MaxY
}

func (rect Rect) ContainsRect(other Rect) bool {
	return !other.Empty() &&
		other.MinX >= rect.MinX &&
		other.MaxX <= rect.MaxX &&
		other.MinY >= rect.MinY &&
		other.MaxY <= rect.MaxY
}

func SegmentBounds(segment WireSegment) Rect {
	minX, maxX := orderedIU(segment.From.X, segment.To.X)
	minY, maxY := orderedIU(segment.From.Y, segment.To.Y)
	return Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
}

func SegmentIntersectsRect(segment WireSegment, rect Rect) bool {
	if rect.Empty() {
		return false
	}
	if rect.ContainsPoint(segment.From) || rect.ContainsPoint(segment.To) {
		return true
	}
	if segment.From.X == segment.To.X {
		x := segment.From.X
		if x < rect.MinX || x > rect.MaxX {
			return false
		}
		minY, maxY := orderedIU(segment.From.Y, segment.To.Y)
		return maxY >= rect.MinY && minY <= rect.MaxY
	}
	if segment.From.Y == segment.To.Y {
		y := segment.From.Y
		if y < rect.MinY || y > rect.MaxY {
			return false
		}
		minX, maxX := orderedIU(segment.From.X, segment.To.X)
		return maxX >= rect.MinX && minX <= rect.MaxX
	}
	topLeft := kicadfiles.Point{X: rect.MinX, Y: rect.MinY}
	topRight := kicadfiles.Point{X: rect.MaxX, Y: rect.MinY}
	bottomRight := kicadfiles.Point{X: rect.MaxX, Y: rect.MaxY}
	bottomLeft := kicadfiles.Point{X: rect.MinX, Y: rect.MaxY}
	return segmentsIntersect(segment.From, segment.To, topLeft, topRight) ||
		segmentsIntersect(segment.From, segment.To, topRight, bottomRight) ||
		segmentsIntersect(segment.From, segment.To, bottomRight, bottomLeft) ||
		segmentsIntersect(segment.From, segment.To, bottomLeft, topLeft)
}

// TextEstimate returns a conservative box for horizontal, left-anchored text.
// More precise KiCad justification/orientation handling belongs in the later
// property-placement pass; this validator uses the estimate to catch obvious
// overlaps without pretending to be a font renderer.
func TextEstimate(text string, position kicadfiles.Point, charWidth, height kicadfiles.IU) Rect {
	if charWidth <= 0 {
		charWidth = kicadfiles.MM(1.27)
	}
	if height <= 0 {
		height = kicadfiles.MM(1.27)
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	maxRunes := 0
	for _, line := range lines {
		if count := utf8.RuneCountInString(line); count > maxRunes {
			maxRunes = count
		}
	}
	width := kicadfiles.IU(maxRunes) * charWidth
	totalHeight := kicadfiles.IU(len(lines)) * height
	return Rect{
		MinX: position.X,
		MinY: position.Y - totalHeight,
		MaxX: position.X + width,
		MaxY: position.Y,
	}
}

func UsableSheet(sheet Sheet) Rect {
	width := sheet.Width
	height := sheet.Height
	if width <= 0 {
		width = kicadfiles.MM(297)
	}
	if height <= 0 {
		height = kicadfiles.MM(210)
	}
	margin := sheet.Margin
	if margin <= 0 {
		margin = kicadfiles.MM(10.16)
	}
	if margin*2 > width {
		margin = width / 2
	}
	if margin*2 > height {
		margin = height / 2
	}
	return Rect{MinX: margin, MinY: margin, MaxX: width - margin, MaxY: height - margin}
}

func componentBody(component PlacedComponent) Rect {
	body := component.Body
	if body.Empty() {
		body = DefaultBodyFor(component)
	}
	body = RotateRect(body, component.Rotation)
	return body.Translate(component.PlacedAt)
}

// RotatePoint rotates a symbol-local point around the symbol origin using the
// same clockwise-on-page coordinate convention used by KiCad's Y-down canvas.
func RotatePoint(point kicadfiles.Point, angle kicadfiles.Angle) kicadfiles.Point {
	if angle == 0 {
		return point
	}
	theta := float64(angle) * math.Pi / 180
	sin, cos := math.Sincos(theta)
	x := float64(point.X)
	y := float64(point.Y)
	return kicadfiles.Point{
		X: kicadfiles.IU(math.Round(x*cos - y*sin)),
		Y: kicadfiles.IU(math.Round(x*sin + y*cos)),
	}
}

// RotateRect returns the axis-aligned bounds of a rotated symbol-local box.
func RotateRect(rect Rect, angle kicadfiles.Angle) Rect {
	if rect.Empty() || angle == 0 {
		return rect
	}
	points := []kicadfiles.Point{
		{X: rect.MinX, Y: rect.MinY},
		{X: rect.MaxX, Y: rect.MinY},
		{X: rect.MaxX, Y: rect.MaxY},
		{X: rect.MinX, Y: rect.MaxY},
	}
	first := RotatePoint(points[0], angle)
	result := Rect{MinX: first.X, MinY: first.Y, MaxX: first.X, MaxY: first.Y}
	for _, point := range points[1:] {
		rotated := RotatePoint(point, angle)
		result.MinX = minIU(result.MinX, rotated.X)
		result.MinY = minIU(result.MinY, rotated.Y)
		result.MaxX = maxIU(result.MaxX, rotated.X)
		result.MaxY = maxIU(result.MaxY, rotated.Y)
	}
	return result
}

func DefaultBodyFor(component PlacedComponent) Rect {
	switch strings.ToLower(strings.TrimSpace(component.LibraryID)) {
	case libraryIDGenericI2C, libraryIDGenericI2C8P:
		return Rect{MinX: kicadfiles.MM(-2.54), MinY: kicadfiles.MM(-3.81), MaxX: kicadfiles.MM(2.54), MaxY: kicadfiles.MM(7.62)}
	}
	width := kicadfiles.MM(10.16)
	height := kicadfiles.MM(7.62)
	role := normalizeRole(component.Role)
	switch {
	case containsNormalizedRole(role, "connector"):
		width = kicadfiles.MM(7.62)
		height = kicadfiles.MM(15.24)
	case containsNormalizedRole(role, "mcu"), containsNormalizedRole(role, "controller"):
		width = kicadfiles.MM(25.4)
		height = kicadfiles.MM(25.4)
	case containsNormalizedRole(role, "opamp"), containsNormalizedRole(role, "op_amp"):
		width = kicadfiles.MM(15.24)
		height = kicadfiles.MM(12.7)
	case containsNormalizedRole(role, "resistor"), containsNormalizedRole(role, "capacitor"), containsNormalizedRole(role, "diode"), containsNormalizedRole(role, "passive"):
		width = kicadfiles.MM(7.62)
		height = kicadfiles.MM(5.08)
	}
	return Rect{MinX: -width / 2, MinY: -height / 2, MaxX: width / 2, MaxY: height / 2}
}

const (
	libraryIDGenericI2C   = "sensor:generic_i2c"
	libraryIDGenericI2C8P = "sensor:generic_i2c_8p"
)

func orderedIU(first, second kicadfiles.IU) (kicadfiles.IU, kicadfiles.IU) {
	if first <= second {
		return first, second
	}
	return second, first
}

func absIU(value kicadfiles.IU) kicadfiles.IU {
	if value < 0 {
		return -value
	}
	return value
}

func segmentsIntersect(a, b, c, d kicadfiles.Point) bool {
	o1 := orientation(a, b, c)
	o2 := orientation(a, b, d)
	o3 := orientation(c, d, a)
	o4 := orientation(c, d, b)
	if o1 != o2 && o3 != o4 {
		return true
	}
	return o1 == 0 && pointOnSegment(a, c, b) ||
		o2 == 0 && pointOnSegment(a, d, b) ||
		o3 == 0 && pointOnSegment(c, a, d) ||
		o4 == 0 && pointOnSegment(c, b, d)
}

func orientation(a, b, c kicadfiles.Point) int {
	value := int64(b.Y-a.Y)*int64(c.X-b.X) - int64(b.X-a.X)*int64(c.Y-b.Y)
	if value == 0 {
		return 0
	}
	if value > 0 {
		return 1
	}
	return 2
}

func pointOnSegment(a, b, c kicadfiles.Point) bool {
	minX, maxX := orderedIU(a.X, c.X)
	minY, maxY := orderedIU(a.Y, c.Y)
	return b.X >= minX && b.X <= maxX && b.Y >= minY && b.Y <= maxY
}
