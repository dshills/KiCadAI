package designworkflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/componentprops"
	"kicadai/internal/components"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestApplyComponentSelectionsToPlanAddsSymbolIdentityProperties(t *testing.T) {
	plan := componentSelectionTestPlan(t, nil)
	registry := componentSelectionTestRegistry{definition: componentSelectionTestDefinition()}
	selection := ComponentSelectionEntry{
		InstanceID:     "status",
		BlockID:        "test_block",
		Role:           "indicator",
		ComponentID:    "led.red.0603",
		VariantID:      "0603",
		Manufacturer:   "Kingbright",
		MPN:            "APT1608",
		ComponentClass: "led",
		SymbolID:       "Device:LED",
		FootprintID:    "LED_SMD:LED_0603_1608Metric",
		PinMapID:       "led-0603-pinmap",
		Confidence:     components.ConfidenceVerified,
		Procurement:    &components.ProcurementEvidence{LifecycleStatus: components.LifecycleActive, AvailabilityStatus: components.AvailabilityInStock},
	}

	issues := ApplyComponentSelectionsToPlan(&plan, registry, []ComponentSelectionEntry{selection})

	if reports.HasBlockingIssue(issues) {
		t.Fatalf("unexpected blocking issues: %#v", issues)
	}
	add := decodeAddSymbolOperation(t, plan.Output.Operations[0])
	if add.LibraryID != "Device:LED" {
		t.Fatalf("library_id = %q", add.LibraryID)
	}
	properties := symbolPropertyValues(add.Properties)
	assertProperty(t, properties, componentprops.PropertyComponentID, "led.red.0603")
	assertProperty(t, properties, componentprops.PropertyVariantID, "0603")
	assertProperty(t, properties, componentprops.PropertyComponentRole, "indicator")
	assertProperty(t, properties, componentprops.PropertyBlockID, "test_block")
	assertProperty(t, properties, componentprops.PropertyManufacturer, "Kingbright")
	assertProperty(t, properties, componentprops.PropertyMPN, "APT1608")
	assertProperty(t, properties, componentprops.PropertyComponentClass, "led")
	assertProperty(t, properties, componentprops.PropertyComponentConfidence, string(components.ConfidenceVerified))
	assertProperty(t, properties, componentprops.PropertyComponentSource, componentprops.SourceCatalogSnapshot)
	assertProperty(t, properties, componentprops.PropertyLifecycleStatus, string(components.LifecycleActive))
	assertProperty(t, properties, componentprops.PropertyAvailabilityStatus, string(components.AvailabilityInStock))
	assertProperty(t, properties, componentprops.PropertyPinmapID, "led-0603-pinmap")
	for _, property := range add.Properties {
		if componentprops.IsOwnedPropertyName(property.Name) && (!property.Hidden || property.ShowName == nil || *property.ShowName || property.DoNotAutoplace == nil || !*property.DoNotAutoplace) {
			t.Fatalf("identity property not hidden metadata: %#v", property)
		}
	}
	assign := decodeAssignFootprintOperation(t, plan.Output.Operations[1])
	if assign.FootprintID != "LED_SMD:LED_0603_1608Metric" {
		t.Fatalf("assign footprint = %q", assign.FootprintID)
	}
}

func TestComponentSelectionEvidenceUsesCatalogLifecycleWithoutProcurementSnapshot(t *testing.T) {
	evidence := componentSelectionEvidence(ComponentSelectionEntry{
		ComponentID: "opamp.example",
		Lifecycle:   "active",
	})
	if evidence.LifecycleStatus != "active" {
		t.Fatalf("lifecycle = %q, want catalog lifecycle", evidence.LifecycleStatus)
	}
}

