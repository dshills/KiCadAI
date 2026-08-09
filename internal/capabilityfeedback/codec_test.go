package capabilityfeedback

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/capabilityevaluation"
)

func TestCaseEvidenceCodecIsStrictBoundedAndHashVerified(t *testing.T) {
	current := feedbackSealedCase(
		t, "case-codec", RoleDiscovery, capabilityevaluation.DomainAnalog,
		capabilityevaluation.SafetyReviewRequired, []string{"dc_sweep"}, Gap{
			Stage: "simulation", Scope: ScopeModel, Capability: "trusted_simulation_model", Code: "MODEL_UNAVAILABLE",
			RequiredEvidence: []string{"reviewed model"}, EvidenceHashes: []string{feedbackHash("model")},
		},
	)
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCaseEvidence(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Hash != current.Hash {
		t.Fatalf("decoded hash = %q, want %q", decoded.Hash, current.Hash)
	}

	unknown := append(bytes.TrimSuffix(encoded, []byte("}")), []byte(`,"unknown":true}`)...)
	if _, err := DecodeCaseEvidence(bytes.NewReader(unknown)); err == nil {
		t.Fatal("unknown case-evidence field was accepted")
	}
	if _, err := DecodeCaseEvidence(strings.NewReader(string(encoded) + `{}`)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	if _, err := DecodeCaseEvidence(strings.NewReader(strings.Repeat(" ", maxArtifactBytes+1))); err == nil {
		t.Fatal("oversized artifact was accepted")
	}

	current.Gaps[0].EvidenceHashes[0] = "not-a-hash"
	mutated, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCaseEvidence(bytes.NewReader(mutated)); err == nil {
		t.Fatal("mutated causal evidence was accepted")
	}
}

func TestAggregateReportCodecReproducesFromCasesAndRegistry(t *testing.T) {
	registry := capabilityevaluation.ImpactRegistry{Version: "codec-v1", Records: []capabilityevaluation.ImpactRecord{{
		Capability: "trusted_simulation_model", Consumers: []string{"fault_analysis"},
	}}}
	current := feedbackSealedCase(
		t, "case-report", RoleDiscovery, capabilityevaluation.DomainMixedSignal,
		capabilityevaluation.SafetyRelevant, []string{"transient"}, Gap{
			Stage: "simulation", Scope: ScopeModel, Capability: "trusted_simulation_model", Code: "MODEL_UNAVAILABLE",
			RequiredEvidence: []string{"reviewed model"}, EvidenceHashes: []string{feedbackHash("report-model")},
		},
	)
	report, err := Evaluate(RoleDiscovery, []CaseEvidence{current}, registry)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := report.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeAggregateReport(bytes.NewReader(encoded), registry)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Hash != report.Hash {
		t.Fatalf("decoded report hash = %q, want %q", decoded.Hash, report.Hash)
	}

	var drifted AggregateReport
	if err := json.Unmarshal(encoded, &drifted); err != nil {
		t.Fatal(err)
	}
	drifted.Clusters[0].Rank = 2
	driftedBytes, err := json.Marshal(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAggregateReport(bytes.NewReader(driftedBytes), registry); err == nil {
		t.Fatal("non-reproducible aggregate report was accepted")
	}
	wrongRegistry := capabilityevaluation.ImpactRegistry{Version: "codec-v2"}
	if _, err := DecodeAggregateReport(bytes.NewReader(encoded), wrongRegistry); err == nil {
		t.Fatal("aggregate report was accepted under a different impact registry")
	}
}
