package boardvalidation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/checks"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

func TestValidateBoardGoodFullyRouted(t *testing.T) {
	board := twoPadBoard(t)
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want pass; issues=%#v", result.Status, result.Issues)
	}
	if !result.FabricationReady {
		t.Fatalf("FabricationReady = false, want true")
	}
	net := findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusFullyRouted {
		t.Fatalf("SIGNAL status = %q, want fully_routed", net.Status)
	}
}

func TestValidateBoardUnknownPadNet(t *testing.T) {
	board := twoPadBoard(t)
	board.Footprints[0].Pads[0].NetCode = 99
	board.Footprints[0].Pads[0].NetName = "MISSING"
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	if !hasIssueCode(result.Issues, reports.CodeInvalidNetAssignment) {
		t.Fatalf("missing invalid net assignment issue: %#v", result.Issues)
	}
}

func TestValidateBoardAllowsNoNetPad(t *testing.T) {
	board := twoPadBoard(t)
	board.Footprints[0].Pads[0].NetCode = 0
	board.Footprints[0].Pads[0].NetName = ""
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	for _, issue := range result.Issues {
		if issue.Code == reports.CodeInvalidNetAssignment && strings.Contains(issue.Path, "footprints.0.pads.0") {
			t.Fatalf("unexpected invalid net assignment for no-net pad: %#v", result.Issues)
		}
	}
}

func TestValidateBoardAllowsSameNetDuplicatePadAlias(t *testing.T) {
	board := twoPadBoard(t)
	duplicate := board.Footprints[0].Pads[0]
	duplicate.UUID = testUUID("U1-pad-alias")
	duplicate.Position = kicadfiles.Point{X: kicadfiles.MM(4), Y: 0}
	board.Footprints[0].Pads = append(board.Footprints[0].Pads, duplicate)

	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if hasIssueMessage(result.Issues, "duplicate pad name") {
		t.Fatalf("unexpected duplicate pad alias issue: %#v", result.Issues)
	}
	if result.Status != StatusPass || findNetStatus(t, result, "SIGNAL").Status != NetStatusFullyRouted {
		t.Fatalf("duplicate package pad alias should not require a second route: %#v", result)
	}
}

func TestValidateBoardRejectsDifferentNetDuplicatePadAlias(t *testing.T) {
	board := twoPadBoard(t)
	board.Nets = append(board.Nets, pcbfiles.Net{Code: 2, Name: "OTHER"})
	duplicate := board.Footprints[0].Pads[0]
	duplicate.UUID = testUUID("U1-pad-conflict")
	duplicate.Position = kicadfiles.Point{X: kicadfiles.MM(0.5), Y: 0}
	duplicate.NetCode = 2
	duplicate.NetName = "OTHER"
	board.Footprints[0].Pads = append(board.Footprints[0].Pads, duplicate)

	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if !hasIssueMessage(result.Issues, "duplicate pad name") {
		t.Fatalf("missing duplicate pad conflict issue: %#v", result.Issues)
	}
}

func TestValidateBoardUnroutedNet(t *testing.T) {
	board := twoPadBoard(t)
	board.Tracks = nil
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	net := findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusUnconnected {
		t.Fatalf("SIGNAL status = %q, want unconnected", net.Status)
	}
}

func TestValidateBoardSplitSameNetCopperIsNotFullyRouted(t *testing.T) {
	board := twoPadBoard(t)
	board.Footprints = append(board.Footprints,
		testFootprint("U3", kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(15)}, 1),
		testFootprint("U4", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(15)}, 1),
	)
	board.Tracks = []pcbfiles.Track{{
		UUID:    testUUID("track-a"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}, {
		UUID:    testUUID("track-b"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(15)},
		End:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(15)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}}

	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})

	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail for split same-net copper", result.Status)
	}
	net := findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusPartiallyRouted {
		t.Fatalf("SIGNAL status = %q, want partially_routed", net.Status)
	}
	if !hasIssueCode(result.Issues, reports.CodeDisconnectedPad) {
		t.Fatalf("missing disconnected pad issue: %#v", result.Issues)
	}
}

