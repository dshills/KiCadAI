package routing

import "math"

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
