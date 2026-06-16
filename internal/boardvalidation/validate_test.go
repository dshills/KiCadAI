package boardvalidation

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

func TestValidateBoardGoodFullyRouted(t *testing.T) {
	board := twoPadBoard(t)
	result := ValidateBoard(context.Background(), board, testTarget(), Options{})
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
	result := ValidateBoard(context.Background(), board, testTarget(), Options{})
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	if !hasIssueCode(result.Issues, reports.CodeInvalidNetAssignment) {
		t.Fatalf("missing invalid net assignment issue: %#v", result.Issues)
	}
}

func TestValidateBoardUnroutedNet(t *testing.T) {
	board := twoPadBoard(t)
	board.Tracks = nil
	result := ValidateBoard(context.Background(), board, testTarget(), Options{})
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	net := findNetStatus(t, result, "SIGNAL")
	if net.Status != NetStatusUnconnected {
		t.Fatalf("SIGNAL status = %q, want unconnected", net.Status)
	}
}

func TestValidateBoardDanglingRouteEndpoint(t *testing.T) {
	board := twoPadBoard(t)
	board.Tracks[0].End = kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(20)}
	result := ValidateBoard(context.Background(), board, testTarget(), Options{})
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
	board.Tracks[0].Start = kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(12)}
	result := ValidateBoard(context.Background(), board, testTarget(), Options{})
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
	result := ValidateBoard(context.Background(), board, testTarget(), Options{})
	if result.Status != StatusPass {
		t.Fatalf("Status = %q, want pass with default zone warning; issues=%#v", result.Status, result.Issues)
	}
	strict := ValidateBoard(context.Background(), board, testTarget(), Options{StrictZones: true})
	if strict.Status != StatusFail {
		t.Fatalf("strict Status = %q, want fail", strict.Status)
	}
}

func TestValidateBoardRequiredDRCMissingFails(t *testing.T) {
	board := twoPadBoard(t)
	result := ValidateBoard(context.Background(), board, testTarget(), Options{RequireDRC: true})
	if result.Status != StatusFail {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
	if !hasIssueCode(result.Issues, reports.CodeSkippedExternalTool) {
		t.Fatalf("missing skipped DRC issue: %#v", result.Issues)
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
