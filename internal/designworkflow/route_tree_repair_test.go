package designworkflow

import (
	"encoding/json"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestClassifyRouteTreeBranchFailureOtherNetPad(t *testing.T) {
	issue := reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityBlocked,
		Path:       `design.inter_block_route_groups["SDA"].branches[1].nets.SDA`,
		Message:    "clearance violation: route intersects obstacle other_net_pad R1.1",
		Refs:       []string{"J1", "U1"},
		Nets:       []string{"SDA"},
		Suggestion: "move components, reduce clearance, or allow another routing layer",
	}
	hints := BuildRouteTreeRepairHints([]reports.Issue{issue})
	if len(hints) != 1 {
		t.Fatalf("hints = %#v, want one", hints)
	}
	hint := hints[0]
	if hint.Category != InterBlockBranchFailureOtherNetPad || hint.NetName != "SDA" || !hint.Repairable {
		t.Fatalf("hint = %#v, want repairable SDA other-net-pad branch blocker", hint)
	}
}

func TestClassifyRouteTreeContactFailures(t *testing.T) {
	hints := BuildRouteTreeRepairHints([]reports.Issue{
		{Code: reports.CodeRouteContactMiss, Severity: reports.SeverityBlocked, Path: "design.inter_block_contact.nets[0].endpoints[1].start", Refs: []string{"J1"}, Nets: []string{"VCC"}, Message: "route endpoint does not contact the required same-net target"},
		{Code: reports.CodeRouteContactMissingTarget, Severity: reports.SeverityBlocked, Path: "design.inter_block_contact.nets[2].endpoints[0].target", Refs: []string{"U1"}, Nets: []string{"SDA"}, Message: "inter-block contact target has no emitted route operation"},
		{Code: reports.CodeRouteGraphIncomplete, Severity: reports.SeverityBlocked, Path: "design.inter_block_contact.nets[3].endpoints[1].end", Refs: []string{"J2"}, Nets: []string{"GND"}, Message: "route copper is in a separate same-net contact graph component"},
	})
	if len(hints) != 3 {
		t.Fatalf("hints = %#v, want three contact hints", hints)
	}
	categories := []InterBlockBranchFailureCategory{hints[0].Category, hints[1].Category, hints[2].Category}
	if !slices.Contains(categories, InterBlockBranchFailureContactMiss) ||
		!slices.Contains(categories, InterBlockBranchFailureMissingTarget) ||
		!slices.Contains(categories, InterBlockBranchFailureGraphSplit) {
		t.Fatalf("categories = %#v, want contact miss, missing target, and graph split", categories)
	}
}

func TestRouteTreeRepairIgnoresNonBlockingFixedNetAndNetClassNotices(t *testing.T) {
	hints := BuildRouteTreeRepairHints([]reports.Issue{
		{Code: reports.CodeFixedNetSkipped, Severity: reports.SeverityInfo, Path: `design.inter_block_route_groups["VCC"].branches[0].nets.GND`, Nets: []string{"GND"}, Message: "fixed net was preserved and skipped"},
		{Code: reports.CodeMissingNetClass, Severity: reports.SeverityWarning, Path: `design.inter_block_route_groups["VCC"].branches[0].nets.VCC.class`, Nets: []string{"VCC"}, Message: "power or high-current net has no explicit net class"},
		{Code: reports.CodeRouteContactMiss, Severity: reports.SeverityBlocked, Path: `design.inter_block_contact.nets[0].endpoints[1].start`, Nets: []string{"VCC"}, Message: "route endpoint does not contact the required same-net target"},
	})
	if len(hints) != 1 {
		t.Fatalf("hints = %#v, want only blocked route-tree repair hint", hints)
	}
	if hints[0].Category != InterBlockBranchFailureContactMiss || hints[0].NetName != "VCC" {
		t.Fatalf("hint = %#v, want VCC contact miss", hints[0])
	}
}

func TestRouteTreeRepairKeepsBlockingWarningsExceptKnownNotices(t *testing.T) {
	hints := BuildRouteTreeRepairHints([]reports.Issue{{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityWarning,
		Path:     `design.inter_block_route_groups["VCC"].branches[0].nets.VCC`,
		Nets:     []string{"VCC"},
		Message:  "route-tree warning requires user-guided repair",
	}})
	if len(hints) != 1 {
		t.Fatalf("hints = %#v, want warning route-tree hint preserved", hints)
	}
}

func TestRouteTreeRepairSummaryJSONStable(t *testing.T) {
	summary := InterBlockRouteTreeRepairSummary{
		BranchFailures:       2,
		RepairableFailures:   1,
		UnrepairableFailures: 1,
		HintCount:            1,
		Nets:                 []string{"SDA", "VCC"},
		Refs:                 []string{"J1", "U1"},
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"branch_failures":2,"repairable_failures":1,"unrepairable_failures":1,"hint_count":1,"nets":["SDA","VCC"],"refs":["J1","U1"]}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func TestBuildRouteTreePlacementRetryHintsMapsRepairableFailures(t *testing.T) {
	hints := BuildRouteTreePlacementRetryHints([]reports.Issue{{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     `design.inter_block_route_groups["SDA"].branches[1].nets.SDA`,
		Message:  "clearance violation: route intersects obstacle other_net_pad R1.1",
		Refs:     []string{"J1", "U1"},
		Nets:     []string{"SDA"},
	}})
	if len(hints) != 1 {
		t.Fatalf("hints = %#v, want one placement retry hint", hints)
	}
	if hints[0].Category != PlacementRetryIncreaseSpacing || !hints[0].RetryEligible {
		t.Fatalf("hint = %#v, want retryable increase-spacing hint", hints[0])
	}
	if !slices.Contains(hints[0].PlacementEvidence, "route_tree_net:SDA") {
		t.Fatalf("evidence = %#v, want route-tree net evidence", hints[0].PlacementEvidence)
	}
}
