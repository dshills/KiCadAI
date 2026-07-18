package designapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
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
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-1), Y: 0}, Rotation: 90},
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
	if design.PCB.Footprints[0].Pads[0].Rotation != 90 {
		t.Fatalf("pad rotation = %v, want 90", design.PCB.Footprints[0].Pads[0].Rotation)
	}
	for _, property := range design.PCB.Footprints[0].Properties {
		if (property.Name == "Reference" || property.Name == "Value") && property.Hide {
			t.Fatalf("direct builder default property %s should stay visible unless explicitly hidden: %#v", property.Name, property)
		}
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

func TestBuilderEmitsNativeVectorBusGeometry(t *testing.T) {
	builder := newTestBuilder(t)
	busPoints := []kicadfiles.Point{{X: kicadfiles.MM(20), Y: kicadfiles.MM(50)}, {X: kicadfiles.MM(80), Y: kicadfiles.MM(50)}}
	if err := builder.AddBus(busPoints); err != nil {
		t.Fatalf("AddBus returned error: %v", err)
	}
	entryAt := kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(50)}
	entrySize := kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(2.54)}
	if err := builder.AddBusEntry(entryAt, entrySize); err != nil {
		t.Fatalf("AddBusEntry returned error: %v", err)
	}
	labelAt := kicadfiles.Point{X: entryAt.X + entrySize.X, Y: entryAt.Y + entrySize.Y}
	if err := builder.AddSchematicWireWithLabel("DATA0", []kicadfiles.Point{
		{X: kicadfiles.MM(10), Y: labelAt.Y},
		labelAt,
	}, "DATA0", &labelAt, 0); err != nil {
		t.Fatalf("AddSchematicWireWithLabel returned error: %v", err)
	}
	design := builder.Design()
	if err := kicaddesign.Validate(design); err != nil {
		t.Fatalf("design validation failed: %v", err)
	}
	if len(design.Schematic.Buses) != 1 || len(design.Schematic.BusEntries) != 1 || len(design.Schematic.Wires) != 1 {
		t.Fatalf("native vector bus geometry = buses:%d entries:%d wires:%d", len(design.Schematic.Buses), len(design.Schematic.BusEntries), len(design.Schematic.Wires))
	}
	if len(design.Schematic.Labels) != 1 || design.Schematic.Labels[0].Text != "DATA0" {
		t.Fatalf("member labels = %#v", design.Schematic.Labels)
	}
	root := filepath.Join(t.TempDir(), "vector_bus")
	if _, err := builder.WriteSchematicProject(root, kicaddesign.WriteOptions{}); err != nil {
		t.Fatalf("WriteSchematicProject returned error: %v", err)
	}
	read, err := schematic.ReadFile(filepath.Join(root, "intent_demo.kicad_sch"))
	if err != nil {
		t.Fatalf("read written vector bus schematic: %v", err)
	}
	if len(read.Buses) != 1 || len(read.BusEntries) != 1 {
		t.Fatalf("written vector bus geometry = buses:%d entries:%d", len(read.Buses), len(read.BusEntries))
	}
}

func TestBuilderSupportsUnitAwareSharedReferences(t *testing.T) {
	builder := newTestBuilder(t)
	for unit, x := range map[int]float64{1: 20, 2: 40} {
		if _, err := builder.AddSymbol(SymbolOptions{
			Reference: "U1",
			Unit:      unit,
			LibraryID: "Device:R",
			Value:     "DUAL",
			Position:  kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(30)},
			Pins: []PinSpec{
				{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-5)}},
				{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(5)}},
			},
		}); err != nil {
			t.Fatalf("AddSymbol U1 unit %d returned error: %v", unit, err)
		}
	}
	if err := builder.Connect(Endpoint{Reference: "U1", Unit: 1, Pin: "2"}, Endpoint{Reference: "U1", Unit: 2, Pin: "1"}, "UNIT_LINK"); err != nil {
		t.Fatalf("unit-aware Connect returned error: %v", err)
	}
	if got := builder.assignedPinNet(Endpoint{Reference: "U1", Unit: 1, Pin: "2"}); got != "UNIT_LINK" {
		t.Fatalf("unit 1 pin net = %q, want UNIT_LINK", got)
	}
	if got := builder.assignedPinNet(Endpoint{Reference: "U1", Unit: 2, Pin: "1"}); got != "UNIT_LINK" {
		t.Fatalf("unit 2 pin net = %q, want UNIT_LINK", got)
	}
	if err := builder.Connect(Endpoint{Reference: "U1", Pin: "1"}, Endpoint{Reference: "U1", Pin: "2"}, "AMBIGUOUS"); err != nil {
		t.Fatalf("unit 1 default endpoint should remain backward compatible: %v", err)
	}
	symbols := builder.Design().Schematic.Symbols
	units := map[int]bool{}
	for _, symbol := range symbols {
		units[symbol.Unit] = true
	}
	if len(symbols) != 2 || !units[1] || !units[2] {
		t.Fatalf("shared-reference symbols = %#v, want units 1 and 2", symbols)
	}
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
			{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(20.32)},
			{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(31.75)},
		},
		{
			{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(31.75)},
			{X: kicadfiles.MM(34.29), Y: kicadfiles.MM(31.75)},
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
	if label := builder.design.Schematic.Labels[0]; label.Text != "SIG" || label.Position != (kicadfiles.Point{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(31.75)}) {
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

func TestBuilderRotatesExplicitSymbolPinAnchors(t *testing.T) {
	builder := newTestBuilder(t)
	position := kicadfiles.Point{X: kicadfiles.MM(50.8), Y: kicadfiles.MM(50.8)}
	_, err := builder.AddSymbol(SymbolOptions{
		Reference: "X1",
		LibraryID: "kicadai:test_rotated_pin",
		Position:  position,
		Rotation:  90,
		Pins:      []PinSpec{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(5.08)}}},
	})
	if err != nil {
		t.Fatalf("add rotated symbol: %v", err)
	}
	design := builder.Design()
	if len(design.Schematic.Symbols) != 1 || len(design.Schematic.Symbols[0].PinAnchors) != 1 {
		t.Fatalf("unexpected symbols: %#v", design.Schematic.Symbols)
	}
	anchor := design.Schematic.Symbols[0].PinAnchors[0]
	want := kicadfiles.Point{X: kicadfiles.MM(50.8), Y: kicadfiles.MM(55.88)}
	if anchor != want {
		t.Fatalf("rotated pin anchor = %#v, want %#v", anchor, want)
	}
}

func TestBuilderCanonicalMultiUnitPinAnchors(t *testing.T) {
	builder := newTestBuilder(t)
	tests := []struct {
		unit     int
		position kicadfiles.Point
		rotation kicadfiles.Angle
		mirror   schematic.SymbolMirror
	}{
		{unit: 1, position: kicadfiles.Point{X: kicadfiles.MM(20.1), Y: kicadfiles.MM(30.9)}},
		{unit: 2, position: kicadfiles.Point{X: kicadfiles.MM(60.1), Y: kicadfiles.MM(30.9)}, rotation: 90, mirror: schematic.SymbolMirrorX},
		{unit: 3, position: kicadfiles.Point{X: kicadfiles.MM(100.1), Y: kicadfiles.MM(30.9)}, rotation: 270, mirror: schematic.SymbolMirrorY},
	}
	offset := kicadfiles.Point{X: kicadfiles.MM(3.81), Y: -kicadfiles.MM(2.54)}
	for _, test := range tests {
		if _, err := builder.AddSymbol(SymbolOptions{
			Reference: "U1", Unit: test.unit, LibraryID: "kicadai:test_multi_unit",
			Position: test.position, Rotation: test.rotation, Mirror: test.mirror,
			Pins: []PinSpec{{Number: "1", Offset: offset}},
		}); err != nil {
			t.Fatalf("add unit %d: %v", test.unit, err)
		}
		state, err := builder.symbolStateForEndpoint(Endpoint{Reference: "U1", Unit: test.unit, Pin: "1"})
		if err != nil {
			t.Fatalf("unit %d state: %v", test.unit, err)
		}
		anchor, err := builder.pinAnchor(Endpoint{Reference: "U1", Unit: test.unit, Pin: "1"})
		if err != nil {
			t.Fatalf("unit %d anchor: %v", test.unit, err)
		}
		want := schematic.CanonicalConnectionAnchor(state.position, offset, state.rotation, state.mirror)
		if anchor != want {
			t.Fatalf("unit %d anchor = %#v, want %#v", test.unit, anchor, want)
		}
	}
}

