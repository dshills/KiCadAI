package designworkflow

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/componentprops"
	"kicadai/internal/components"
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

func TestConcreteI2CSensorSelectionReachesGeneratedOperations(t *testing.T) {
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