func TestValidateBoardPadBridgesNearbySameNetCopperSegments(t *testing.T) {
	board := twoPadBoard(t)
	board.Footprints = append(board.Footprints, testFootprint("U3", kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(10)}, 1))
	board.Tracks = []pcbfiles.Track{{
		UUID:    testUUID("track-a"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(20) - routePointTolerance/2, Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}, {
		UUID:    testUUID("track-b"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(20) + routePointTolerance/2, Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}}

	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})

	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want pass for pad-bridged copper; issues=%#v", result.Status, result.Issues)
	}
	net := findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusFullyRouted {
		t.Fatalf("SIGNAL status = %q, want fully_routed", net.Status)
	}
}

func TestValidateBoardNearbySameLayerRouteEndpointsAreConnected(t *testing.T) {
	board := twoPadBoard(t)
	board.Tracks = []pcbfiles.Track{{
		UUID:    testUUID("track-a"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(15) - routePointTolerance/2, Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}, {
		UUID:    testUUID("track-b"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(15) + routePointTolerance/2, Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}}

	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})

	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want pass for nearby route endpoints; issues=%#v", result.Status, result.Issues)
	}
	net := findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusFullyRouted {
		t.Fatalf("SIGNAL status = %q, want fully_routed", net.Status)
	}
}

func TestValidateBoardSameXYDifferentLayersRequiresVia(t *testing.T) {
	board := twoPadBoard(t)
	internalLayer := kicadfiles.BoardLayer("In1.Cu")
	board.Layers = append([]pcbfiles.LayerDefinition{
		{Number: 0, Name: kicadfiles.LayerFCu, Kind: "signal"},
		{Number: 4, Name: internalLayer, Kind: "signal"},
		{Number: 2, Name: kicadfiles.LayerBCu, Kind: "signal"},
	}, board.Layers[2:]...)
	for footprintIndex := range board.Footprints {
		pad := &board.Footprints[footprintIndex].Pads[0]
		pad.Type = "thru_hole"
		pad.Shape = "circle"
		pad.Size = kicadfiles.Point{X: kicadfiles.MM(1.2), Y: kicadfiles.MM(1.2)}
		pad.Drill = kicadfiles.MM(0.6)
		pad.Layers = []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}
	}
	board.Tracks = []pcbfiles.Track{{
		UUID:    testUUID("track-f"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}, {
		UUID:    testUUID("track-b"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerBCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}}

	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	net := findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusPartiallyRouted {
		t.Fatalf("SIGNAL status without via = %q, want partially_routed", net.Status)
	}

	board.Vias = []pcbfiles.Via{{
		UUID:     testUUID("via-fb"),
		Position: kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		Size:     kicadfiles.MM(0.6),
		Drill:    kicadfiles.MM(0.3),
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		NetCode:  1,
		NetName:  "SIGNAL",
	}}
	result = ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusPass {
		t.Fatalf("Status with via = %q, want pass; issues=%#v", result.Status, result.Issues)
	}
	net = findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusFullyRouted {
		t.Fatalf("SIGNAL status with via = %q, want fully_routed", net.Status)
	}

	board.Tracks = []pcbfiles.Track{{
		UUID:    testUUID("track-f-internal"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
		NetName: "SIGNAL",
	}, {
		UUID:    testUUID("track-internal"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		End:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		Width:   kicadfiles.MM(0.25),
		Layer:   internalLayer,
		NetCode: 1,
		NetName: "SIGNAL",
	}}
	result = ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusPass {
		t.Fatalf("Status with through-via internal layer = %q, want pass; issues=%#v", result.Status, result.Issues)
	}
	net = findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusFullyRouted {
		t.Fatalf("SIGNAL internal status = %q, want fully_routed", net.Status)
	}
}

func TestViaConnectivityLayersExpandsThroughStackup(t *testing.T) {
	internalLayer := kicadfiles.BoardLayer("In1.Cu")
	layers := viaConnectivityLayers(
		[]kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		[]kicadfiles.BoardLayer{kicadfiles.LayerFCu, internalLayer, kicadfiles.LayerBCu},
	)
	if got := strings.Join(boardLayersToStrings(layers), " "); got != "F.Cu In1.Cu B.Cu" {
		t.Fatalf("via layers = %s, want all copper layers in stackup", got)
	}
}

func TestValidateBoardDanglingRouteEndpoint(t *testing.T) {
	board := twoPadBoard(t)
	board.Tracks[0].End = kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(20)}
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	if !hasIssueMessage(result.Issues, "route endpoint is not connected") {
		t.Fatalf("missing dangling endpoint issue: %#v", result.Issues)
	}
}

func TestValidateBoardRotatedFootprintPadPosition(t *testing.T) {
	board := twoPadBoard(t)
	board.Footprints[0].Position = kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	board.Footprints[0].Rotation = 90
	board.Footprints[0].Pads[0].Position = kicadfiles.Point{X: kicadfiles.MM(2), Y: 0}
	board.Tracks[0].Start = kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(8)}
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want pass for rotated footprint route; issues=%#v", result.Status, result.Issues)
	}
}

