package physicalrules

import (
	"testing"

	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	projectfiles "kicadai/internal/kicadfiles/project"
)

func TestEvaluateBoardPassesStackupEdgeCutsAndNetClasses(t *testing.T) {
	board := physicalRuleTestBoard()
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	if report.Status != StatusPass {
		t.Fatalf("status = %q issues=%#v checks=%#v", report.Status, report.Issues, report.Checks)
	}
	assertCheckStatus(t, report, CheckStackupCopperLayers, StatusPass)
	assertCheckStatus(t, report, CheckNetClassDefault, StatusPass)
	assertCheckStatus(t, report, CheckNetClassRoutedWidth, StatusPass)
	assertCheckStatus(t, report, CheckEdgeCutsOutline, StatusPass)
	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusPass)
}

func TestEvaluateBoardBlocksMissingCopperAndOutline(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Layers = nil
	board.Drawings = nil
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	if report.Status != StatusBlocked {
		t.Fatalf("status = %q want blocked", report.Status)
	}
	assertCheckStatus(t, report, CheckStackupCopperLayers, StatusBlocked)
	assertCheckStatus(t, report, CheckEdgeCutsOutline, StatusBlocked)
	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusSkipped)
}

func TestEvaluateBoardAllowsNegativePadToMaskClearance(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Setup.PadToMaskClearance = -kicadfiles.MM(0.05)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckStackupSolderMask, StatusPass)
}

func TestEvaluateBoardWarnsWhenProjectNetClassesMissing(t *testing.T) {
	board := physicalRuleTestBoard()

	report := EvaluateBoard(&board, nil, Options{})

	assertCheckStatus(t, report, CheckNetClassDefault, StatusWarning)
	if report.Status != StatusWarning {
		t.Fatalf("status = %q want warning", report.Status)
	}
}

func TestEvaluateBoardBlocksInvalidNetClassAndNarrowTrack(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Tracks[0].Width = 0
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckNetClassRoutedWidth, StatusBlocked)
	if report.Status != StatusBlocked {
		t.Fatalf("status = %q want blocked", report.Status)
	}

	project.NetClasses[0].ViaDrill = project.NetClasses[0].ViaDiameter
	board = physicalRuleTestBoard()
	report = EvaluateBoard(&board, &project, Options{})
	assertCheckStatus(t, report, CheckNetClassDefault, StatusBlocked)
}

func TestEvaluateBoardChecksAnnularRings(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Pads = append(board.Footprints[0].Pads, pcbfiles.Pad{
		UUID:     kicadfiles.UUID("30000000-0000-4000-8000-000000000002"),
		Name:     "2",
		Type:     "thru_hole",
		Position: point(1, 0),
		Size:     point(1.0, 1.0),
		Drill:    kicadfiles.MM(0.6),
		NetCode:  1,
		NetName:  "VCC",
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask},
	})
	board.Vias = []pcbfiles.Via{{
		UUID:     kicadfiles.UUID("41000000-0000-4000-8000-000000000001"),
		Position: point(6, 5),
		Size:     kicadfiles.MM(0.6),
		Drill:    kicadfiles.MM(0.3),
		NetCode:  1,
		NetName:  "VCC",
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckAnnularRingProfile, StatusPass)
	assertCheckStatus(t, report, CheckAnnularRingPlatedPad, StatusPass)
	assertCheckStatus(t, report, CheckAnnularRingVia, StatusPass)
}

func TestEvaluateBoardBlocksSmallAnnularRings(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Pads = append(board.Footprints[0].Pads, pcbfiles.Pad{
		UUID:     kicadfiles.UUID("30000000-0000-4000-8000-000000000003"),
		Name:     "2",
		Type:     "thru_hole",
		Position: point(1, 0),
		Size:     point(0.8, 0.8),
		Drill:    kicadfiles.MM(0.7),
		NetCode:  1,
		NetName:  "VCC",
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask},
	})
	board.Vias = []pcbfiles.Via{{
		UUID:     kicadfiles.UUID("41000000-0000-4000-8000-000000000002"),
		Position: point(6, 5),
		Size:     kicadfiles.MM(0.42),
		Drill:    kicadfiles.MM(0.30),
		NetCode:  1,
		NetName:  "VCC",
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckAnnularRingPlatedPad, StatusBlocked)
	assertCheckStatus(t, report, CheckAnnularRingVia, StatusBlocked)
	if report.Status != StatusBlocked {
		t.Fatalf("status = %q want blocked", report.Status)
	}
}

