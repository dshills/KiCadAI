package designapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/routing"
)

func TestBuilderCreatesValidDesignFromIntent(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "D1", "Device:LED", "LED", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})

	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "D1", Pin: "1"}, "LED_OUT"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint R1 returned error: %v", err)
	}
	if err := builder.AssignFootprint("D1", "LED_SMD:LED_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint D1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Position: kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(25)},
		Pads: []PadSpec{
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-1), Y: 0}},
			{Name: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(1), Y: 0}, Net: "LED_OUT"},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint R1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("D1", PlaceFootprintOptions{
		Position: kicadfiles.Point{X: kicadfiles.MM(45), Y: kicadfiles.MM(25)},
		Pads: []PadSpec{
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-1), Y: 0}, Net: "LED_OUT"},
			{Name: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(1), Y: 0}},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint D1 returned error: %v", err)
	}
	route, err := builder.Route("LED_OUT", []kicadfiles.Point{
		{X: kicadfiles.MM(26), Y: kicadfiles.MM(25)},
		{X: kicadfiles.MM(44), Y: kicadfiles.MM(25)},
	}, RouteOptions{})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if route.Count != 1 {
		t.Fatalf("route count = %d, want 1", route.Count)
	}
	if _, err := builder.AddZone("LED_OUT", []kicadfiles.Point{
		{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		{X: kicadfiles.MM(60), Y: kicadfiles.MM(10)},
		{X: kicadfiles.MM(60), Y: kicadfiles.MM(40)},
		{X: kicadfiles.MM(10), Y: kicadfiles.MM(40)},
	}, ZoneOptions{ConnectPads: true}); err != nil {
		t.Fatalf("AddZone returned error: %v", err)
	}

	design := builder.Design()
	if err := kicaddesign.Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if len(design.Schematic.Wires) != 1 {
		t.Fatalf("schematic wires = %d, want 1", len(design.Schematic.Wires))
	}
	if len(design.PCB.Footprints) != 2 {
		t.Fatalf("footprints = %d, want 2", len(design.PCB.Footprints))
	}
	if len(design.PCB.Tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(design.PCB.Tracks))
	}
	if len(design.PCB.Zones) != 1 {
		t.Fatalf("zones = %d, want 1", len(design.PCB.Zones))
	}
	assertPadNet(t, design.PCB.Footprints, "R1", "2", "LED_OUT")
	assertPadNet(t, design.PCB.Footprints, "D1", "1", "LED_OUT")
}

func TestBuilderConnectUsesOrthogonalSchematicWire(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(32)})

	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "SIG"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if got := len(builder.design.Schematic.Wires); got != 2 {
		t.Fatalf("wire count = %d, want 2 orthogonal segments", got)
	}
	if got := len(builder.design.Schematic.Junctions); got != 1 {
		t.Fatalf("junction count = %d, want 1 dogleg bend junction", got)
	}
	if got := len(builder.design.Schematic.Labels); got != 1 {
		t.Fatalf("label count = %d, want 1 dogleg bend label", got)
	}
	wantSegments := [][2]kicadfiles.Point{
		{
			{X: kicadfiles.MM(25), Y: kicadfiles.MM(20)},
			{X: kicadfiles.MM(25), Y: kicadfiles.MM(32)},
		},
		{
			{X: kicadfiles.MM(25), Y: kicadfiles.MM(32)},
			{X: kicadfiles.MM(35), Y: kicadfiles.MM(32)},
		},
	}
	for wireIndex, wire := range builder.design.Schematic.Wires {
		if len(wire.Points) != 2 {
			t.Fatalf("wire %d points = %#v, want 2-point KiCad segment", wireIndex, wire.Points)
		}
		if wire.Points[0] != wantSegments[wireIndex][0] || wire.Points[1] != wantSegments[wireIndex][1] {
			t.Fatalf("wire %d points = %#v, want %#v", wireIndex, wire.Points, wantSegments[wireIndex])
		}
		if wire.Points[0].X != wire.Points[1].X && wire.Points[0].Y != wire.Points[1].Y {
			t.Fatalf("wire %d is diagonal: %#v", wireIndex, wire.Points)
		}
	}
	if label := builder.design.Schematic.Labels[0]; label.Text != "SIG" || label.Position != (kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(32)}) {
		t.Fatalf("dogleg label = %#v, want SIG at bend", label)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "SIG"); err != nil {
		t.Fatalf("duplicate Connect returned error: %v", err)
	}
	if got := len(builder.design.Schematic.Wires); got != 2 {
		t.Fatalf("duplicate connect added wire count = %d, want 2", got)
	}
	if got := len(builder.design.Schematic.Junctions); got != 1 {
		t.Fatalf("duplicate connect added junction count = %d, want 1", got)
	}
	if got := len(builder.design.Schematic.Labels); got != 1 {
		t.Fatalf("duplicate connect added label count = %d, want 1", got)
	}
}