func TestApplyComponentSelectionsToPlanWarnsOnIdentityReplacement(t *testing.T) {
	falseValue := false
	trueValue := true
	plan := componentSelectionTestPlan(t, []transactions.SymbolProperty{{
		Name:           componentprops.PropertyComponentID,
		Value:          "old.component",
		Hidden:         true,
		ShowName:       &falseValue,
		DoNotAutoplace: &trueValue,
	}})
	registry := componentSelectionTestRegistry{definition: componentSelectionTestDefinition()}

	issues := ApplyComponentSelectionsToPlan(&plan, registry, []ComponentSelectionEntry{{
		InstanceID:  "status",
		BlockID:     "test_block",
		Role:        "indicator",
		ComponentID: "new.component",
		Confidence:  components.ConfidenceVerified,
	}})

	if len(issues) != 1 || issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("issues = %#v, want one warning", issues)
	}
	add := decodeAddSymbolOperation(t, plan.Output.Operations[0])
	properties := symbolPropertyValues(add.Properties)
	assertProperty(t, properties, componentprops.PropertyComponentID, "new.component")
}

func TestMatchingWorkflowRefPrefersAuthoritativeRoleOverSharedGeometry(t *testing.T) {
	component := blocks.BlockComponent{
		Role:        "output_isolation",
		SymbolID:    "Device:R",
		FootprintID: "Resistor_SMD:R_0805_2012Metric",
	}
	facts := map[string]workflowComponentFact{
		"R1": {Role: "gain_to_star", SymbolID: component.SymbolID, FootprintID: component.FootprintID},
		"R2": {Role: "feedback", SymbolID: component.SymbolID, FootprintID: component.FootprintID},
		"R3": {Role: "output_isolation", SymbolID: component.SymbolID, FootprintID: component.FootprintID},
	}

	if got := matchingWorkflowRefForComponent(component, []string{"R1", "R2", "R3"}, facts, nil); got != "R3" {
		t.Fatalf("matching ref = %q, want role-authoritative R3", got)
	}
}

func TestAmplifierOutputPairRequestSeparatesSpeakerContract(t *testing.T) {
	request, ok := amplifierOutputPairRequestForBlock("class_ab_speaker_power_stage", map[string]any{"supply_voltage": "36V", "target_load": "8Ω", "application": "headphone"}, components.AcceptanceFabricationCandidate)
	if !ok || request.Application != "power" || request.LoadImpedance != "8Ω" || request.RequireHeadphone {
		t.Fatalf("speaker pair request = %#v, want distinct power-stage contract", request)
	}
}

func TestAmplifierOutputPairRequestUsesSpeakerBlockDefaultLoad(t *testing.T) {
	request, ok := amplifierOutputPairRequestForBlock("class_ab_speaker_power_stage", map[string]any{"supply_voltage": "36V"}, components.AcceptanceFabricationCandidate)
	if !ok || request.LoadImpedance != "8Ω" {
		t.Fatalf("speaker pair request = %#v, want block-default 8 ohm load", request)
	}
}