func TestSymbolPinSpecsUsePhysicalConnectionAnchors(t *testing.T) {
	pins := symbolPinSpecs("Connector_Generic:Conn_01x02", nil)
	if len(pins) != 2 {
		t.Fatalf("connector pins = %#v", pins)
	}
	if pins[1].Number != "2" || pins[1].Offset != (kicadfiles.Point{X: kicadfiles.MM(-5.08), Y: kicadfiles.MM(2.54)}) {
		t.Fatalf("connector pin 2 physical anchor = %#v", pins[1])
	}
}

func TestBuilderConnectUsesExplicitOrthogonalWaypoints(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(60.96), Y: kicadfiles.MM(40.64)})
	start, err := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	if err != nil {
		t.Fatalf("start anchor: %v", err)
	}
	end, err := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	if err != nil {
		t.Fatalf("end anchor: %v", err)
	}
	waypoints := []kicadfiles.Point{start, {X: start.X, Y: kicadfiles.MM(10.16)}, {X: end.X, Y: kicadfiles.MM(10.16)}, end}
	useLabels := false
	if err := builder.ConnectWithOptions(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "SIG", ConnectOptions{UseLabels: &useLabels, Waypoints: waypoints}); err != nil {
		t.Fatalf("connect with waypoints: %v", err)
	}
	if got := len(builder.design.Schematic.Wires); got != 3 {
		t.Fatalf("wire count = %d, want 3 explicit segments", got)
	}
	for index, wire := range builder.design.Schematic.Wires {
		if wire.Points[0] != waypoints[index] || wire.Points[1] != waypoints[index+1] {
			t.Fatalf("wire %d = %#v, want %#v -> %#v", index, wire.Points, waypoints[index], waypoints[index+1])
		}
	}
}

func TestBuilderConnectRejectsDiagonalWaypoints(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(60.96), Y: kicadfiles.MM(40.64)})
	start, _ := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	end, _ := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	useLabels := false
	err := builder.ConnectWithOptions(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "SIG", ConnectOptions{UseLabels: &useLabels, Waypoints: []kicadfiles.Point{start, end}})
	if err == nil {
		t.Fatal("expected diagonal waypoint rejection")
	}
}

func TestBuilderConnectRejectsUniformDriftedWaypoints(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(40.64), Y: kicadfiles.MM(20.32)})
	start, _ := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	end, _ := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	// Drift both endpoints (and thus every interior bend) by one grid unit in X
	// and Y, mimicking a stale placement snapshot. The path stays orthogonal.
	drift := kicadfiles.Point{X: kicadfiles.MM(1.27), Y: kicadfiles.MM(1.27)}
	drifted := []kicadfiles.Point{
		{X: start.X + drift.X, Y: start.Y + drift.Y},
		{X: end.X + drift.X, Y: end.Y + drift.Y},
	}
	useLabels := false
	if err := builder.ConnectWithOptions(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "SIG", ConnectOptions{UseLabels: &useLabels, Waypoints: drifted}); err == nil {
		t.Fatal("stale uniformly drifted waypoints must fail closed")
	}
}

func TestBuilderConnectRejectsNonUniformDriftedWaypoints(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(40.64), Y: kicadfiles.MM(20.32)})
	start, _ := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	end, _ := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	drift := kicadfiles.Point{X: kicadfiles.MM(1.27), Y: kicadfiles.MM(1.27)}
	waypoints := []kicadfiles.Point{{X: start.X + drift.X, Y: start.Y + drift.Y}, end}
	useLabels := false
	err := builder.ConnectWithOptions(
		Endpoint{Reference: "R1", Pin: "2"},
		Endpoint{Reference: "R2", Pin: "1"},
		"SIG",
		ConnectOptions{UseLabels: &useLabels, Waypoints: waypoints},
	)
	if err == nil {
		t.Fatal("nonuniform waypoint drift must remain fail-closed")
	}
	if got := builder.assignedPinNet(Endpoint{Reference: "R1", Pin: "2"}); got != "" {
		t.Fatalf("failed connection assigned source net %q", got)
	}
}

func TestBuilderConnectUsesLabelStubsInsteadOfCrossingForeignWire(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(40.64), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R3", "Device:R", "2k", kicadfiles.Point{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(10.16)})
	addTwoPinSymbol(t, builder, "R4", "Device:R", "20k", kicadfiles.Point{X: kicadfiles.MM(35.56), Y: kicadfiles.MM(30.48)})

	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "NET_A"); err != nil {
		t.Fatalf("connect first net: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "R3", Pin: "2"}, Endpoint{Reference: "R4", Pin: "1"}, "NET_B"); err != nil {
		t.Fatalf("connect crossing net: %v", err)
	}

	labels := 0
	for _, label := range builder.design.Schematic.Labels {
		if label.Text == "NET_B" {
			labels++
		}
	}
	if labels != 2 {
		t.Fatalf("NET_B labels = %d, want two safe endpoint stubs; labels=%#v", labels, builder.design.Schematic.Labels)
	}
	for _, wire := range builder.design.Schematic.Wires {
		if builder.canonicalNet(builder.schematicWireNets[wire.UUID]) != "NET_B" {
			continue
		}
		if schematicSegmentsIntersect(
			wire.Points[0], wire.Points[1],
			kicadfiles.Point{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(20.32)},
			kicadfiles.Point{X: kicadfiles.MM(35.56), Y: kicadfiles.MM(20.32)},
		) {
			t.Fatalf("NET_B wire still crosses NET_A: %#v", wire.Points)
		}
	}
}

func TestBuilderConnectUsesExplicitLabelStubPositions(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(60.96), Y: kicadfiles.MM(40.64)})
	start, _ := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	end, _ := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	fromLabel := kicadfiles.Point{X: start.X + kicadfiles.MM(5.08), Y: start.Y}
	toLabel := kicadfiles.Point{X: end.X - kicadfiles.MM(5.08), Y: end.Y}
	useLabels := true
	if err := builder.ConnectWithOptions(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "SIG", ConnectOptions{UseLabels: &useLabels, FromLabelAt: &fromLabel, ToLabelAt: &toLabel}); err != nil {
		t.Fatalf("connect with label points: %v", err)
	}
	if len(builder.design.Schematic.Wires) != 2 || len(builder.design.Schematic.Labels) != 2 {
		t.Fatalf("wires=%#v labels=%#v", builder.design.Schematic.Wires, builder.design.Schematic.Labels)
	}
	if builder.design.Schematic.Labels[0].Position != fromLabel || builder.design.Schematic.Labels[1].Position != toLabel {
		t.Fatalf("label positions = %#v", builder.design.Schematic.Labels)
	}
}