func TestBuilderDefaultPaperNamePreservesCustomSize(t *testing.T) {
	builder, err := New(Options{
		Name:     "custom_paper",
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "custom-paper",
		Paper: kicadfiles.Paper{
			Width:  kicadfiles.MM(100),
			Height: kicadfiles.MM(80),
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	design := builder.Design()
	if design.Project.PageSettings.Paper.Name != "A4" {
		t.Fatalf("paper name = %q, want A4", design.Project.PageSettings.Paper.Name)
	}
	if design.Project.PageSettings.Paper.Width != kicadfiles.MM(100) || design.Project.PageSettings.Paper.Height != kicadfiles.MM(80) {
		t.Fatalf("paper size = %+v", design.Project.PageSettings.Paper)
	}
}

func TestBuilderUpdatesPlacedFootprintWhenConnectedLater(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "D1", "Device:LED", "LED", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint R1 returned error: %v", err)
	}
	if err := builder.AssignFootprint("D1", "LED_SMD:LED_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint D1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{}); err != nil {
		t.Fatalf("PlaceFootprint R1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("D1", PlaceFootprintOptions{}); err != nil {
		t.Fatalf("PlaceFootprint D1 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "D1", Pin: "1"}, "LATE_NET"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	design := builder.Design()
	assertPadNet(t, design.PCB.Footprints, "R1", "2", "LATE_NET")
	assertPadNet(t, design.PCB.Footprints, "D1", "1", "LATE_NET")
}

func TestBuilderCustomPadsInheritSchematicNets(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "D1", "Device:LED", "LED", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "D1", Pin: "1"}, "LED_OUT"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-1), Y: 0}},
			{Name: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(1), Y: 0}},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	design := builder.Design()
	assertPadNet(t, design.PCB.Footprints, "R1", "2", "LED_OUT")
}

func TestBuilderPlaceFootprintAcceptsLibraryGeometry(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Description:        "library resistor",
		Tags:               "resistor 0805",
		Attributes:         []string{"smd"},
		MetadataProperties: []pcb.FootprintMetadataProperty{{Name: "ki_description", Value: "library resistor"}},
		Texts: []pcb.FootprintText{
			{Kind: "user", Text: "LIB", Layer: kicadfiles.LayerFSilkS},
		},
		Graphics: []pcb.FootprintGraphic{
			pcb.FootprintGraphic(pcb.Drawing{
				Kind:  "line",
				Layer: kicadfiles.LayerFSilkS,
				Line:  &pcb.LineDrawing{Start: kicadfiles.Point{}, End: kicadfiles.Point{X: kicadfiles.MM(1)}, Width: kicadfiles.MM(0.12)},
			}),
		},
		Models: []pcb.Model3D{{Path: "${KICAD9_3DMODEL_DIR}/Resistor_SMD.3dshapes/R_0805.wrl"}},
		Pads: []PadSpec{
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-1)}},
			{Name: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(1)}},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	footprint := builder.Design().PCB.Footprints[0]
	if footprint.Description != "library resistor" || footprint.Tags != "resistor 0805" {
		t.Fatalf("metadata not preserved: %#v", footprint)
	}
	if len(footprint.Texts) != 1 || !footprint.Texts[0].UUID.Valid() {
		t.Fatalf("texts not preserved with UUIDs: %#v", footprint.Texts)
	}
	if len(footprint.Graphics) != 1 || !pcb.Drawing(footprint.Graphics[0]).UUID.Valid() {
		t.Fatalf("graphics not preserved with UUIDs: %#v", footprint.Graphics)
	}
	if len(footprint.Models) != 1 || footprint.Models[0].Path == "" {
		t.Fatalf("models not preserved: %#v", footprint.Models)
	}
}