func TestValidateBoardZoneStrictness(t *testing.T) {
	board := twoPadBoard(t)
	board.Zones = []pcbfiles.Zone{{
		UUID:    testUUID("zone"),
		NetCode: 1,
		NetName: "SIGNAL",
		Name:    "SIGNAL pour",
		Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		Polygons: [][]kicadfiles.Point{{
			{X: kicadfiles.MM(5), Y: kicadfiles.MM(5)},
			{X: kicadfiles.MM(35), Y: kicadfiles.MM(5)},
			{X: kicadfiles.MM(35), Y: kicadfiles.MM(25)},
		}},
	}}
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want pass with default zone warning; issues=%#v", result.Status, result.Issues)
	}
	strict := ValidateBoard(context.Background(), &board, testTarget(), Options{StrictZones: true})
	if strict.Status != StatusFail {
		t.Fatalf("strict Status = %q, want fail", strict.Status)
	}
}

func TestValidateBoardRequiredDRCMissingFails(t *testing.T) {
	board := twoPadBoard(t)
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{RequireDRC: true})
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	if !hasIssueCode(result.Issues, reports.CodeSkippedExternalTool) {
		t.Fatalf("missing skipped DRC issue: %#v", result.Issues)
	}
}

func TestValidateBoardOptionalDRCMissingCLISkipsCleanly(t *testing.T) {
	board := twoPadBoard(t)
	result := ValidateBoard(context.Background(), &board, testTarget(), Options{})
	check := findCheck(t, result, CheckKiCadDRC)
	if check.Status != StatusSkipped {
		t.Fatalf("DRC status = %q, want skipped; issues=%#v", check.Status, check.Issues)
	}
	if len(check.Issues) != 0 {
		t.Fatalf("optional missing DRC issues = %#v, want none", check.Issues)
	}
	if result.Status != StatusPass {
		t.Fatalf("result status = %q, want pass; issues=%#v", result.Status, result.Issues)
	}
}

func TestDRCArtifactsSkipsMissingReport(t *testing.T) {
	artifacts := drcArtifacts(checks.CheckResult{
		Kind:       checks.CheckKindDRC,
		ReportPath: filepath.Join(t.TempDir(), "missing-drc.json"),
	})
	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for missing report", artifacts)
	}
}

func TestDRCNoOutputCrashWarnsAndMarksCheckError(t *testing.T) {
	result := checks.CheckResult{
		Kind:          checks.CheckKindDRC,
		Status:        checks.CheckStatusError,
		TargetPath:    "demo",
		ToolErrorKind: checks.ToolErrorNoOutputCrash,
	}

	issues := drcIssues(result, errors.New("drc check failed with exit code -1"), "demo")

	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one tool issue", issues)
	}
	if issues[0].Code != reports.CodeKiCadCLIFailed || issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("tool issue = %#v, want warning KiCad CLI failure", issues[0])
	}
	if status := drcCheckStatus(result, errors.New("drc check failed with exit code -1"), issues); status != StatusError {
		t.Fatalf("status = %q, want error because DRC did not complete", status)
	}
}