func TestBuilderConnectRelocatesConflictingExplicitLabelStub(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(40.64)})
	addTwoPinSymbol(t, builder, "R3", "Device:R", "2k", kicadfiles.Point{X: kicadfiles.MM(30.48), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R4", "Device:R", "20k", kicadfiles.Point{X: kicadfiles.MM(30.48), Y: kicadfiles.MM(40.64)})

	firstStart, _ := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	firstEnd, _ := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	secondStart, _ := builder.pinAnchor(Endpoint{Reference: "R3", Pin: "2"})
	collisionPoint := kicadfiles.Point{X: secondStart.X, Y: firstStart.Y}
	firstEndLabel := kicadfiles.Point{X: firstEnd.X + kicadfiles.MM(5.08), Y: firstEnd.Y}
	useLabels := true
	if err := builder.ConnectWithOptions(
		Endpoint{Reference: "R1", Pin: "2"},
		Endpoint{Reference: "R2", Pin: "1"},
		"NET_A",
		ConnectOptions{UseLabels: &useLabels, FromLabelAt: &collisionPoint, ToLabelAt: &firstEndLabel},
	); err != nil {
		t.Fatalf("connect first net: %v", err)
	}

	secondEnd, _ := builder.pinAnchor(Endpoint{Reference: "R4", Pin: "1"})
	secondEndLabel := kicadfiles.Point{X: secondEnd.X - kicadfiles.MM(5.08), Y: secondEnd.Y}
	if err := builder.ConnectWithOptions(
		Endpoint{Reference: "R3", Pin: "2"},
		Endpoint{Reference: "R4", Pin: "1"},
		"NET_B",
		ConnectOptions{UseLabels: &useLabels, FromLabelAt: &collisionPoint, ToLabelAt: &secondEndLabel},
	); err != nil {
		t.Fatalf("connect second net: %v", err)
	}

	var labelsAtCollision []string
	for _, label := range builder.design.Schematic.Labels {
		if label.Position == collisionPoint {
			labelsAtCollision = append(labelsAtCollision, label.Text)
		}
	}
	if len(labelsAtCollision) != 1 || labelsAtCollision[0] != "NET_A" {
		t.Fatalf("labels at collision point = %v, want [NET_A]", labelsAtCollision)
	}
}

func TestBuilderIndexesForeignWireLabelStubConflicts(t *testing.T) {
	builder := newTestBuilder(t)
	wireStart := kicadfiles.Point{X: kicadfiles.MM(30.48), Y: kicadfiles.MM(20.32)}
	wireEnd := kicadfiles.Point{X: kicadfiles.MM(40.64), Y: kicadfiles.MM(20.32)}
	builder.addSchematicWirePoints("NET_A", Endpoint{}, Endpoint{}, []kicadfiles.Point{wireStart, wireEnd})

	crossingStart := kicadfiles.Point{X: kicadfiles.MM(30.48), Y: kicadfiles.MM(10.16)}
	crossingEnd := kicadfiles.Point{X: kicadfiles.MM(30.48), Y: kicadfiles.MM(30.48)}
	if !builder.schematicStubTouchesForeignWire("NET_B", crossingStart, crossingEnd) {
		t.Fatal("expected foreign-net crossing to be detected")
	}
	if builder.schematicStubTouchesForeignWire("NET_A", crossingStart, crossingEnd) {
		t.Fatal("same-net crossing should remain available")
	}
	if !builder.schematicStubTouchesForeignWire("NET_B", wireStart, kicadfiles.Point{X: wireStart.X, Y: wireStart.Y + kicadfiles.MM(5.08)}) {
		t.Fatal("expected foreign wire at stub anchor to be detected")
	}

	offGrid := newTestBuilder(t)
	diagonalStart := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}
	diagonalEnd := kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(30)}
	offGrid.addSchematicWirePoints("NET_DIAGONAL", Endpoint{}, Endpoint{}, []kicadfiles.Point{diagonalStart, diagonalEnd})
	if !offGrid.schematicStubTouchesForeignWire("NET_B", diagonalStart, kicadfiles.Point{X: diagonalStart.X, Y: diagonalStart.Y + kicadfiles.MM(5)}) {
		t.Fatal("expected off-grid diagonal wire endpoint conflict to be detected")
	}
	if !offGrid.schematicStubTouchesForeignWire(
		"NET_B",
		kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(25)},
		kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(25)},
	) {
		t.Fatal("expected off-grid diagonal wire crossing to be detected")
	}
}

func TestBuilderConnectReanchorsMovedMultiUnitLabelStub(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(60.96), Y: kicadfiles.MM(40.64)})
	start, _ := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	end, _ := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	state, err := builder.symbolStateForEndpoint(Endpoint{Reference: "R1", Pin: "2"})
	if err != nil {
		t.Fatal(err)
	}
	state.requestedPosition = kicadfiles.Point{X: state.position.X - kicadfiles.MM(1.27), Y: state.position.Y}
	staleFromLabel := kicadfiles.Point{X: start.X - kicadfiles.MM(6.35), Y: start.Y}
	toLabel := kicadfiles.Point{X: end.X - kicadfiles.MM(5.08), Y: end.Y}
	useLabels := true
	if err := builder.ConnectWithOptions(
		Endpoint{Reference: "R1", Pin: "2"},
		Endpoint{Reference: "R2", Pin: "1"},
		"SIG",
		ConnectOptions{UseLabels: &useLabels, FromLabelAt: &staleFromLabel, ToLabelAt: &toLabel, ReanchorFromLabel: true},
	); err != nil {
		t.Fatalf("connect with moved-unit label point: %v", err)
	}
	wantFromLabel := kicadfiles.Point{X: start.X - kicadfiles.MM(5.08), Y: start.Y}
	if got := builder.design.Schematic.Labels[0].Position; got != wantFromLabel {
		t.Fatalf("reanchored label = %#v, want %#v", got, wantFromLabel)
	}
}

func TestBuilderConnectRejectsDiagonalExplicitLabelStub(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(60.96), Y: kicadfiles.MM(40.64)})
	start, _ := builder.pinAnchor(Endpoint{Reference: "R1", Pin: "2"})
	end, _ := builder.pinAnchor(Endpoint{Reference: "R2", Pin: "1"})
	fromLabel := kicadfiles.Point{X: start.X + kicadfiles.MM(1.27), Y: start.Y - kicadfiles.MM(3.81)}
	toLabel := kicadfiles.Point{X: end.X - kicadfiles.MM(5.08), Y: end.Y}
	useLabels := true
	if err := builder.ConnectWithOptions(
		Endpoint{Reference: "R1", Pin: "2"},
		Endpoint{Reference: "R2", Pin: "1"},
		"SIG",
		ConnectOptions{UseLabels: &useLabels, FromLabelAt: &fromLabel, ToLabelAt: &toLabel},
	); err == nil {
		t.Fatal("expected diagonal explicit label rejection")
	}
}

func TestBuilderConnectCanReplaceOneLocalLabel(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(20.32)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(60.96), Y: kicadfiles.MM(40.64)})
	useLabels := true
	if err := builder.ConnectWithOptions(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "PORT_SIG", ConnectOptions{UseLabels: &useLabels, SkipFromLabel: true}); err != nil {
		t.Fatalf("connect with one skipped label: %v", err)
	}
	if len(builder.design.Schematic.Labels) != 1 || builder.design.Schematic.Labels[0].Text != "PORT_SIG" {
		t.Fatalf("labels = %#v, want only the non-port local label", builder.design.Schematic.Labels)
	}
}

func TestBuilderLongNetLabelStubsAreConnectionGridSafe(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(80), Y: kicadfiles.MM(22)})

	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "LONG_SIG"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Labels) != 2 {
		t.Fatalf("labels = %#v, want two long-net label stubs", design.Schematic.Labels)
	}
	assertSchematicConnectivityGridSafe(t, design.Schematic)
	report := schematic.InspectGeneratedConnectivity(*design.Schematic)
	if len(report.OffGridObjects) != 0 {
		t.Fatalf("off-grid diagnostics = %#v, want none", report.OffGridObjects)
	}
}