func TestBuilderAllowsUnconnectedCustomPadWithoutSymbolPin(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}

	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1"},
			{Name: "2"},
			{Name: "9"},
		},
		AllowUnmatchedUnconnectedPads: true,
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error for unconnected package-only pad: %v", err)
	}
}

func TestBuilderRejectsUnconnectedCustomPadWithoutSymbolPinByDefault(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}

	_, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1"},
			{Name: "2"},
			{Name: "9"},
		},
	})
	if err == nil {
		t.Fatal("expected unknown pad error")
	}
	if !strings.Contains(err.Error(), "does not match a symbol pin") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuilderRejectsNettedCustomPadWithoutSymbolPin(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}

	_, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1"},
			{Name: "2"},
			{Name: "9", Net: "SIG"},
		},
		AllowUnmatchedUnconnectedPads: true,
	})
	if err == nil {
		t.Fatal("expected netted unknown pad error")
	}
	if !strings.Contains(err.Error(), "does not match a symbol pin") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuilderAllowsDuplicateCustomPadsForOneSymbolPin(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint R1 returned error: %v", err)
	}
	if err := builder.AssignFootprint("R2", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint R2 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1"},
			{Name: " 1 "},
			{Name: "2"},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint R1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R2", PlaceFootprintOptions{}); err != nil {
		t.Fatalf("PlaceFootprint R2 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "1"}, Endpoint{Reference: "R2", Pin: "1"}, "LATE_NET"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	assertPadsNet(t, design.PCB.Footprints, "R1", "1", "LATE_NET", 2)
}

func TestBuilderRejectsMissingCustomPadForSymbolPin(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}

	_, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{{Name: "1"}},
	})
	if err == nil {
		t.Fatal("expected missing pad error")
	}
	if !strings.Contains(err.Error(), "missing pad for symbol pin 2") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuilderPlaceFootprintUsesSideAwareDefaultsAndAttributes(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "J1", "Connector:Conn_01x02_Pin", "Conn", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("J1", "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("J1", PlaceFootprintOptions{
		Layer:      kicadfiles.LayerBCu,
		Attributes: []string{"smd"},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	design := builder.Design()
	if len(design.PCB.Footprints) != 1 {
		t.Fatalf("footprints = %d, want 1", len(design.PCB.Footprints))
	}
	footprint := design.PCB.Footprints[0]
	if len(footprint.Attributes) != 1 || footprint.Attributes[0] != "smd" {
		t.Fatalf("attributes = %+v", footprint.Attributes)
	}
	for _, pad := range footprint.Pads {
		if pad.Type != "smd" {
			t.Fatalf("pad type = %q, want smd", pad.Type)
		}
		if !containsLayer(pad.Layers, kicadfiles.LayerBCu) || !containsLayer(pad.Layers, kicadfiles.LayerBMask) || !containsLayer(pad.Layers, kicadfiles.LayerBPaste) {
			t.Fatalf("back-side pad layers = %+v", pad.Layers)
		}
		if containsLayer(pad.Layers, kicadfiles.LayerFCu) {
			t.Fatalf("back-side pad contains front copper: %+v", pad.Layers)
		}
	}
}

func TestBuilderPlaceFootprintSupportsThroughHoleDefaults(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "J1", "Connector:Conn_01x02_Pin", "Conn", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("J1", "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("J1", PlaceFootprintOptions{
		Attributes: []string{"through_hole"},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	design := builder.Design()
	footprint := design.PCB.Footprints[0]
	if len(footprint.Attributes) != 1 || footprint.Attributes[0] != "through_hole" {
		t.Fatalf("attributes = %+v", footprint.Attributes)
	}
	for _, pad := range footprint.Pads {
		if pad.Type != "thru_hole" {
			t.Fatalf("pad type = %q, want thru_hole", pad.Type)
		}
		if pad.Drill != kicadfiles.MM(0.8) {
			t.Fatalf("pad drill = %d, want %d", pad.Drill, kicadfiles.MM(0.8))
		}
		if !containsLayer(pad.Layers, kicadfiles.LayerAllCu) || !containsLayer(pad.Layers, kicadfiles.LayerAllMask) {
			t.Fatalf("through-hole pad layers = %+v", pad.Layers)
		}
	}
}

func TestBuilderReplacesExistingFootprintInPlace(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "D1", "Device:LED", "LED", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint R1 returned error: %v", err)
	}
	if err := builder.AssignFootprint("D1", "LED_SMD:LED_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint D1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}}); err != nil {
		t.Fatalf("PlaceFootprint R1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("D1", PlaceFootprintOptions{Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}}); err != nil {
		t.Fatalf("PlaceFootprint D1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Position: kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(30)},
		Pads: []PadSpec{
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-1), Y: 0}},
			{Name: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(1), Y: 0}, Net: "LED_OUT"},
		},
	}); err != nil {
		t.Fatalf("second PlaceFootprint R1 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "D1", Pin: "1"}, "LED_OUT"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.PCB.Footprints) != 2 {
		t.Fatalf("footprints = %d, want 2", len(design.PCB.Footprints))
	}
	if design.PCB.Footprints[0].Reference != "R1" {
		t.Fatalf("first footprint reference = %q, want R1", design.PCB.Footprints[0].Reference)
	}
	if design.PCB.Footprints[0].Position != (kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(30)}) {
		t.Fatalf("R1 position = %+v", design.PCB.Footprints[0].Position)
	}
	assertPadNet(t, design.PCB.Footprints, "R1", "2", "LED_OUT")
	assertPadNet(t, design.PCB.Footprints, "D1", "1", "LED_OUT")
}

