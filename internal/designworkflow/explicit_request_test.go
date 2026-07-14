package designworkflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
)

func TestValidateRequestAcceptsExplicitCircuitMode(t *testing.T) {
	request := validExplicitCircuitRequest()
	if issues := ValidateRequest(request); reports.HasBlockingIssue(issues) {
		t.Fatalf("validation issues = %#v", issues)
	}
}

func TestExplicitSchematicTransactionPrefersResolverSymbols(t *testing.T) {
	tx, issues := explicitSchematicTransaction(validExplicitCircuitRequest(), nil)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	for _, operation := range tx.Operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatal(err)
		}
		if !payload.PreferResolverSymbol {
			t.Fatalf("add-symbol operation does not prefer resolver: %#v", payload)
		}
		return
	}
	t.Fatal("missing add-symbol operation")
}

func TestExplicitRequiredRoutesFailClosed(t *testing.T) {
	nets := []ExplicitNetSpec{{Name: "REQ", Required: true}, {Name: "OPTIONAL"}}
	issues := explicitRequiredRouteIssues(nets, routing.Result{Routes: []routing.Route{{Net: "OPTIONAL", Status: routing.RouteStatusFailed}}})
	if len(issues) != 1 || issues[0].Path != "explicit_circuit.nets.REQ" {
		t.Fatalf("required route issues = %#v", issues)
	}
}

func TestExplicitZoneOperationsPreserveClearance(t *testing.T) {
	request := validExplicitCircuitRequest()
	request.ExplicitCircuit.Zones = []ExplicitZoneSpec{{Net: "SIG", Layers: []string{"B.Cu"}, ClearanceMM: 0.35}}
	operations, issues := explicitZoneOperations(request)
	if reports.HasBlockingIssue(issues) || len(operations) != 1 {
		t.Fatalf("zone operations/issues = %#v %#v", operations, issues)
	}
	var payload transactions.AddZoneOperation
	if err := json.Unmarshal(operations[0].Raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ClearanceMM != 0.35 || len(payload.Layers) != 1 || payload.Layers[0] != "B.Cu" {
		t.Fatalf("zone payload = %#v", payload)
	}
}

func TestCreateExplicitCircuitRequiresLibraryIndex(t *testing.T) {
	request := validExplicitCircuitRequest()
	result := Create(context.Background(), request, CreateOptions{OutputDir: t.TempDir(), Overwrite: true})
	stage := findExplicitWorkflowStage(result, StageComponentSelection)
	if stage == nil || stage.Status != StageStatusBlocked || !hasDesignWorkflowIssuePath(stage.Issues, "library_index") {
		t.Fatalf("component selection stage = %#v", stage)
	}
	if findExplicitWorkflowStage(result, StageProjectWrite).Status != StageStatusSkipped {
		t.Fatalf("project write should be skipped: %#v", result.Stages)
	}
}

func TestValidateRequestRequiresExactlyOneDesignMode(t *testing.T) {
	request := validExplicitCircuitRequest()
	request.Blocks = []BlockInstanceSpec{{ID: "extra", BlockID: "led_indicator"}}
	if issues := ValidateRequest(request); !hasDesignWorkflowIssuePath(issues, "design_mode") {
		t.Fatalf("mixed mode issues = %#v", issues)
	}
	request.Blocks = nil
	request.ExplicitCircuit = nil
	if issues := ValidateRequest(request); !hasDesignWorkflowIssuePath(issues, "design_mode") {
		t.Fatalf("missing mode issues = %#v", issues)
	}
}

func TestValidateRequestRejectsExplicitPadNetDisagreement(t *testing.T) {
	request := validExplicitCircuitRequest()
	request.ExplicitCircuit.Components[0].Pads[0].Net = "WRONG"
	if issues := ValidateRequest(request); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.nets") {
		t.Fatalf("pad net issues = %#v", issues)
	}
}

