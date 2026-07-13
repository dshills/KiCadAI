package transactions

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
)

func TestFootprintGraphicsFromRecordOmitsSilkscreenLinesOverPadMask(t *testing.T) {
	line := func(startX, startY, endX, endY float64) libraryresolver.FootprintGraphic {
		start := kicadfiles.Point{X: kicadfiles.MM(startX), Y: kicadfiles.MM(startY)}
		end := kicadfiles.Point{X: kicadfiles.MM(endX), Y: kicadfiles.MM(endY)}
		return libraryresolver.FootprintGraphic{
			Kind: "line", Layer: "F.SilkS", Width: kicadfiles.MM(0.1), Start: &start, End: &end,
		}
	}
	pads := []libraryresolver.FootprintPad{{
		Name: "5", Type: "smd", Shape: "rect",
		Position: kicadfiles.Point{X: kicadfiles.MM(0.975), Y: kicadfiles.MM(0.8)},
		Rotation: 90,
		Size:     kicadfiles.Point{X: kicadfiles.MM(0.5), Y: kicadfiles.MM(0.35)},
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask},
	}}
	graphics := []libraryresolver.FootprintGraphic{
		line(1.075, 1.075, 1.225, 1.075),
		line(-1.35, 1.1, -1.2, 1.1),
	}

	got := footprintGraphicsFromRecord(graphics, pads, kicadfiles.LayerFCu)
	if len(got) != 1 {
		t.Fatalf("graphics = %#v, want only clearance-safe line", got)
	}
	drawing := got[0]
	if drawing.Line == nil || drawing.Line.Start.X != kicadfiles.MM(-1.35) {
		t.Fatalf("remaining graphic = %#v, want non-overlapping line", drawing)
	}
}

func TestSilkscreenLineOverlapIgnoresNonMaskPadSide(t *testing.T) {
	start := kicadfiles.Point{X: 0, Y: 0}
	end := kicadfiles.Point{X: kicadfiles.MM(1), Y: 0}
	graphic := libraryresolver.FootprintGraphic{
		Kind: "line", Layer: "F.SilkS", Width: kicadfiles.MM(0.1), Start: &start, End: &end,
	}
	pad := libraryresolver.FootprintPad{
		Name: "1", Shape: "rect", Size: kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(2)},
		Layers: []kicadfiles.BoardLayer{kicadfiles.LayerBCu, kicadfiles.LayerBMask},
	}

	obstacles := footprintMaskObstacles([]libraryresolver.FootprintPad{pad}, kicadfiles.LayerFCu)
	if silkscreenLineOverlapsPadMask(graphic, obstacles, kicadfiles.LayerFCu) {
		t.Fatal("front silkscreen must not be clipped against a back-only mask opening")
	}
}

func TestFootprintMaskObstaclesIncludeCommonPadShapes(t *testing.T) {
	for _, shape := range []string{"rect", "roundrect", "oval", "circle"} {
		pad := libraryresolver.FootprintPad{
			Name: shape, Shape: shape,
			Size:   kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
			Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask},
		}
		if got := footprintMaskObstacles([]libraryresolver.FootprintPad{pad}, kicadfiles.LayerFCu); len(got) != 1 {
			t.Errorf("%s mask obstacles = %#v, want one", shape, got)
		}
	}
}