func TestBuilderDesignReturnsMutationSafeSnapshot(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	snapshot := builder.Design()
	originalPropertyValue := snapshot.Schematic.Symbols[0].Properties[0].Value
	snapshot.Schematic.Symbols[0].Reference = "BROKEN"
	snapshot.Schematic.Symbols[0].Properties[0].Value = "BROKEN"
	snapshot.PCB.Footprints[0].Reference = "BROKEN"
	snapshot.PCB.Footprints[0].Pads[0].Layers[0] = kicadfiles.LayerBCu
	snapshot.KnownSymbolLibraries[0] = "BROKEN"

	fresh := builder.Design()
	if fresh.Schematic.Symbols[0].Reference != "R1" {
		t.Fatalf("schematic reference mutated to %q", fresh.Schematic.Symbols[0].Reference)
	}
	if fresh.Schematic.Symbols[0].Properties[0].Value != originalPropertyValue {
		t.Fatalf("schematic property mutated to %q", fresh.Schematic.Symbols[0].Properties[0].Value)
	}
	if fresh.PCB.Footprints[0].Reference != "R1" {
		t.Fatalf("footprint reference mutated to %q", fresh.PCB.Footprints[0].Reference)
	}
	if fresh.PCB.Footprints[0].Pads[0].Layers[0] != kicadfiles.LayerFCu {
		t.Fatalf("pad layers mutated to %+v", fresh.PCB.Footprints[0].Pads[0].Layers)
	}
	if fresh.KnownSymbolLibraries[0] != "Device" {
		t.Fatalf("known symbol libraries mutated to %+v", fresh.KnownSymbolLibraries)
	}
}

func TestBuilderAddSymbolClonesInputProperties(t *testing.T) {
	builder := newTestBuilder(t)
	showName := false
	properties := []schematic.Property{{
		Name:     "Manufacturer",
		Value:    "Yageo",
		Hidden:   true,
		ShowName: &showName,
	}}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference:  "R1",
		Value:      "10k",
		LibraryID:  "Device:R",
		Position:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		Properties: properties,
	}); err != nil {
		t.Fatalf("AddSymbol returned error: %v", err)
	}
	properties[0].Value = "BROKEN"
	*properties[0].ShowName = true

	fresh := builder.Design()
	got := fresh.Schematic.Symbols[0].Properties[0]
	if got.Value != "Yageo" || got.ShowName == nil || *got.ShowName {
		t.Fatalf("symbol property shares input backing data: %#v", got)
	}
}

