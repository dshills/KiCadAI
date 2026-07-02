package physicalrules

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
)

func dfmZonePolygons(board *pcbfiles.PCBFile) []dfmPolygon {
	if board == nil {
		return nil
	}
	out := make([]dfmPolygon, 0, len(board.Zones)*4)
	for zoneIndex := range board.Zones {
		zone := &board.Zones[zoneIndex]
		layers := dfmCopperLayers(zone.Layers)
		for polygonIndex, points := range zone.Polygons {
			if len(layers) == 0 {
				polygon := dfmPolygon{
					Points:     slices.Clone(points),
					SourcePath: fmt.Sprintf("zones[%d].polygons[%d]", zoneIndex, polygonIndex),
					ObjectID:   string(zone.UUID),
					NetCode:    zone.NetCode,
					NetName:    zone.NetName,
					Kind:       dfmGeometryZonePolygon,
				}
				polygon.UnsupportedReason = "zone does not declare a copper layer"
				out = append(out, dfmValidatedPolygon(polygon))
				continue
			}
			for _, layer := range layers {
				out = append(out, dfmValidatedPolygon(dfmPolygon{
					Points:     slices.Clone(points),
					SourcePath: fmt.Sprintf("zones[%d].polygons[%d][%s]", zoneIndex, polygonIndex, layer),
					ObjectID:   string(zone.UUID),
					Layer:      layer,
					NetCode:    zone.NetCode,
					NetName:    zone.NetName,
					Kind:       dfmGeometryZonePolygon,
				}))
			}
		}
		for polygonIndex, filled := range zone.FilledPolygons {
			polygon := dfmPolygon{
				Points:     slices.Clone(filled.Points),
				SourcePath: fmt.Sprintf("zones[%d].filled_polygons[%d]", zoneIndex, polygonIndex),
				ObjectID:   string(zone.UUID),
				Layer:      filled.Layer,
				NetCode:    zone.NetCode,
				NetName:    zone.NetName,
				Kind:       dfmGeometryFilledZonePolygon,
			}
			if !isCopperLayer(filled.Layer) {
				polygon.UnsupportedReason = "filled zone polygon is not on a copper layer"
			}
			out = append(out, dfmValidatedPolygon(polygon))
		}
	}
	return dfmSortPolygons(out)
}

func dfmCopperGraphicPolygons(board *pcbfiles.PCBFile) []dfmPolygon {
	if board == nil {
		return nil
	}
	out := make([]dfmPolygon, 0, dfmCopperGraphicObservationCapacity(board))
	for index := range board.Drawings {
		drawing := &board.Drawings[index]
		if !isCopperLayer(drawing.Layer) {
			continue
		}
		if drawing.Poly == nil {
			out = append(out, dfmValidatedPolygon(dfmPolygon{
				SourcePath:        fmt.Sprintf("drawings[%d]", index),
				ObjectID:          string(drawing.UUID),
				Layer:             drawing.Layer,
				NetCode:           drawing.NetCode,
				NetName:           drawing.NetName,
				Kind:              dfmGeometryCopperGraphic,
				UnsupportedReason: "copper graphic is not polygon geometry",
			}))
			continue
		}
		out = append(out, dfmValidatedPolygon(dfmPolygon{
			Points:     slices.Clone(drawing.Poly.Points),
			SourcePath: fmt.Sprintf("drawings[%d].poly", index),
			ObjectID:   string(drawing.UUID),
			Layer:      drawing.Layer,
			NetCode:    drawing.NetCode,
			NetName:    drawing.NetName,
			Kind:       dfmGeometryCopperGraphic,
		}))
	}
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		transform := footprintTransform(footprint)
		for graphicIndex := range footprint.Graphics {
			graphic := &footprint.Graphics[graphicIndex]
			if !isCopperLayer(graphic.Layer) {
				continue
			}
			if graphic.Poly == nil {
				out = append(out, dfmValidatedPolygon(dfmPolygon{
					SourcePath:        fmt.Sprintf("footprints[%d].graphics[%d]", footprintIndex, graphicIndex),
					ObjectID:          string(graphic.UUID),
					Reference:         footprint.Reference,
					Layer:             graphic.Layer,
					Kind:              dfmGeometryCopperGraphic,
					UnsupportedReason: "footprint copper graphic is not polygon geometry",
				}))
				continue
			}
			out = append(out, dfmValidatedPolygon(dfmPolygon{
				Points:     transformFootprintPointsWith(footprint, transform, graphic.Poly.Points),
				SourcePath: fmt.Sprintf("footprints[%d].graphics[%d].poly", footprintIndex, graphicIndex),
				ObjectID:   string(graphic.UUID),
				Reference:  footprint.Reference,
				Layer:      graphic.Layer,
				Kind:       dfmGeometryCopperGraphic,
			}))
		}
	}
	return dfmSortPolygons(out)
}

