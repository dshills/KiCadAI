package physicalrules

import (
	"math"
	"testing"

	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
)

func TestDFMZonePolygonObservations(t *testing.T) {
	board := &pcbfiles.PCBFile{Zones: []pcbfiles.Zone{{
		UUID:     kicadfiles.UUID("10000000-0000-4000-8000-000000000001"),
		NetCode:  1,
		NetName:  "GND",
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		Polygons: [][]kicadfiles.Point{{point(0, 0), point(3, 0), point(3, 3), point(0, 3)}},
		FilledPolygons: []pcbfiles.ZoneFilledPolygon{{
			Layer:  kicadfiles.LayerFCu,
			Points: []kicadfiles.Point{point(0, 0), point(2, 0), point(2, 2), point(0, 2)},
		}},
	}, {
		UUID:     kicadfiles.UUID("10000000-0000-4000-8000-000000000002"),
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerDwgs},
		Polygons: [][]kicadfiles.Point{{point(0, 0), point(1, 0), point(1, 1), point(0, 1)}},
		FilledPolygons: []pcbfiles.ZoneFilledPolygon{{
			Layer:  kicadfiles.LayerDwgs,
			Points: []kicadfiles.Point{point(0, 0), point(1, 0), point(1, 1), point(0, 1)},
		}},
	}}}

	observations := dfmZonePolygons(board)
	if len(observations) != 5 {
		t.Fatalf("observations = %#v", observations)
	}
	if observations[0].SourcePath != "zones[0].filled_polygons[0]" || observations[0].Kind != dfmGeometryFilledZonePolygon {
		t.Fatalf("first observation = %#v", observations[0])
	}
	if observations[1].SourcePath != "zones[0].polygons[0][B.Cu]" || observations[1].NetName != "GND" {
		t.Fatalf("second observation = %#v", observations[1])
	}
	if observations[2].SourcePath != "zones[0].polygons[0][F.Cu]" {
		t.Fatalf("third observation = %#v", observations[2])
	}
	if observations[3].UnsupportedReason != "filled zone polygon is not on a copper layer" {
		t.Fatalf("unsupported filled zone = %#v", observations[3])
	}
	if observations[4].UnsupportedReason != "zone does not declare a copper layer" {
		t.Fatalf("unsupported zone = %#v", observations[4])
	}
}

func TestDFMCopperGraphicAndOutlineObservations(t *testing.T) {
	board := &pcbfiles.PCBFile{
		Drawings: []pcbfiles.Drawing{
			{UUID: kicadfiles.UUID("20000000-0000-4000-8000-000000000001"), Layer: kicadfiles.LayerFCu, Poly: &pcbfiles.PolylineDrawing{Points: []kicadfiles.Point{point(1, 1), point(3, 1), point(3, 2), point(1, 2)}}},
			{UUID: kicadfiles.UUID("20000000-0000-4000-8000-000000000003"), Layer: kicadfiles.LayerFCu, Line: &pcbfiles.LineDrawing{Start: point(0, 0), End: point(1, 0), Width: kicadfiles.MM(0.2)}},
			{UUID: kicadfiles.UUID("20000000-0000-4000-8000-000000000002"), Layer: kicadfiles.LayerEdge, Rect: &pcbfiles.RectDrawing{Start: point(0, 0), End: point(10, 5)}},
		},
	}
	copper := dfmCopperGraphicPolygons(board)
	if len(copper) != 2 || copper[0].SourcePath != "drawings[0].poly" || !copper[0].supported() || copper[1].UnsupportedReason == "" {
		t.Fatalf("copper observations = %#v", copper)
	}
	outline := dfmBoardOutlinePolygons(board, boardBounds{})
	if len(outline) != 1 || outline[0].Kind != dfmGeometryBoardOutline || len(outline[0].Points) != 4 {
		t.Fatalf("outline observations = %#v", outline)
	}
}

func TestDFMFootprintCopperGraphicObservationTransformsPoints(t *testing.T) {
	board := &pcbfiles.PCBFile{Footprints: []pcbfiles.Footprint{{
		Reference: "U1",
		Position:  point(10, 20),
		Graphics: []pcbfiles.FootprintGraphic{{
			UUID:  kicadfiles.UUID("30000000-0000-4000-8000-000000000001"),
			Layer: kicadfiles.LayerFCu,
			Poly:  &pcbfiles.PolylineDrawing{Points: []kicadfiles.Point{point(0, 0), point(1, 0), point(1, 1), point(0, 1)}},
		}},
	}}}

	observations := dfmCopperGraphicPolygons(board)
	if len(observations) != 1 || observations[0].Reference != "U1" {
		t.Fatalf("observations = %#v", observations)
	}
	if observations[0].Points[0] != point(10, 20) {
		t.Fatalf("transformed point = %#v", observations[0].Points[0])
	}
}

