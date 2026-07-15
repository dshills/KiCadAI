package circuitgraph

import (
	"testing"

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