func TestValidateRequestRejectsExplicitSchematicProjectionDisagreement(t *testing.T) {
	request := validExplicitCircuitRequest()
	request.ExplicitCircuit.Schematic.Circuit.Components[0].Footprint = "Resistor_SMD:R_0603_1608Metric"
	if issues := ValidateRequest(request); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.components") {
		t.Fatalf("component projection issues = %#v", issues)
	}
	request = validExplicitCircuitRequest()
	request.ExplicitCircuit.Schematic.Circuit.Nets[0].Connect[0] = "r1.2"
	if issues := ValidateRequest(request); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.nets") {
		t.Fatalf("net projection issues = %#v", issues)
	}
}

func TestValidateRequestAcceptsOnlyAuthorizedPowerFlagSupport(t *testing.T) {
	request := validExplicitCircuitRequest()
	id := "kicadai_pwr_flag_0123456789abcdef"
	request.ExplicitCircuit.Schematic.Circuit.Components = append(request.ExplicitCircuit.Schematic.Circuit.Components, schematicir.Component{
		ID: id, Ref: "#FLG01", Role: schematicir.ComponentRolePowerSymbol,
		Symbol: "power:PWR_FLAG", Value: "PWR_FLAG",
		Pins: []schematicir.Pin{{Number: "1", Name: "PWR_FLAG", Role: schematicir.PinRolePower}},
	})
	request.ExplicitCircuit.Schematic.Circuit.Nets[0].Connect = append(request.ExplicitCircuit.Schematic.Circuit.Nets[0].Connect, schematicir.EndpointRef(id+".1"))
	request.ExplicitCircuit.SchematicSupport = []ExplicitSchematicSupportSpec{{ID: id, Kind: ExplicitSchematicSupportPowerFlag, Net: "SIG"}}
	request.ExplicitCircuit.Schematic = schematicir.NormalizeLayoutIntent(request.ExplicitCircuit.Schematic)
	if issues := ValidateRequest(request); reports.HasBlockingIssue(issues) {
		t.Fatalf("authorized power flag issues = %#v", issues)
	}

	wrongSymbol := request
	wrongSymbol.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	wrongSymbol.ExplicitCircuit.Schematic.Circuit.Components[2].Symbol = "Device:R"
	if issues := ValidateRequest(wrongSymbol); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.schematic_support") {
		t.Fatalf("wrong-symbol issues = %#v", issues)
	}

	wrongNet := request
	wrongNet.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	wrongNet.ExplicitCircuit.SchematicSupport[0].Net = "OTHER"
	if issues := ValidateRequest(wrongNet); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.schematic_support") {
		t.Fatalf("wrong-net issues = %#v", issues)
	}

	missingAuthorization := request
	missingAuthorization.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	missingAuthorization.ExplicitCircuit.SchematicSupport = nil
	if issues := ValidateRequest(missingAuthorization); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.schematic") {
		t.Fatalf("missing-authorization issues = %#v", issues)
	}
}

func TestValidateRequestRejectsExplicitPCBIntentOutsideBoard(t *testing.T) {
	request := validExplicitCircuitRequest()
	request.ExplicitCircuit.Regions = []ExplicitRegionSpec{{ID: "bad", XMM: 29, YMM: 1, WidthMM: 5, HeightMM: 5}}
	request.ExplicitCircuit.Components[0].Placement.Region = "bad"
	if issues := ValidateRequest(request); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.regions") {
		t.Fatalf("region bounds issues = %#v", issues)
	}
}

