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
	minX := iuToMM(record.BoundingBox.Min.X)
	minY := iuToMM(record.BoundingBox.Min.Y)
	maxX := iuToMM(record.BoundingBox.Max.X)
	maxY := iuToMM(record.BoundingBox.Max.Y)
	if maxX <= minX || maxY <= minY {
		issues = append(issues, issue("footprint.bounding_box", "footprint bounding box must be positive"))
	}
	source := BoundsLibraryPads
	if record.GraphicsSummary.HasCourtyard {
		source = BoundsLibraryCourtyard
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
			Name:     strings.TrimSpace(pad.Name),
			XMM:      iuToMM(pad.Position.X),
			YMM:      iuToMM(pad.Position.Y),
			WidthMM:  iuToMM(pad.Size.X),
			HeightMM: iuToMM(pad.Size.Y),
		})
	}
	return bounds, pads, issues
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
