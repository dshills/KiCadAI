package schematicir

import (
	"encoding/json"
	"math"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

func TestToTransactionLEDIndicator(t *testing.T) {
	tx, issues := ToTransaction(validLEDDocument())
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	if tx.Name != "LED1" || tx.Project != "LED1" {
		t.Fatalf("unexpected transaction identity: name=%q project=%q", tx.Name, tx.Project)
	}
	if got := countOperations(tx, transactions.OpCreateProject); got != 1 {
		t.Fatalf("expected one create_project operation, got %d", got)
	}
	if got := countOperations(tx, transactions.OpAddSymbol); got != 3 {
		t.Fatalf("expected three add_symbol operations, got %d", got)
	}
	if got := countOperations(tx, transactions.OpConnect); got != 3 {
		t.Fatalf("expected three connect operations, got %d", got)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	for _, symbol := range symbols {
		if symbol.At.XMM == 0 && symbol.At.YMM == 0 {
			t.Fatalf("symbol %s was not assigned a deterministic placement", symbol.Ref)
		}
	}
	result := transactions.Validate(tx)
	if len(result.Issues) != 0 {
		t.Fatalf("transaction validation issues: %+v", result.Issues)
	}

	connects := decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect)
	expected := map[string]bool{
		"VIN:J1.1->R1.1":   false,
		"LED_A:R1.2->D1.1": false,
		"GND:D1.2->J1.2":   false,
	}
	for _, connect := range connects {
		key := connect.NetName + ":" + connect.From.Ref + "." + connect.From.Pin + "->" + connect.To.Ref + "." + connect.To.Pin
		if _, ok := expected[key]; !ok {
			t.Fatalf("unexpected connect operation %s", key)
		}
		expected[key] = true
	}
	for key, seen := range expected {
		if !seen {
			t.Fatalf("missing connect operation %s", key)
		}
	}
}

func TestToTransactionWithLibraryIndexFailsClosedOnMissingRecords(t *testing.T) {
	document := validLEDDocument()
	document.Circuit.Components[0].Symbol = "Custom:MissingSymbol"
	document.Circuit.Components[0].Footprint = "Custom:MissingFootprint"
	document.Circuit.Components[0].Pins[0].OffsetXMM = floatPtr(-2.54)
	document.Circuit.Components[0].Pins[0].OffsetYMM = floatPtr(0)
	document.Circuit.Components[0].Pins[1].OffsetXMM = floatPtr(2.54)
	document.Circuit.Components[0].Pins[1].OffsetYMM = floatPtr(0)
	index := &libraryresolver.LibraryIndex{}

	_, issues := ToTransactionWithLibraryIndex(document, index)
	if !schematicIRIssueCode(issues, reports.CodeUnknownSymbolLibrary) {
		t.Fatalf("missing symbol record did not fail closed: %#v", issues)
	}
	if !schematicIRIssueCode(issues, reports.CodeUnknownFootprintLibrary) {
		t.Fatalf("missing footprint record did not fail closed: %#v", issues)
	}
}

func schematicIRIssueCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.Blocking() {
			return true
		}
	}
	return false
}

func TestToTransactionEmitsGlobalLabelsForPorts(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Ports = []Port{{Name: "VIN_EXT", Direction: PortDirectionInput, Net: "VIN", Side: SideLeft}}
	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	labels := decodeOperations[transactions.AddLabelOperation](t, tx, transactions.OpAddLabel)
	if len(labels) != 1 || labels[0].Text != "VIN_EXT" || labels[0].Kind != "global" {
		t.Fatalf("port labels = %#v, want one global VIN_EXT label", labels)
	}
	connects := decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect)
	for _, connect := range connects {
		if connect.NetName == "VIN" && (connect.UseLabels == nil || !*connect.UseLabels || !connect.SkipFromLabel || connect.SkipToLabel || len(connect.Waypoints) != 0) {
			t.Fatalf("port net should use global-plus-local labels by default: %#v", connect)
		}
	}
	if result := transactions.Validate(tx); len(result.Issues) != 0 {
		t.Fatalf("transaction validation issues: %+v", result.Issues)
	}
}