func TestValidateRequestAcceptsMultipleSchematicUnitsOwnedByOnePhysicalComponent(t *testing.T) {
	request := validExplicitCircuitRequest()
	physical := request.ExplicitCircuit.Components[0]
	physical.ID = "u1"
	physical.Reference = "U1"
	physical.FootprintID = "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm"
	physical.SchematicUnits = []string{"u1_a", "u1_b"}
	physical.Pads = []ExplicitPadSpec{
		{Name: "1", SymbolPin: "1", Net: "SIG"},
		{Name: "7", SymbolPin: "7", Net: "SIG"},
	}
	request.ExplicitCircuit.Components = []ExplicitComponentSpec{physical}
	request.ExplicitCircuit.Schematic.Circuit.Components = []schematicir.Component{
		{ID: "u1_a", Ref: "U1", Unit: "A", Role: schematicir.ComponentRoleIC, Symbol: "Amplifier_Operational:LM358", Footprint: physical.FootprintID, Pins: []schematicir.Pin{{Number: "1"}}},
		{ID: "u1_b", Ref: "U1", Unit: "B", Role: schematicir.ComponentRoleIC, Symbol: "Amplifier_Operational:LM358", Footprint: physical.FootprintID, Pins: []schematicir.Pin{{Number: "7"}}},
	}
	request.ExplicitCircuit.Schematic.Circuit.Nets = []schematicir.Net{{Name: "SIG", Role: schematicir.NetRoleSignal, Connect: []schematicir.EndpointRef{"u1_a.1", "u1_b.7"}}}
	request.ExplicitCircuit.Schematic.Layout = schematicir.Layout{}
	request.ExplicitCircuit.Schematic = schematicir.NormalizeLayoutIntent(request.ExplicitCircuit.Schematic)
	request.ExplicitCircuit.Nets = []ExplicitNetSpec{{Name: "SIG", Endpoints: []ExplicitNetEndpoint{{Component: "u1", Pad: "1"}, {Component: "u1", Pad: "7"}}}}

	if issues := ValidateRequest(request); len(issues) != 0 {
		t.Fatalf("multi-unit ownership issues = %#v", issues)
	}

	conflict := request
	conflict.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	duplicate := conflict.ExplicitCircuit.Components[0]
	duplicate.ID = "u2"
	duplicate.Reference = "U2"
	duplicate.SchematicUnits = []string{"u1_a"}
	conflict.ExplicitCircuit.Components = append(conflict.ExplicitCircuit.Components, duplicate)
	if issues := ValidateRequest(conflict); !hasDesignWorkflowIssuePath(issues, "explicit_circuit.components") {
		t.Fatalf("conflicting ownership issues = %#v", issues)
	}
}

func validExplicitCircuitRequest() Request {
	document := *schematicir.NewDocument()
	document.Metadata.Name = "explicit_test"
	document.Circuit.Components = []schematicir.Component{
		{ID: "r1", Ref: "R1", Role: schematicir.ComponentRoleResistor, Symbol: "Device:R", Value: "1k", Footprint: "Resistor_SMD:R_0805_2012Metric", Pins: []schematicir.Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "r2", Ref: "R2", Role: schematicir.ComponentRoleResistor, Symbol: "Device:R", Value: "1k", Footprint: "Resistor_SMD:R_0805_2012Metric", Pins: []schematicir.Pin{{Number: "1"}, {Number: "2"}}},
	}
	document.Circuit.Nets = []schematicir.Net{{Name: "SIG", Role: schematicir.NetRoleSignal, Connect: []schematicir.EndpointRef{"r1.1", "r2.1"}}}
	document = schematicir.NormalizeLayoutIntent(document)
	hash := strings.Repeat("a", 64)
	return Request{
		Version: RequestVersion, Name: "explicit_test", Board: BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 2},
		ExplicitCircuit: &ExplicitCircuitSpec{
			ResolutionHash: hash, CatalogID: "test", CatalogHash: hash, Schematic: document,
			Components: []ExplicitComponentSpec{
				{ID: "r1", Reference: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pads: []ExplicitPadSpec{{Name: "1", SymbolPin: "1", Net: "SIG"}, {Name: "2", SymbolPin: "2"}}},
				{ID: "r2", Reference: "R2", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pads: []ExplicitPadSpec{{Name: "1", SymbolPin: "1", Net: "SIG"}, {Name: "2", SymbolPin: "2"}}},
			},
			Nets: []ExplicitNetSpec{{Name: "SIG", Endpoints: []ExplicitNetEndpoint{{Component: "r1", Pad: "1"}, {Component: "r2", Pad: "1"}}}},
		},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true, SkipKiCadChecks: true},
	}
}

func hasDesignWorkflowIssuePath(issues []reports.Issue, prefix string) bool {
	for _, issue := range issues {
		if strings.HasPrefix(issue.Path, prefix) {
			return true
		}
	}
	return false
}

func findExplicitWorkflowStage(result WorkflowResult, name StageName) *StageResult {
	for index := range result.Stages {
		if result.Stages[index].Name == name {
			return &result.Stages[index]
		}
	}
	return nil
}