func TestDRCFindingsStillFailBoardValidation(t *testing.T) {
	result := checks.CheckResult{
		Kind:     checks.CheckKindDRC,
		Status:   checks.CheckStatusFail,
		ExitCode: 5,
		Findings: []checks.CheckFinding{{
			Kind:           checks.CheckKindDRC,
			Severity:       "error",
			Message:        "unconnected pad",
			RepairCategory: checks.RepairConnectivity,
		}},
	}

	issues := drcIssues(result, nil, "demo")

	if len(issues) != 1 || issues[0].Code != reports.CodeDisconnectedPad {
		t.Fatalf("issues = %#v, want disconnected pad validation issue", issues)
	}
	if status := drcCheckStatus(result, nil, issues); status != StatusFail {
		t.Fatalf("status = %q, want fail for real DRC findings", status)
	}
}

func TestDRCUntypedErrorIsReported(t *testing.T) {
	result := checks.CheckResult{
		Kind:       checks.CheckKindDRC,
		Status:     checks.CheckStatusError,
		TargetPath: "demo",
	}

	issues := drcIssues(result, errors.New("context deadline exceeded"), "demo")

	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one fallback tool issue", issues)
	}
	if issues[0].Code != reports.CodeKiCadCLIFailed || issues[0].Severity != reports.SeverityError {
		t.Fatalf("tool issue = %#v, want error KiCad CLI failure", issues[0])
	}
	if status := drcCheckStatus(result, errors.New("context deadline exceeded"), issues); status != StatusError {
		t.Fatalf("status = %q, want error", status)
	}
}

func TestDRCViolationExitWithFindingsDoesNotAddToolError(t *testing.T) {
	result := checks.CheckResult{
		Kind:     checks.CheckKindDRC,
		Status:   checks.CheckStatusFail,
		ExitCode: 5,
		Findings: []checks.CheckFinding{{
			Kind:           checks.CheckKindDRC,
			Severity:       "error",
			Message:        "clearance violation",
			RepairCategory: checks.RepairClearance,
		}},
	}

	issues := drcIssues(result, errors.New("drc check failed with exit code 5"), "demo")

	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want only DRC finding", issues)
	}
	if issues[0].Code == reports.CodeKiCadCLIFailed {
		t.Fatalf("issues = %#v, did not expect tool failure for DRC finding exit", issues)
	}
	if status := drcCheckStatus(result, errors.New("drc check failed with exit code 5"), issues); status != StatusFail {
		t.Fatalf("status = %q, want fail for DRC finding exit", status)
	}
}

func TestDRCPartialFindingsWithAbnormalErrorReportsToolError(t *testing.T) {
	result := checks.CheckResult{
		Kind:     checks.CheckKindDRC,
		Status:   checks.CheckStatusError,
		ExitCode: -1,
		Findings: []checks.CheckFinding{{
			Kind:           checks.CheckKindDRC,
			Severity:       "error",
			Message:        "partial clearance violation",
			RepairCategory: checks.RepairClearance,
		}},
	}

	issues := drcIssues(result, errors.New("signal: killed"), "demo")

	if len(issues) != 2 {
		t.Fatalf("issues = %#v, want tool issue and partial finding", issues)
	}
	if issues[0].Code != reports.CodeKiCadCLIFailed {
		t.Fatalf("issues = %#v, want tool issue first", issues)
	}
	if status := drcCheckStatus(result, errors.New("signal: killed"), issues); status != StatusError {
		t.Fatalf("status = %q, want error for abnormal execution with partial findings", status)
	}
}

func TestValidateBoardDoesNotMutateInput(t *testing.T) {
	board := twoPadBoard(t)
	board.Footprints[0].Path = ""
	_ = ValidateBoard(context.Background(), &board, testTarget(), Options{})
	if board.Footprints[0].Path != "" {
		t.Fatalf("ValidateBoard mutated footprint path to %q", board.Footprints[0].Path)
	}
}

