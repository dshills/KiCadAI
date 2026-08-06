package pcb

import "kicadai/internal/kicadfiles"

// ApplyFootprintLocalPlacementGeometry applies KiCad's deterministic
// TOP_BOTTOM side transform to canonical footprint-local geometry. Layer and
// text-mirroring projection remain the responsibility of the caller because
// imported footprints may already carry explicit board-side layers.
func ApplyFootprintLocalPlacementGeometry(footprint *Footprint) {
	if footprint == nil || footprint.Layer != kicadfiles.LayerBCu {
		return
	}
	for index := range footprint.Properties {
		footprint.Properties[index].Position = kicadfiles.BoardLocalPointForPlacement(footprint.Properties[index].Position, footprint.Layer)
		footprint.Properties[index].Rotation = kicadfiles.BoardTextAngleForPlacement(footprint.Properties[index].Rotation, footprint.Layer)
	}
	for index := range footprint.Texts {
		footprint.Texts[index].Position = kicadfiles.BoardLocalPointForPlacement(footprint.Texts[index].Position, footprint.Layer)
		footprint.Texts[index].Rotation = kicadfiles.BoardTextAngleForPlacement(footprint.Texts[index].Rotation, footprint.Layer)
	}
	for index := range footprint.Pads {
		footprint.Pads[index].Position = kicadfiles.BoardLocalPointForPlacement(footprint.Pads[index].Position, footprint.Layer)
		footprint.Pads[index].Rotation = kicadfiles.BoardLocalAngleForPlacement(footprint.Pads[index].Rotation, footprint.Layer)
	}
	for index := range footprint.Graphics {
		footprint.Graphics[index] = footprintGraphicForPlacement(footprint.Graphics[index], footprint.Layer)
	}
}

func footprintGraphicForPlacement(graphic FootprintGraphic, placementLayer kicadfiles.BoardLayer) FootprintGraphic {
	drawing := Drawing(graphic)
	if drawing.Line != nil {
		line := *drawing.Line
		line.Start = kicadfiles.BoardLocalPointForPlacement(line.Start, placementLayer)
		line.End = kicadfiles.BoardLocalPointForPlacement(line.End, placementLayer)
		drawing.Line = &line
	}
	if drawing.Rect != nil {
		rect := *drawing.Rect
		rect.Start = kicadfiles.BoardLocalPointForPlacement(rect.Start, placementLayer)
		rect.End = kicadfiles.BoardLocalPointForPlacement(rect.End, placementLayer)
		drawing.Rect = &rect
	}
	if drawing.Circle != nil {
		circle := *drawing.Circle
		circle.Center = kicadfiles.BoardLocalPointForPlacement(circle.Center, placementLayer)
		circle.End = kicadfiles.BoardLocalPointForPlacement(circle.End, placementLayer)
		drawing.Circle = &circle
	}
	if drawing.Arc != nil {
		arc := *drawing.Arc
		arc.Start = kicadfiles.BoardLocalPointForPlacement(arc.Start, placementLayer)
		arc.Mid = kicadfiles.BoardLocalPointForPlacement(arc.Mid, placementLayer)
		arc.End = kicadfiles.BoardLocalPointForPlacement(arc.End, placementLayer)
		drawing.Arc = &arc
	}
	if drawing.Poly != nil {
		poly := *drawing.Poly
		poly.Points = footprintPointsForPlacement(poly.Points, placementLayer)
		drawing.Poly = &poly
	}
	if drawing.Curve != nil {
		curve := *drawing.Curve
		curve.Points = footprintPointsForPlacement(curve.Points, placementLayer)
		drawing.Curve = &curve
	}
	if drawing.Text != nil {
		text := *drawing.Text
		text.Position = kicadfiles.BoardLocalPointForPlacement(text.Position, placementLayer)
		text.Rotation = kicadfiles.BoardTextAngleForPlacement(text.Rotation, placementLayer)
		drawing.Text = &text
	}
	return FootprintGraphic(drawing)
}

func footprintPointsForPlacement(points []kicadfiles.Point, placementLayer kicadfiles.BoardLayer) []kicadfiles.Point {
	if len(points) == 0 {
		return nil
	}
	result := make([]kicadfiles.Point, len(points))
	for index, point := range points {
		result[index] = kicadfiles.BoardLocalPointForPlacement(point, placementLayer)
	}
	return result
}
