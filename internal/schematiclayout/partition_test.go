package schematiclayout

import (
	"fmt"
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestPartitionOversizedGraphPreservesCrossSheetNetEvidence(t *testing.T) {
	request := Request{Sheet: SheetForPaper("A4"), Rules: DefaultRules(ProfileStandard)}
	for index := 0; index < 80; index++ {
		ref := fmt.Sprintf("U%d", index+1)
		request.Components = append(request.Components, Component{
			Ref:  ref,
			Role: "ic",
			Pins: []Pin{{Number: "1", Role: "output"}, {Number: "2", Role: "input"}},
		})
		if index == 0 {
			continue
		}
		request.Nets = append(request.Nets, Net{
			Name:      fmt.Sprintf("N%d", index),
			Endpoints: []Endpoint{{Ref: fmt.Sprintf("U%d", index), Pin: "1"}, {Ref: ref, Pin: "2"}},
		})
	}

	result := Layout(request)
	if result.Partition == nil {
		t.Fatalf("missing partition evidence: %#v", result.Report)
	}
	if len(result.Partition.Sheets) < 2 {
		t.Fatalf("partition sheets = %#v, want multiple sheets", result.Partition.Sheets)
	}
	if result.Report.PartitionCount != len(result.Partition.Sheets) {
		t.Fatalf("partition report = %#v, evidence = %#v", result.Report, result.Partition)
	}
	if len(result.Partition.CrossSheetNets) == 0 {
		t.Fatalf("missing cross-sheet net evidence: %#v", result.Partition)
	}
	if !result.Partition.Complete {
		t.Fatalf("partition should fit standard sheets: %#v", result.Partition)
	}
}

func TestPartitionSplitsOnlyOversizedExplicitGroup(t *testing.T) {
	request := Request{Sheet: SheetForPaper("A4"), Rules: DefaultRules(ProfileStandard), Groups: []Group{{ID: "large_stage"}}}
	for index := 0; index < 240; index++ {
		ref := fmt.Sprintf("U%d", index+1)
		request.Components = append(request.Components, Component{
			Ref: ref, Role: "ic", GroupID: "large_stage", FlowRank: 0, RankFixed: true,
			Pins: []Pin{{Number: "1", Role: "output"}, {Number: "2", Role: "input"}},
		})
		if index == 0 {
			continue
		}
		request.Nets = append(request.Nets, Net{
			Name: fmt.Sprintf("N%d", index), Endpoints: []Endpoint{{Ref: fmt.Sprintf("U%d", index), Pin: "1"}, {Ref: ref, Pin: "2"}},
		})
	}

	result := Layout(request)
	if result.Partition == nil || !result.Partition.Complete {
		t.Fatalf("oversized explicit group did not produce a complete hierarchy: %#v", result.Partition)
	}
	if len(result.Partition.Sheets) < 2 {
		t.Fatalf("partition sheets = %#v, want multiple sheets", result.Partition.Sheets)
	}
	if len(result.Partition.SplitGroups) != 1 || result.Partition.SplitGroups[0] != "large_stage" {
		t.Fatalf("split groups = %#v, want large_stage", result.Partition.SplitGroups)
	}
	if result.Report.PartitionSplitGroupCount != 1 {
		t.Fatalf("partition split-group count = %d, want 1", result.Report.PartitionSplitGroupCount)
	}
}

func TestLayoutPartitionsForRequestedComponentLimit(t *testing.T) {
	request := Request{
		Sheet:                 SheetForPaper("A4"),
		Rules:                 DefaultRules(ProfileStandard),
		MaxComponentsPerSheet: 2,
	}
	for index := 0; index < 5; index++ {
		ref := fmt.Sprintf("R%d", index+1)
		request.Components = append(request.Components, Component{
			Ref: ref, Role: "resistor", Pins: []Pin{{Number: "1", Role: "passive"}, {Number: "2", Role: "passive"}},
		})
		if index > 0 {
			request.Nets = append(request.Nets, Net{Name: fmt.Sprintf("N%d", index), Endpoints: []Endpoint{{Ref: fmt.Sprintf("R%d", index), Pin: "2"}, {Ref: ref, Pin: "1"}}})
		}
	}

	result := Layout(request)
	if result.Partition == nil || len(result.Partition.Sheets) != 3 || !result.Partition.Complete {
		t.Fatalf("requested partition = %#v", result.Partition)
	}
	for _, sheet := range result.Partition.Sheets {
		if len(sheet.Components) > request.MaxComponentsPerSheet {
			t.Fatalf("sheet %s contains %d components, limit %d", sheet.ID, len(sheet.Components), request.MaxComponentsPerSheet)
		}
	}
}

func TestLayoutComponentLimitPreservesExplicitFunctionalGroups(t *testing.T) {
	request := Request{
		Sheet:                 SheetForPaper("A4"),
		Rules:                 DefaultRules(ProfileStandard),
		MaxComponentsPerSheet: 3,
		Groups: []Group{
			{ID: "input_stage"},
			{ID: "control_stage"},
			{ID: "output_stage"},
		},
	}
	groups := []string{"input_stage", "input_stage", "control_stage", "control_stage", "output_stage"}
	for index, group := range groups {
		ref := fmt.Sprintf("U%d", index+1)
		request.Components = append(request.Components, Component{
			Ref: ref, Role: "ic", GroupID: group, FlowRank: index / 2, RankFixed: true,
			Pins: []Pin{{Number: "1", Role: "output"}, {Number: "2", Role: "input"}},
		})
		if index > 0 {
			request.Nets = append(request.Nets, Net{
				Name:      fmt.Sprintf("N%d", index),
				Endpoints: []Endpoint{{Ref: fmt.Sprintf("U%d", index), Pin: "1"}, {Ref: ref, Pin: "2"}},
			})
		}
	}

	result := Layout(request)
	if result.Partition == nil || len(result.Partition.Sheets) < 2 || !result.Partition.Complete {
		t.Fatalf("functional partition = %#v", result.Partition)
	}
	sheetByRef := map[string]string{}
	for _, sheet := range result.Partition.Sheets {
		for _, ref := range sheet.Components {
			sheetByRef[ref] = sheet.ID
		}
	}
	if sheetByRef["U1"] != sheetByRef["U2"] || sheetByRef["U3"] != sheetByRef["U4"] {
		t.Fatalf("functional group split across sheets: %#v", sheetByRef)
	}
}

func TestLayoutComponentLimitKeepsMultiUnitReferenceOnOneSheet(t *testing.T) {
	request := Request{
		Sheet:                 SheetForPaper("A4"),
		Rules:                 DefaultRules(ProfileStandard),
		MaxComponentsPerSheet: 1,
		Groups:                []Group{{ID: "input_stage"}, {ID: "output_stage"}},
		Components: []Component{
			{Ref: "opamp_a", DisplayRef: "U1", GroupID: "input_stage", Role: "opamp", Pins: []Pin{{Number: "1", Role: "output"}}},
			{Ref: "opamp_b", DisplayRef: "U1", GroupID: "output_stage", Role: "opamp", Pins: []Pin{{Number: "7", Role: "output"}}},
			{Ref: "load", DisplayRef: "R1", GroupID: "output_stage", Role: "resistor", Pins: []Pin{{Number: "1", Role: "passive"}}},
		},
		Nets: []Net{{Name: "signal", Endpoints: []Endpoint{{Ref: "opamp_a", Pin: "1"}, {Ref: "opamp_b", Pin: "7"}}}},
	}

	result := Layout(request)
	if result.Partition == nil || len(result.Partition.Sheets) < 2 || !result.Partition.Complete {
		t.Fatalf("multi-unit partition = %#v", result.Partition)
	}
	sheetByRef := map[string]string{}
	for _, sheet := range result.Partition.Sheets {
		for _, ref := range sheet.Components {
			sheetByRef[ref] = sheet.ID
		}
	}
	if sheetByRef["opamp_a"] == "" || sheetByRef["opamp_a"] != sheetByRef["opamp_b"] {
		t.Fatalf("multi-unit reference split across sheets: %#v", sheetByRef)
	}
}

func TestLimitPartitionAssignmentsIndependentOfComponentOrder(t *testing.T) {
	components := []PlacedComponent{
		{Component: Component{Ref: "A1"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "A2"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(10)}},
		{Component: Component{Ref: "B1"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}},
		{Component: Component{Ref: "B2"}, PlacedAt: kicadfiles.Point{X: kicadfiles.MM(40), Y: kicadfiles.MM(10)}},
	}
	permuted := []PlacedComponent{components[1], components[0], components[2], components[3]}
	assignments := map[string]string{"A1": "sheet", "A2": "sheet", "B1": "sheet", "B2": "sheet"}
	groups := map[string]string{"A1": "stage_a", "A2": "stage_a", "B1": "stage_b", "B2": "stage_b"}

	want := limitPartitionAssignments(components, assignments, groups, 2)
	got := limitPartitionAssignments(permuted, assignments, groups, 2)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("permuted assignments = %#v, want %#v", got, want)
	}
}
