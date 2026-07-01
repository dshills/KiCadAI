package physicalrules

import (
	"math"

	"kicadai/internal/kicadfiles"
)

type dfmWidthResult struct {
	Measured          bool
	WidthMM           float64
	Method            string
	SampleCount       int
	UnsupportedReason string
}

func dfmEstimatePolygonWidth(polygon dfmPolygon) dfmWidthResult {
	if !polygon.supported() {
		return dfmWidthResult{UnsupportedReason: polygon.UnsupportedReason}
	}
	points := dfmNormalizePolygon(polygon.Points)
	if len(points) < 3 {
		return dfmWidthResult{UnsupportedReason: "polygon has fewer than three distinct points"}
	}
	if dfmSelfIntersects(points) {
		return dfmWidthResult{UnsupportedReason: "self-intersecting polygon is not modeled"}
	}
	if len(points) > dfmWidthMaxEdges {
		return dfmWidthResult{UnsupportedReason: "polygon has too many edges for first-pass width estimation"}
	}
	if width, ok := dfmAxisAlignedRectangleWidthMM(points); ok {
		return dfmMeasuredWidth(width, "axis_aligned_rectangle", 1)
	}
	width, samples, method := dfmNonAdjacentEdgeWidthMM(points)
	if width < 0 || math.IsInf(width, 1) || math.IsNaN(width) {
		return dfmWidthResult{UnsupportedReason: "polygon width could not be estimated conservatively", SampleCount: samples}
	}
	return dfmMeasuredWidth(width, method, samples)
}

const dfmWidthMaxEdges = 256

func dfmMeasuredWidth(width float64, method string, samples int) dfmWidthResult {
	return dfmWidthResult{Measured: true, WidthMM: width, Method: method, SampleCount: samples}
}

func dfmAxisAlignedRectangleWidthMM(points []kicadfiles.Point) (float64, bool) {
	if len(points) != 4 {
		return 0, false
	}
	minX, minY, maxX, maxY, ok := dfmPointBoundsIU(points)
	if !ok {
		return 0, false
	}
	if minX == maxX || minY == maxY {
		return 0, false
	}
	seen := [4]bool{}
	for _, point := range points {
		switch {
		case dfmPointNearIU(point, kicadfiles.Point{X: minX, Y: minY}):
			seen[0] = true
		case dfmPointNearIU(point, kicadfiles.Point{X: maxX, Y: minY}):
			seen[1] = true
		case dfmPointNearIU(point, kicadfiles.Point{X: maxX, Y: maxY}):
			seen[2] = true
		case dfmPointNearIU(point, kicadfiles.Point{X: minX, Y: maxY}):
			seen[3] = true
		default:
			return 0, false
		}
	}
	for _, ok := range seen {
		if !ok {
			return 0, false
		}
	}
	return math.Min(iuToMM(maxX-minX), iuToMM(maxY-minY)), true
}

func dfmNonAdjacentEdgeWidthMM(points []kicadfiles.Point) (float64, int, string) {
	if len(points) == 3 {
		return dfmTriangleWidthMM(points), 3, "triangle_altitude"
	}
	minWidth := math.Inf(1)
	method := "non_adjacent_edge_distance"
	if minX, minY, maxX, maxY, ok := dfmPointBoundsIU(points); ok {
		if boundsWidthMM := math.Min(iuToMM(maxX-minX), iuToMM(maxY-minY)); boundsWidthMM > 0 {
			minWidth = boundsWidthMM
			method = "bounding_box_width"
		}
	}
	segmentBounds := make([]dfmRect, len(points))
	for index := range points {
		segmentBounds[index] = dfmSegmentBounds(points[index], points[(index+1)%len(points)])
	}
	samples := 0
	for i := range points {
		a1 := points[i]
		a2 := points[(i+1)%len(points)]
		aBounds := segmentBounds[i]
		for j := i + 1; j < len(points); j++ {
			if i == j || (i+1)%len(points) == j || i == (j+1)%len(points) {
				continue
			}
			b1 := points[j]
			b2 := points[(j+1)%len(points)]
			if dfmRectDistanceMM(aBounds, segmentBounds[j]) >= minWidth {
				continue
			}
			samples++
			distance := dfmSegmentSegmentDistanceMM(a1, a2, b1, b2)
			if distance >= 0 && distance < minWidth {
				minWidth = distance
				method = "non_adjacent_edge_distance"
			}
		}
	}
	return minWidth, samples, method
}

func dfmPointBoundsIU(points []kicadfiles.Point) (kicadfiles.IU, kicadfiles.IU, kicadfiles.IU, kicadfiles.IU, bool) {
	if len(points) == 0 {
		return 0, 0, 0, 0, false
	}
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	for _, point := range points[1:] {
		if point.X < minX {
			minX = point.X
		}
		if point.X > maxX {
			maxX = point.X
		}
		if point.Y < minY {
			minY = point.Y
		}
		if point.Y > maxY {
			maxY = point.Y
		}
	}
	return minX, minY, maxX, maxY, true
}

func dfmPointNearIU(a, b kicadfiles.Point) bool {
	const tolerance kicadfiles.IU = 1
	return absIU(a.X-b.X) <= tolerance && absIU(a.Y-b.Y) <= tolerance
}

func dfmTriangleWidthMM(points []kicadfiles.Point) float64 {
	minWidth := math.Inf(1)
	for index, point := range points {
		start := points[(index+1)%len(points)]
		end := points[(index+2)%len(points)]
		distance := dfmPointSegmentDistanceMM(point, start, end)
		if distance >= 0 && distance < minWidth {
			minWidth = distance
		}
	}
	return minWidth
}