func TestConcreteI2CSensorSelectionReachesWrittenSchematic(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	request := Request{
		Version: RequestVersion,
		Name:    "humidity_sensor",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "sensor",
			BlockID: "i2c_sensor",
			Params: map[string]any{
				"sensor_component_id": "sensor.sensirion.sht31_dis.dfn8",
				"i2c_address":         "0x44",
				"supply_voltage":      "3.3V",
			},
		}},
		Validation: ValidationSpec{Acceptance: AcceptanceConnectivity},
	}
	plan := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues = %#v", plan.Stage.Issues)
	}
	selectionResult := SelectWorkflowComponents(context.Background(), registry, plan, ComponentSelectionOptions{})
	if reports.HasBlockingIssue(selectionResult.Stage.Issues) {
		t.Fatalf("selection issues = %#v", selectionResult.Stage.Issues)
	}
	var sensorSelection *ComponentSelectionEntry
	for i := range selectionResult.Selections {
		if selectionResult.Selections[i].InstanceID == "sensor" && selectionResult.Selections[i].Role == "sensor" {
			sensorSelection = &selectionResult.Selections[i]
			break
		}
	}
	if sensorSelection == nil || sensorSelection.ComponentID != "sensor.sensirion.sht31_dis.dfn8" || sensorSelection.SymbolID != "Sensor_Humidity:SHT31-DIS" {
		t.Fatalf("sensor selection = %#v", sensorSelection)
	}
	if issues := ApplyComponentSelectionsToPlan(&plan, registry, selectionResult.Selections); reports.HasBlockingIssue(issues) {
		t.Fatalf("apply selection issues = %#v", issues)
	}
	found := false
	for _, operation := range plan.Output.Operations {
		if operation.Op != transactions.OpAddSymbol || operation.Ref != plan.Output.Instances[0].Refs[0] {
			continue
		}
		add := decodeAddSymbolOperation(t, operation)
		properties := symbolPropertyValues(add.Properties)
		assertProperty(t, properties, componentprops.PropertyComponentID, "sensor.sensirion.sht31_dis.dfn8")
		assertProperty(t, properties, componentprops.PropertyMPN, "SHT31-DIS")
		found = true
	}
	if !found {
		t.Fatal("selected sensor add-symbol operation not found")
	}
	stage := schematicStageFromPlan(plan)
	readability, ok := stage.Summary["readability"].(map[string]any)
	if !ok {
		t.Fatalf("readability summary missing: %#v", stage.Summary)
	}
	for _, key := range []string{"diagonal_wire_count", "decode_error_count", "stage_order_violation_count", "power_placement_violation_count"} {
		if got := summaryInt(t, readability, key); got != 0 {
			t.Fatalf("%s = %d, want 0; summary=%#v", key, got, readability)
		}
	}
	tx, err := blocks.ProjectTransactionForCompositionOutput(request.Name, plan.Output, false)
	if err != nil {
		t.Fatal(err)
	}
	index := componentSelectionResolverIndex(t, plan.Output.Operations)
	outputDir := filepath.Join(t.TempDir(), request.Name)
	applied := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, LibraryIndex: &index})
	if reports.HasBlockingIssue(applied.Issues) {
		t.Fatalf("apply issues = %#v", applied.Issues)
	}
	written, err := schematic.ReadFile(filepath.Join(outputDir, request.Name+".kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	writtenSensorCount := 0
	for _, symbol := range written.Symbols {
		if symbol.LibraryID != "Sensor_Humidity:SHT31-DIS" {
			continue
		}
		writtenSensorCount++
		properties := map[string]string{}
		for _, property := range symbol.Properties {
			properties[property.Name] = property.Value
		}
		assertProperty(t, properties, componentprops.PropertyComponentID, "sensor.sensirion.sht31_dis.dfn8")
		assertProperty(t, properties, componentprops.PropertyMPN, "SHT31-DIS")
	}
	if writtenSensorCount != 1 {
		t.Fatalf("written concrete sensor count = %d, want 1", writtenSensorCount)
	}
}

func TestClassABOutputStageSelectsComplementaryPairAsOneEnvelope(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	request := Request{
		Version: RequestVersion,
		Name:    "catalog_class_ab",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "output",
			BlockID: "class_ab_output_stage",
			Params: map[string]any{
				"application":    "headphone",
				"supply_voltage": "9V",
				"load_impedance": "32Ω",
			},
		}},
		Validation: ValidationSpec{Acceptance: AcceptanceConnectivity},
	}
	plan := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues = %#v", plan.Stage.Issues)
	}
	selectionResult := SelectWorkflowComponents(context.Background(), registry, plan, ComponentSelectionOptions{})
	if reports.HasBlockingIssue(selectionResult.Stage.Issues) {
		t.Fatalf("selection issues = %#v", selectionResult.Stage.Issues)
	}
	selected := map[string]ComponentSelectionEntry{}
	for _, selection := range selectionResult.Selections {
		if selection.InstanceID == "output" {
			selected[selection.Role] = selection
		}
	}
	if selected["upper_output"].ComponentID != "bjt.onsemi.mmbt3904.sot23" || selected["lower_output"].ComponentID != "bjt.onsemi.mmbt3906.sot23" {
		t.Fatalf("selected pair = %#v", selected)
	}
	upperGroup := selected["upper_output"].AmplifierOutput.ComplementaryGroup
	lowerGroup := selected["lower_output"].AmplifierOutput.ComplementaryGroup
	if upperGroup == "" || upperGroup != lowerGroup {
		t.Fatalf("complementary groups = %q / %q", upperGroup, lowerGroup)
	}
}

