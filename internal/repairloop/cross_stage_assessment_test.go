package repairloop

import "testing"

func TestAssessCrossStageCandidateCountsNewPassedRequiredGate(t *testing.T) {
	diagnostic := CrossStageDiagnostic{Key: "blocking", Severity: CrossStageSeverityBlocking}
	before := CrossStageSnapshot{
		StateHash: "before",
		Gates: []CrossStageGate{{
			ID: "existing", Required: true, Status: CrossStageGatePassed,
		}},
	}
	after := CrossStageSnapshot{
		StateHash: "after",
		Gates: []CrossStageGate{
			{ID: "existing", Required: true, Status: CrossStageGatePassed},
			{ID: "new", Required: true, Status: CrossStageGatePassed},
		},
	}
	rejections, resolved, passedGateDelta := assessCrossStageCandidate(
		before, []CrossStageDiagnostic{diagnostic}, after, nil, diagnostic, CrossStageProposal{}, 0,
	)
	if len(rejections) != 0 {
		t.Fatalf("unexpected candidate rejection: %v", rejections)
	}
	if resolved != 1 {
		t.Fatalf("resolved blocking diagnostics = %d, want 1", resolved)
	}
	if passedGateDelta != 1 {
		t.Fatalf("passed gate delta = %d, want 1", passedGateDelta)
	}
}

func TestAppendUniqueCrossStageDiagnosticsKeepsFirstOccurrence(t *testing.T) {
	seen := map[string]struct{}{}
	first := CrossStageDiagnostic{Hash: "same", EvidenceHash: "first"}
	second := CrossStageDiagnostic{Hash: "same", EvidenceHash: "second"}
	unique := appendUniqueCrossStageDiagnostics(nil, seen, []CrossStageDiagnostic{first, second})
	unique = appendUniqueCrossStageDiagnostics(unique, seen, []CrossStageDiagnostic{{Hash: "new"}, first})
	if len(unique) != 2 || unique[0].EvidenceHash != "first" || unique[1].Hash != "new" {
		t.Fatalf("unexpected unique diagnostic sequence: %#v", unique)
	}
}
