package physicalrules

import (
	"math"
	"strings"

	"kicadai/internal/kicadfiles"
)

type dfmGeometryKind string

const (
	dfmGeometryZonePolygon       dfmGeometryKind = "zone_polygon"
	dfmGeometryFilledZonePolygon dfmGeometryKind = "filled_zone_polygon"
	dfmGeometryCopperGraphic     dfmGeometryKind = "copper_graphic"
	dfmGeometryPadAperture       dfmGeometryKind = "pad_aperture"
	dfmGeometryMaskOpening       dfmGeometryKind = "mask_opening"
	dfmGeometryBoardOutline      dfmGeometryKind = "board_outline"
)

type dfmPolygon struct {
	Points            []kicadfiles.Point
	SourcePath        string
	ObjectID          string
	Reference         string
	Layer             kicadfiles.BoardLayer
	NetCode           int
	NetName           string
	Kind              dfmGeometryKind
	UnsupportedReason string
}

type dfmRect struct {
	Valid bool
	MinX  float64
	MinY  float64
	MaxX  float64
	MaxY  float64
}

func (polygon dfmPolygon) supported() bool {
	return strings.TrimSpace(polygon.UnsupportedReason) == ""
}

func dfmNormalizePolygon(points []kicadfiles.Point) []kicadfiles.Point {
	if len(points) == 0 {
		return nil
	}
	out := make([]kicadfiles.Point, 0, len(points))
	for _, point := range points {
		if len(out) > 0 && out[len(out)-1] == point {
			continue
		}
		out = append(out, point)
	}
	if len(out) > 1 && out[0] == out[len(out)-1] {
		out = out[:len(out)-1]
	}
	return out
}

func dfmPolygonAreaMM2(points []kicadfiles.Point) float64 {
	normalized := dfmNormalizePolygon(points)
	return dfmPolygonAreaMM2Normalized(normalized)
}

func dfmPolygonAreaMM2Normalized(normalized []kicadfiles.Point) float64 {
	if len(normalized) < 3 {
		return 0
	}
	sum := 0.0
	for i, current := range normalized {
		next := normalized[(i+1)%len(normalized)]
		sum += float64(current.X)*float64(next.Y) - float64(next.X)*float64(current.Y)
	}
	return sum / 2 / 1_000_000 / 1_000_000
}

func dfmPolygonAbsAreaMM2(points []kicadfiles.Point) float64 {
	return math.Abs(dfmPolygonAreaMM2(points))
}

func dfmNormalizeWinding(points []kicadfiles.Point) []kicadfiles.Point {
	normalized := dfmNormalizePolygon(points)
	if dfmPolygonAreaMM2Normalized(normalized) < 0 {
		for i, j := 0, len(normalized)-1; i < j; i, j = i+1, j-1 {
			normalized[i], normalized[j] = normalized[j], normalized[i]
		}
	}
	return normalized
}

func dfmPolygonBounds(points []kicadfiles.Point) dfmRect {
	normalized := dfmNormalizePolygon(points)
	if len(normalized) == 0 {
		return dfmRect{}
	}
	bounds := dfmRect{Valid: true, MinX: iuToMM(normalized[0].X), MinY: iuToMM(normalized[0].Y), MaxX: iuToMM(normalized[0].X), MaxY: iuToMM(normalized[0].Y)}
	for _, point := range normalized[1:] {
		x := iuToMM(point.X)
		y := iuToMM(point.Y)
		bounds.MinX = math.Min(bounds.MinX, x)
		bounds.MinY = math.Min(bounds.MinY, y)
		bounds.MaxX = math.Max(bounds.MaxX, x)
		bounds.MaxY = math.Max(bounds.MaxY, y)
	}
	return bounds
}

func dfmPointOnSegment(a, b, point kicadfiles.Point) bool {
	return pointOnSegment(a, b, point)
}

func dfmSegmentsIntersect(a1, a2, b1, b2 kicadfiles.Point) bool {
	return segmentsIntersect(a1, a2, b1, b2)
}