func TestBuilderGeneratedIOAliasLabelStubsAreConnectionGridSafe(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "J1", "Connector_Generic:Conn_01x02", "IN", kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)})
	addTwoPinSymbol(t, builder, "U1", "Device:R", "load", kicadfiles.Point{X: kicadfiles.MM(28), Y: kicadfiles.MM(10)})

	if err := builder.Connect(Endpoint{Reference: "J1", Pin: "1"}, Endpoint{Reference: "U1", Pin: "1"}, "io_sda"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Labels) != 2 {
		t.Fatalf("labels = %#v, want two io_ alias label stubs", design.Schematic.Labels)
	}
	assertSchematicConnectivityGridSafe(t, design.Schematic)
	report := schematic.InspectGeneratedConnectivity(*design.Schematic)
	if len(report.OffGridObjects) != 0 {
		t.Fatalf("off-grid diagnostics = %#v, want none", report.OffGridObjects)
	}
}

func TestBuilderGridSnapAvoidsAdjacentSymbolPinAnchorCollision(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "4k7", kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(-10)})
	addTwoPinSymbol(t, builder, "R2", "Device:R", "4k7", kicadfiles.Point{X: kicadfiles.MM(35), Y: kicadfiles.MM(-10)})

	design := builder.Design()
	seen := map[kicadfiles.Point]string{}
	for _, symbol := range design.Schematic.Symbols {
		for pinIndex, anchor := range symbol.PinAnchors {
			if existing := seen[anchor]; existing != "" {
				t.Fatalf("%s pin %d anchor %#v collides with %s", symbol.Reference, pinIndex, anchor, existing)
			}
			seen[anchor] = symbol.Reference
		}
	}
	assertSchematicConnectivityGridSafe(t, design.Schematic)
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
		Description:              "library resistor",
		Tags:                     "resistor 0805",
		Attributes:               []string{"smd"},
		MetadataProperties:       []pcb.FootprintMetadataProperty{{Name: "ki_description", Value: "library resistor"}},
		HideDefaultFootprintText: true,
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
	for _, property := range footprint.Properties {
		if (property.Name == "Reference" || property.Name == "Value") && !property.Hide {
			t.Fatalf("default generated property %s should be hidden to avoid silkscreen DRC noise: %#v", property.Name, property)
		}
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

func TestBuilderAllowsUnnamedPasteOnlyPad(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1"},
			{Name: "2"},
			{Type: "smd", Shape: "custom", Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFPaste}},
		},
		AllowUnmatchedUnconnectedPads: true,
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error for unnamed paste-only pad: %v", err)
	}
	footprint := builder.Design().PCB.Footprints[0]
	if len(footprint.Pads) != 3 || footprint.Pads[2].Name != "" {
		t.Fatalf("unnamed paste-only pad not preserved: %#v", footprint.Pads)
	}
}

func TestBuilderRejectsUnnamedCopperPad(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	_, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads:                          []PadSpec{{Name: "1"}, {Name: "2"}, {Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}}},
		AllowUnmatchedUnconnectedPads: true,
	})
	if err == nil || !strings.Contains(err.Error(), "pad name required") {
		t.Fatalf("unnamed copper pad error = %v", err)
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

func TestBuilderAcceptsPhysicalPadsForGroupedSymbolPin(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		LibraryID: "RF_Module:Grouped",
		Reference: "U1",
		Value:     "Grouped",
		Position:  kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
		Pins:      []PinSpec{{Number: "[1,15,38,39]"}},
	}); err != nil {
		t.Fatalf("AddSymbol: %v", err)
	}
	if err := builder.AssignFootprint("U1", "RF_Module:Grouped"); err != nil {
		t.Fatalf("AssignFootprint: %v", err)
	}
	if _, err := builder.PlaceFootprint("U1", PlaceFootprintOptions{Pads: []PadSpec{{Name: "1"}, {Name: "15"}, {Name: "38"}, {Name: "39"}}}); err != nil {
		t.Fatalf("PlaceFootprint rejected grouped physical pads: %v", err)
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
		Layer:                    kicadfiles.LayerBCu,
		Attributes:               []string{"smd"},
		HideDefaultFootprintText: true,
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
	for _, property := range footprint.Properties {
		switch property.Name {
		case "Reference", "Value":
			if property.Layer != kicadfiles.LayerBSilkS || !property.Hide {
				t.Fatalf("bottom-side default property = %#v, want hidden B.SilkS", property)
			}
		case "Datasheet", "Description":
			if property.Layer != kicadfiles.LayerBFab || !property.Hide {
				t.Fatalf("bottom-side metadata property = %#v, want hidden B.Fab", property)
			}
		}
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

func TestBuilderAddSymbolKeepsCanonicalTemplateOverResolverBody(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Device:R": {
			LibraryID: "Device:R",
			Raw:       `(symbol "R" (property "Description" "resolver marker"))`,
		},
	}}
	builder, err := New(Options{
		Name:         "canonical_symbol",
		DesignID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		LibraryIndex: &index,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "R1",
		LibraryID: "Device:R",
		Value:     "1k",
		Position:  kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
	}); err != nil {
		t.Fatal(err)
	}
	design := builder.Design()
	if len(design.Schematic.LibSymbols) != 1 || strings.Contains(fmt.Sprint(design.Schematic.LibSymbols[0].Body), "resolver marker") {
		t.Fatalf("canonical body was replaced: %#v", design.Schematic.LibSymbols)
	}
}

func TestBuilderSerializesGroupedResolverPinOnceWhileKeepingMemberAccess(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"RF_Module:Grouped": {
			LibraryID: "RF_Module:Grouped",
			Raw:       `(symbol "Grouped")`,
			Pins: []libraryresolver.SymbolPin{{
				Number:   "[1,15,38,39]",
				Position: kicadfiles.Point{Y: kicadfiles.MM(5.08)},
			}},
		},
	}}
	builder, err := New(Options{Name: "grouped_resolver", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"), LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "U1", LibraryID: "RF_Module:Grouped", Value: "Grouped",
		Position:             kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
		Pins:                 []PinSpec{{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(5.08)}}, {Number: "15", Offset: kicadfiles.Point{Y: kicadfiles.MM(5.08)}}, {Number: "38", Offset: kicadfiles.Point{Y: kicadfiles.MM(5.08)}}, {Number: "39", Offset: kicadfiles.Point{Y: kicadfiles.MM(5.08)}}},
		PreferResolverSymbol: true,
	}); err != nil {
		t.Fatal(err)
	}
	for _, pin := range []string{"1", "15", "38", "39"} {
		if _, err := builder.pinAnchor(Endpoint{Reference: "U1", Pin: pin}); err != nil {
			t.Fatalf("member pin %s is not addressable: %v", pin, err)
		}
	}
	symbol := builder.Design().Schematic.Symbols[0]
	if len(symbol.Pins) != 1 || symbol.Pins[0].Number != "[1,15,38,39]" || len(symbol.PinAnchors) != 1 {
		t.Fatalf("serialized grouped pins = %#v anchors=%#v", symbol.Pins, symbol.PinAnchors)
	}
}

func TestBuilderAddSymbolCanPreferResolverBody(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Device:R": {LibraryID: "Device:R", Raw: `(symbol "R" (property "Description" "resolver marker"))`},
	}}
	builder, err := New(Options{Name: "resolver_symbol", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"), LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "R1", LibraryID: "Device:R", Value: "1k", PreferResolverSymbol: true,
		Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
	}); err != nil {
		t.Fatal(err)
	}
	design := builder.Design()
	if len(design.Schematic.LibSymbols) != 1 || !strings.Contains(fmt.Sprint(design.Schematic.LibSymbols[0].Body), "resolver marker") {
		t.Fatalf("resolver body was not selected: %#v", design.Schematic.LibSymbols)
	}
}

