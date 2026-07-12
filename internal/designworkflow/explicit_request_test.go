package designworkflow

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
)

func TestValidateRequestAcceptsExplicitCircuitMode(t *testing.T) {
	request := validExplicitCircuitRequest()
	if issues := ValidateRequest(request); reports.HasBlockingIssue(issues) {
		t.Fatalf("validation issues = %#v", issues)
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
