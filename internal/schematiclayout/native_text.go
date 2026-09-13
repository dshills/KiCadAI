package schematiclayout

import (
	"strings"
	"unicode/utf8"

	"kicadai/internal/kicadfiles"
)

const NativeAnnotationV2 = "annotation-v2"

// NativeLabelBounds models upright KiCad local labels with explicit bottom
// justification. Both vertical directions draw left of the anchor baseline;
// horizontal justification determines whether text runs up or down.
func NativeLabelBounds(text string, position kicadfiles.Point, rotation kicadfiles.Angle, right bool) Rect {
	lines := strings.Split(text, "\n")
	count := 0
	for _, line := range lines {
		count = max(count, utf8.RuneCountInString(line))
	}
	w, h := kicadfiles.IU(count)*kicadfiles.MM(1.27), kicadfiles.IU(max(1, len(lines)))*kicadfiles.MM(1.27)
	angle := (int(rotation)%360 + 360) % 360
	if angle == 90 || angle == 270 {
		if right {
			return Rect{MinX: position.X - h, MinY: position.Y, MaxX: position.X, MaxY: position.Y + w}
		}
		return Rect{MinX: position.X - h, MinY: position.Y - w, MaxX: position.X, MaxY: position.Y}
	}
	if right {
		return Rect{MinX: position.X - w, MinY: position.Y - h, MaxX: position.X, MaxY: position.Y}
	}
	return Rect{MinX: position.X, MinY: position.Y - h, MaxX: position.X + w, MaxY: position.Y}
}

func NativeFieldBounds(text string, position kicadfiles.Point) Rect {
	box := TextEstimate(text, position, 0, 0)
	return box.Translate(kicadfiles.Point{X: -box.Width() / 2, Y: box.Height() / 2})
}