func TestToTransactionPortAlwaysUsesGlobalLabel(t *testing.T) {
	doc := validLEDDocument()
	useLabel := true
	doc.Circuit.Nets[0].UseLabel = &useLabel
	doc.Circuit.Ports = []Port{{Name: "VIN_EXT", Direction: PortDirectionInput, Net: doc.Circuit.Nets[0].Name, Side: SideLeft}}
	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	labels := decodeOperations[transactions.AddLabelOperation](t, tx, transactions.OpAddLabel)
	if len(labels) != 1 || labels[0].Kind != "global" || labels[0].Text != "VIN_EXT" {
		t.Fatalf("port label = %#v, want one global label", labels)
	}
	connects := decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect)
	for _, connect := range connects {
		if connect.NetName == doc.Circuit.Nets[0].Name && (connect.UseLabels == nil || !*connect.UseLabels || !connect.SkipFromLabel) {
			t.Fatalf("port net should preserve global label semantics even when use_label is explicit: %#v", connect)
		}
	}
}

func TestPortEndpointSelectionFollowsDeclaredSide(t *testing.T) {
	doc := validLEDDocument()
	state, issues := newAdapterState(doc, nil)
	if len(issues) != 0 {
		t.Fatalf("adapter state issues: %+v", issues)
	}
	net := doc.Circuit.Nets[0]
	if got := state.portEndpointForSide(net.Name, net.Connect, SideLeft); got != "vin.1" {
		t.Fatalf("left port endpoint = %q, want vin.1", got)
	}
	if got := state.portEndpointForSide(net.Name, net.Connect, SideRight); got != "r_limit.1" {
		t.Fatalf("right port endpoint = %q, want r_limit.1", got)
	}
}

func TestReadableAcceptanceBlocksLayoutWarnings(t *testing.T) {
	doc := *NewDocument()
	doc.Policy.Acceptance = AcceptanceReadable
	result := schematiclayout.Result{Diagnostics: []schematiclayout.Diagnostic{
		{Severity: schematiclayout.SeverityWarning, Code: "wire_crossing", Message: "crossing", Repair: "reroute"},
		{Severity: schematiclayout.SeverityInfo, Code: "page_escalated", Message: "escalated"},
	}}
	issues := schematicLayoutAcceptanceIssues(doc, result)
	if len(issues) != 1 || issues[0].Severity != reports.SeverityBlocked || issues[0].Path != "layout.wire_crossing" {
		t.Fatalf("readable acceptance issues = %#v", issues)
	}
}

func TestToTransactionPreservesSharedReferenceUnits(t *testing.T) {
	doc := *NewDocument()
	doc.Metadata.Name = "dual_unit"
	doc.Circuit.Components = []Component{
		{ID: "u1a", Ref: "U1", Unit: "1", Role: ComponentRoleIC, Symbol: "Device:R", Value: "DUAL", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "u1b", Ref: "U1", Unit: "2", Role: ComponentRoleIC, Symbol: "Device:R", Value: "DUAL", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
	}
	doc.Circuit.Nets = []Net{{Name: "UNIT_LINK", Role: NetRoleSignal, Connect: []EndpointRef{"u1a.2", "u1b.1"}}}
	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if len(symbols) != 2 || symbols[0].Unit != 1 || symbols[1].Unit != 2 {
		t.Fatalf("symbols = %#v, want units 1 and 2", symbols)
	}
	connects := decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect)
	if len(connects) != 1 || connects[0].From.Unit != 1 || connects[0].To.Unit != 2 {
		t.Fatalf("connect = %#v, want unit-aware endpoints", connects)
	}
}

func TestSchematicLayoutPrefersLabelsForLogicalBusNets(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets[1].Role = NetRoleBus
	result := schematicLayout(doc)
	for _, connection := range result.Connections {
		if connection.NetName == doc.Circuit.Nets[1].Name && !connection.UseLabels {
			t.Fatalf("bus connection should use labels: %#v", connection)
		}
	}
}

