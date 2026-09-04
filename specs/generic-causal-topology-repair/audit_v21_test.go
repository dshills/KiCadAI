package genericcausaltopologyrepair

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	v21ReportFileSHA256 = "4d74d04613aef46f161c54661dd7851845094623a87528c3172d99af786749a0"
	v21ReportHash       = "7e7e4a73a3e78f15c8c30f3d2dc7eb5f058d128515cf91b7ce858aeb93afd428"
)

type v21AuditReport struct {
	CaseCount     int `json:"case_count"`
	Hash          string
	OutcomeCounts []struct {
		Outcome string `json:"outcome"`
		Count   int    `json:"count"`
	} `json:"outcome_counts"`
	Cases []v21AuditCaseEvidence `json:"cases"`
}

type v21AuditCaseEvidence struct {
	Case         json.RawMessage   `json:"case"`
	ReplaySHA256 []string          `json:"replay_sha256"`
	Promotions   []json.RawMessage `json:"promotions"`
	Gates        map[string]bool   `json:"gates"`
}

type v21AuditCase struct {
	ID              string `json:"id"`
	ReportingDomain string `json:"reporting_domain"`
	Outcome         string `json:"outcome"`
	Frontier        []struct {
		Path []struct {
			Capability string `json:"capability"`
			Code       string `json:"code"`
		} `json:"path"`
	} `json:"frontier"`
}

type v21AuditPopulation struct {
	SelectedCases []struct {
		ID string `json:"id"`
	} `json:"selected_cases"`
}

func TestV21GenerationZeroAudit(t *testing.T) {
	root := filepath.Join("..", "..")
	reportPath := filepath.Join(root, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v21_generation_zero", "report.json")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(reportData)
	if actual := hex.EncodeToString(digest[:]); actual != v21ReportFileSHA256 {
		t.Fatalf("V21 report digest = %s, want %s", actual, v21ReportFileSHA256)
	}
	report := decodeV21AuditReport(t, reportData)
	if report.Hash != v21ReportHash || report.CaseCount != 24 || len(report.Cases) != 24 {
		t.Fatalf("invalid V21 report identity: hash=%s case_count=%d cases=%d", report.Hash, report.CaseCount, len(report.Cases))
	}
	wantOutcomes := map[string]int{"pass": 1, "unsupported": 5, "unsafe": 1, "exhausted": 17}
	for _, outcome := range report.OutcomeCounts {
		if wantOutcomes[outcome.Outcome] != outcome.Count {
			t.Fatalf("V21 outcome %s = %d, want %d", outcome.Outcome, outcome.Count, wantOutcomes[outcome.Outcome])
		}
		delete(wantOutcomes, outcome.Outcome)
	}
	if len(wantOutcomes) != 0 {
		t.Fatalf("V21 report omitted outcomes: %v", wantOutcomes)
	}

	// This population is a pre-existing, committed V21 contract input whose
	// digest is independently pinned by contract_v21_test.go.
	populationData, err := os.ReadFile("V21_PUBLIC_TOPOLOGY_POPULATION.json")
	if err != nil {
		t.Fatal(err)
	}
	var population v21AuditPopulation
	if err := json.Unmarshal(populationData, &population); err != nil {
		t.Fatal(err)
	}
	selected := make(map[string]bool, len(population.SelectedCases))
	for _, selectedCase := range population.SelectedCases {
		selected[selectedCase.ID] = true
	}
	if len(selected) != 8 {
		t.Fatalf("selected V21 cases = %d, want 8", len(selected))
	}

	v20Data, err := os.ReadFile(filepath.Join(root, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v20_generation_zero", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	v20 := decodeV21AuditReport(t, v20Data)
	v20Cases := make(map[string]json.RawMessage, len(v20.Cases))
	for _, evidence := range v20.Cases {
		decoded := decodeV21AuditCase(t, evidence.Case)
		v20Cases[decoded.ID] = evidence.Case
	}

	ineligible := 0
	qualifying := 0
	qualifyingDomains := map[string]bool{}
	passCount := 0
	for _, evidence := range report.Cases {
		decoded := decodeV21AuditCase(t, evidence.Case)
		if len(evidence.ReplaySHA256) != 2 || evidence.ReplaySHA256[0] != evidence.ReplaySHA256[1] {
			t.Fatalf("V21 case %s replay mismatch: %v", decoded.ID, evidence.ReplaySHA256)
		}
		if !selected[decoded.ID] {
			ineligible++
			// The frozen preservation rule is byte identity, including JSON key
			// order and encoding, rather than semantic JSON equivalence.
			if !bytes.Equal(evidence.Case, v20Cases[decoded.ID]) {
				t.Fatalf("V21-ineligible case %s drifted from V20", decoded.ID)
			}
		}
		if decoded.Outcome == "pass" {
			passCount++
			if len(evidence.Promotions) != 2 || !allV21PassGates(evidence.Gates) {
				t.Fatalf("V21 pass %s lacks two complete promotions or gates: promotions=%d gates=%v", decoded.ID, len(evidence.Promotions), evidence.Gates)
			}
		}
		if selected[decoded.ID] && v21EndsAtNoPassingGraph(decoded) {
			qualifying++
			qualifyingDomains[decoded.ReportingDomain] = true
		}
	}
	if ineligible != 16 || passCount != 1 {
		t.Fatalf("V21 preservation counts: ineligible=%d pass=%d", ineligible, passCount)
	}
	if qualifying != 6 || len(qualifyingDomains) != 4 {
		t.Fatalf("V21 material advancement: cases=%d domains=%d, want 6 and 4", qualifying, len(qualifyingDomains))
	}
}

func decodeV21AuditReport(t *testing.T, data []byte) v21AuditReport {
	t.Helper()
	var report v21AuditReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func decodeV21AuditCase(t *testing.T, data []byte) v21AuditCase {
	t.Helper()
	var record v21AuditCase
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func v21EndsAtNoPassingGraph(record v21AuditCase) bool {
	if len(record.Frontier) == 0 {
		return false
	}
	for _, frontier := range record.Frontier {
		if len(frontier.Path) == 0 {
			return false
		}
		last := frontier.Path[len(frontier.Path)-1]
		if last.Capability != "passing_behavioral_evidence" || last.Code != "OPEN_TOPOLOGY_NO_PASSING_GRAPH" {
			return false
		}
	}
	return true
}

func allV21PassGates(gates map[string]bool) bool {
	required := []string{
		"primitive_only", "topology_search", "simulation", "all_corners",
		"model_provenance", "closed_loop_evidence", "complete_routing",
		"connectivity", "writer_correctness", "round_trip_zero_diff", "erc",
		"strict_drc", "deterministic_replay", "fail_closed",
	}
	for _, gate := range required {
		if !gates[gate] {
			return false
		}
	}
	return true
}