func dfmCopperGraphicObservationCapacity(board *pcbfiles.PCBFile) int {
	if board == nil {
		return 0
	}
	count := 0
	for _, drawing := range board.Drawings {
		if isCopperLayer(drawing.Layer) {
			count++
		}
	}
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		for _, graphic := range footprint.Graphics {
			if isCopperLayer(graphic.Layer) {
				count++
			}
		}
	}
	return count
}

func dfmBoardOutlinePolygons(board *pcbfiles.PCBFile, bounds boardBounds) []dfmPolygon {
	if board == nil {
		return nil
	}
	var out []dfmPolygon
	for index, points := range outlinePolygons(board, bounds) {
		out = append(out, dfmValidatedPolygon(dfmPolygon{
			Points:     points,
			SourcePath: fmt.Sprintf("board_outline[%d]", index),
			Layer:      kicadfiles.LayerEdge,
			Kind:       dfmGeometryBoardOutline,
		}))
	}
	return dfmSortPolygons(out)
}

func dfmMaskOpenings(board *pcbfiles.PCBFile, expansion kicadfiles.IU) []dfmPolygon {
	if board == nil {
		return nil
	}
	var out []dfmPolygon
	rotationCache := map[int64]dfmRotation2D{}
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		transform := footprintTransform(footprint)
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			padRotation := dfmCachedRotation(rotationCache, pad.Rotation)
			for _, layer := range padMaskLayers(pad) {
				polygon := dfmPadMaskOpening(footprint, transform, padRotation, pad, layer, expansion)
				polygon.SourcePath = fmt.Sprintf("footprints[%d].pads[%d].mask_opening[%s]", footprintIndex, padIndex, layer)
				polygon.ObjectID = string(pad.UUID)
				polygon.Reference = footprint.Reference
				polygon.Layer = layer
				polygon.NetCode = pad.NetCode
				polygon.NetName = pad.NetName
				polygon.Kind = dfmGeometryMaskOpening
				out = append(out, dfmValidatedPolygon(polygon))
			}
		}
	}
	return dfmSortPolygons(out)
}

func dfmPadMaskOpening(footprint *pcbfiles.Footprint, transform transform2D, padRotation dfmRotation2D, pad *pcbfiles.Pad, layer kicadfiles.BoardLayer, expansion kicadfiles.IU) dfmPolygon {
	if pad.Size.X <= 0 || pad.Size.Y <= 0 {
		return dfmPolygon{UnsupportedReason: "pad size is missing"}
	}
	padWidth := pad.Size.X
	padHeight := pad.Size.Y
	shape := strings.ToLower(strings.TrimSpace(pad.Shape))
	switch shape {
	case "", "rect", "roundrect", "oval", "oblong":
	case "circle":
		if padWidth != padHeight {
			return dfmPolygon{UnsupportedReason: "circle pad has non-uniform size"}
		}
	default:
		return dfmPolygon{UnsupportedReason: "pad shape is not modeled for mask opening"}
	}
	width := padWidth + 2*expansion
	height := padHeight + 2*expansion
	if width <= 0 || height <= 0 {
		return dfmPolygon{UnsupportedReason: "pad mask opening collapsed after expansion"}
	}
	center := transformFootprintPointWith(footprint, transform, pad.Position)
	var points []kicadfiles.Point
	if shape == "circle" || shape == "oval" || shape == "oblong" {
		points = dfmRoundedEndOpeningPoints(kicadfiles.Point{}, width, height)
	} else if shape == "roundrect" {
		cornerRadius := 0.0
		if pad.RoundRectRRatio > 0 {
			cornerRadius = math.Min(float64(padWidth), float64(padHeight))*pad.RoundRectRRatio + float64(expansion)
		}
		points = dfmRoundedRectOpeningPoints(kicadfiles.Point{}, width, height, cornerRadius)
	} else {
		points = dfmRectangleOpeningPoints(kicadfiles.Point{}, width, height)
	}
	return dfmPolygon{Points: dfmTransformPadOpeningPoints(center, transform, padRotation, points)}
}