func TestBuilderPreferredResolverBodyUsesResolverPinAnchors(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Regulator_Linear:AP2112K-3.3": {
			LibraryID: "Regulator_Linear:AP2112K-3.3",
			Raw:       `(symbol "AP2112K-3.3" (property "Description" "resolver marker"))`,
			Pins: []libraryresolver.SymbolPin{{
				Number:   "1",
				Position: kicadfiles.Point{X: kicadfiles.MM(-7.62), Y: kicadfiles.MM(-2.54)},
			}},
		},
	}}
	builder, err := New(Options{Name: "resolver_pin_anchor", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"), LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	position := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "U1", LibraryID: "Regulator_Linear:AP2112K-3.3", Value: "3V3", PreferResolverSymbol: true,
		Position: position,
	}); err != nil {
		t.Fatal(err)
	}

	anchor, err := builder.pinAnchor(Endpoint{Reference: "U1", Pin: "1"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := builder.symbolState("U1")
	if err != nil {
		t.Fatal(err)
	}
	want := kicadfiles.Point{X: state.position.X + kicadfiles.MM(-7.62), Y: state.position.Y + kicadfiles.MM(-2.54)}
	if anchor != want {
		t.Fatalf("resolver pin anchor = %#v, want %#v", anchor, want)
	}
	foundMarker := false
	for _, property := range builder.Design().Schematic.Symbols[0].Properties {
		if property.Name == schematic.ResolverGeometryPropertyName && property.Value == schematic.ResolverGeometryPropertyValue && property.Hidden && property.Private {
			foundMarker = true
		}
	}
	if !foundMarker {
		t.Fatal("resolver-backed symbol is missing its persisted geometry marker")
	}
}

func TestBuilderAddLabelAtEndpointUsesResolvedPinAnchor(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Device:D": {
			LibraryID: "Device:D",
			Raw:       `(symbol "D")`,
			Pins: []libraryresolver.SymbolPin{{
				Number:   "2",
				Position: kicadfiles.Point{X: kicadfiles.MM(3.81)},
			}},
		},
	}}
	builder, err := New(Options{Name: "anchored_label", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"), LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	position := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "D1", LibraryID: "Device:D", Value: "D", Position: position,
		Pins: []PinSpec{{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(3.81)}}}, PreferResolverSymbol: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddLabelAtEndpointWithOptions("DRIVER_OUT", Endpoint{Reference: "D1", Pin: "2"}, schematic.LabelLocal, LabelOptions{}); err != nil {
		t.Fatal(err)
	}
	labels := builder.Design().Schematic.Labels
	symbolPosition := builder.Design().Schematic.Symbols[0].Position
	want := kicadfiles.Point{X: symbolPosition.X + kicadfiles.MM(3.81), Y: symbolPosition.Y}
	if len(labels) != 1 || labels[0].Position != want {
		t.Fatalf("labels = %#v, want DRIVER_OUT at resolved pin anchor %#v", labels, want)
	}
}

func TestBuilderResolverLibraryMaterializesUsedSiblingSymbols(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Connector_Generic:Conn_01x04": {
			LibraryID:       "Connector_Generic:Conn_01x04",
			LibraryNickname: "Connector_Generic",
			Raw:             `(symbol "Conn_01x04" (property "Value" "Conn_01x04"))`,
		},
		"Connector_Generic:Conn_01x06": {
			LibraryID:       "Connector_Generic:Conn_01x06",
			LibraryNickname: "Connector_Generic",
			Raw:             `(symbol "Conn_01x06" (property "Value" "Conn_01x06"))`,
		},
	}}
	builder, err := New(Options{Name: "resolver_siblings", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"), LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	for index, options := range []SymbolOptions{
		{Reference: "J1", LibraryID: "Connector_Generic:Conn_01x04", Value: "UART", Pins: []PinSpec{{Number: "1"}}},
		{Reference: "J2", LibraryID: "Connector_Generic:Conn_01x06", Value: "GPIO", Pins: []PinSpec{{Number: "1"}}, PreferResolverSymbol: true},
	} {
		options.Position = kicadfiles.Point{X: kicadfiles.MM(20 + float64(index)*20), Y: kicadfiles.MM(20)}
		if _, err := builder.AddSymbol(options); err != nil {
			t.Fatal(err)
		}
	}
	result, err := builder.WriteSchematicProject(filepath.Join(t.TempDir(), "resolver_siblings"), kicaddesign.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(result.ProjectDir, "lib", "kicadai_resolved_Connector_Generic.kicad_sym"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`"Conn_01x04"`, `"Conn_01x06"`} {
		if !strings.Contains(string(contents), name) {
			t.Fatalf("resolver library missing used sibling %s:\n%s", name, contents)
		}
	}
}

func TestBuilderResolverLibraryUpgradesWriterOwnedGeneratedSiblingLibrary(t *testing.T) {
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Connector_Generic:Conn_01x02": {
			LibraryID:       "Connector_Generic:Conn_01x02",
			LibraryNickname: "Connector_Generic",
			Raw:             `(symbol "Conn_01x02" (property "Value" "Conn_01x02"))`,
		},
		"Connector_Generic:Conn_01x06": {
			LibraryID:       "Connector_Generic:Conn_01x06",
			LibraryNickname: "Connector_Generic",
			Raw:             `(symbol "Conn_01x06" (property "Value" "Conn_01x06"))`,
		},
	}}
	builder, err := New(Options{Name: "resolver_generated_siblings", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"), LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	for index, options := range []SymbolOptions{
		{Reference: "J1", LibraryID: "Connector_Generic:Conn_01x02", Value: "POWER", Pins: []PinSpec{{Number: "1"}, {Number: "2"}}},
		{Reference: "J2", LibraryID: "Connector_Generic:Conn_01x06", Value: "GPIO", Pins: []PinSpec{{Number: "1"}}, PreferResolverSymbol: true},
	} {
		options.Position = kicadfiles.Point{X: kicadfiles.MM(20 + float64(index)*20), Y: kicadfiles.MM(20)}
		if _, err := builder.AddSymbol(options); err != nil {
			t.Fatal(err)
		}
	}
	result, err := builder.WriteSchematicProject(filepath.Join(t.TempDir(), "resolver_generated_siblings"), kicaddesign.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(result.ProjectDir, "lib", "kicadai_connector_generic.kicad_sym"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`"Conn_01x02"`, `"Conn_01x06"`} {
		if !strings.Contains(string(contents), name) {
			t.Fatalf("upgraded writer-owned library missing used sibling %s:\n%s", name, contents)
		}
	}
}

func TestBuilderPreferResolverEmbedsExplicitPinFallbackWhenResolverUnavailable(t *testing.T) {
	emptyIndex := &libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{}}
	for _, test := range []struct {
		name  string
		index *libraryresolver.LibraryIndex
	}{
		{name: "no index"},
		{name: "partial index", index: emptyIndex},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder, err := New(Options{
				Name: "intent_demo", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
				Seed: "intent-test", LibraryIndex: test.index,
			})
			if err != nil {
				t.Fatal(err)
			}
			position := kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(30)}
			pins := []PinSpec{
				{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-7.62), Y: kicadfiles.MM(-5.08)}},
				{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(7.62), Y: kicadfiles.MM(5.08)}},
			}
			if _, err := builder.AddSymbol(SymbolOptions{
				Reference: "U1", LibraryID: "Vendor:Unresolved_Module", Value: "MODULE",
				Position: position, Pins: pins, PreferResolverSymbol: true,
			}); err != nil {
				t.Fatal(err)
			}
			for _, pin := range pins {
				if err := builder.AddNoConnect(Endpoint{Reference: "U1", Pin: pin.Number}); err != nil {
					t.Fatal(err)
				}
			}
			if !schematic.EmbeddedSymbolPresent(builder.Design().Schematic, "Vendor:Unresolved_Module") {
				t.Fatal("unresolved preferred symbol is missing its embedded fallback body")
			}
			root := filepath.Join(t.TempDir(), "fallback_symbol")
			if _, err := builder.WriteSchematicProject(root, kicaddesign.WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			read, err := schematic.ReadFile(filepath.Join(root, "intent_demo.kicad_sch"))
			if err != nil {
				t.Fatal(err)
			}
			if err := schematic.ValidateGeneratedConnectivity(read); err != nil {
				t.Fatalf("fallback symbol connectivity validation failed: %v", err)
			}
		})
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
	if len(design.Schematic.Wires) != 3 {
		t.Fatalf("wires = %d, want orthogonal physical-pin connection", len(design.Schematic.Wires))
	}
	if len(design.Schematic.Labels) != 2 || design.Schematic.Labels[0].Text != "FILTER" || design.Schematic.Labels[1].Text != "FILTER" {
		t.Fatalf("labels = %#v, want FILTER labels at the orthogonal bends", design.Schematic.Labels)
	}
	if len(design.Schematic.Junctions) != 2 {
		t.Fatalf("junctions = %d, want junctions at both orthogonal bends", len(design.Schematic.Junctions))
	}
	assertSchematicPinNet(t, builder, Endpoint{Reference: "R1", Pin: "2"}, "FILTER")
	assertSchematicPinNet(t, builder, Endpoint{Reference: "C1", Pin: "1"}, "FILTER")
}