func TestSchematicLayoutPropagatesExplicitBodyGeometry(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Body = &BodyGeometry{MinXMM: -9, MinYMM: -2, MaxXMM: 4, MaxYMM: 11}
	result := schematicLayout(doc)
	for _, component := range result.Components {
		if component.Ref != "r_limit" {
			continue
		}
		want := schematiclayout.Rect{MinX: kicadfiles.MM(-9), MinY: kicadfiles.MM(-2), MaxX: kicadfiles.MM(4), MaxY: kicadfiles.MM(11)}
		if component.Body != want {
			t.Fatalf("body = %#v, want %#v", component.Body, want)
		}
		return
	}
	t.Fatal("missing r_limit layout component")
}

func TestSchematicLayoutUsesResolverPinEnvelopeWithoutGraphics(t *testing.T) {
	doc := *NewDocument()
	doc.Circuit.Components = []Component{{
		ID: "custom", Ref: "X1", Role: ComponentRoleGeneric, Symbol: "Custom:PinOnly",
		Pins: []Pin{{Number: "1"}, {Number: "2"}},
	}}
	doc.Circuit.Nets = []Net{{Name: "N", Connect: []EndpointRef{"custom.1", "custom.1"}}}
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{
		"Custom:PinOnly": {
			LibraryID: "Custom:PinOnly",
			Pins: []libraryresolver.SymbolPin{
				{Number: "1", Position: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
				{Number: "2", Position: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
			},
		},
	}}
	result := schematicLayoutWithLibraryIndex(NormalizeLayoutIntent(doc), &index)
	if len(result.Components) != 1 || result.Components[0].Body.Empty() {
		t.Fatalf("resolver pin-only body = %#v, want conservative non-empty bounds", result.Components)
	}
	body := result.Components[0].Body
	if body.MinX != kicadfiles.MM(-3.81) || body.MaxX != kicadfiles.MM(3.81) {
		t.Fatalf("resolver pin-only bounds = %#v, want both pin positions plus padding", body)
	}
}

func TestToTransactionUsesSharedGraphLayoutAndCentersResult(t *testing.T) {
	doc := validLEDDocument()
	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	points := map[string]transactions.Point{}
	for _, symbol := range symbols {
		points[symbol.Ref] = symbol.At
	}
	if !(points["J1"].XMM < points["R1"].XMM && points["R1"].XMM < points["D1"].XMM) {
		t.Fatalf("graph layout points = %#v, want connector-to-resistor-to-LED flow", points)
	}
	minX, maxX := points["J1"].XMM, points["J1"].XMM
	for _, point := range points {
		minX = math.Min(minX, point.XMM)
		maxX = math.Max(maxX, point.XMM)
	}
	if center := (minX + maxX) / 2; math.Abs(center-148.5) > 10 {
		t.Fatalf("layout center x = %.2f, want near A4 center", center)
	}
}

func TestToTransactionEmitsTemplatePinOffsetsAndLabelPolicy(t *testing.T) {
	tx, issues := ToTransaction(validLEDDocument())
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	for _, symbol := range symbols {
		for _, pin := range symbol.Pins {
			if pin.XMM == 0 && pin.YMM == 0 {
				t.Fatalf("symbol %s pin %s has no template offset", symbol.Ref, pin.Number)
			}
		}
	}
	connects := decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect)
	preferences := map[string]*bool{}
	waypointCounts := map[string]int{}
	for _, connect := range connects {
		preferences[connect.NetName] = connect.UseLabels
		waypointCounts[connect.NetName] = len(connect.Waypoints)
	}
	for _, netName := range []string{"VIN", "GND"} {
		if preferences[netName] == nil || !*preferences[netName] {
			t.Fatalf("%s label preference = %#v, want forced labels", netName, preferences[netName])
		}
	}
	if preferences["LED_A"] == nil || *preferences["LED_A"] {
		t.Fatalf("LED_A label preference = %#v, want explicit local routing", preferences["LED_A"])
	}
	if waypointCounts["LED_A"] < 2 {
		t.Fatalf("LED_A waypoint count = %d, want explicit orthogonal path", waypointCounts["LED_A"])
	}
}

