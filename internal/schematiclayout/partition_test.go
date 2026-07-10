package schematiclayout

import (
	"fmt"
	"testing"
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
