package physicalrules

import (
	"math"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestDFMEstimatePolygonWidthRectangles(t *testing.T) {
	wide := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(4, 0), point(4, 2), point(0, 2),
	}})
	if !wide.Measured || wide.Method != "axis_aligned_rectangle" || math.Abs(wide.WidthMM-2) > 1e-9 {
		t.Fatalf("wide rectangle width = %#v", wide)
	}

	narrow := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(4, 0), point(4, 0.08), point(0, 0.08),
	}})
	if !narrow.Measured || math.Abs(narrow.WidthMM-0.08) > 1e-9 {
		t.Fatalf("narrow rectangle width = %#v", narrow)
	}
}

func TestDFMEstimatePolygonWidthLShapedCorridor(t *testing.T) {
	result := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(4, 0), point(4, 1), point(1, 1), point(1, 4), point(0, 4),
	}})

	if !result.Measured || result.Method != "non_adjacent_edge_distance" {
		t.Fatalf("L width result = %#v", result)
	}
	if math.Abs(result.WidthMM-1) > 1e-9 {
		t.Fatalf("L width = %g, want 1", result.WidthMM)
	}
	if result.SampleCount == 0 {
		t.Fatalf("sample count = %d", result.SampleCount)
	}
}

func TestDFMEstimatePolygonWidthUnsupportedGeometry(t *testing.T) {
	bowtie := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(2, 2), point(0, 2), point(2, 0),
	}})
	if bowtie.Measured || bowtie.UnsupportedReason != "self-intersecting polygon is not modeled" {
		t.Fatalf("bowtie result = %#v", bowtie)
	}

	degenerate := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(1, 0), point(1, 0), point(0, 0),
	}})
	if degenerate.Measured || degenerate.UnsupportedReason == "" {
		t.Fatalf("degenerate result = %#v", degenerate)
	}

	preUnsupported := dfmEstimatePolygonWidth(dfmPolygon{UnsupportedReason: "raw polygon is unavailable"})
	if preUnsupported.Measured || preUnsupported.UnsupportedReason != "raw polygon is unavailable" {
		t.Fatalf("pre-unsupported result = %#v", preUnsupported)
	}
}

func TestDFMEstimatePolygonWidthStableAcrossWindingAndClosure(t *testing.T) {
	open := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(3, 0), point(3, 1), point(0, 1),
	}})
	closedReversed := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(0, 1), point(3, 1), point(3, 0), point(0, 0),
	}})

	if !open.Measured || !closedReversed.Measured {
		t.Fatalf("open=%#v closedReversed=%#v", open, closedReversed)
	}
	if math.Abs(open.WidthMM-closedReversed.WidthMM) > 1e-9 {
		t.Fatalf("width mismatch open=%#v closedReversed=%#v", open, closedReversed)
	}
}

func TestDFMEstimatePolygonWidthRotatedRectangle(t *testing.T) {
	result := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(-1.0606601718, -1.767766953),
		point(1.767766953, 1.0606601718),
		point(1.0606601718, 1.767766953),
		point(-1.767766953, -1.0606601718),
	}})

	if !result.Measured || result.Method != "non_adjacent_edge_distance" {
		t.Fatalf("rotated rectangle result = %#v", result)
	}
	if math.Abs(result.WidthMM-1) > 1e-6 {
		t.Fatalf("rotated rectangle width = %g, want 1", result.WidthMM)
	}
}

func TestDFMEstimatePolygonWidthTriangleUsesAltitude(t *testing.T) {
	result := dfmEstimatePolygonWidth(dfmPolygon{Points: []kicadfiles.Point{
		point(0, 0), point(4, 0), point(2, 1),
	}})

	if !result.Measured || result.Method != "triangle_altitude" {
		t.Fatalf("triangle result = %#v", result)
	}
	if math.Abs(result.WidthMM-1) > 1e-9 {
		t.Fatalf("triangle width = %g, want shortest altitude 1", result.WidthMM)
	}
}

func TestDFMEstimatePolygonWidthReportsZeroPinch(t *testing.T) {
	width, samples, _ := dfmNonAdjacentEdgeWidthMM([]kicadfiles.Point{
		point(0, 0), point(2, 0), point(2, 2), point(1, 0), point(0, 2),
	})

	if samples == 0 {
		t.Fatalf("samples = %d", samples)
	}
	if width != 0 {
		t.Fatalf("pinch width = %g, want 0", width)
	}
}

func TestDFMEstimatePolygonWidthRejectsVeryLargePolygon(t *testing.T) {
	points := make([]kicadfiles.Point, 0, dfmWidthMaxEdges+1)
	for index := 0; index < dfmWidthMaxEdges+1; index++ {
		angle := 2 * math.Pi * float64(index) / float64(dfmWidthMaxEdges+1)
		points = append(points, kicadfiles.Point{
			X: kicadfiles.MM(10 + 5*math.Cos(angle)),
			Y: kicadfiles.MM(10 + 5*math.Sin(angle)),
		})
	}

	result := dfmEstimatePolygonWidth(dfmPolygon{Points: points})

	if result.Measured || result.UnsupportedReason != "polygon has too many edges for first-pass width estimation" {
		t.Fatalf("large polygon result = %#v", result)
	}
}