func TestEvaluateBoardSkipsNPTHForAnnularRing(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Pads = []pcbfiles.Pad{{
		UUID:     kicadfiles.UUID("30000000-0000-4000-8000-000000000004"),
		Name:     "MH",
		Type:     "np_thru_hole",
		Position: point(0, 0),
		Size:     point(1.0, 1.0),
		Drill:    kicadfiles.MM(0.9),
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask},
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckAnnularRingPlatedPad, StatusSkipped)
}

func TestEvaluateBoardBlocksNarrowCopperFeatures(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Tracks[0].Width = kicadfiles.MM(0.05)
	board.TrackArcs = []pcbfiles.TrackArc{{
		UUID:    kicadfiles.UUID("42000000-0000-4000-8000-000000000001"),
		Start:   point(4, 4),
		Mid:     point(5, 3),
		End:     point(6, 4),
		Width:   kicadfiles.MM(0.05),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "VCC",
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckCopperSliverTrackWidth, StatusBlocked)
}

func TestEvaluateBoardChecksZoneCopperMinimumWidth(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Zones = []pcbfiles.Zone{{
		UUID:         kicadfiles.UUID("43000000-0000-4000-8000-000000000001"),
		NetCode:      1,
		NetName:      "VCC",
		Layers:       []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		MinThickness: kicadfiles.MM(0.05),
		Polygons: [][]kicadfiles.Point{{
			point(1, 1),
			point(9, 1),
			point(9, 9),
			point(1, 9),
			point(1, 1),
		}},
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckCopperSliverZoneMinWidth, StatusBlocked)
}

func TestEvaluateBoardBlocksZeroThicknessCopperZone(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Zones = []pcbfiles.Zone{{
		UUID:    kicadfiles.UUID("43000000-0000-4000-8000-000000000002"),
		NetCode: 1,
		NetName: "VCC",
		Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		Polygons: [][]kicadfiles.Point{{
			point(1, 1),
			point(9, 1),
			point(9, 9),
			point(1, 9),
			point(1, 1),
		}},
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckCopperSliverZoneMinWidth, StatusBlocked)
}

func TestEvaluateBoardIgnoresNonCopperZoneForCopperSlivers(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Zones = []pcbfiles.Zone{{
		UUID:         kicadfiles.UUID("43000000-0000-4000-8000-000000000003"),
		Layers:       []kicadfiles.BoardLayer{kicadfiles.LayerFSilkS},
		MinThickness: kicadfiles.MM(0.05),
		Polygons: [][]kicadfiles.Point{{
			point(1, 1),
			point(9, 1),
			point(9, 9),
			point(1, 9),
			point(1, 1),
		}},
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckCopperSliverZoneMinWidth, StatusSkipped)
}

func TestEvaluateBoardWarnsOnOpenLineOutlineAndBlocksOutsideObject(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Drawings = []pcbfiles.Drawing{
		testEdgeLine("a", 0, 0, 10, 0),
		testEdgeLine("b", 10, 0, 10, 10),
		testEdgeLine("c", 10, 10, 0, 10),
	}
	board.Footprints[0].Position = point(20, 20)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsOutline, StatusWarning)
	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusBlocked)
	if report.Status != StatusBlocked {
		t.Fatalf("status = %q want blocked", report.Status)
	}
}

func TestEvaluateBoardHandlesRotatedPadContainment(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Position = point(9, 5)
	board.Footprints[0].Rotation = 90
	board.Footprints[0].Pads[0].Position = point(0, 1)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusPass)
}

func TestEvaluateBoardAcceptsTriangularLineOutline(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Drawings = []pcbfiles.Drawing{
		testEdgeLine("a", 0, 0, 10, 0),
		testEdgeLine("b", 10, 0, 5, 10),
		testEdgeLine("c", 5, 10, 0, 0),
	}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsOutline, StatusPass)
}

func TestEvaluateBoardBlocksObjectOutsidePolygonButInsideBounds(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Drawings = []pcbfiles.Drawing{{
		UUID:  kicadfiles.UUID("61000000-0000-4000-8000-000000000001"),
		Layer: kicadfiles.LayerEdge,
		Poly: &pcbfiles.PolylineDrawing{Points: []kicadfiles.Point{
			point(0, 0),
			point(10, 0),
			point(5, 10),
			point(0, 0),
		}, Width: kicadfiles.MM(0.1)},
	}}
	board.Footprints[0].Position = point(1, 9)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusBlocked)
}