func dfmPointInPolygon(points []kicadfiles.Point, point kicadfiles.Point) bool {
	normalized := dfmNormalizePolygon(points)
	if len(normalized) < 3 {
		return false
	}
	return pointInPolygon(normalized, point)
}

func dfmPointSegmentDistanceMM(point, start, end kicadfiles.Point) float64 {
	return pointSegmentDistanceMM(point, start, end)
}

func dfmSegmentSegmentDistanceMM(a1, a2, b1, b2 kicadfiles.Point) float64 {
	return segmentSegmentDistanceMM(a1, a2, b1, b2)
}

func dfmPolygonDistanceMM(a, b []kicadfiles.Point) float64 {
	a = dfmNormalizePolygon(a)
	b = dfmNormalizePolygon(b)
	if len(a) < 3 || len(b) < 3 {
		return math.Inf(1)
	}
	aBounds := dfmPolygonBounds(a)
	bBounds := dfmPolygonBounds(b)
	if dfmRectsOverlap(aBounds, bBounds) {
		for _, point := range a {
			if pointInPolygon(b, point) {
				return 0
			}
		}
		for _, point := range b {
			if pointInPolygon(a, point) {
				return 0
			}
		}
	}
	minDistance := math.Inf(1)
	for i := range a {
		aNext := a[(i+1)%len(a)]
		aBounds := dfmSegmentBounds(a[i], aNext)
		for j := range b {
			bNext := b[(j+1)%len(b)]
			if dfmRectDistanceMM(aBounds, dfmSegmentBounds(b[j], bNext)) >= minDistance {
				continue
			}
			distance := segmentSegmentDistanceMM(a[i], aNext, b[j], bNext)
			if distance < minDistance {
				minDistance = distance
			}
		}
	}
	return minDistance
}

func dfmRectsOverlap(a, b dfmRect) bool {
	if !a.Valid || !b.Valid {
		return false
	}
	return a.MinX <= b.MaxX && a.MaxX >= b.MinX && a.MinY <= b.MaxY && a.MaxY >= b.MinY
}

func dfmSelfIntersects(points []kicadfiles.Point) bool {
	normalized := dfmNormalizePolygon(points)
	if len(normalized) < 4 {
		return false
	}
	for i := range normalized {
		a1 := normalized[i]
		a2 := normalized[(i+1)%len(normalized)]
		for j := i + 1; j < len(normalized); j++ {
			if i == j || (i+1)%len(normalized) == j || i == (j+1)%len(normalized) {
				continue
			}
			b1 := normalized[j]
			b2 := normalized[(j+1)%len(normalized)]
			if !dfmRectsOverlap(dfmSegmentBounds(a1, a2), dfmSegmentBounds(b1, b2)) {
				continue
			}
			if segmentsIntersect(a1, a2, b1, b2) {
				return true
			}
		}
	}
	return false
}

func dfmSegmentBounds(a, b kicadfiles.Point) dfmRect {
	x1 := iuToMM(a.X)
	y1 := iuToMM(a.Y)
	x2 := iuToMM(b.X)
	y2 := iuToMM(b.Y)
	return dfmRect{
		Valid: true,
		MinX:  math.Min(x1, x2),
		MinY:  math.Min(y1, y2),
		MaxX:  math.Max(x1, x2),
		MaxY:  math.Max(y1, y2),
	}
}

func dfmRectDistanceMM(a, b dfmRect) float64 {
	if !a.Valid || !b.Valid {
		return math.Inf(1)
	}
	dx := 0.0
	switch {
	case a.MaxX < b.MinX:
		dx = b.MinX - a.MaxX
	case b.MaxX < a.MinX:
		dx = a.MinX - b.MaxX
	}
	dy := 0.0
	switch {
	case a.MaxY < b.MinY:
		dy = b.MinY - a.MaxY
	case b.MaxY < a.MinY:
		dy = a.MinY - b.MaxY
	}
	return math.Hypot(dx, dy)
}
