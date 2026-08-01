package repairloop

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTraceNormalizesAndReplays(t *testing.T) {
	diagnostic := NewDiagnostic("routing", "blocked", "endpoint_access", "", "evidence", []string{"N1", "R1", "N1"})
	proposal := NewProposal(diagnostic, "improve_endpoint_fanout", "routing", "make the required endpoint reachable", []string{"R1", "N1"}, true, "")
	first := NewTrace(2, 1, []Diagnostic{diagnostic, diagnostic}, []Proposal{proposal}, []Outcome{{ProposalID: proposal.ID, Status: "passed", BeforeHash: "before", AfterHash: "after", ResultHash: "result"}})
	second := NewTrace(2, 1, []Diagnostic{diagnostic}, []Proposal{proposal}, []Outcome{{ProposalID: proposal.ID, Status: "passed", BeforeHash: "before", AfterHash: "after", ResultHash: "result"}})
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if first.Hash == "" || !bytes.Equal(firstJSON, secondJSON) || len(first.Diagnostics) != 1 || first.Consumed != 1 {
		t.Fatalf("normalized trace replay differs: first=%s second=%s trace=%#v", first.Hash, second.Hash, first)
	}
}
