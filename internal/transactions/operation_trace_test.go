package transactions

import (
	"testing"

	"kicadai/internal/reports"
)

func TestOperationTraceMapAnnotatesUniqueRefAndNet(t *testing.T) {
	traceMap := NewOperationTraceMap([]OperationTrace{
		{OperationID: "op-symbol-r1", Index: 0, Op: OpAddSymbol, Refs: []string{"R1"}},
		{OperationID: "op-route-sig", Index: 1, Op: OpRoute, Nets: []string{"SIG"}},
	})
	issues := []reports.Issue{
		{Refs: []string{"R1"}, Message: "bad ref"},
		{Nets: []string{"SIG"}, Message: "bad net"},
	}
	traceMap.AnnotateIssues(issues)
	if issues[0].OperationID != "op-symbol-r1" || issues[1].OperationID != "op-route-sig" {
		t.Fatalf("issues not annotated: %#v", issues)
	}
}

func TestOperationTraceMapLeavesAmbiguousRefAndNetUnlinked(t *testing.T) {
	traceMap := NewOperationTraceMap([]OperationTrace{
		{OperationID: "op-a", Index: 0, Op: OpAddSymbol, Refs: []string{"R1"}, Nets: []string{"GND"}},
		{OperationID: "op-b", Index: 1, Op: OpConnect, Refs: []string{"R1"}, Nets: []string{"GND"}},
	})
	issues := []reports.Issue{
		{Refs: []string{"R1"}, Message: "ambiguous ref"},
		{Nets: []string{"GND"}, Message: "ambiguous net"},
	}
	traceMap.AnnotateIssues(issues)
	if issues[0].OperationID != "" || issues[1].OperationID != "" {
		t.Fatalf("ambiguous issues should not be annotated: %#v", issues)
	}
}

func TestOperationTraceMapAnnotatesArtifactPath(t *testing.T) {
	traceMap := NewOperationTraceMap([]OperationTrace{{
		OperationID: "op-write",
		Index:       2,
		Op:          OpWriteProject,
		Artifacts:   []reports.Artifact{{Kind: reports.ArtifactPCB, Path: "out/demo.kicad_pcb"}},
	}})
	issues := []reports.Issue{{Path: "out/demo.kicad_pcb", Message: "bad file"}}
	traceMap.AnnotateIssues(issues)
	if issues[0].OperationID != "op-write" {
		t.Fatalf("artifact issue not annotated: %#v", issues)
	}
}

func TestOperationTraceMapLeavesSharedArtifactPathUnlinked(t *testing.T) {
	traceMap := NewOperationTraceMap([]OperationTrace{
		{OperationID: "op-a", Index: 0, Op: OpWriteProject, Artifacts: []reports.Artifact{{Kind: reports.ArtifactPCB, Path: "out/demo.kicad_pcb"}}},
		{OperationID: "op-b", Index: 1, Op: OpWriteProject, Paths: []string{"out/demo.kicad_pcb"}},
	})
	issues := []reports.Issue{{Path: "out/demo.kicad_pcb", Message: "shared file"}}
	traceMap.AnnotateIssues(issues)
	if issues[0].OperationID != "" {
		t.Fatalf("shared artifact should not be annotated: %#v", issues)
	}
}

func TestOperationTraceMapLookupByIDAndIndex(t *testing.T) {
	traceMap := NewOperationTraceMap([]OperationTrace{{OperationID: "op-write", Index: 2, Op: OpWriteProject}})
	if trace, ok := traceMap.ByOperationID("op-write"); !ok || trace.Index != 2 {
		t.Fatalf("lookup by id failed: %#v %v", trace, ok)
	}
	if trace, ok := traceMap.ByIndex(2); !ok || trace.OperationID != "op-write" {
		t.Fatalf("lookup by index failed: %#v %v", trace, ok)
	}
}