func TestClassABSpeakerPowerStageSelectsFabricationProvenPowerPair(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	request := Request{
		Version: RequestVersion,
		Name:    "catalog_speaker_power",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 70, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "power_output",
			BlockID: "class_ab_speaker_power_stage",
			Params: map[string]any{
				"supply_voltage": "36V",
				"target_load":    "8Ω",
				"minimum_load":   "4Ω",
				"target_power":   10.0,
			},
		}},
		Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate},
	}
	plan := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues = %#v", plan.Stage.Issues)
	}
	selectionResult := SelectWorkflowComponents(context.Background(), registry, plan, ComponentSelectionOptions{})
	if reports.HasBlockingIssue(selectionResult.Stage.Issues) {
		t.Fatalf("selection issues = %#v", selectionResult.Stage.Issues)
	}
	selected := map[string]ComponentSelectionEntry{}
	for _, selection := range selectionResult.Selections {
		if selection.InstanceID == "power_output" {
			selected[selection.Role] = selection
		}
	}
	if selected["upper_output"].ComponentID != "bjt.onsemi.d44h11g.to220" || selected["lower_output"].ComponentID != "bjt.onsemi.d45h11g.to220" {
		t.Fatalf("selected speaker pair = %#v", selected)
	}
	if selected["upper_output"].AmplifierOutput.SafeOperatingAreaStatus != "proven" || selected["lower_output"].AmplifierOutput.SafeOperatingAreaStatus != "proven" {
		t.Fatalf("selected speaker SOA evidence = %#v / %#v", selected["upper_output"].AmplifierOutput, selected["lower_output"].AmplifierOutput)
	}
}

func TestSpeakerOpAmpDriverSelectsFabricationProvenOPA134(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	request := Request{
		Version: RequestVersion,
		Name:    "catalog_speaker_gain",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 70, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "gain",
			BlockID: "speaker_opamp_driver",
			Params:  map[string]any{"gain": 11.0},
		}},
		Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate},
	}
	plan := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues = %#v", plan.Stage.Issues)
	}
	selectionResult := SelectWorkflowComponents(context.Background(), registry, plan, ComponentSelectionOptions{})
	if reports.HasBlockingIssue(selectionResult.Stage.Issues) {
		t.Fatalf("selection issues = %#v", selectionResult.Stage.Issues)
	}
	selected := map[string]ComponentSelectionEntry{}
	for _, selection := range selectionResult.Selections {
		if selection.InstanceID == "gain" {
			selected[selection.Role] = selection
		}
	}
	if selected["opamp"].ComponentID != "opamp.ti.opa134ua.soic8" || selected["output_isolation"].ComponentID != "resistor.yageo.rc0805fr_0747rl.0805" {
		t.Fatalf("selected speaker gain components = %#v", selected)
	}
	if selected["gain_to_star"].ComponentID != "resistor.vishay.tnpw0805.1k00.1p0" || selected["feedback"].ComponentID != "resistor.yageo.rc0805fr_0710kl.0805" {
		t.Fatalf("selected derived speaker feedback values = %#v", selected)
	}
}

func TestSpeakerOutputProtectionSelectsConcreteDetectorsRelayAndClamp(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	request := Request{
		Version:    RequestVersion,
		Name:       "catalog_speaker_protection",
		Board:      BoardSpec{WidthMM: 100, HeightMM: 70, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "protect", BlockID: "speaker_output_protection"}},
		Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate},
	}
	plan := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues = %#v", plan.Stage.Issues)
	}
	selectionResult := SelectWorkflowComponents(context.Background(), registry, plan, ComponentSelectionOptions{})
	if reports.HasBlockingIssue(selectionResult.Stage.Issues) {
		t.Fatalf("selection issues = %#v", selectionResult.Stage.Issues)
	}
	selected := map[string]ComponentSelectionEntry{}
	for _, selection := range selectionResult.Selections {
		if selection.InstanceID == "protect" {
			selected[selection.Role] = selection
		}
	}
	for _, role := range []string{"positive_detector", "negative_detector"} {
		if selected[role].ComponentID != "comparator.ti.tlv1701aidbvr.sot23_5" {
			t.Fatalf("%s selection = %#v", role, selected[role])
		}
	}
	if selected["relay"].ComponentID != "relay.omron.g5q_1a.dc12" || selected["relay_driver"].ComponentID != "bjt.onsemi.mmbt3904.sot23" || selected["relay_flyback"].ComponentID != "diode.onsemi.1n4148w.sod_123" {
		t.Fatalf("protection selections = %#v", selected)
	}
}

