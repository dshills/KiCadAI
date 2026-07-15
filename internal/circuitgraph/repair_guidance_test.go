package circuitgraph

import (
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestRepairOptionsAreDeterministicAndBounded(t *testing.T) {
	document := validTestDocument()
	document.Components[0].Units = []ComponentUnit{{ID: "A"}, {ID: "B"}}
	document.Nets[0].Endpoints[0].Unit = "Z"
	issues := []reports.Issue{{IssueID: "issue-unit", Code: CodeUnitInvalid, Path: "nets[0].endpoints[0].unit", Stage: "connectivity", RetryScope: "connectivity"}}
	first := RepairOptions(document, nil, issues)
	second := RepairOptions(document, nil, issues)
	if len(first) != 1 || len(second) != 1 || first[0].OperationTemplate.Op != "replace_endpoint" || len(first[0].AllowedValues["replacement.unit"]) != 2 || first[0].AllowedValues["replacement.unit"][0] != "A" || first[0].AllowedValues["replacement.unit"][1] != "B" {
		t.Fatalf("repair options = %#v", first)
	}
}

func TestRepairOptionsOmitAmbiguousOrExternalFindings(t *testing.T) {
	document := validTestDocument()
	issues := []reports.Issue{{IssueID: "ambiguous", Code: CodeComponentAmbiguous, Path: "components[0]", Stage: "component_selection", RetryScope: "component_selection"}, {IssueID: "external", Code: "KICAD_DRC", Path: "drc", Stage: "kicad_drc", RetryScope: "manual_review"}}
	if options := RepairOptions(document, nil, issues); len(options) != 0 {
		t.Fatalf("unsafe guidance = %#v", options)
	}
}

func TestRepairOptionsDeriveOnlyUnusedVerifiedUnit(t *testing.T) {
	document := validTestDocument()
	document.Components[0].ComponentID = "opamp.dual"
	document.Components[0].Units = []ComponentUnit{{ID: "A"}, {ID: "B"}}
	document.Nets[0].Endpoints[0] = Endpoint{Component: "j1", Unit: "Z", SelectorKind: SelectorFunction, Selector: "IN_PLUS"}
	document.Nets[1].Endpoints[0] = Endpoint{Component: "j1", Unit: "A", SelectorKind: SelectorFunction, Selector: "IN_PLUS"}
	catalog := &components.Catalog{Records: []components.ComponentRecord{{
		ID: "opamp.dual",
		Symbols: []components.SymbolBinding{
			{UnitID: "A", FunctionPins: []components.FunctionPin{{Function: "IN_PLUS", SymbolPin: "3"}}},
			{UnitID: "B", FunctionPins: []components.FunctionPin{{Function: "IN_PLUS", SymbolPin: "5"}}},
		},
	}}}
	issues := []reports.Issue{{IssueID: "issue-unit", Code: CodeUnitInvalid, Path: "nets[0].endpoints[0].unit", Stage: "connectivity", RetryScope: "connectivity"}}
	options := RepairOptions(document, catalog, issues)
	if len(options) != 1 || len(options[0].RequiredValues) != 0 || options[0].OperationTemplate.Replacement == nil || options[0].OperationTemplate.Replacement.Unit != "B" {
		t.Fatalf("unit repair options = %#v", options)
	}
}

func TestRepairOptionsClampOnlyOverflowingRegionBounds(t *testing.T) {
	document := validTestDocument()
	document.PCB.Regions[0].Bounds.WidthMM = 100
	issues := []reports.Issue{{IssueID: "issue-region", Code: CodePCBConstraintInvalid, Path: "pcb.regions[0].bounds", Stage: "placement", RetryScope: "placement"}}
	options := RepairOptions(document, nil, issues)
	if len(options) != 1 || len(options[0].RequiredValues) != 0 || options[0].OperationTemplate.Bounds == nil || options[0].OperationTemplate.Bounds.WidthMM != 38 || options[0].OperationTemplate.Bounds.HeightMM != 21 {
		t.Fatalf("region repair options = %#v", options)
	}
}
