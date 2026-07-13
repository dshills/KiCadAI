package placement

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
)

func TestBoundsFromFootprintUsesBoundingBoxAndPads(t *testing.T) {
	record := libraryresolver.FootprintRecord{
		FootprintID: "Test:R_0603",
		BoundingBox: libraryresolver.BoundingBox{
			Min: kicadfiles.Point{X: kicadfiles.MM(-1), Y: kicadfiles.MM(-0.5)},
			Max: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(0.5)},
		},
		GraphicsSummary: libraryresolver.GraphicsSummary{HasCourtyard: true},
		Pads: []libraryresolver.FootprintPad{{
			Name:     " 1 ",
			Position: kicadfiles.Point{X: kicadfiles.MM(-0.6), Y: 0},
			Rotation: 90,
			Size:     kicadfiles.Point{X: kicadfiles.MM(0.5), Y: kicadfiles.MM(0.8)},
		}},
	}

	bounds, pads, issues := BoundsFromFootprint(record)
	if len(issues) != 0 {
		t.Fatalf("BoundsFromFootprint returned issues: %#v", issues)
	}
	if !nearlyEqual(bounds.WidthMM, 2) || !nearlyEqual(bounds.HeightMM, 1) {
		t.Fatalf("bounds size = %.3fx%.3f, want 2x1", bounds.WidthMM, bounds.HeightMM)
	}
	if !nearlyEqual(bounds.AnchorOffset.XMM, 1) || !nearlyEqual(bounds.AnchorOffset.YMM, 0.5) {
		t.Fatalf("anchor offset = %#v, want {1, 0.5}", bounds.AnchorOffset)
	}
	if bounds.Source != BoundsLibraryCourtyard {
		t.Fatalf("source = %q, want library courtyard", bounds.Source)
	}
	if len(pads) != 1 || pads[0].Name != "1" || !nearlyEqual(pads[0].XMM, -0.6) || pads[0].RotationDeg != 90 {
		t.Fatalf("pads = %#v, want converted pad summary", pads)
	}
}

func TestBoundsFromFootprintPrefersCourtyardBox(t *testing.T) {
	record := libraryresolver.FootprintRecord{
		FootprintID: "Test:R_0603",
		BoundingBox: libraryresolver.BoundingBox{
			Min: kicadfiles.Point{X: kicadfiles.MM(-10), Y: kicadfiles.MM(-10)},
			Max: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		},
		CourtyardBox: libraryresolver.BoundingBox{
			Min: kicadfiles.Point{X: kicadfiles.MM(-1), Y: kicadfiles.MM(-0.5)},
			Max: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(0.5)},
		},
		GraphicsSummary: libraryresolver.GraphicsSummary{HasCourtyard: true},
	}

	bounds, _, issues := BoundsFromFootprint(record)
	if len(issues) != 0 {
		t.Fatalf("BoundsFromFootprint returned issues: %#v", issues)
	}
	if !nearlyEqual(bounds.WidthMM, 2) || !nearlyEqual(bounds.HeightMM, 1) {
		t.Fatalf("bounds size = %.3fx%.3f, want courtyard 2x1", bounds.WidthMM, bounds.HeightMM)
	}
	if bounds.Source != BoundsLibraryCourtyard {
		t.Fatalf("source = %q, want library courtyard", bounds.Source)
	}
}

func TestBoundsFromFootprintFallsBackToOverallBoxWhenCourtyardBoxInvalid(t *testing.T) {
	record := libraryresolver.FootprintRecord{
		FootprintID: "Test:R_0603",
		BoundingBox: libraryresolver.BoundingBox{
			Min: kicadfiles.Point{X: kicadfiles.MM(-1), Y: kicadfiles.MM(-0.5)},
			Max: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(0.5)},
		},
		GraphicsSummary: libraryresolver.GraphicsSummary{HasCourtyard: true},
	}

	bounds, _, issues := BoundsFromFootprint(record)
	if len(issues) != 0 {
		t.Fatalf("BoundsFromFootprint returned issues: %#v", issues)
	}
	if !nearlyEqual(bounds.WidthMM, 2) || bounds.Source != BoundsLibraryCourtyard {
		t.Fatalf("bounds = %#v, want fallback overall box with courtyard source", bounds)
	}
}

func TestBoundsFromFootprintRejectsInvalidBoundingBox(t *testing.T) {
	record := libraryresolver.FootprintRecord{FootprintID: "Test:Bad"}

	_, _, issues := BoundsFromFootprint(record)
	assertIssueContains(t, issues, "footprint bounding box must be positive")
}

func TestHydrateComponentFootprint(t *testing.T) {
	component := Component{Ref: "R1"}
	record := libraryresolver.FootprintRecord{
		FootprintID: "Test:R",
		BoundingBox: libraryresolver.BoundingBox{
			Min: kicadfiles.Point{},
			Max: kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(1)},
		},
		Pads: []libraryresolver.FootprintPad{{Name: "1"}},
	}

	got, issues := HydrateComponentFootprint(component, record)
	if len(issues) != 0 {
		t.Fatalf("HydrateComponentFootprint returned issues: %#v", issues)
	}
	if got.FootprintID != "Test:R" || got.Bounds.WidthMM != 2 || len(got.Pads) != 1 {
		t.Fatalf("hydrated component = %#v", got)
	}
}