func TestBuilderUsesDirectLabelsForAlignedVerticalPinAnchors(t *testing.T) {
	builder := newTestBuilder(t)
	for _, symbol := range []SymbolOptions{
		{Reference: "C1", LibraryID: "Device:C", Value: "100n", Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Reference: "C2", LibraryID: "Device:C", Value: "10u", Position: kicadfiles.Point{X: kicadfiles.MM(35), Y: kicadfiles.MM(20)}},
	} {
		if _, err := builder.AddSymbol(symbol); err != nil {
			t.Fatalf("AddSymbol %s returned error: %v", symbol.Reference, err)
		}
	}

	if err := builder.Connect(Endpoint{Reference: "C1", Pin: "2"}, Endpoint{Reference: "C2", Pin: "2"}, "GND"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Wires) != 0 {
		t.Fatalf("wires = %d, want aligned vertical pins connected by direct labels", len(design.Schematic.Wires))
	}
	if len(design.Schematic.Labels) != 2 || design.Schematic.Labels[0].Text != "GND" || design.Schematic.Labels[1].Text != "GND" {
		t.Fatalf("labels = %#v, want GND labels on both physical pin anchors", design.Schematic.Labels)
	}
	assertSchematicPinNet(t, builder, Endpoint{Reference: "C1", Pin: "2"}, "GND")
	assertSchematicPinNet(t, builder, Endpoint{Reference: "C2", Pin: "2"}, "GND")
}

func TestBuilderConn01x04Pin4UsesKiCadERCConnectionAnchor(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "J1",
		LibraryID: "Connector_Generic:Conn_01x04",
		Value:     "I2C",
		Position:  kicadfiles.Point{X: kicadfiles.MM(91.44), Y: 0},
	}); err != nil {
		t.Fatalf("AddSymbol J1 returned error: %v", err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "R1",
		LibraryID: "Device:R",
		Value:     "4.7k",
		Position:  kicadfiles.Point{X: kicadfiles.MM(10), Y: 0},
	}); err != nil {
		t.Fatalf("AddSymbol R1 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "J1", Pin: "4"}, Endpoint{Reference: "R1", Pin: "1"}, "SCL"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if got := design.Schematic.Symbols[0].PinAnchors[3]; got != (kicadfiles.Point{X: kicadfiles.MM(86.36), Y: kicadfiles.MM(5.08)}) {
		t.Fatalf("Conn_01x04 pin 4 metadata anchor = %v, want KiCad ERC connection anchor", got)
	}
	wantStart := kicadfiles.Point{X: kicadfiles.MM(86.36), Y: kicadfiles.MM(5.08)}
	wantEnd := kicadfiles.Point{X: kicadfiles.MM(85.09), Y: kicadfiles.MM(5.08)}
	for _, wire := range design.Schematic.Wires {
		if len(wire.Points) == 2 && wire.Points[0] == wantStart && wire.Points[1] == wantEnd {
			return
		}
	}
	t.Fatalf("missing Conn_01x04 pin 4 KiCad ERC stub %v -> %v: %#v", wantStart, wantEnd, design.Schematic.Wires)
}

func TestBuilderUSBCPowerCCConnectionUsesDirectLabels(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "J1",
		LibraryID: "kicadai:USB_C_Receptacle_PowerOnly_6P",
		Value:     "USB-C",
		Position:  kicadfiles.Point{},
	}); err != nil {
		t.Fatalf("AddSymbol J1 returned error: %v", err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "R1",
		LibraryID: "kicadai:USB_CC_R",
		Value:     "5.1k",
		Position:  kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(1.27)},
		Pins: []PinSpec{
			{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(3.81)}},
			{Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(-3.81)}},
		},
	}); err != nil {
		t.Fatalf("AddSymbol R1 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "J1", Pin: "A5"}, Endpoint{Reference: "R1", Pin: "1"}, "USB_CC1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Wires) != 0 {
		t.Fatalf("CC connection emitted wires = %#v, want direct labels only", design.Schematic.Wires)
	}
	wantLabels := map[kicadfiles.Point]bool{
		{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(5.08)}: false,
		{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(5.08)}: false,
	}
	for _, label := range design.Schematic.Labels {
		if label.Text == "USB_CC1" {
			if _, ok := wantLabels[label.Position]; ok {
				wantLabels[label.Position] = true
			}
		}
	}
	for point, found := range wantLabels {
		if !found {
			t.Fatalf("missing USB_CC1 label at %v: %#v", point, design.Schematic.Labels)
		}
	}
}

func TestBuilderSuppressesUSBCPreFuseBendLabels(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "J1",
		LibraryID: "kicadai:USB_C_Receptacle_PowerOnly_6P",
		Value:     "USB-C",
		Position:  kicadfiles.Point{},
	}); err != nil {
		t.Fatalf("AddSymbol J1 returned error: %v", err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "F1",
		LibraryID: "Device:Fuse",
		Value:     "500mA",
		Position:  kicadfiles.Point{X: kicadfiles.MM(20.32), Y: kicadfiles.MM(-11.43)},
	}); err != nil {
		t.Fatalf("AddSymbol F1 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "J1", Pin: "A9"}, Endpoint{Reference: "F1", Pin: "1"}, "usb_power_vbus_connector"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Junctions) == 0 {
		t.Fatalf("junctions = 0, want bend junction for pre-fuse VBUS")
	}
	usbAnchor, err := builder.pinAnchor(Endpoint{Reference: "J1", Pin: "A9"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSchematicJunction(design.Schematic.Junctions, usbAnchor) {
		t.Fatalf("missing junction at stacked USB VBUS pin anchor %v: %#v", usbAnchor, design.Schematic.Junctions)
	}
	for _, label := range design.Schematic.Labels {
		if label.Text == "usb_power_vbus_connector" {
			t.Fatalf("unexpected pre-fuse VBUS bend label: %#v", design.Schematic.Labels)
		}
	}
	assertSchematicPinNet(t, builder, Endpoint{Reference: "J1", Pin: "A9"}, "usb_power_vbus_connector")
	assertSchematicPinNet(t, builder, Endpoint{Reference: "F1", Pin: "1"}, "usb_power_vbus_connector")
}

func TestBuilderAvoidsRoutingThroughOtherSymbolPinAnchor(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "C1",
		LibraryID: "Device:C",
		Value:     "100n",
		Position:  kicadfiles.Point{X: 0, Y: kicadfiles.MM(10.16)},
	}); err != nil {
		t.Fatalf("AddSymbol C1 returned error: %v", err)
	}
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "U1",
		LibraryID: "Sensor:Generic_I2C",
		Value:     "Sensor",
		Position:  kicadfiles.Point{X: kicadfiles.MM(10.16), Y: 0},
	}); err != nil {
		t.Fatalf("AddSymbol U1 returned error: %v", err)
	}
	if err := builder.Connect(Endpoint{Reference: "C1", Pin: "1"}, Endpoint{Reference: "U1", Pin: "1"}, "VCC"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	otherPin, err := builder.pinAnchor(Endpoint{Reference: "C1", Pin: "2"})
	if err != nil {
		t.Fatalf("pinAnchor returned error: %v", err)
	}
	for _, wire := range builder.Design().Schematic.Wires {
		for index := 1; index < len(wire.Points); index++ {
			if pointOnSchematicSegment(otherPin, wire.Points[index-1], wire.Points[index]) {
				t.Fatalf("wire segment crosses C1 pin 2 at %v: %#v", otherPin, wire.Points)
			}
		}
	}
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
	viaPoint := kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)}
	via := RouteViaSpec{At: viaPoint, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu}}
	if _, err := builder.Route("POWER", points, RouteOptions{Layer: kicadfiles.LayerBCu, Vias: []RouteViaSpec{via}}); err != nil {
		t.Fatalf("first via Route returned error: %v", err)
	}
	if _, err := builder.Route("POWER", points, RouteOptions{Layer: kicadfiles.LayerBCu, Vias: []RouteViaSpec{via}}); err != nil {
		t.Fatalf("second via Route returned error: %v", err)
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
	for _, zone := range builder.Design().PCB.Zones {
		if got := zone.Polygons[0]; len(got) != len(polygon)+1 || got[0] != got[len(got)-1] {
			t.Fatalf("zone polygon is not explicitly closed: %#v", got)
		}
	}

	design := builder.Design()
	if len(design.PCB.Tracks) != 4 {
		t.Fatalf("tracks = %d, want 4", len(design.PCB.Tracks))
	}
	if design.PCB.Tracks[0].UUID == design.PCB.Tracks[1].UUID {
		t.Fatalf("duplicate route UUID %s", design.PCB.Tracks[0].UUID)
	}
	if len(design.PCB.Vias) != 2 {
		t.Fatalf("vias = %d, want 2", len(design.PCB.Vias))
	}
	if design.PCB.Vias[0].UUID == design.PCB.Vias[1].UUID {
		t.Fatalf("duplicate via UUID %s", design.PCB.Vias[0].UUID)
	}
	if len(design.PCB.Zones) != 2 {
		t.Fatalf("zones = %d, want 2", len(design.PCB.Zones))
	}
	if design.PCB.Zones[0].UUID == design.PCB.Zones[1].UUID {
		t.Fatalf("duplicate zone UUID %s", design.PCB.Zones[0].UUID)
	}
}