func TestBuilderAddSymbolEmbedsSupportedSymbolTemplates(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "C1", "Device:C", "100n", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "X1", "Custom:Block", "Block", kicadfiles.Point{X: kicadfiles.MM(60), Y: kicadfiles.MM(20)})

	design := builder.Design()
	if len(design.Schematic.LibSymbols) != 2 {
		t.Fatalf("lib symbols = %d, want 2: %#v", len(design.Schematic.LibSymbols), design.Schematic.LibSymbols)
	}
	seen := map[string]bool{}
	for _, symbol := range design.Schematic.LibSymbols {
		seen[strings.ToLower(symbol.LibraryID)] = len(symbol.Body) > 0
	}
	if !seen["device:r"] || !seen["device:c"] {
		t.Fatalf("missing embedded templates: %#v", seen)
	}
	if seen["custom:block"] {
		t.Fatalf("unsupported custom symbol should not be embedded: %#v", seen)
	}
}

func TestBuilderAddSymbolDerivesPinsFromEmbeddedTemplate(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "R1",
		LibraryID: "Device:R",
		Value:     "1k",
		Position:  kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
	}); err != nil {
		t.Fatalf("AddSymbol R1 returned error: %v", err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "C1",
		LibraryID: "Device:C",
		Value:     "100n",
		Position:  kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)},
	}); err != nil {
		t.Fatalf("AddSymbol C1 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "C1", Pin: "1"}, "FILTER"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Symbols[0].Pins) != 2 || len(design.Schematic.Symbols[1].Pins) != 2 {
		t.Fatalf("template pins not rendered: %#v", design.Schematic.Symbols)
	}
	if len(design.Schematic.Wires) != 2 {
		t.Fatalf("wires = %d, want 2", len(design.Schematic.Wires))
	}
	if len(design.Schematic.Labels) != 1 {
		t.Fatalf("labels = %d, want 1 dogleg label", len(design.Schematic.Labels))
	}
	if len(design.Schematic.Junctions) != 1 {
		t.Fatalf("junctions = %d, want 1 dogleg junction", len(design.Schematic.Junctions))
	}
	assertSchematicPinNet(t, builder, Endpoint{Reference: "R1", Pin: "2"}, "FILTER")
	assertSchematicPinNet(t, builder, Endpoint{Reference: "C1", Pin: "1"}, "FILTER")
}

func TestBuilderAvoidsRouteAndZoneUUIDCollisions(t *testing.T) {
	builder := newTestBuilder(t)
	points := []kicadfiles.Point{
		{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}
	if _, err := builder.Route("POWER", points, RouteOptions{Layer: kicadfiles.LayerFCu}); err != nil {
		t.Fatalf("front Route returned error: %v", err)
	}
	if _, err := builder.Route("POWER", points, RouteOptions{Layer: kicadfiles.LayerBCu}); err != nil {
		t.Fatalf("back Route returned error: %v", err)
	}
	polygon := []kicadfiles.Point{
		{X: kicadfiles.MM(5), Y: kicadfiles.MM(5)},
		{X: kicadfiles.MM(25), Y: kicadfiles.MM(5)},
		{X: kicadfiles.MM(25), Y: kicadfiles.MM(25)},
		{X: kicadfiles.MM(5), Y: kicadfiles.MM(25)},
	}
	if _, err := builder.AddZone("GND", polygon, ZoneOptions{Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}}); err != nil {
		t.Fatalf("front AddZone returned error: %v", err)
	}
	if _, err := builder.AddZone("GND", polygon, ZoneOptions{Layers: []kicadfiles.BoardLayer{kicadfiles.LayerBCu}}); err != nil {
		t.Fatalf("back AddZone returned error: %v", err)
	}

	design := builder.Design()
	if len(design.PCB.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(design.PCB.Tracks))
	}
	if design.PCB.Tracks[0].UUID == design.PCB.Tracks[1].UUID {
		t.Fatalf("duplicate route UUID %s", design.PCB.Tracks[0].UUID)
	}
	if len(design.PCB.Zones) != 2 {
		t.Fatalf("zones = %d, want 2", len(design.PCB.Zones))
	}
	if design.PCB.Zones[0].UUID == design.PCB.Zones[1].UUID {
		t.Fatalf("duplicate zone UUID %s", design.PCB.Zones[0].UUID)
	}
}

