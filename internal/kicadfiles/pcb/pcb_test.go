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
		"(pad_to_mask_clearance 0)",
		"(tenting",
		"(front yes)",
		"(back yes)",
		"(pcbplotparams",
		"(usegerberattributes yes)",
		"(outputdirectory \"\")",
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
				board.Setup.HasStackup = true
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

func TestWriteRendersPreservedNodes(t *testing.T) {
	board := minimalPCB()
	board.Preserved = []PreservedNode{{Raw: `(embedded_fonts no)`}}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "(embedded_fonts no)") {
		t.Fatalf("preserved node missing:\n%s", buf.String())
	}
}

func TestWriteUsesNetNamesForPadsAndRoutes(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 2, Name: "LED_OUT"}, {Code: 1, Name: "GND"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "D1")
	footprint.Pads[0].NetCode = 2
	board.Footprints = []Footprint{footprint}
	board.Tracks = []Track{{
		UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
		Start:   point(1, 1),
		End:     point(2, 1),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	output := buf.String()
	for _, want := range []string{`(net "LED_OUT")`, `(net "GND")`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, `(net 1 "GND")`) || strings.Contains(output, `(net 2 "LED_OUT")`) {
		t.Fatalf("top-level net declarations should not be rendered:\n%s", output)
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
	if !strings.Contains(buf.String(), `(net "A")`) {
		t.Fatalf("net name resolution missing:\n%s", buf.String())
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
		"(net \"VCC\")",
		"(net \"LED_OUT\")",
		"(net \"GND\")",
		"\"LED_SMD:LED_0805_2012Metric\"",
		"\"Resistor_SMD:R_0805_2012Metric\"",
		"(property",
		"\"Reference\"",
		"\"Value\"",
		"\"Datasheet\"",
		"\"Description\"",
		"(attr smd)",
		"(duplicate_pad_numbers_are_jumpers no)",
		"\"D1\"",
		"roundrect",
		"(roundrect_rratio 0.25)",
		"(layers \"F.Cu\" \"F.Mask\" \"F.Paste\")",
		"(gr_line",
		"(segment",
		"(via",
		"(embedded_fonts no)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteRendersFootprintPropertiesAndModel(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
	footprint.Texts = nil
	footprint.Description = "Chip resistor"
	footprint.Tags = "resistor"
	footprint.SheetName = "/"
	footprint.SheetFile = "test.kicad_sch"
	footprint.Locked = true
	footprint.Attributes = []string{"smd"}
	footprint.MetadataProperties = []FootprintMetadataProperty{
		{Name: "ki_fp_filters", Value: "R_*"},
	}
	footprint.Units = []FootprintUnit{
		{Name: "A", Pins: []string{"1", "2"}},
	}
	footprint.NetTiePadGroups = []string{"1,2"}
	footprint.Properties = []FootprintProperty{
		{
			UUID:     kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Name:     "Reference",
			Value:    "R1",
			Position: point(0, -1),
			Layer:    kicadfiles.LayerFSilkS,
			Unlocked: true,
		},
		{
			UUID:     kicadfiles.UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
			Name:     "Value",
			Value:    "value",
			Position: point(0, 1),
			Layer:    kicadfiles.LayerFFab,
		},
	}
	embeddedFonts := false
	footprint.EmbeddedFonts = &embeddedFonts
	footprint.Models = []Model3D{{
		Path:   "${KICAD6_3DMODEL_DIR}/Resistor_SMD.3dshapes/R_0603_1608Metric.wrl",
		Offset: XYZ{X: 1},
	}}
	board.Footprints = []Footprint{footprint}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(locked yes)",
		"(descr \"Chip resistor\")",
		"(tags \"resistor\")",
		"(property",
		"\"Reference\"",
		"\"R1\"",
		"(property \"ki_fp_filters\" \"R_*\")",
		"(unlocked yes)",
		"(effects",
		"(sheetname \"/\")",
		"(sheetfile \"test.kicad_sch\")",
		"(units",
		"(unit",
		"(name \"A\")",
		"(pins \"1\" \"2\")",
		"(attr smd)",
		"(net_tie_pad_groups \"1,2\")",
		"(embedded_fonts no)",
		"(model",
		"\"${KICAD6_3DMODEL_DIR}/Resistor_SMD.3dshapes/R_0603_1608Metric.wrl\"",
		"(offset",
		"(xyz 1 0 0)",
		"(scale",
		"(xyz 1 1 1)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestValidateRejectsInvalidFootprintMetadata(t *testing.T) {
	tests := []struct {
		name      string
		footprint Footprint
		want      string
	}{
		{
			name: "duplicate property name",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.Texts = nil
				footprint.Properties = []FootprintProperty{
					{UUID: kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"), Name: "Reference", Value: "R1", Layer: kicadfiles.LayerFSilkS},
					{UUID: kicadfiles.UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"), Name: "Value", Value: "value", Layer: kicadfiles.LayerFFab},
				}
				footprint.MetadataProperties = []FootprintMetadataProperty{{Name: "Value", Value: "duplicate"}}
				return footprint
			}(),
			want: "metadata_properties[0].name",
		},
		{
			name: "duplicate property name after trimming",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.Texts = nil
				footprint.Properties = []FootprintProperty{
					{UUID: kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"), Name: "Reference", Value: "R1", Layer: kicadfiles.LayerFSilkS},
					{UUID: kicadfiles.UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"), Name: "Value", Value: "value", Layer: kicadfiles.LayerFFab},
				}
				footprint.MetadataProperties = []FootprintMetadataProperty{{Name: " Value ", Value: "duplicate"}}
				return footprint
			}(),
			want: "metadata_properties[0].name",
		},
		{
			name: "duplicate attribute",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.Attributes = []string{"smd", "smd"}
				return footprint
			}(),
			want: "attributes[1]",
		},
		{
			name: "empty unit pin",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.Units = []FootprintUnit{{Name: "A", Pins: []string{" "}}}
				return footprint
			}(),
			want: "units[0].pins[0]",
		},
		{
			name: "duplicate unit name",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.Units = []FootprintUnit{
					{Name: "A", Pins: []string{"1"}},
					{Name: "A", Pins: []string{"2"}},
				}
				return footprint
			}(),
			want: "units[1].name",
		},
		{
			name: "duplicate unit pin",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.Units = []FootprintUnit{{Name: "A", Pins: []string{"1", "1"}}}
				return footprint
			}(),
			want: "units[0].pins[1]",
		},
		{
			name: "empty net tie group",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.NetTiePadGroups = []string{" "}
				return footprint
			}(),
			want: "net_tie_pad_groups[0]",
		},
		{
			name: "duplicate net tie group",
			footprint: func() Footprint {
				footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
				footprint.NetTiePadGroups = []string{"1,2", "1,2"}
				return footprint
			}(),
			want: "net_tie_pad_groups[1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := minimalPCB()
			board.Nets = []Net{{Code: 1, Name: "A"}}
			board.Footprints = []Footprint{tt.footprint}

			err := Validate(board)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateRejectsFootprintPropertyWithoutUUID(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "R1")
	footprint.Texts = nil
	footprint.Properties = []FootprintProperty{
		{Name: "Reference", Value: "R1", Layer: kicadfiles.LayerFSilkS},
		{UUID: kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"), Name: "Value", Value: "value", Layer: kicadfiles.LayerFFab},
	}
	board.Footprints = []Footprint{footprint}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "properties[0].uuid") {
		t.Fatalf("error = %v", err)
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

func TestWriteRendersPadMetadata(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	removeUnused := false
	thermalAngle := 45.0
	footprint.Pads[0] = Pad{
		Name:               "1",
		Type:               "thru_hole",
		NetCode:            1,
		NetName:            "A",
		Shape:              "oval",
		Position:           point(0, 0),
		Size:               point(2, 2),
		Drill:              kicadfiles.MM(1),
		Layers:             []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask},
		RemoveUnusedLayers: &removeUnused,
		PinFunction:        "Pin_1",
		PinType:            "passive",
		ThermalBridgeAngle: &thermalAngle,
		Teardrops: &TeardropSettings{
			BestLengthRatio:      0.5,
			MaxLength:            kicadfiles.MM(1),
			BestWidthRatio:       1,
			MaxWidth:             kicadfiles.MM(2),
			FilterRatio:          0.9,
			Enabled:              true,
			AllowTwoSegments:     true,
			PreferZoneConnection: true,
		},
	}
	board.Footprints = []Footprint{footprint}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"thru_hole",
		"oval",
		"(remove_unused_layers no)",
		"(pinfunction \"Pin_1\")",
		"(pintype \"passive\")",
		"(thermal_bridge_angle 45)",
		"(teardrops",
		"(prefer_zone_connections yes)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteOmitsNetForUnconnectedPad(t *testing.T) {
	board := minimalPCB()
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].NetCode = 0
	board.Footprints = []Footprint{footprint}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if strings.Contains(buf.String(), "(net ") {
		t.Fatalf("unconnected pad should not render net:\n%s", buf.String())
	}
}

func TestValidateRejectsPadNetNameMismatch(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	footprint := minimalFootprint("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "J1")
	footprint.Pads[0].NetName = "B"
	board.Footprints = []Footprint{footprint}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pads[0].net_name") {
		t.Fatalf("error = %v", err)
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

func TestLayerNumberFallbackMatchesKiCad10InternalCopperIDs(t *testing.T) {
	layers := layerNumberMap(DefaultTwoLayerStack())

	tests := []struct {
		layer kicadfiles.BoardLayer
		want  int
	}{
		{layer: kicadfiles.LayerFCu, want: 0},
		{layer: kicadfiles.LayerBCu, want: 2},
		{layer: kicadfiles.BoardLayer("In1.Cu"), want: 4},
		{layer: kicadfiles.BoardLayer("In30.Cu"), want: 62},
	}

	for _, test := range tests {
		t.Run(string(test.layer), func(t *testing.T) {
			if got := layerNumber(test.layer, layers); got != test.want {
				t.Fatalf("layerNumber(%q) = %d, want %d", test.layer, got, test.want)
			}
		})
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

func TestWriteRendersTrackArcAndViaTenting(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.TrackArcs = []TrackArc{{
		UUID:    kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Start:   point(1, 1),
		Mid:     point(2, 2),
		End:     point(3, 1),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}
	board.Vias = []Via{{
		UUID:         kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
		Position:     point(2, 2),
		Size:         kicadfiles.MM(0.8),
		Drill:        kicadfiles.MM(0.4),
		NetCode:      1,
		Layers:       []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		TentingFront: true,
		TentingBack:  true,
	}}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(arc",
		"(mid 2 2)",
		"(tenting",
		"(front yes)",
		"(back yes)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteSortsRoutedItemsLikeKiCad(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{
		{Code: 1, Name: "GND"},
		{Code: 2, Name: "VCC"},
	}
	board.Tracks = []Track{
		{
			UUID:    kicadfiles.UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
			Start:   point(1, 1),
			End:     point(2, 1),
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 2,
		},
		{
			UUID:    kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Start:   point(1, 2),
			End:     point(2, 2),
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: 1,
		},
	}
	board.Vias = []Via{{
		UUID:     kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
		Position: point(2, 2),
		Size:     kicadfiles.MM(0.8),
		Drill:    kicadfiles.MM(0.4),
		NetCode:  1,
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	output := buf.String()
	gndSegment := strings.Index(output, "(net \"GND\")")
	via := strings.Index(output, "(via")
	vccSegment := strings.Index(output, "(net \"VCC\")")
	if gndSegment < 0 || via < 0 || vccSegment < 0 {
		t.Fatalf("expected routed items missing:\n%s", output)
	}
	if !(gndSegment < via && via < vccSegment) {
		t.Fatalf("routed items not in KiCad order:\n%s", output)
	}
}

func TestValidateRejectsInvalidTrackArc(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}}
	board.TrackArcs = []TrackArc{{
		UUID:    kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Start:   point(1, 1),
		Mid:     point(1, 1),
		End:     point(2, 1),
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: 1,
	}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "track_arcs[0].points") {
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

func TestValidateAcceptsClosedRectOutline(t *testing.T) {
	board := minimalPCB()
	board.RequireClosedOutline = true
	board.Drawings = []Drawing{{
		UUID:  kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Layer: kicadfiles.LayerEdge,
		Rect:  &RectDrawing{Start: point(0, 0), End: point(10, 10), Width: kicadfiles.MM(0.1)},
	}}

	if err := Validate(board); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsDanglingLineWithRectOutline(t *testing.T) {
	board := minimalPCB()
	board.RequireClosedOutline = true
	board.Drawings = []Drawing{
		{
			UUID:  kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Layer: kicadfiles.LayerEdge,
			Rect:  &RectDrawing{Start: point(0, 0), End: point(10, 10), Width: kicadfiles.MM(0.1)},
		},
		{
			UUID:  kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Layer: kicadfiles.LayerEdge,
			Line:  &LineDrawing{Start: point(20, 20), End: point(21, 20), Width: kicadfiles.MM(0.1)},
		},
	}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "drawings.edge_cuts") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteRendersRectTextAndCopperPoly(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "GND"}}
	board.Drawings = []Drawing{
		{
			UUID:       kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Layer:      kicadfiles.LayerEdge,
			StrokeType: "default",
			Fill:       "none",
			Rect:       &RectDrawing{Start: point(0, 0), End: point(10, 10), Width: kicadfiles.MM(0.1)},
		},
		{
			UUID:  kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			Layer: kicadfiles.LayerCmts,
			Text:  &TextDrawing{Text: "hello", Position: point(5, 5)},
		},
		{
			UUID:    kicadfiles.UUID("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
			Layer:   kicadfiles.LayerFCu,
			Fill:    "yes",
			NetCode: 1,
			NetName: "GND",
			Poly:    &PolylineDrawing{Points: []kicadfiles.Point{point(1, 1), point(2, 1), point(2, 2)}, Width: 0},
		},
	}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(gr_rect",
		"(type default)",
		"(gr_text",
		"\"hello\"",
		"(gr_poly",
		"(fill yes)",
		"(net 1)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
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

func TestWriteRendersRichZone(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "GND"}}
	board.Zones = []Zone{{
		UUID:                 kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		NetCode:              1,
		NetName:              "GND",
		Name:                 "GND pour",
		Layers:               []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
		HatchStyle:           "full",
		HatchPitch:           kicadfiles.MM(0.1),
		Priority:             2,
		ConnectPads:          true,
		Clearance:            kicadfiles.MM(0.2),
		MinThickness:         kicadfiles.MM(0.25),
		FilledAreasThickness: false,
		Fill: ZoneFillSettings{
			Enabled:            true,
			ThermalGap:         kicadfiles.MM(0.5),
			ThermalBridgeWidth: kicadfiles.MM(0.5),
			IslandRemovalMode:  1,
			IslandAreaMin:      10,
		},
		Attributes: []ZoneAttribute{{Name: "teardrop", Values: map[string]string{"type": "padvia"}}},
		Polygons: [][]kicadfiles.Point{{
			point(0, 0), point(10, 0), point(10, 10), point(0, 10),
		}},
		FilledPolygons: []ZoneFilledPolygon{{
			Layer:  kicadfiles.LayerFCu,
			Points: []kicadfiles.Point{point(1, 1), point(9, 1), point(9, 9), point(1, 9)},
		}},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(name \"GND pour\")",
		"(hatch full 0.1)",
		"(connect_pads",
		"yes",
		"(min_thickness 0.25)",
		"(fill",
		"(thermal_gap 0.5)",
		"(attr",
		"(teardrop",
		"\"padvia\"",
		"(filled_polygon",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
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
			{UUID: kicadfiles.UUID("dddddddd-dddd-4ddd-8ddd-dddddddddddd"), Kind: "reference", Text: reference, Layer: kicadfiles.LayerFSilkS},
			{UUID: kicadfiles.UUID("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"), Kind: "value", Text: "value", Layer: kicadfiles.LayerFSilkS},
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