func TestBuilderRouteAcceptsViaOnlyOperation(t *testing.T) {
	builder := newTestBuilder(t)
	via := RouteViaSpec{
		At:     kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		Layers: []kicadfiles.BoardLayer{kicadfiles.LayerBCu, kicadfiles.LayerFCu},
	}
	handle, err := builder.Route("POWER", nil, RouteOptions{Vias: []RouteViaSpec{via}})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if handle.Count != 0 {
		t.Fatalf("handle.Count = %d, want 0 track segments", handle.Count)
	}
	design := builder.Design()
	if len(design.PCB.Tracks) != 0 {
		t.Fatalf("tracks = %d, want 0", len(design.PCB.Tracks))
	}
	if len(design.PCB.Vias) != 1 {
		t.Fatalf("vias = %d, want 1", len(design.PCB.Vias))
	}
	if design.PCB.Vias[0].NetName != "POWER" {
		t.Fatalf("via net = %q, want POWER", design.PCB.Vias[0].NetName)
	}
	if got := design.PCB.Vias[0].Layers; len(got) != 2 || got[0] != kicadfiles.LayerFCu || got[1] != kicadfiles.LayerBCu {
		t.Fatalf("via layers = %+v, want KiCad canonical F.Cu/B.Cu order", got)
	}
}

func TestBuilderRouteRejectsSinglePointWithVia(t *testing.T) {
	builder := newTestBuilder(t)
	via := RouteViaSpec{
		At:     kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
	}
	_, err := builder.Route("POWER", []kicadfiles.Point{{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}}, RouteOptions{Vias: []RouteViaSpec{via}})
	if err == nil {
		t.Fatal("Route returned nil error for single-point route with via")
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

func TestBuilderWriteSchematicProjectDoesNotMutatePCBState(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}

	root := filepath.Join(t.TempDir(), "schematic_only")
	result, err := builder.WriteSchematicProject(root, kicaddesign.WriteOptions{})
	if err != nil {
		t.Fatalf("WriteSchematicProject returned error: %v", err)
	}
	if len(result.WrittenFiles) != 2 {
		t.Fatalf("written files = %v, want project and schematic only", result.WrittenFiles)
	}
	for _, path := range result.WrittenFiles {
		if filepath.Ext(path) == ".kicad_pcb" {
			t.Fatalf("schematic-only write emitted PCB file: %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected written file %s: %v", path, err)
		}
	}

	design := builder.Design()
	if design.PCB == nil {
		t.Fatal("WriteSchematicProject mutated builder PCB state")
	}
}

func TestBuilderWriteProjectAddsGeneratedLocalSensorSymbolLibrary(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "U1",
		LibraryID: "Sensor:Generic_I2C",
		Value:     "Generic I2C Sensor",
		Position:  kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(30)},
	}); err != nil {
		t.Fatalf("AddSymbol returned error: %v", err)
	}
	if err := builder.AssignFootprint("U1", "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("U1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1"},
			{Name: "2"},
			{Name: "3"},
			{Name: "4"},
			{Name: "5"},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	root := filepath.Join(t.TempDir(), "sensor_demo")
	result, err := builder.WriteProject(root, kicaddesign.WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProject returned error: %v", err)
	}
	table, err := os.ReadFile(filepath.Join(result.ProjectDir, "sym-lib-table"))
	if err != nil {
		t.Fatalf("expected sym-lib-table: %v", err)
	}
	if !strings.Contains(string(table), `"Sensor"`) || !strings.Contains(string(table), `"${KIPRJMOD}/lib/kicadai_sensor.kicad_sym"`) {
		t.Fatalf("sym-lib-table missing generated Sensor library:\n%s", table)
	}
	localLibrary, err := os.ReadFile(filepath.Join(result.ProjectDir, "lib", "kicadai_sensor.kicad_sym"))
	if err != nil {
		t.Fatalf("expected generated sensor library: %v", err)
	}
	if !strings.Contains(string(localLibrary), `"Generic_I2C"`) || strings.Contains(string(localLibrary), `"Sensor:Generic_I2C"`) {
		t.Fatalf("generated sensor library should contain unqualified symbol name:\n%s", localLibrary)
	}
	schematicFiles, err := filepath.Glob(filepath.Join(result.ProjectDir, "*.kicad_sch"))
	if err != nil {
		t.Fatalf("glob schematic files: %v", err)
	}
	if len(schematicFiles) != 1 {
		t.Fatalf("schematic files = %v, want one", schematicFiles)
	}
	schematicContents, err := os.ReadFile(schematicFiles[0])
	if err != nil {
		t.Fatalf("expected schematic: %v", err)
	}
	if !strings.Contains(string(schematicContents), `"Sensor:Generic_I2C"`) {
		t.Fatalf("schematic should retain qualified symbol reference:\n%s", schematicContents)
	}
}