func twoPadBoard(t *testing.T) pcbfiles.PCBFile {
	t.Helper()
	return pcbfiles.PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "pcbnew",
		GeneratorVersion: "10.0",
		General:          pcbfiles.DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           pcbfiles.DefaultTwoLayerStack(),
		Setup:            pcbfiles.DefaultSetup(),
		Nets:             []pcbfiles.Net{{Code: 0, Name: ""}, {Code: 1, Name: "SIGNAL"}},
		Footprints: []pcbfiles.Footprint{
			testFootprint("U1", kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}, 1),
			testFootprint("U2", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}, 1),
		},
		Tracks: []pcbfiles.Track{{
			UUID:    testUUID("track"),
			Start:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			End:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 1,
			NetName: "SIGNAL",
		}},
		Drawings: []pcbfiles.Drawing{
			{UUID: testUUID("edge-1"), Layer: kicadfiles.LayerEdge, Line: &pcbfiles.LineDrawing{Start: kicadfiles.Point{X: 0, Y: 0}, End: kicadfiles.Point{X: kicadfiles.MM(30), Y: 0}, Width: kicadfiles.MM(0.1)}},
			{UUID: testUUID("edge-2"), Layer: kicadfiles.LayerEdge, Line: &pcbfiles.LineDrawing{Start: kicadfiles.Point{X: kicadfiles.MM(30), Y: 0}, End: kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(20)}, Width: kicadfiles.MM(0.1)}},
			{UUID: testUUID("edge-3"), Layer: kicadfiles.LayerEdge, Line: &pcbfiles.LineDrawing{Start: kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(20)}, End: kicadfiles.Point{X: 0, Y: kicadfiles.MM(20)}, Width: kicadfiles.MM(0.1)}},
			{UUID: testUUID("edge-4"), Layer: kicadfiles.LayerEdge, Line: &pcbfiles.LineDrawing{Start: kicadfiles.Point{X: 0, Y: kicadfiles.MM(20)}, End: kicadfiles.Point{X: 0, Y: 0}, Width: kicadfiles.MM(0.1)}},
		},
		RequireClosedOutline: true,
	}
}

func testFootprint(ref string, position kicadfiles.Point, netCode int) pcbfiles.Footprint {
	return pcbfiles.Footprint{
		UUID:      testUUID(ref),
		Path:      "/" + string(testUUID(ref+"-path")),
		LibraryID: "Test:Pad",
		Reference: ref,
		Value:     ref,
		Layer:     kicadfiles.LayerFCu,
		Properties: []pcbfiles.FootprintProperty{
			{Name: "Reference", Value: ref, UUID: testUUID(ref + "-ref"), Layer: kicadfiles.LayerFSilkS},
			{Name: "Value", Value: ref, UUID: testUUID(ref + "-value"), Layer: kicadfiles.LayerFFab},
		},
		Position: position,
		Pads: []pcbfiles.Pad{{
			UUID:    testUUID(ref + "-pad"),
			Name:    "1",
			Type:    "smd",
			Shape:   "rect",
			Size:    kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFPaste, kicadfiles.LayerFMask},
			NetCode: netCode,
			NetName: "SIGNAL",
		}},
	}
}

func testTarget() Target {
	return Target{InputPath: "memory", BoardPath: "memory.kicad_pcb"}
}

func testUUID(seed string) kicadfiles.UUID {
	generator, err := kicadfiles.NewDeterministicIDGenerator("11111111-1111-1111-1111-111111111111", seed)
	if err != nil {
		panic(err)
	}
	return generator.New("test", seed)
}

func findNetStatus(t *testing.T, result Result, name string) NetStatus {
	t.Helper()
	for _, net := range result.Nets {
		if net.Name == name {
			return net
		}
	}
	t.Fatalf("net status %q not found in %#v", name, result.Nets)
	return NetStatus{}
}

func findCheck(t *testing.T, result Result, name string) Check {
	t.Helper()
	for _, check := range result.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("check %q not found in %#v", name, result.Checks)
	return Check{}
}

func hasIssueCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasIssueMessage(issues []reports.Issue, text string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, text) {
			return true
		}
	}
	return false
}