func TestEvaluateBoardBlocksObjectInsideCutoutLoop(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Drawings = []pcbfiles.Drawing{
		{
			UUID:  kicadfiles.UUID("63000000-0000-4000-8000-000000000001"),
			Layer: kicadfiles.LayerEdge,
			Poly: &pcbfiles.PolylineDrawing{Points: []kicadfiles.Point{
				point(0, 0),
				point(10, 0),
				point(10, 10),
				point(0, 10),
				point(0, 0),
			}, Width: kicadfiles.MM(0.1)},
		},
		{
			UUID:  kicadfiles.UUID("63000000-0000-4000-8000-000000000002"),
			Layer: kicadfiles.LayerEdge,
			Poly: &pcbfiles.PolylineDrawing{Points: []kicadfiles.Point{
				point(4, 4),
				point(6, 4),
				point(6, 6),
				point(4, 6),
				point(4, 4),
			}, Width: kicadfiles.MM(0.1)},
		},
	}
	board.Footprints[0].Position = point(5, 5)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusBlocked)
}

func TestEvaluateBoardBlocksObjectInsideLineCutoutLoop(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Drawings = []pcbfiles.Drawing{
		testEdgeLine("a", 0, 0, 10, 0),
		testEdgeLine("b", 10, 0, 10, 10),
		testEdgeLine("c", 10, 10, 0, 10),
		testEdgeLine("d", 0, 10, 0, 0),
		testEdgeLine("e", 4, 4, 6, 4),
		testEdgeLine("f", 6, 4, 6, 6),
		testEdgeLine("7", 6, 6, 4, 6),
		testEdgeLine("8", 4, 6, 4, 4),
	}
	board.Footprints[0].Position = point(5, 5)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusBlocked)
}

func TestEvaluateBoardBlocksTrackCrossingConcavePolygonBoundary(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Drawings = []pcbfiles.Drawing{{
		UUID:  kicadfiles.UUID("62000000-0000-4000-8000-000000000001"),
		Layer: kicadfiles.LayerEdge,
		Poly: &pcbfiles.PolylineDrawing{Points: []kicadfiles.Point{
			point(0, 0),
			point(10, 0),
			point(10, 10),
			point(6, 10),
			point(6, 4),
			point(4, 4),
			point(4, 10),
			point(0, 10),
			point(0, 0),
		}, Width: kicadfiles.MM(0.1)},
	}}
	board.Footprints = nil
	board.Tracks[0].Start = point(2, 8)
	board.Tracks[0].End = point(8, 8)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusBlocked)
}

func TestEvaluateBoardBlocksWideTrackNearBoardEdge(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Tracks[0].Start = point(1, 0.1)
	board.Tracks[0].End = point(9, 0.1)
	board.Tracks[0].Width = kicadfiles.MM(0.30)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckEdgeCutsContainment, StatusBlocked)
}

func TestEdgeBoundsUsesArcSegmentNotWholeCircle(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Drawings = []pcbfiles.Drawing{{
		UUID:  kicadfiles.UUID("60000000-0000-4000-8000-000000000001"),
		Layer: kicadfiles.LayerEdge,
		Arc:   &pcbfiles.ArcDrawing{Start: point(0, 0), Mid: point(5, 5), End: point(10, 0), Width: kicadfiles.MM(0.1)},
	}}

	bounds := edgeBounds(&board)

	if !bounds.Valid {
		t.Fatal("bounds should be valid")
	}
	if bounds.MinY < kicadfiles.MM(-0.001) || bounds.MaxY != kicadfiles.MM(5) {
		t.Fatalf("arc bounds y = %s..%s, want 0..5", kicadfiles.ToMMString(bounds.MinY), kicadfiles.ToMMString(bounds.MaxY))
	}
}

func TestEvaluateBoardWarnsForMultipleNetClassesWithoutAssignments(t *testing.T) {
	board := physicalRuleTestBoard()
	project := physicalRuleTestProject()
	project.NetClasses = append(project.NetClasses, projectfiles.NetClass{
		Name:        "Power",
		Clearance:   kicadfiles.MM(0.20),
		TrackWidth:  kicadfiles.MM(0.50),
		ViaDiameter: kicadfiles.MM(0.80),
		ViaDrill:    kicadfiles.MM(0.40),
	})

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckNetClassAssignmentCoverage, StatusWarning)
	assertCheckStatus(t, report, CheckNetClassRoutedWidth, StatusPass)
}

func TestEvaluateBoardBlocksUndersizedVia(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Vias = []pcbfiles.Via{{
		UUID:     kicadfiles.UUID("70000000-0000-4000-8000-000000000001"),
		Position: point(5, 5),
		Size:     kicadfiles.MM(0.40),
		Drill:    kicadfiles.MM(0.20),
		NetCode:  1,
		NetName:  "VCC",
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
	}}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckNetClassViaDimensions, StatusBlocked)
}