func TestBuilderWriteSchematicProjectCanonicalizesLocalUSBCAlias(t *testing.T) {
	builder := newTestBuilder(t)
	if _, err := builder.AddSymbol(SymbolOptions{
		Reference: "J1",
		LibraryID: "kicadai:usb_c_receptacle_poweronly_full",
		Value:     "USB-C",
		Position:  kicadfiles.Point{X: kicadfiles.MM(100), Y: kicadfiles.MM(80)},
	}); err != nil {
		t.Fatalf("AddSymbol returned error: %v", err)
	}

	root := filepath.Join(t.TempDir(), "usb_c_alias")
	result, err := builder.WriteSchematicProject(root, kicaddesign.WriteOptions{})
	if err != nil {
		t.Fatalf("WriteSchematicProject returned error: %v", err)
	}
	schematicFiles, err := filepath.Glob(filepath.Join(result.ProjectDir, "*.kicad_sch"))
	if err != nil {
		t.Fatalf("find generated schematic: %v", err)
	}
	if len(schematicFiles) != 1 {
		t.Fatalf("generated schematic files = %v, want one", schematicFiles)
	}
	contents, err := os.ReadFile(schematicFiles[0])
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	if !strings.Contains(string(contents), `"kicadai:USB_C_Receptacle_PowerOnly_Full"`) || strings.Contains(string(contents), `"kicadai:usb_c_receptacle_poweronly_full"`) {
		t.Fatalf("schematic should use the canonical local symbol ID:\n%s", contents)
	}
	library, err := os.ReadFile(filepath.Join(result.ProjectDir, "lib", "kicadai_kicadai.kicad_sym"))
	if err != nil {
		t.Fatalf("read generated local symbol library: %v", err)
	}
	if !strings.Contains(string(library), `"USB_C_Receptacle_PowerOnly_Full"`) || !strings.Contains(string(library), `"USB_C_Receptacle_PowerOnly_Full_1_1"`) {
		t.Fatalf("local library should use matching canonical symbol names:\n%s", library)
	}
}

func TestBuilderWriteProjectAddsGeneratedLocalFootprintLibrary(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-0.5), Y: 0}, Size: kicadfiles.Point{X: kicadfiles.MM(0.7), Y: kicadfiles.MM(0.8)}, Shape: "roundrect"},
			{Name: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(0.5), Y: 0}, Size: kicadfiles.Point{X: kicadfiles.MM(0.7), Y: kicadfiles.MM(0.8)}, Shape: "roundrect"},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	root := filepath.Join(t.TempDir(), "footprint_demo")
	result, err := builder.WriteProject(root, kicaddesign.WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProject returned error: %v", err)
	}
	table, err := os.ReadFile(filepath.Join(result.ProjectDir, "fp-lib-table"))
	if err != nil {
		t.Fatalf("expected fp-lib-table: %v", err)
	}
	if !strings.Contains(string(table), `"Resistor_SMD"`) || !strings.Contains(string(table), `"${KIPRJMOD}/footprints/Resistor_SMD.pretty"`) {
		t.Fatalf("fp-lib-table missing generated Resistor_SMD library:\n%s", table)
	}
	modulePath := filepath.Join(result.ProjectDir, "footprints", "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod")
	module, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatalf("expected generated footprint module: %v", err)
	}
	moduleText := string(module)
	for _, want := range []string{`"R_0805_2012Metric"`, `(version 20240108)`, `(generator "kicadai")`, `"Reference"`, `"REF**"`, `"Value"`, `roundrect`, `"1"`, `"2"`} {
		if !strings.Contains(moduleText, want) {
			t.Fatalf("generated footprint missing %q:\n%s", want, moduleText)
		}
	}
	if strings.Contains(moduleText, `"R1"`) || strings.Contains(moduleText, `"10k"`) {
		t.Fatalf("generated footprint library module should not include instance reference or value:\n%s", moduleText)
	}
	if strings.Contains(moduleText, "(net ") {
		t.Fatalf("generated footprint library module should not include board net assignments:\n%s", moduleText)
	}
	boardFiles, err := filepath.Glob(filepath.Join(result.ProjectDir, "*.kicad_pcb"))
	if err != nil {
		t.Fatalf("glob PCB files: %v", err)
	}
	if len(boardFiles) != 1 {
		t.Fatalf("PCB files = %v, want one", boardFiles)
	}
	board, err := os.ReadFile(boardFiles[0])
	if err != nil {
		t.Fatalf("expected PCB file: %v", err)
	}
	if !strings.Contains(string(board), `"Resistor_SMD:R_0805_2012Metric"`) {
		t.Fatalf("PCB should retain qualified footprint library ID:\n%s", board)
	}
}

func TestBuilderWriteProjectRejectsUnsafeGeneratedFootprintPath(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "R1", "Device:R", "10k", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.AssignFootprint("R1", "Resistor_SMD:../R_0805_2012Metric"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("R1", PlaceFootprintOptions{
		Pads: []PadSpec{{Name: "1"}, {Name: "2"}},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	root := filepath.Join(t.TempDir(), "unsafe_footprint")
	if _, err := builder.WriteProject(root, kicaddesign.WriteOptions{}); err == nil || !strings.Contains(err.Error(), "invalid footprint library name") {
		t.Fatalf("WriteProject error = %v, want invalid footprint library name", err)
	}
}

func TestBuilderDesignDoesNotAddImplicitDuplicatePadCopper(t *testing.T) {
	builder := newTestBuilder(t)
	addTwoPinSymbol(t, builder, "U1", "Device:R", "Alias", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)})
	if err := builder.Connect(Endpoint{Reference: "U1", Pin: "1"}, Endpoint{Reference: "U1", Pin: "2"}, "OUT"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := builder.AssignFootprint("U1", "Package_TO_SOT_SMD:SOT-223-3_TabPin2"); err != nil {
		t.Fatalf("AssignFootprint returned error: %v", err)
	}
	if _, err := builder.PlaceFootprint("U1", PlaceFootprintOptions{
		Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		Pads: []PadSpec{
			{Name: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2), Y: 0}, Net: "GND"},
			{Name: "2", Offset: kicadfiles.Point{X: 0, Y: kicadfiles.MM(-2)}, Net: "OUT"},
			{Name: "2", Offset: kicadfiles.Point{X: 0, Y: kicadfiles.MM(2)}, Net: "OUT"},
		},
	}); err != nil {
		t.Fatalf("PlaceFootprint returned error: %v", err)
	}

	design := builder.Design()
	if len(design.PCB.Tracks) != 0 {
		t.Fatalf("tracks = %#v, want no copper without an explicit route", design.PCB.Tracks)
	}
	again := builder.Design()
	if len(again.PCB.Tracks) != 0 {
		t.Fatalf("second Design tracks = %#v, want no implicit copper", again.PCB.Tracks)
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
	firstWireCount := len(builder.Design().Schematic.Wires)
	if firstWireCount == 0 {
		t.Fatal("first Connect did not create wires")
	}
	if err := builder.Connect(endpointB, endpointA, "SIG"); err != nil {
		t.Fatalf("second Connect returned error: %v", err)
	}

	design := builder.Design()
	if len(design.Schematic.Wires) != firstWireCount {
		t.Fatalf("wires = %d, want unchanged count %d", len(design.Schematic.Wires), firstWireCount)
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

func assertSchematicConnectivityGridSafe(t *testing.T, schematicFile *schematic.SchematicFile) {
	t.Helper()
	if schematicFile == nil {
		t.Fatal("schematic required")
	}
	for symbolIndex, symbol := range schematicFile.Symbols {
		if !schematicPointOnConnectionGrid(symbol.Position) {
			t.Fatalf("symbol %d %s position = %#v, want connection-grid aligned", symbolIndex, symbol.Reference, symbol.Position)
		}
		for pinIndex, anchor := range symbol.PinAnchors {
			if !schematicPointOnConnectionGrid(anchor) {
				t.Fatalf("symbol %d %s pin anchor %d = %#v, want connection-grid aligned", symbolIndex, symbol.Reference, pinIndex, anchor)
			}
		}
	}
	for wireIndex, wire := range schematicFile.Wires {
		for pointIndex, point := range wire.Points {
			if !schematicPointOnConnectionGrid(point) {
				t.Fatalf("wire %d point %d = %#v, want connection-grid aligned", wireIndex, pointIndex, point)
			}
		}
	}
	for labelIndex, label := range schematicFile.Labels {
		if !schematicPointOnConnectionGrid(label.Position) {
			t.Fatalf("label %d %s = %#v, want connection-grid aligned", labelIndex, label.Text, label.Position)
		}
	}
}

func schematicPointOnConnectionGrid(point kicadfiles.Point) bool {
	return point.X%schematicConnectionGrid == 0 && point.Y%schematicConnectionGrid == 0
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