func TestToTransactionSupportsAllQuarterTurnOrientations(t *testing.T) {
	doc := validLEDDocument()
	doc.Layout.Placements = []Placement{
		{Target: "vin", Orientation: OrientationNormal},
		{Target: "r_limit", Orientation: OrientationRotated180},
		{Target: "led", Orientation: OrientationRotated270},
	}
	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	rotations := map[string]float64{}
	for _, symbol := range symbols {
		rotations[symbol.Ref] = symbol.Rotation
	}
	if rotations["J1"] != 0 || rotations["R1"] != 180 || rotations["D1"] != 270 {
		t.Fatalf("rotations = %#v", rotations)
	}
}

func TestToTransactionEmitsExplicitReadableFieldPositions(t *testing.T) {
	tx, issues := ToTransaction(validLEDDocument())
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	for _, symbol := range decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol) {
		reference := symbolPropertyByName(t, symbol.Properties, "Reference")
		value := symbolPropertyByName(t, symbol.Properties, "Value")
		if reference.At == nil || reference.DoNotAutoplace == nil || !*reference.DoNotAutoplace {
			t.Fatalf("symbol %s reference layout property = %#v", symbol.Ref, reference)
		}
		if (!value.Hidden && value.At == nil) || value.DoNotAutoplace == nil || !*value.DoNotAutoplace {
			t.Fatalf("symbol %s value layout property = %#v", symbol.Ref, value)
		}
	}
}

func TestToTransactionEmitsLabelPointsForEveryHighFanoutEndpoint(t *testing.T) {
	doc := loadExampleDocument(t, "usb_c_led_indicator.json")
	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	for _, connect := range decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect) {
		if connect.NetName != "VBUS" {
			continue
		}
		if connect.UseLabels == nil || !*connect.UseLabels || connect.FromLabelAt == nil || connect.ToLabelAt == nil {
			t.Fatalf("VBUS connection missing explicit label geometry: %#v", connect)
		}
	}
}

func TestToTransactionAssignsMissingReferencesWhenAllowed(t *testing.T) {
	doc := validLEDDocument()
	for index := range doc.Circuit.Components {
		doc.Circuit.Components[index].Ref = ""
	}
	doc.Circuit.Components[2].Ref = "R1"

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if got, want := symbols[0].Ref, "J1"; got != want {
		t.Fatalf("first generated ref = %q, want %q", got, want)
	}
	if got, want := symbols[1].Ref, "R2"; got != want {
		t.Fatalf("second generated ref = %q, want %q", got, want)
	}
	if got, want := symbols[2].Ref, "R1"; got != want {
		t.Fatalf("third generated ref = %q, want %q", got, want)
	}
}

func TestToTransactionRejectsMissingReferenceWhenRepairDisabled(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[0].Ref = ""
	doc.Policy.Repair.AllowRefAssignment = false

	_, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected issue for missing reference")
	}
}

func TestToTransactionRejectsDuplicateExplicitReferences(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Ref = "D1"

	tx, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected issue for duplicate reference")
	}
	if len(tx.Operations) != 0 {
		t.Fatalf("expected no operations for duplicate reference, got %d", len(tx.Operations))
	}
}