func TestDFMMaskOpeningsAndUnsupportedPads(t *testing.T) {
	board := &pcbfiles.PCBFile{Footprints: []pcbfiles.Footprint{{
		Reference: "J1",
		Pads: []pcbfiles.Pad{
			{UUID: kicadfiles.UUID("40000000-0000-4000-8000-000000000001"), Shape: "rect", Rotation: 90, Position: point(1, 1), Size: point(1, 2), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFMask}},
			{UUID: kicadfiles.UUID("40000000-0000-4000-8000-000000000002"), Shape: "custom", Position: point(4, 1), Size: point(1, 1), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFMask}},
			{UUID: kicadfiles.UUID("40000000-0000-4000-8000-000000000003"), Shape: "rect", Position: point(7, 1), Size: point(1, 1), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllMask}},
			{UUID: kicadfiles.UUID("40000000-0000-4000-8000-000000000004"), Shape: "roundrect", RoundRectRRatio: 0.25, Position: point(9, 1), Size: point(1, 1), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFMask}},
			{UUID: kicadfiles.UUID("40000000-0000-4000-8000-000000000005"), Shape: "circle", Rotation: 22.5, Position: point(11, 1), Size: point(2, 2), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFMask}},
			{UUID: kicadfiles.UUID("40000000-0000-4000-8000-000000000006"), Shape: "oval", Position: point(14, 1), Size: point(3, 1), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFMask}},
		},
	}}}

	observations := dfmMaskOpenings(board, kicadfiles.MM(0.1))
	if len(observations) != 7 {
		t.Fatalf("observations = %#v", observations)
	}
	if !observations[0].supported() || observations[0].Kind != dfmGeometryMaskOpening {
		t.Fatalf("supported opening = %#v", observations[0])
	}
	bounds := dfmPolygonBounds(observations[0].Points)
	if math.Abs((bounds.MaxX-bounds.MinX)-2.2) > 1e-9 || math.Abs((bounds.MaxY-bounds.MinY)-1.2) > 1e-9 {
		t.Fatalf("rotated opening bounds = %#v, want swapped dimensions with expansion", bounds)
	}
	unsupported := 0
	for _, observation := range observations {
		if !observation.supported() {
			unsupported++
		}
	}
	if unsupported != 1 {
		t.Fatalf("unsupported count = %d observations=%#v", unsupported, observations)
	}
	if len(observations[4].Points) <= 4 {
		t.Fatalf("roundrect point count = %d, want rounded-corner polygon", len(observations[4].Points))
	}
	circleBounds := dfmPolygonBounds(observations[5].Points)
	if math.Abs((circleBounds.MaxX-circleBounds.MinX)-2.2) > 1e-9 || math.Abs((circleBounds.MaxY-circleBounds.MinY)-2.2) > 1e-9 {
		t.Fatalf("circle opening bounds = %#v, want square bounding box with expansion", circleBounds)
	}
	if len(observations[5].Points) != 32 {
		t.Fatalf("circle point count = %d", len(observations[5].Points))
	}
	ovalBounds := dfmPolygonBounds(observations[6].Points)
	if math.Abs((ovalBounds.MaxX-ovalBounds.MinX)-3.2) > 1e-9 || math.Abs((ovalBounds.MaxY-ovalBounds.MinY)-1.2) > 1e-9 {
		t.Fatalf("oval opening bounds = %#v, want rounded-end bounding box with expansion", ovalBounds)
	}
	if len(observations[6].Points) < 32 {
		t.Fatalf("oval point count = %d", len(observations[6].Points))
	}
}

func TestDFMTransformPadOpeningPointsUsesKiCadRotationDirection(t *testing.T) {
	rotation := dfmCachedRotation(map[int64]dfmRotation2D{}, kicadfiles.Angle(90))

	points := dfmTransformPadOpeningPoints(kicadfiles.Point{}, transform2D{Cosine: 1}, rotation, []kicadfiles.Point{point(1, 0)})

	if len(points) != 1 || points[0] != point(0, -1) {
		t.Fatalf("rotated points = %#v, want +90 degrees to move rightward point upward in KiCad coordinates", points)
	}
}