func dfmRectangleOpeningPoints(center kicadfiles.Point, width, height kicadfiles.IU) []kicadfiles.Point {
	left := width / 2
	right := width - left
	top := height / 2
	bottom := height - top
	return []kicadfiles.Point{
		{X: center.X - left, Y: center.Y - top},
		{X: center.X + right, Y: center.Y - top},
		{X: center.X + right, Y: center.Y + bottom},
		{X: center.X - left, Y: center.Y + bottom},
	}
}

type dfmRotation2D struct {
	Cosine float64
	Sine   float64
}

func dfmCachedRotation(cache map[int64]dfmRotation2D, angle kicadfiles.Angle) dfmRotation2D {
	key := int64(math.Round(float64(angle) * 1_000_000))
	if rotation, ok := cache[key]; ok {
		return rotation
	}
	radians := float64(angle) * math.Pi / 180
	rotation := dfmRotation2D{Cosine: math.Cos(radians), Sine: math.Sin(radians)}
	cache[key] = rotation
	return rotation
}

func dfmTransformPadOpeningPoints(center kicadfiles.Point, footprintTransform transform2D, padRotation dfmRotation2D, points []kicadfiles.Point) []kicadfiles.Point {
	out := make([]kicadfiles.Point, 0, len(points))
	for _, point := range points {
		rotated := kicadfiles.Point{
			X: dfmRoundIU(float64(point.X)*padRotation.Cosine - float64(point.Y)*padRotation.Sine),
			Y: dfmRoundIU(float64(point.X)*padRotation.Sine + float64(point.Y)*padRotation.Cosine),
		}
		offset := transformedOffset(footprintTransform, rotated)
		out = append(out, kicadfiles.Point{X: center.X + offset.X, Y: center.Y + offset.Y})
	}
	return out
}

func dfmRoundedEndOpeningPoints(center kicadfiles.Point, width, height kicadfiles.IU) []kicadfiles.Point {
	const arcSegments = 16
	widthIU := float64(width)
	heightIU := float64(height)
	if width <= 0 || height <= 0 {
		return nil
	}
	if width == height {
		points := make([]kicadfiles.Point, 0, arcSegments*2+1)
		radius := widthIU / 2
		for index := 0; index <= arcSegments*2; index++ {
			angle := 2 * math.Pi * float64(index) / float64(arcSegments*2)
			point := kicadfiles.Point{
				X: dfmRoundIU(float64(center.X) + radius*math.Cos(angle)),
				Y: dfmRoundIU(float64(center.Y) + radius*math.Sin(angle)),
			}
			if len(points) == 0 || points[len(points)-1] != point {
				points = append(points, point)
			}
		}
		return points
	}
	points := make([]kicadfiles.Point, 0, arcSegments*2+6)
	appendOffset := func(x, y float64) {
		point := kicadfiles.Point{
			X: dfmRoundIU(float64(center.X) + x),
			Y: dfmRoundIU(float64(center.Y) + y),
		}
		if len(points) == 0 || points[len(points)-1] != point {
			points = append(points, point)
		}
	}
	if width > height {
		radius := heightIU / 2
		arcOffset := (widthIU - heightIU) / 2
		appendOffset(-arcOffset, -radius)
		appendOffset(arcOffset, -radius)
		for index := 0; index <= arcSegments; index++ {
			angle := -math.Pi/2 + math.Pi*float64(index)/float64(arcSegments)
			appendOffset(arcOffset+radius*math.Cos(angle), radius*math.Sin(angle))
		}
		appendOffset(-arcOffset, radius)
		for index := 0; index <= arcSegments; index++ {
			angle := math.Pi/2 + math.Pi*float64(index)/float64(arcSegments)
			appendOffset(-arcOffset+radius*math.Cos(angle), radius*math.Sin(angle))
		}
		return points
	}
	radius := widthIU / 2
	arcOffset := (heightIU - widthIU) / 2
	appendOffset(radius, -arcOffset)
	appendOffset(radius, arcOffset)
	for index := 0; index <= arcSegments; index++ {
		angle := math.Pi * float64(index) / float64(arcSegments)
		appendOffset(radius*math.Cos(angle), arcOffset+radius*math.Sin(angle))
	}
	appendOffset(-radius, -arcOffset)
	for index := 0; index <= arcSegments; index++ {
		angle := math.Pi + math.Pi*float64(index)/float64(arcSegments)
		appendOffset(radius*math.Cos(angle), -arcOffset+radius*math.Sin(angle))
	}
	return points
}