func componentSelectionResolverIndex(t *testing.T, operations []transactions.Operation) libraryresolver.LibraryIndex {
	t.Helper()
	// This minimal passive geometry exists only to prove that workflow identity
	// properties survive KiCad serialization. Concrete pin semantics and
	// footprint mappings are covered by block and pinmap tests.
	index := libraryresolver.LibraryIndex{
		Symbols:    map[string]libraryresolver.SymbolRecord{},
		Footprints: map[string]libraryresolver.FootprintRecord{},
	}
	pinsByRef := map[string][]transactions.PinSpec{}
	for _, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		add := decodeAddSymbolOperation(t, operation)
		parts := strings.SplitN(add.LibraryID, ":", 2)
		name := parts[len(parts)-1]
		nickname := ""
		if len(parts) == 2 {
			nickname = parts[0]
		}
		unit := add.Unit
		if unit == 0 {
			unit = 1
		}
		pins := make([]libraryresolver.SymbolPin, 0, len(add.Pins))
		for _, pin := range add.Pins {
			pins = append(pins, libraryresolver.SymbolPin{
				Number:     pin.Number,
				Electrical: "passive",
				Unit:       unit,
				BodyStyle:  1,
				Position:   kicadfiles.Point{X: kicadfiles.MM(pin.XMM), Y: kicadfiles.MM(pin.YMM)},
			})
		}
		record, exists := index.Symbols[add.LibraryID]
		if !exists {
			record = libraryresolver.SymbolRecord{
				LibraryID:       add.LibraryID,
				LibraryNickname: nickname,
				Name:            name,
			}
		}
		unitExists := false
		for _, existing := range record.Units {
			if existing.Unit == unit && existing.BodyStyle == 1 {
				unitExists = true
				break
			}
		}
		if !unitExists {
			record.Units = append(record.Units, libraryresolver.SymbolUnit{Unit: unit, BodyStyle: 1})
		}
		for _, pin := range pins {
			pinExists := false
			for _, existing := range record.Pins {
				if existing.Unit == pin.Unit && existing.BodyStyle == pin.BodyStyle && existing.Number == pin.Number {
					pinExists = true
					break
				}
			}
			if !pinExists {
				record.Pins = append(record.Pins, pin)
			}
		}
		index.Symbols[add.LibraryID] = record
		pinsByRef[add.Ref] = append(pinsByRef[add.Ref], add.Pins...)
	}
	for _, operation := range operations {
		if operation.Op != transactions.OpAssignFootprint {
			continue
		}
		assign := decodeAssignFootprintOperation(t, operation)
		refPins, ok := pinsByRef[assign.Ref]
		if !ok || len(refPins) == 0 {
			t.Fatalf("footprint assignment %s has no matching generated symbol pins for %s", assign.FootprintID, assign.Ref)
		}
		parts := strings.SplitN(assign.FootprintID, ":", 2)
		name := parts[len(parts)-1]
		nickname := ""
		if len(parts) == 2 {
			nickname = parts[0]
		}
		record, exists := index.Footprints[assign.FootprintID]
		if !exists {
			record = libraryresolver.FootprintRecord{
				FootprintID:     assign.FootprintID,
				LibraryNickname: nickname,
				Name:            name,
				Attributes:      []string{"smd"},
			}
		}
		pads := slices.Clone(record.Pads)
		seenPads := map[string]bool{}
		for _, pad := range pads {
			seenPads[pad.Name] = true
		}
		for _, pin := range refPins {
			if seenPads[pin.Number] {
				continue
			}
			seenPads[pin.Number] = true
			pads = append(pads, libraryresolver.FootprintPad{
				Name:   pin.Number,
				Type:   "smd",
				Shape:  "rect",
				Size:   kicadfiles.Point{X: kicadfiles.MM(0.8), Y: kicadfiles.MM(0.8)},
				Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFPaste, kicadfiles.LayerFMask},
			})
		}
		record.Pads = pads
		index.Footprints[assign.FootprintID] = record
	}
	return index
}