func TestBuilderSetRectangularBoardOutline(t *testing.T) {
	builder := newTestBuilder(t)
	handle, err := builder.SetRectangularBoardOutline(kicadfiles.MM(50), kicadfiles.MM(30))
	if err != nil {
		t.Fatalf("SetRectangularBoardOutline returned error: %v", err)
	}
	if handle.SegmentCount != 4 {
		t.Fatalf("segments = %d, want 4", handle.SegmentCount)
	}
	design := builder.Design()
	if !design.PCB.RequireClosedOutline {
		t.Fatal("expected closed outline validation to be required")
	}
	if len(design.PCB.Drawings) != 4 {
		t.Fatalf("drawings = %d, want 4", len(design.PCB.Drawings))
	}
	for _, drawing := range design.PCB.Drawings {
		if drawing.Layer != kicadfiles.LayerEdge || drawing.Line == nil {
			t.Fatalf("unexpected outline drawing: %#v", drawing)
		}
	}
}

func TestBuilderWriteProject(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "D1", "Device:LED", "LED", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "D1", Pin: "1"}, "LED_OUT"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint R1 returned error: %v", err)
	}
	if err := builder.AssignFootprint("D1", "LED_SMD:LED_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint D1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{}); err != nil {
		t.Fatalf("PlaceFootprint R1 returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("D1", PlaceFootprintOptions{}); err != nil {
		t.Fatalf("PlaceFootprint D1 returned error: %v", err)
	}

	root := filepath.Join(t.TempDir(), "intent_demo")
	result, err := builder.WriteProject(root, kicaddesign.WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProject returned error: %v", err)
	}
	for _, name := range []string{"intent_demo.kicad_pro", "intent_demo.kicad_sch", "intent_demo.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(result.ProjectDir, name)); err != nil {
			t.Fatalf("expected written file %s: %v", name, err)
		}
	}
}

func TestBuilderRouteBoardAddsPCBTracks(t *testing.T) {
	builder := newTestBuilder(t)
	result, err := builder.RouteBoard(designAPIRoutingRequest(), RouteBoardOptions{})
	if err != nil {
		t.Fatalf("RouteBoard returned error: %v", err)
	}
	if result.Status != routing.StatusRouted {
		t.Fatalf("status = %s issues = %#v", result.Status, result.Issues)
	}
	if len(builder.Design().PCB.Tracks) == 0 {
		t.Fatalf("tracks = %#v, want routed tracks", builder.Design().PCB.Tracks)
	}
}

func TestBuilderRouteBoardDryRunDoesNotMutatePCB(t *testing.T) {
	builder := newTestBuilder(t)
	result, err := builder.RouteBoard(designAPIRoutingRequest(), RouteBoardOptions{DryRun: true})
	if err != nil {
		t.Fatalf("RouteBoard returned error: %v", err)
	}
	if result.Status != routing.StatusRouted {
		t.Fatalf("status = %s issues = %#v", result.Status, result.Issues)
	}
	if len(builder.Design().PCB.Tracks) != 0 {
		t.Fatalf("tracks = %#v, want dry-run to leave PCB untouched", builder.Design().PCB.Tracks)
	}
}

func designAPIRoutingRequest() routing.Request {
	return routing.Request{
		Board: routing.Board{WidthMM: 30, HeightMM: 20, Layers: []routing.Layer{{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true}}},
		Components: []routing.Component{
			{Ref: "J1", Position: routing.Placement{XMM: 5, YMM: 10, Layer: "F.Cu"}, Pads: []routing.Pad{{Name: "1", Net: "SIG", Shape: routing.PadCircle, Type: routing.PadSMD, Size: routing.Size{WidthMM: 1, HeightMM: 1}, Layers: []string{"F.Cu"}}}},
			{Ref: "J2", Position: routing.Placement{XMM: 20, YMM: 10, Layer: "F.Cu"}, Pads: []routing.Pad{{Name: "1", Net: "SIG", Shape: routing.PadCircle, Type: routing.PadSMD, Size: routing.Size{WidthMM: 1, HeightMM: 1}, Layers: []string{"F.Cu"}}}},
		},
		Nets:     []routing.Net{{Name: "SIG", Endpoints: []routing.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "J2", Pin: "1"}}}},
		Rules:    routing.Rules{GridMM: 1, TraceWidthMM: 0.1, ClearanceMM: 0.01, EdgeClearanceMM: 0.01},
		Strategy: routing.Strategy{Mode: routing.ModeSingleLayer},
	}
}

