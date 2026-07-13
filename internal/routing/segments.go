package routing

import (
	"math"
	"slices"
)

const distanceEpsilonSquared = distanceEpsilon * distanceEpsilon

func BuildSegmentsFromPath(path GridPath, widthMM float64) ([]Segment, Metrics) {
	widthMM = roundMM(widthMM)
	if widthMM <= 0 || math.IsNaN(widthMM) || math.IsInf(widthMM, 0) {
		widthMM = roundMM(DefaultRules().TraceWidthMM)
	}
	if len(path.Coordinates) == len(path.Points) && len(path.Coordinates) > 0 && len(path.LayerNames) != 0 {
		return buildLayeredSegmentsFromPath(path, widthMM)
	}
	return buildSegmentsForPoints(path.Net, path.Layer, path.Points, widthMM, path.SearchNodes)
}

func BuildSegmentsFromPathWithNeckdown(path GridPath, widthMM, neckdownWidthMM, neckdownLengthMM float64) ([]Segment, Metrics) {
	segments, metrics := BuildSegmentsFromPath(path, widthMM)
	if neckdownWidthMM <= 0 || neckdownLengthMM <= 0 || neckdownWidthMM >= widthMM || len(segments) == 0 {
		return segments, metrics
	}
	segments = applyEndpointNeckdown(segments, neckdownWidthMM, neckdownLengthMM)
	metrics.SegmentCount = len(segments)
	return segments, metrics
}

func applyEndpointNeckdown(segments []Segment, widthMM, lengthMM float64) []Segment {
	totalLength := 0.0
	for _, segment := range segments {
		totalLength += pointDistance(segment.Start, segment.End)
	}
	if totalLength <= distanceEpsilon {
		return segments
	}
	startBoundary := min(lengthMM, totalLength)
	endBoundary := max(0.0, totalLength-lengthMM)
	result := make([]Segment, 0, len(segments)*3)
	distance := 0.0
	for _, segment := range segments {
		segmentLength := pointDistance(segment.Start, segment.End)
		if segmentLength <= distanceEpsilon {
			continue
		}
		cuts := []float64{0, segmentLength}
		for _, boundary := range []float64{startBoundary, endBoundary} {
			local := boundary - distance
			if local > distanceEpsilon && local < segmentLength-distanceEpsilon {
				cuts = append(cuts, local)
			}
		}
		slices.Sort(cuts)
		for index := 1; index < len(cuts); index++ {
			fromDistance := cuts[index-1]
			toDistance := cuts[index]
			midpoint := distance + (fromDistance+toDistance)/2
			width := segment.WidthMM
			if midpoint <= startBoundary || midpoint >= endBoundary {
				width = widthMM
			}
			part := segment
			part.Start = interpolateSegmentPoint(segment, fromDistance/segmentLength)
			part.End = interpolateSegmentPoint(segment, toDistance/segmentLength)
			part.WidthMM = roundMM(width)
			if pointDistanceSquared(part.Start, part.End) <= distanceEpsilonSquared {
				continue
			}
			result = append(result, part)
		}
		distance += segmentLength
	}
	return result
}

func interpolateSegmentPoint(segment Segment, fraction float64) Point {
	return roundPoint(Point{
		XMM: segment.Start.XMM + (segment.End.XMM-segment.Start.XMM)*fraction,
		YMM: segment.Start.YMM + (segment.End.YMM-segment.Start.YMM)*fraction,
	})
}

func buildLayeredSegmentsFromPath(path GridPath, widthMM float64) ([]Segment, Metrics) {
	segments := []Segment{}
	totalLength := 0.0
	runStart := 0
	for index := 1; index <= len(path.Coordinates); index++ {
		if index < len(path.Coordinates) && path.Coordinates[index].Layer == path.Coordinates[index-1].Layer {
			continue
		}
		layer := layerNameForPath(path, path.Coordinates[runStart].Layer)
		runSegments, metrics := buildSegmentsForPoints(path.Net, layer, path.Points[runStart:index], widthMM, 0)
		segments = append(segments, runSegments...)
		totalLength += metrics.TotalLengthMM
		if index < len(path.Coordinates) && pointDistanceSquared(path.Points[index-1], path.Points[index]) > distanceEpsilonSquared {
			transitionLayer := layerNameForPath(path, path.Coordinates[index-1].Layer)
			transitionSegments, metrics := buildSegmentsForPoints(path.Net, transitionLayer, path.Points[index-1:index+1], widthMM, 0)
			segments = append(segments, transitionSegments...)
			totalLength += metrics.TotalLengthMM
		}
		runStart = index
	}
	return segments, Metrics{
		SegmentCount:  len(segments),
		TotalLengthMM: roundMM(totalLength),
		SearchNodes:   path.SearchNodes,
	}
}

func layerNameForPath(path GridPath, layerIndex int) string {
	if path.LayerNames != nil {
		if layer, ok := path.LayerNames[layerIndex]; ok && layer != "" {
			return layer
		}
	}
	return path.Layer
}

func buildSegmentsForPoints(netName string, layer string, points []Point, widthMM float64, searchNodes int) ([]Segment, Metrics) {
	points = simplifyPathPoints(points)
	segments := make([]Segment, 0, max(0, len(points)-1))
	totalLength := 0.0
	for index := 1; index < len(points); index++ {
		start := points[index-1]
		end := points[index]
		length := pointDistance(start, end)
		if length <= distanceEpsilon {
			continue
		}
		totalLength += length
		segments = append(segments, Segment{
			Net:     netName,
			Layer:   layer,
			Start:   start,
			End:     end,
			WidthMM: widthMM,
		})
	}
	return segments, Metrics{
		SegmentCount:  len(segments),
		TotalLengthMM: roundMM(totalLength),
		SearchNodes:   searchNodes,
	}
}

func simplifyPathPoints(points []Point) []Point {
	unique := make([]Point, 0, len(points))
	for _, point := range points {
		point = roundPoint(point)
		if len(unique) == 0 || pointDistanceSquared(unique[len(unique)-1], point) > distanceEpsilonSquared {
			unique = append(unique, point)
		}
	}
	if len(unique) <= 2 {
		return unique
	}
	simplified := make([]Point, 1, len(unique))
	simplified[0] = unique[0]
	for index := 1; index < len(unique)-1; index++ {
		previous := simplified[len(simplified)-1]
		current := unique[index]
		next := unique[index+1]
		if collinear(previous, current, next) {
			continue
		}
		simplified = append(simplified, current)
	}
	simplified = append(simplified, unique[len(unique)-1])
	return simplified
}

func collinear(a Point, b Point, c Point) bool {
	abX := b.XMM - a.XMM
	abY := b.YMM - a.YMM
	bcX := c.XMM - b.XMM
	bcY := c.YMM - b.YMM
	scaleSq := math.Max(abX*abX+abY*abY, bcX*bcX+bcY*bcY)
	if scaleSq <= distanceEpsilonSquared {
		return true
	}
	if abX*bcX+abY*bcY <= 0 {
		return false
	}
	cross := abX*bcY - abY*bcX
	return cross*cross <= distanceEpsilonSquared*scaleSq
}

func pointDistance(start Point, end Point) float64 {
	return math.Sqrt(pointDistanceSquared(start, end))
}

func pointDistanceSquared(start Point, end Point) float64 {
	dx := end.XMM - start.XMM
	dy := end.YMM - start.YMM
	return dx*dx + dy*dy
}

func roundPoint(point Point) Point {
	return Point{XMM: roundMM(point.XMM), YMM: roundMM(point.YMM)}
}