func dfmRoundedRectOpeningPoints(center kicadfiles.Point, width, height kicadfiles.IU, cornerRadius float64) []kicadfiles.Point {
	const cornerSegments = 6
	if width <= 0 || height <= 0 {
		return nil
	}
	widthF := float64(width)
	heightF := float64(height)
	radius := math.Min(cornerRadius, math.Min(widthF, heightF)/2)
	if radius <= 0 {
		return dfmRectangleOpeningPoints(center, width, height)
	}
	halfWidth := widthF / 2
	halfHeight := heightF / 2
	points := make([]kicadfiles.Point, 0, cornerSegments*4+4)
	appendOffset := func(x, y float64) {
		point := kicadfiles.Point{
			X: dfmRoundIU(float64(center.X) + x),
			Y: dfmRoundIU(float64(center.Y) + y),
		}
		if len(points) == 0 || points[len(points)-1] != point {
			points = append(points, point)
		}
	}
	appendCorner := func(cx, cy, start, end float64) {
		for index := 0; index <= cornerSegments; index++ {
			t := float64(index) / float64(cornerSegments)
			angle := start + (end-start)*t
			appendOffset(cx+radius*math.Cos(angle), cy+radius*math.Sin(angle))
		}
	}
	appendOffset(-halfWidth+radius, -halfHeight)
	appendOffset(halfWidth-radius, -halfHeight)
	appendCorner(halfWidth-radius, -halfHeight+radius, -math.Pi/2, 0)
	appendOffset(halfWidth, halfHeight-radius)
	appendCorner(halfWidth-radius, halfHeight-radius, 0, math.Pi/2)
	appendOffset(-halfWidth+radius, halfHeight)
	appendCorner(-halfWidth+radius, halfHeight-radius, math.Pi/2, math.Pi)
	appendOffset(-halfWidth, -halfHeight+radius)
	appendCorner(-halfWidth+radius, -halfHeight+radius, math.Pi, 3*math.Pi/2)
	return points
}

func dfmRoundIU(value float64) kicadfiles.IU {
	// Half-up rounding keeps curved odd-IU openings aligned with rectangle splitting.
	return kicadfiles.IU(math.Floor(value + 0.5))
}

func padMaskLayers(pad *pcbfiles.Pad) []kicadfiles.BoardLayer {
	var layers []kicadfiles.BoardLayer
	for _, layer := range pad.Layers {
		switch layer {
		case kicadfiles.LayerFMask:
			layers = append(layers, layer)
		case kicadfiles.LayerBMask:
			layers = append(layers, layer)
		case kicadfiles.LayerAllMask:
			layers = append(layers, kicadfiles.LayerFMask, kicadfiles.LayerBMask)
		}
	}
	slices.Sort(layers)
	return slices.Compact(layers)
}

func dfmValidatedPolygon(polygon dfmPolygon) dfmPolygon {
	if strings.TrimSpace(polygon.UnsupportedReason) != "" {
		return polygon
	}
	polygon.Points = dfmNormalizePolygon(polygon.Points)
	if len(polygon.Points) < 3 {
		polygon.UnsupportedReason = "polygon has fewer than three distinct points"
		return polygon
	}
	selfIntersects, ok := dfmSelfIntersectsBounded(polygon.Points)
	if !ok {
		polygon.UnsupportedReason = "polygon is too complex for bounded self-intersection validation"
		return polygon
	}
	if selfIntersects {
		polygon.UnsupportedReason = "self-intersecting polygon is not modeled"
		return polygon
	}
	return polygon
}

func dfmSortPolygons(polygons []dfmPolygon) []dfmPolygon {
	slices.SortFunc(polygons, func(a, b dfmPolygon) int {
		if a.SourcePath != b.SourcePath {
			return cmp.Compare(a.SourcePath, b.SourcePath)
		}
		return cmp.Compare(a.ObjectID, b.ObjectID)
	})
	return polygons
}

func dfmCopperLayers(layers []kicadfiles.BoardLayer) []kicadfiles.BoardLayer {
	var out []kicadfiles.BoardLayer
	for _, layer := range layers {
		if isCopperLayer(layer) {
			out = append(out, layer)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