func TestEvaluateBoardBlocksMissingSMDMaskAndPaste(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerFCu}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckSolderMaskPadLayers, StatusBlocked)
	assertCheckStatus(t, report, CheckSolderPastePadLayers, StatusBlocked)
}

func TestEvaluateBoardBlocksPasteOnThroughHolePad(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Pads[0].Type = "thru_hole"
	board.Footprints[0].Pads[0].Drill = kicadfiles.MM(0.6)
	board.Footprints[0].Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask, kicadfiles.LayerFPaste}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckSolderMaskPadLayers, StatusPass)
	assertCheckStatus(t, report, CheckSolderPastePadLayers, StatusBlocked)
}

func TestEvaluateBoardAllCuSMDRequiresBothPasteSides(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask, kicadfiles.LayerFPaste}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckSolderPastePadLayers, StatusBlocked)
}

func TestEvaluateBoardBlocksExtraPasteSideOnSMDPad(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask, kicadfiles.LayerFPaste, kicadfiles.LayerBPaste}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckSolderPastePadLayers, StatusBlocked)
}

func TestEvaluateBoardWarnsMissingCourtyard(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Graphics = nil
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckCourtyardPresence, StatusWarning)
}

func TestEvaluateBoardBlocksCourtyardOverlap(t *testing.T) {
	board := physicalRuleTestBoard()
	second := board.Footprints[0]
	second.UUID = kicadfiles.UUID("20000000-0000-4000-8000-000000000002")
	second.Reference = "U2"
	second.Position = point(5.5, 5)
	board.Footprints = append(board.Footprints, second)
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckCourtyardOverlap, StatusBlocked)
}

func TestEvaluateBoardBlocksSilkscreenOutsideBoard(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Graphics = append(board.Footprints[0].Graphics, pcbfiles.FootprintGraphic{
		UUID:  kicadfiles.UUID("22000000-0000-4000-8000-000000000002"),
		Layer: kicadfiles.LayerFSilkS,
		Line:  &pcbfiles.LineDrawing{Start: point(20, 20), End: point(21, 20), Width: kicadfiles.MM(0.1)},
	})
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckSilkscreenBoardClearance, StatusBlocked)
}

func TestEvaluateBoardBlocksRequiredMissingMountingHoles(t *testing.T) {
	board := physicalRuleTestBoard()
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{RequireMountingHoles: true})

	assertCheckStatus(t, report, CheckMountingHolePresence, StatusBlocked)
}

func TestEvaluateBoardChecksMountingHoleGeometryAndEdge(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints = append(board.Footprints, pcbfiles.Footprint{
		UUID:      kicadfiles.UUID("80000000-0000-4000-8000-000000000001"),
		Reference: "H1",
		Value:     "MountingHole",
		Position:  point(0.5, 0.5),
		Pads: []pcbfiles.Pad{{
			UUID:     kicadfiles.UUID("81000000-0000-4000-8000-000000000001"),
			Name:     "1",
			Type:     "np_thru_hole",
			Position: point(0, 0),
			Size:     point(1, 1),
			Drill:    kicadfiles.MM(0.6),
			Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerAllMask},
		}},
	})
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{MinHoleEdgeMM: 1.0})

	assertCheckStatus(t, report, CheckMountingHolePresence, StatusPass)
	assertCheckStatus(t, report, CheckMountingHoleGeometry, StatusPass)
	assertCheckStatus(t, report, CheckMountingHoleEdgeClearance, StatusBlocked)
}

func TestEvaluateBoardBlocksInvalidMountingHoleDrill(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints = append(board.Footprints, pcbfiles.Footprint{
		UUID:      kicadfiles.UUID("82000000-0000-4000-8000-000000000001"),
		Reference: "H1",
		Value:     "MountingHole",
		Position:  point(5, 5),
		Pads: []pcbfiles.Pad{{
			UUID:     kicadfiles.UUID("83000000-0000-4000-8000-000000000001"),
			Name:     "1",
			Type:     "np_thru_hole",
			Position: point(0, 0),
			Size:     point(1, 1),
			Drill:    0,
			Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerAllMask},
		}},
	})
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckMountingHoleGeometry, StatusBlocked)
}

func TestEvaluateBoardDoesNotTreatOrdinaryThroughHoleAsMountingHole(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Reference = "R1"
	board.Footprints[0].Pads[0].Type = "thru_hole"
	board.Footprints[0].Pads[0].Drill = kicadfiles.MM(0.6)
	board.Footprints[0].Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckMountingHolePresence, StatusSkipped)
}