func TestBuilderRejectsInvalidOperations(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.PlaceFootprint("R404", PlaceFootprintOptions{}); err == nil {
		t.Fatal("expected unknown symbol error")
	}
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "1"}, Endpoint{Reference: "R1", Pin: "9"}, "BAD"); err == nil {
		t.Fatal("expected unknown pin error")
	}
	if _, err := builder.Route("BAD", []kicadfiles.Point{{}}, RouteOptions{}); err == nil {
		t.Fatal("expected invalid route error")
	}
	if _, err := builder.AddZone("BAD", []kicadfiles.Point{{}, {}}, ZoneOptions{}); err == nil {
		t.Fatal("expected invalid zone error")
	}
}

func TestBuilderRejectsReferenceKeyCollisions(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "U/1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "U\\1",
		LibraryID: "Device:R",
		Value:     "1k",
		Position:  kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(20)},
		Pins:      []PinSpec{{Number: "1"}, {Number: "2"}},
	}); err == nil {
		t.Fatal("expected normalized reference collision error")
	}
}

func TestBuilderMergesConflictingPinNetAssignments(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "R3", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(60), Y: kicadfiles.MM(20)})
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "1"}, Endpoint{Reference: "R2", Pin: "1"}, "NET_A"); err != nil {
		t.Fatalf("Connect NET_A returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R3", Pin: "1"}, "NET_B"); err != nil {
		t.Fatalf("Connect NET_B returned error: %v", err)
	}
	for _, reference := range []string{"R1", "R2", "R3"} {
		if err := builder.AssignFootprint(reference, "Resistor_SMD:R_0805_2012Metric"); err != nil {
			t.Fatalf("AssignFootprint %s returned error: %v", reference, err)
		}
		if _, err := builder.PlaceFootprint(reference, PlaceFootprintOptions{}); err != nil {
			t.Fatalf("PlaceFootprint %s returned error: %v", reference, err)
		}
	}
	if _, err := builder.Route("NET_B", []kicadfiles.Point{
		{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}, RouteOptions{}); err != nil {
		t.Fatalf("Route NET_B returned error: %v", err)
	}
	if _, err := builder.AddZone("NET_B", []kicadfiles.Point{
		{X: kicadfiles.MM(5), Y: kicadfiles.MM(5)},
		{X: kicadfiles.MM(25), Y: kicadfiles.MM(5)},
		{X: kicadfiles.MM(25), Y: kicadfiles.MM(25)},
		{X: kicadfiles.MM(5), Y: kicadfiles.MM(25)},
	}, ZoneOptions{}); err != nil {
		t.Fatalf("AddZone NET_B returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R2", Pin: "1"}, Endpoint{Reference: "R3", Pin: "1"}, ""); err != nil {
		t.Fatalf("Connect merged nets returned error: %v", err)
	}

	design := builder.Design()
	assertPadNet(t, design.PCB.Footprints, "R3", "1", "NET_A")
	if design.PCB.Tracks[0].NetName != "NET_A" {
		t.Fatalf("track net = %q, want NET_A", design.PCB.Tracks[0].NetName)
	}
	if design.PCB.Zones[0].NetName != "NET_A" {
		t.Fatalf("zone net = %q, want NET_A", design.PCB.Zones[0].NetName)
	}
	for _, netName := range design.ExpectedNets {
		if netName == "NET_B" {
			t.Fatalf("expected nets still contains merged net: %+v", design.ExpectedNets)
		}
	}
}