func TestToTransactionAssignsFootprintsAndProperties(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Footprint = "Resistor_SMD:R_0603_1608Metric"
	doc.Circuit.Components[1].Properties = map[string]string{
		"Tolerance": "1%",
		"MPN":       "RC0603FR-071KL",
	}

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	assigns := decodeOperations[transactions.AssignFootprintOperation](t, tx, transactions.OpAssignFootprint)
	if len(assigns) != 1 {
		t.Fatalf("expected one assign_footprint operation, got %d", len(assigns))
	}
	if assigns[0].Ref != "R1" || assigns[0].FootprintID != "Resistor_SMD:R_0603_1608Metric" {
		t.Fatalf("unexpected footprint assignment: %+v", assigns[0])
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if len(symbols[1].Properties) != 5 {
		t.Fatalf("expected three custom/footprint and two layout properties, got %+v", symbols[1].Properties)
	}
	if symbols[1].Properties[0].Name != "Footprint" || symbols[1].Properties[1].Name != "MPN" || symbols[1].Properties[2].Name != "Reference" || symbols[1].Properties[3].Name != "Tolerance" || symbols[1].Properties[4].Name != "Value" {
		t.Fatalf("properties are not sorted deterministically: %+v", symbols[1].Properties)
	}
	footprint := symbols[1].Properties[0]
	if footprint.Name != "Footprint" || footprint.Value != "Resistor_SMD:R_0603_1608Metric" || !footprint.Hidden {
		t.Fatalf("footprint was not emitted as a hidden symbol property: %+v", footprint)
	}
}

func TestToTransactionPreservesGenericFootprintPropertyWithoutExplicitFootprint(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Footprint = ""
	doc.Circuit.Components[1].Properties = map[string]string{
		"Footprint": "Resistor_SMD:R_0603_1608Metric",
	}

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if len(symbols[1].Properties) != 3 {
		t.Fatalf("expected footprint and two layout properties, got %+v", symbols[1].Properties)
	}
	footprint := symbolPropertyByName(t, symbols[1].Properties, "Footprint")
	if footprint.Name != "Footprint" || footprint.Value != "Resistor_SMD:R_0603_1608Metric" || footprint.Hidden {
		t.Fatalf("generic footprint property was not preserved: %+v", footprint)
	}
}

func TestToTransactionDeduplicatesPropertyNamesCaseInsensitively(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Properties = map[string]string{
		"MPN": "first",
		"mpn": "duplicate",
	}

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	symbols := decodeOperations[transactions.AddSymbolOperation](t, tx, transactions.OpAddSymbol)
	if len(symbols[1].Properties) != 3 {
		t.Fatalf("expected deduped MPN and two layout properties, got %+v", symbols[1].Properties)
	}
	mpn := symbolPropertyByName(t, symbols[1].Properties, "MPN")
	if mpn.Value != "first" {
		t.Fatalf("unexpected deduped property: %+v", mpn)
	}
}

func symbolPropertyByName(t *testing.T, properties []transactions.SymbolProperty, name string) transactions.SymbolProperty {
	t.Helper()
	for _, property := range properties {
		if property.Name == name {
			return property
		}
	}
	t.Fatalf("missing property %s in %+v", name, properties)
	return transactions.SymbolProperty{}
}

func TestToTransactionNoConnectNet(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[0].Pins = append(doc.Circuit.Components[0].Pins, Pin{Number: "3"})
	doc.Circuit.Nets = append(doc.Circuit.Nets, Net{
		Name:    "NC_SPARE",
		Role:    NetRoleNoConnect,
		Connect: []EndpointRef{"vin.3"},
	})

	tx, issues := ToTransaction(doc)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	noConnects := decodeOperations[transactions.AddNoConnectOperation](t, tx, transactions.OpAddNoConnect)
	if len(noConnects) != 1 {
		t.Fatalf("expected one no-connect operation, got %d", len(noConnects))
	}
	if noConnects[0].Endpoint.Ref != "J1" || noConnects[0].Endpoint.Pin != "3" {
		t.Fatalf("unexpected no-connect endpoint: %+v", noConnects[0].Endpoint)
	}
}

func TestToTransactionRejectsInvalidIR(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets[0].Connect[0] = "missing.1"

	tx, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected validation issue")
	}
	if len(tx.Operations) != 0 {
		t.Fatalf("expected no operations for invalid IR, got %d", len(tx.Operations))
	}
}

func TestToTransactionRejectsInvalidUnit(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Unit = "A"

	tx, issues := ToTransaction(doc)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid unit")
	}
	if len(tx.Operations) != 0 {
		t.Fatalf("expected no operations for invalid unit, got %d", len(tx.Operations))
	}
}

func countOperations(tx transactions.Transaction, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range tx.Operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}

func decodeOperations[T any](t *testing.T, tx transactions.Transaction, kind transactions.OperationKind) []T {
	t.Helper()
	var out []T
	for _, operation := range tx.Operations {
		if operation.Op != kind {
			continue
		}
		var payload T
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("decode %s: %v", kind, err)
		}
		out = append(out, payload)
	}
	return out
}