func TestEvaluateBoardDoesNotTreatHeaderReferenceAsMountingHole(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Reference = "H1"
	board.Footprints[0].Value = "PinHeader_1x01"
	board.Footprints[0].LibraryID = "Connector_PinHeader_2.54mm:PinHeader_1x01_P2.54mm_Vertical"
	board.Footprints[0].Pads[0].Type = "thru_hole"
	board.Footprints[0].Pads[0].Drill = kicadfiles.MM(0.6)
	board.Footprints[0].Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckMountingHolePresence, StatusSkipped)
}

func TestEvaluateBoardDoesNotTreatSMDPadInMountingHoleFootprintAsMountingHole(t *testing.T) {
	board := physicalRuleTestBoard()
	board.Footprints[0].Reference = "H1"
	board.Footprints[0].Value = "MountingHole"
	board.Footprints[0].LibraryID = "MountingHole:MountingHole_3.2mm"
	project := physicalRuleTestProject()

	report := EvaluateBoard(&board, &project, Options{})

	assertCheckStatus(t, report, CheckMountingHolePresence, StatusSkipped)
}

func assertCheckStatus(t *testing.T, report Report, id string, status Status) {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			if check.Status != status {
				t.Fatalf("%s status = %q, want %q; check=%#v", id, check.Status, status, check)
			}
			return
		}
	}
	t.Fatalf("missing check %s in %#v", id, report.Checks)
}

func physicalRuleTestBoard() pcbfiles.PCBFile {
	return pcbfiles.PCBFile{
		Layers:  pcbfiles.DefaultTwoLayerStack(),
		General: pcbfiles.DefaultGeneral(),
		Setup:   pcbfiles.DefaultSetup(),
		Nets: []pcbfiles.Net{
			{Code: 0, Name: ""},
			{Code: 1, Name: "VCC"},
		},
		Drawings: []pcbfiles.Drawing{{
			UUID:  kicadfiles.UUID("10000000-0000-4000-8000-000000000001"),
			Layer: kicadfiles.LayerEdge,
			Rect:  &pcbfiles.RectDrawing{Start: point(0, 0), End: point(10, 10), Width: kicadfiles.MM(0.1)},
		}},
		Footprints: []pcbfiles.Footprint{{
			UUID:      kicadfiles.UUID("20000000-0000-4000-8000-000000000001"),
			Reference: "U1",
			Position:  point(5, 5),
			Graphics: []pcbfiles.FootprintGraphic{
				pcbfiles.FootprintGraphic{UUID: kicadfiles.UUID("21000000-0000-4000-8000-000000000001"), Layer: kicadfiles.LayerFCrtYd, Rect: &pcbfiles.RectDrawing{Start: point(-1, -1), End: point(1, 1), Width: kicadfiles.MM(0.05)}},
				pcbfiles.FootprintGraphic{UUID: kicadfiles.UUID("22000000-0000-4000-8000-000000000001"), Layer: kicadfiles.LayerFSilkS, Rect: &pcbfiles.RectDrawing{Start: point(-0.8, -0.8), End: point(0.8, 0.8), Width: kicadfiles.MM(0.1)}},
			},
			Pads: []pcbfiles.Pad{{
				UUID:     kicadfiles.UUID("30000000-0000-4000-8000-000000000001"),
				Name:     "1",
				Type:     "smd",
				Position: point(0, 0),
				Size:     point(1, 1),
				NetCode:  1,
				NetName:  "VCC",
				Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask, kicadfiles.LayerFPaste},
			}},
		}},
		Tracks: []pcbfiles.Track{{
			UUID:    kicadfiles.UUID("40000000-0000-4000-8000-000000000001"),
			Start:   point(4, 5),
			End:     point(6, 5),
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 1,
			NetName: "VCC",
		}},
	}
}

func physicalRuleTestProject() projectfiles.ProjectFile {
	return projectfiles.ProjectFile{
		Name: "demo",
		NetClasses: []projectfiles.NetClass{{
			Name:        "Default",
			Clearance:   kicadfiles.MM(0.20),
			TrackWidth:  kicadfiles.MM(0.25),
			ViaDiameter: kicadfiles.MM(0.60),
			ViaDrill:    kicadfiles.MM(0.30),
		}},
	}
}

func testEdgeLine(seed string, x1, y1, x2, y2 float64) pcbfiles.Drawing {
	return pcbfiles.Drawing{
		UUID:  kicadfiles.UUID("50000000-0000-4000-8000-0000000000" + seed),
		Layer: kicadfiles.LayerEdge,
		Line:  &pcbfiles.LineDrawing{Start: point(x1, y1), End: point(x2, y2), Width: kicadfiles.MM(0.1)},
	}
}

func point(x, y float64) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
}