func TestBuilderTreatsNetNamesAsCaseSensitive(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "R3", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(60), Y: kicadfiles.MM(20)})
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "1"}, Endpoint{Reference: "R2", Pin: "1"}, "sig"); err != nil {
		t.Fatalf("Connect sig returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R3", Pin: "1"}, "SIG"); err != nil {
		t.Fatalf("Connect SIG returned error: %v", err)
	}

	design := builder.Design()
	if !containsStringExact(design.ExpectedNets, "sig") || !containsStringExact(design.ExpectedNets, "SIG") {
		t.Fatalf("expected case-distinct nets, got %+v", design.ExpectedNets)
	}
}

func TestBuilderRepeatedConnectionsDoNotDuplicateWires(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(20)})
	endpointA := Endpoint{Reference: "R1", Pin: "1"}
	endpointB := Endpoint{Reference: "R2", Pin: "1"}
	if err := builder.Connect(endpointA, endpointB, "SIG"); err != nil {
		t.Fatalf("first Connect returned error: %v", err)
	}
	if err := builder.Connect(endpointB, endpointA, "SIG"); err != nil {
		t.Fatalf("second Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Wires) != 1 {
		t.Fatalf("wires = %d, want 1", len(design.Schematic.Wires))
	}
	if net := builder.assignedPinNet(endpointA); net != "SIG" {
		t.Fatalf("R1 pin 1 net = %q, want SIG", net)
	}
	if net := builder.assignedPinNet(endpointB); net != "SIG" {
		t.Fatalf("R2 pin 1 net = %q, want SIG", net)
	}
}

func newTestBuilder(t *testing.T) *Builder {
	t.Helper()
	builder, err := New(Options{
		Name:     "intent_demo",
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "intent-test",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return builder
}

func addTwoPinSymbol(t *testing.T, builder *Builder, reference, libraryID, value string, position kicadfiles.Point) {
	t.Helper()
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: reference,
		LibraryID: libraryID,
		Value:     value,
		Position:  position,
		Pins: []PinSpec{
			{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-5), Y: 0}},
			{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(5), Y: 0}},
		},
	}); err != nil {
		t.Fatalf("AddSymbol %s returned error: %v", reference, err)
	}
}

func assertSchematicPinNet(t *testing.T, builder *Builder, endpoint Endpoint, netName string) {
	t.Helper()
	if net := builder.assignedPinNet(endpoint); net != netName {
		t.Fatalf("%s pin %s net = %q, want %q", endpoint.Reference, endpoint.Pin, net, netName)
	}
}

func assertPadNet(t *testing.T, footprints []pcb.Footprint, reference, padName, netName string) {
	t.Helper()
	for _, footprint := range footprints {
		if footprint.Reference != reference {
			continue
		}
		for _, pad := range footprint.Pads {
			if pad.Name == padName {
				if pad.NetName != netName {
					t.Fatalf("%s pad %s net = %q, want %q", reference, padName, pad.NetName, netName)
				}
				return
			}
		}
		t.Fatalf("footprint %s missing pad %s", reference, padName)
	}
	t.Fatalf("missing footprint %s", reference)
}

func assertPadsNet(t *testing.T, footprints []pcb.Footprint, reference, padName, netName string, wantCount int) {
	t.Helper()
	for _, footprint := range footprints {
		if footprint.Reference != reference {
			continue
		}
		count := 0
		for _, pad := range footprint.Pads {
			if pad.Name != padName {
				continue
			}
			count++
			if pad.NetName != netName {
				t.Fatalf("%s pad %s net = %q, want %q", reference, padName, pad.NetName, netName)
			}
		}
		if count != wantCount {
			t.Fatalf("%s pad %s count = %d, want %d", reference, padName, count, wantCount)
		}
		return
	}
	t.Fatalf("missing footprint %s", reference)
}

func containsLayer(layers []kicadfiles.BoardLayer, want kicadfiles.BoardLayer) bool {
	for _, layer := range layers {
		if layer == want {
			return true
		}
	}
	return false
}

func containsStringExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestReferenceKeySanitizesDesignators(t *testing.T) {
	got := referenceKey(" U:1/A ")
	if !strings.Contains(got, "u_1_a") {
		t.Fatalf("referenceKey = %q", got)
	}
}
