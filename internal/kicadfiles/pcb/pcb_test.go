package pcb

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestWriteMinimalPCB(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, minimalPCB())
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"(version 20260206)",
		"(generator \"pcbnew\")",
		"(generator_version \"10.0\")",
		"(general",
		"(thickness 1.6)",
		"(legacy_teardrops no)",
		"(0 \"F.Cu\" signal)",
		"(2 \"B.Cu\" signal)",
		"(5 \"F.SilkS\" user \"F.Silkscreen\")",
		"(7 \"B.SilkS\" user \"B.Silkscreen\")",
		"(25 \"Edge.Cuts\" user)",
		"(127 \"User.45\" user)",
		"(allow_soldermask_bridges_in_footprints no)",
		"(tenting",
		"(front yes)",
		"(back yes)",
		"(pcbplotparams",
		"(usegerberattributes yes)",
		"(outputdirectory \"\")",
		"(net 0 \"\")",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestValidateRejectsMissingGenerator(t *testing.T) {
	board := minimalPCB()
	board.Generator = " "

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pcb.generator") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateNetCode(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}, {Code: 1, Name: "B"}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[1].code") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateNetName(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}, {Code: 2, Name: "A"}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[1].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsEmptyNonZeroNetName(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: " "}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[0].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsNamedNetZero(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 0, Name: "GND"}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[0].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidLayer(t *testing.T) {
	board := minimalPCB()
	board.Layers[0].Name = "Nope.Cu"

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "layers[0].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidSetup(t *testing.T) {
	tests := []struct {
		name  string
		board PCBFile
		want  string
	}{
		{
			name: "zero thickness",
			board: func() PCBFile {
				board := minimalPCB()
				board.Setup.Stackup.Thickness = 0
				return board
			}(),
			want: "setup.stackup.thickness",
		},
		{
			name: "negative solder mask min width",
			board: func() PCBFile {
				board := minimalPCB()
				board.Setup.SolderMaskMinWidth = -1
				return board
			}(),
			want: "setup.solder_mask_min_width",
		},
		{
			name: "negative pad to mask clearance",
			board: func() PCBFile {
				board := minimalPCB()
				board.Setup.PadToMaskClearance = -1
				return board
			}(),
			want: "setup.pad_to_mask_clearance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.board)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateRejectsDuplicateLayerName(t *testing.T) {
	board := minimalPCB()
	board.Layers[1].Name = board.Layers[0].Name

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "layers[1].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteRendersTitleBlock(t *testing.T) {
	board := minimalPCB()
	board.TitleBlock = kicadfiles.TitleBlock{
		Title:    "LED Board",
		Date:     "2026-06-09",
		Revision: "A",
		Company:  "KiCadAI",
		Comments: []string{"generated"},
	}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"(title_block",
		"(title \"LED Board\")",
		"(date \"2026-06-09\")",
		"(rev \"A\")",
		"(company \"KiCadAI\")",
		"(comment 1 \"generated\")",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteSortsNetsByCode(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 2, Name: "LED_OUT"}, {Code: 1, Name: "GND"}}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	output := buf.String()
	first := strings.Index(output, "(net 0 \"\")")
	second := strings.Index(output, "(net 1 \"GND\")")
	third := strings.Index(output, "(net 2 \"LED_OUT\")")
	if !(first < second && second < third) {
		t.Fatalf("nets not ordered by code:\n%s", output)
	}
}

func TestNetRegistryAssignsDeterministicCodes(t *testing.T) {
	registry := NewNetRegistry("GND", "VCC")

	if got := registry.EnsureNet("GND"); got.Code != 1 || got.Name != "GND" {
		t.Fatalf("EnsureNet(GND) = %#v", got)
	}
	if got := registry.EnsureNet("LED_OUT"); got.Code != 3 || got.Name != "LED_OUT" {
		t.Fatalf("EnsureNet(LED_OUT) = %#v", got)
	}
	if code, ok := registry.NetCode("VCC"); !ok || code != 2 {
		t.Fatalf("NetCode(VCC) = %d, %v", code, ok)
	}

	want := []Net{{Code: 0, Name: ""}, {Code: 1, Name: "GND"}, {Code: 2, Name: "VCC"}, {Code: 3, Name: "LED_OUT"}}
	if got := registry.Nets(); !equalNets(got, want) {
		t.Fatalf("Nets = %#v, want %#v", got, want)
	}
}

func TestNormalizeNetsAddsNetZero(t *testing.T) {
	got := NormalizeNets([]Net{{Code: 2, Name: "B"}, {Code: 1, Name: "A"}})
	want := []Net{{Code: 0, Name: ""}, {Code: 1, Name: "A"}, {Code: 2, Name: "B"}}
	if !equalNets(got, want) {
		t.Fatalf("NormalizeNets = %#v, want %#v", got, want)
	}

}

func TestWriteNormalizesNetZeroBeforeValidation(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Footprints = []Footprint{minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "(net 0 \"\")") {
		t.Fatalf("normalized net 0 missing:\n%s", buf.String())
	}
}

func TestWriteRejectsNamedNetZeroBeforeNormalization(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 0, Name: "GND"}}

	var buf bytes.Buffer
	err := Write(&buf, board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[0].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestLEDIndicatorPCBWritesDeterministicBoard(t *testing.T) {
	input := LEDIndicatorInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "phase-7",
	}
	board, err := LEDIndicatorPCB(input)
	if err != nil {
		t.Fatalf("LEDIndicatorPCB returned error: %v", err)
	}

	var first bytes.Buffer
	if err := Write(&first, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	var second bytes.Buffer
	if err := Write(&second, board); err != nil {
		t.Fatalf("second Write returned error: %v", err)
	}
	if first.String() != second.String() {
		t.Fatal("LED board output is not deterministic")
	}

	output := first.String()
	for _, want := range []string{
		"(title \"LED Indicator\")",
		"(net 1 \"VCC\")",
		"(net 2 \"LED_OUT\")",
		"(net 3 \"GND\")",
		"\"LED_SMD:LED_0805_2012Metric\"",
		"\"Resistor_SMD:R_0805_2012Metric\"",
		"\"D1\"",
		"roundrect",
		"(roundrect_rratio 0.25)",
		"(layers \"F.Cu\" \"F.Paste\" \"F.Mask\")",
		"(gr_line",
		"(segment",
		"(via",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteSortsFootprintsByReference(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	resistor := minimalFootprint("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "R1")
	led := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "D1")
	board.Footprints = []Footprint{resistor, led}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	first := strings.Index(output, "\"D1\"")
	second := strings.Index(output, "\"R1\"")
	if !(first >= 0 && second >= 0 && first < second) {
		t.Fatalf("footprints not sorted by reference:\n%s", output)
	}
}

func TestValidateRejectsInvalidFootprint(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
	footprint.Texts = footprint.Texts[:1]
	board.Footprints = []Footprint{footprint}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "footprints[0].texts.value") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsUnknownPadNet(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
	footprint.Pads[0].NetCode = 99
	board.Footprints = []Footprint{footprint}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pads[0].net_code") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteRendersDrilledPadAsThruHole(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].Drill = kicadfiles.MM(0.6)
	footprint.Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}
	board.Footprints = []Footprint{footprint}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{"thru_hole", "(layers \"*.Cu\" \"*.Mask\")", "(drill 0.6)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestValidateAcceptsDrilledPadWithExplicitCopperAndMaskLayers(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].Drill = kicadfiles.MM(0.6)
	footprint.Pads[0].Layers = []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu, kicadfiles.LayerFMask, kicadfiles.LayerBMask}
	board.Footprints = []Footprint{footprint}

	if err := Validate(board); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestWriteRendersCustomRoundRectRatio(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].Shape = "roundrect"
	footprint.Pads[0].RoundRectRRatio = 0.125
	board.Footprints = []Footprint{footprint}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "(roundrect_rratio 0.125)") {
		t.Fatalf("custom roundrect ratio missing:\n%s", buf.String())
	}
}

func TestValidateRejectsInvalidRoundRectRatio(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].Shape = "roundrect"
	footprint.Pads[0].RoundRectRRatio = 1.5
	board.Footprints = []Footprint{footprint}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "roundrect_rratio") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDrilledPadWithoutThroughHoleLayers(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].Drill = kicadfiles.MM(0.6)
	board.Footprints = []Footprint{footprint}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pads[0].layers") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateReportsInvalidPadLayerIndex(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].Layers = []kicadfiles.BoardLayer{"Nope.Layer"}
	board.Footprints = []Footprint{footprint}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "layers[0].name") {
		t.Fatalf("error path should reference layer index, not .name: %v", err)
	}
	if !strings.Contains(err.Error(), "pads[0].layers[0]") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidTrack(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Tracks = []Track{{
		UUID:    kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
		End:     kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(1)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFSilkS,
		NetCode: 1,
	}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "tracks[0].layer") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAcceptsInternalCopperLayers(t *testing.T) {
	board := minimalPCB()
	board.Layers = append(board.Layers, LayerDefinition{Number: 4, Name: kicadfiles.BoardLayer("In1.Cu"), Kind: "signal"})
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Tracks = []Track{{
		UUID:    kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Start:   kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
		End:     kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(1)},
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.BoardLayer("In1.Cu"),
		NetCode: 1,
	}}
	board.Vias = []Via{{
		UUID:     kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
		Position: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
		Size:     kicadfiles.MM(0.8),
		Drill:    kicadfiles.MM(0.4),
		NetCode:  1,
		Layers:   []kicadfiles.BoardLayer{kicadfiles.BoardLayer("In1.Cu"), kicadfiles.LayerBCu},
	}}

	if err := Validate(board); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsInvalidVia(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.Vias = []Via{{
		UUID:     kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Position: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
		Size:     kicadfiles.MM(0.4),
		Drill:    kicadfiles.MM(0.4),
		NetCode:  1,
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
	}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "vias[0].drill") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRequiresClosedOutline(t *testing.T) {
	board := minimalPCB()
	board.RequireClosedOutline = true
	board.Drawings = []Drawing{{
		UUID:  kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Layer: kicadfiles.LayerEdge,
		Kind:  "line",
		Line: &LineDrawing{
			Start: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
			End:   kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(1)},
			Width: kicadfiles.MM(0.1),
		},
	}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "drawings.edge_cuts") {
		t.Fatalf("error = %v", err)
	}
	if got := strings.Count(err.Error(), "outline endpoint"); got != 2 {
		t.Fatalf("reported %d open endpoints, want 2: %v", got, err)
	}
}

func TestWriteAdvancedGeometry(t *testing.T) {
	board := advancedGeometryPCB()

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(fp_line",
		"(fp_circle",
		"(fp_arc",
		"(fp_poly",
		"(gr_line",
		"(gr_circle",
		"(gr_arc",
		"(gr_poly",
		"(zone",
		"(net_name \"GND\")",
		"(polygon",
		"(dimension",
		"(type aligned)",
		"(gr_text",
		"\"20 mm\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestValidateRejectsGraphicWithMultipleShapes(t *testing.T) {
	board := minimalPCB()
	board.Drawings = []Drawing{{
		UUID:  kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Layer: kicadfiles.LayerDwgs,
		Line:  &LineDrawing{Start: point(1, 1), End: point(2, 1), Width: kicadfiles.MM(0.1)},
		Circle: &CircleDrawing{
			Center: point(1, 1),
			End:    point(2, 1),
			Width:  kicadfiles.MM(0.1),
		},
	}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "exactly one shape") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidZone(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "GND"}}
	board.Zones = []Zone{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		NetCode:  1,
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		Polygons: [][]kicadfiles.Point{{point(0, 0), point(1, 0)}},
	}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "zones[0].polygons[0].points") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidDimension(t *testing.T) {
	board := minimalPCB()
	board.Dimensions = []Dimension{{
		UUID:   kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Layer:  kicadfiles.LayerDwgs,
		Points: []kicadfiles.Point{point(0, 0), point(1, 0)},
		Height: kicadfiles.MM(2),
		Text:   "1 mm",
	}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "dimensions[0].type") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteValidatesBeforeRendering(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, PCBFile{})
	var validationErrors kicadfiles.ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("error = %v, want ValidationErrors", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Write emitted output despite validation error: %q", buf.String())
	}
}

func advancedGeometryPCB() PCBFile {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "GND"}}
	footprint := minimalFootprint("11111111-1111-4111-8111-111111111111", "U1")
	footprint.Graphics = []FootprintGraphic{
		{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Layer: kicadfiles.LayerFSilkS, Line: &LineDrawing{Start: point(0, 0), End: point(2, 0), Width: kicadfiles.MM(0.1)}},
		{UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"), Layer: kicadfiles.LayerFSilkS, Circle: &CircleDrawing{Center: point(0, 0), End: point(1, 0), Width: kicadfiles.MM(0.1)}},
		{UUID: kicadfiles.UUID("44444444-4444-4444-8444-444444444444"), Layer: kicadfiles.LayerFSilkS, Arc: &ArcDrawing{Start: point(0, 0), Mid: point(1, 1), End: point(2, 0), Width: kicadfiles.MM(0.1)}},
		{UUID: kicadfiles.UUID("55555555-5555-4555-8555-555555555555"), Layer: kicadfiles.LayerFSilkS, Poly: &PolylineDrawing{Points: []kicadfiles.Point{point(0, 0), point(1, 0), point(1, 1)}, Width: kicadfiles.MM(0.1)}},
	}
	board.Footprints = []Footprint{footprint}
	board.Drawings = []Drawing{
		{UUID: kicadfiles.UUID("66666666-6666-4666-8666-666666666666"), Layer: kicadfiles.LayerDwgs, Line: &LineDrawing{Start: point(0, 0), End: point(20, 0), Width: kicadfiles.MM(0.1)}},
		{UUID: kicadfiles.UUID("77777777-7777-4777-8777-777777777777"), Layer: kicadfiles.LayerDwgs, Circle: &CircleDrawing{Center: point(5, 5), End: point(6, 5), Width: kicadfiles.MM(0.1)}},
		{UUID: kicadfiles.UUID("88888888-8888-4888-8888-888888888888"), Layer: kicadfiles.LayerDwgs, Arc: &ArcDrawing{Start: point(0, 0), Mid: point(1, 1), End: point(2, 0), Width: kicadfiles.MM(0.1)}},
		{UUID: kicadfiles.UUID("99999999-9999-4999-8999-999999999999"), Layer: kicadfiles.LayerDwgs, Poly: &PolylineDrawing{Points: []kicadfiles.Point{point(0, 0), point(20, 0), point(20, 10), point(0, 0)}, Width: kicadfiles.MM(0.1)}},
	}
	board.Zones = []Zone{{
		UUID:     kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		NetCode:  1,
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		Polygons: [][]kicadfiles.Point{{point(0, 0), point(20, 0), point(20, 10), point(0, 10)}},
		Priority: 1,
	}}
	board.Dimensions = []Dimension{{
		UUID:     kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
		Type:     "aligned",
		Layer:    kicadfiles.LayerDwgs,
		Points:   []kicadfiles.Point{point(0, 0), point(20, 0)},
		Height:   kicadfiles.MM(2),
		Text:     "20 mm",
		Position: point(10, -2),
	}}
	return board
}

func point(x, y float64) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
}

func equalNets(a, b []Net) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func minimalFootprint(uuid, reference string) Footprint {
	return Footprint{
		UUID:      kicadfiles.UUID(uuid),
		Path:      "root.component." + strings.ToLower(reference),
		LibraryID: "Test:Footprint",
		Reference: reference,
		Value:     "value",
		Position:  kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
		Layer:     kicadfiles.LayerFCu,
		Texts: []FootprintText{
			{Kind: "reference", Text: reference, Layer: kicadfiles.LayerFSilkS},
			{Kind: "value", Text: "value", Layer: kicadfiles.LayerFSilkS},
		},
		Pads: []Pad{
			{Name: "1", NetCode: 1, Shape: "rect", Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}},
		},
	}
}

func minimalPCB() PCBFile {
	return PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "pcbnew",
		GeneratorVersion: "10.0",
		General:          DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           DefaultTwoLayerStack(),
		Setup:            DefaultSetup(),
	}
}
