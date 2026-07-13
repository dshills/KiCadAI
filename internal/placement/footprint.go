package placement

import (
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

const iuPerMillimeter = 1_000_000.0

func BoundsFromFootprint(record libraryresolver.FootprintRecord) (Bounds, []PadSummary, []reports.Issue) {
	var issues []reports.Issue
	if strings.TrimSpace(record.FootprintID) == "" {
		issues = append(issues, issue("footprint.footprint_id", "footprint id required"))
	}
	box := record.BoundingBox
	source := BoundsLibraryPads
	if record.GraphicsSummary.HasCourtyard {
		source = BoundsLibraryCourtyard
		if validBoundingBox(record.CourtyardBox) {
			box = record.CourtyardBox
		}
	}
	minX := iuToMM(box.Min.X)
	minY := iuToMM(box.Min.Y)
	maxX := iuToMM(box.Max.X)
	maxY := iuToMM(box.Max.Y)
	if maxX <= minX || maxY <= minY {
		issues = append(issues, issue("footprint.bounding_box", "footprint bounding box must be positive"))
	}
	bounds := Bounds{
		WidthMM:      maxX - minX,
		HeightMM:     maxY - minY,
		AnchorOffset: Point{XMM: -minX, YMM: -minY},
		Source:       source,
	}
	pads := make([]PadSummary, 0, len(record.Pads))
	for _, pad := range record.Pads {
		pads = append(pads, PadSummary{
			Name:        strings.TrimSpace(pad.Name),
			XMM:         iuToMM(pad.Position.X),
			YMM:         iuToMM(pad.Position.Y),
			RotationDeg: float64(pad.Rotation),
			WidthMM:     iuToMM(pad.Size.X),
			HeightMM:    iuToMM(pad.Size.Y),
			Type:        strings.TrimSpace(pad.Type),
			DrillMM:     iuToMM(pad.Drill),
			Layers:      footprintPadLayers(pad.Layers),
		})
	}
	return bounds, pads, issues
}

func footprintPadLayers(layers []kicadfiles.BoardLayer) []string {
	out := make([]string, 0, len(layers))
	for _, layer := range layers {
		value := strings.TrimSpace(string(layer))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func validBoundingBox(box libraryresolver.BoundingBox) bool {
	return box.Max.X > box.Min.X && box.Max.Y > box.Min.Y
}

func HydrateComponentFootprint(component Component, record libraryresolver.FootprintRecord) (Component, []reports.Issue) {
	bounds, pads, issues := BoundsFromFootprint(record)
	component.FootprintID = strings.TrimSpace(record.FootprintID)
	component.Bounds = bounds
	component.Pads = pads
	return component, issues
}

func iuToMM(value kicadfiles.IU) float64 {
	return float64(value) / iuPerMillimeter
}