func componentSelectionTestDefinition() blocks.BlockDefinition {
	return blocks.BlockDefinition{
		ID:      "test_block",
		Name:    "Test Block",
		Version: "1.0.0",
		Components: []blocks.BlockComponent{{
			Role:        "indicator",
			SymbolID:    "Device:D",
			FootprintID: "LED_SMD:LED_0805_2012Metric",
		}},
	}
}

type componentSelectionTestRegistry struct {
	definition blocks.BlockDefinition
}

func (registry componentSelectionTestRegistry) ListBlocks() []blocks.BlockSummary {
	return []blocks.BlockSummary{blocks.Summary(registry.definition)}
}

func (registry componentSelectionTestRegistry) GetBlock(id string) (blocks.BlockDefinition, bool) {
	return registry.definition, id == registry.definition.ID
}

func (registry componentSelectionTestRegistry) ValidateDefinition(definition blocks.BlockDefinition) []reports.Issue {
	return nil
}

func (registry componentSelectionTestRegistry) ValidateRequest(request blocks.BlockRequest) []reports.Issue {
	return nil
}

func (registry componentSelectionTestRegistry) Instantiate(ctx context.Context, request blocks.BlockRequest) (blocks.BlockOutput, []reports.Issue) {
	return blocks.BlockOutput{}, nil
}

func componentSelectionTestPlan(t *testing.T, properties []transactions.SymbolProperty) BlockPlanResult {
	t.Helper()
	add := transactions.AddSymbolOperation{
		Op:        transactions.OpAddSymbol,
		Ref:       "D1",
		Role:      "indicator",
		LibraryID: "Device:D",
		At:        transactions.Point{XMM: 10, YMM: 20},
		Properties: append([]transactions.SymbolProperty{{
			Name:  "Custom",
			Value: "kept",
		}}, properties...),
	}
	assign := transactions.AssignFootprintOperation{
		Op:          transactions.OpAssignFootprint,
		Ref:         "D1",
		FootprintID: "LED_SMD:LED_0805_2012Metric",
	}
	return BlockPlanResult{
		Request: Request{Blocks: []BlockInstanceSpec{{ID: "status", BlockID: "test_block"}}},
		Output: blocks.CompositionOutput{
			Instances: []blocks.BlockInstance{{BlockID: "test_block", InstanceID: "status", Refs: []string{"D1"}}},
			Operations: []transactions.Operation{
				mustComponentSelectionOperation(t, transactions.OpAddSymbol, add),
				mustComponentSelectionOperation(t, transactions.OpAssignFootprint, assign),
			},
		},
	}
}

func mustComponentSelectionOperation(t *testing.T, kind transactions.OperationKind, payload any) transactions.Operation {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return transactions.NewOperation(kind, data)
}

func decodeAddSymbolOperation(t *testing.T, operation transactions.Operation) transactions.AddSymbolOperation {
	t.Helper()
	var payload transactions.AddSymbolOperation
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func decodeAssignFootprintOperation(t *testing.T, operation transactions.Operation) transactions.AssignFootprintOperation {
	t.Helper()
	var payload transactions.AssignFootprintOperation
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func symbolPropertyValues(properties []transactions.SymbolProperty) map[string]string {
	values := map[string]string{}
	for _, property := range properties {
		values[property.Name] = property.Value
	}
	return values
}

func assertProperty(t *testing.T, properties map[string]string, name string, want string) {
	t.Helper()
	if properties[name] != want {
		t.Fatalf("%s = %q, want %q in %#v", name, properties[name], want, properties)
	}
}
