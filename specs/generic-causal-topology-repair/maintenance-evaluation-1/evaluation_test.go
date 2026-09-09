package maintenanceevaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityexecutorv10"
)

const publication = "internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json"

type caseView struct {
	ID          string `json:"id"`
	Domain      string `json:"reporting_domain"`
	Role        string `json:"role"`
	CircuitRole string `json:"circuit_role"`
	Safety      string `json:"safety_impact"`
	Outcome     string `json:"outcome"`
	Frontier    []struct {
		Path []struct {
			Capability string `json:"capability"`
			Code       string `json:"code"`
		} `json:"path"`
	} `json:"frontier"`
}

type assessment struct {
	Schema               string                               `json:"schema"`
	ReportHash           string                               `json:"report_hash"`
	CaseCount            int                                  `json:"case_count"`
	ReplayCount          int                                  `json:"replay_count"`
	OutcomeCounts        []capabilitybaselinev10.OutcomeCount `json:"outcome_counts"`
	IneligibleIdentical  int                                  `json:"ineligible_byte_identical"`
	Passes               int                                  `json:"passes"`
	Promotions           int                                  `json:"installed_kicad_promotions"`
	QualifyingCases      []string                             `json:"qualifying_cases"`
	QualifyingDomains    []string                             `json:"qualifying_domains"`
	Regressions          []string                             `json:"regressions"`
	TerminalFrontier     map[string]int                       `json:"terminal_frontier_occurrences"`
	MaterialThresholdMet bool                                 `json:"material_threshold_met"`
	PreservationPassed   bool                                 `json:"preservation_passed"`
	EvaluationPassed     bool                                 `json:"evaluation_passed"`
	Experimental         bool                                 `json:"experimental"`
}

func root() string { return filepath.Join("..", "..", "..") }

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestMaintenanceFreeze(t *testing.T) {
	data, err := os.ReadFile("RUN.json")
	if err != nil {
		t.Fatal(err)
	}
	var run map[string]any
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	if run["schema"] != "kicadai.v21-maintenance-evaluation.v1" || run["run_id"] != "v21-maintenance-1" || run["source_commit"] != "b961b7a73aa9d19398d2adbaeee89de008e5d390" || run["go_version"] != "go1.26.8" || run["platform"] != "darwin/arm64" || run["kicad_version"] != "10.0.3" || run["timeout"] != "6h" || run["publication_root"] != filepath.ToSlash(filepath.Dir(publication)) {
		t.Fatal("maintenance run identity or environment drifted")
	}
	if run["goenv"] != "off" || run["gowork"] != "off" || run["goflags"] != "" || run["goexperiment"] != "" {
		t.Fatal("implicit build environment overrides are forbidden")
	}
	for key, value := range map[string]int{"maintenance_revision": 1, "discovery_case_count": 24, "replays_per_case": 2, "maximum_parallel_cases": 1, "maximum_depth": 3, "maximum_width": 8, "maximum_work": 48, "maximum_retained": 64, "maximum_graph_bytes": 1048576, "topology_workers": 1, "material_case_threshold": 3, "material_reporting_domain_threshold": 2} {
		if run[key] != float64(value) {
			t.Fatalf("frozen %s differs", key)
		}
	}
	for _, key := range []string{"held_out_access", "capability_expansion", "outcome_driven_retries", "automatic_v1_admission"} {
		if run[key] != false {
			t.Fatalf("forbidden %s", key)
		}
	}
	paths := map[string]string{
		"evaluator_manifest_sha256":     "specs/generic-causal-topology-repair/V21_EVALUATOR.sha256",
		"contract_manifest_sha256":      "specs/generic-causal-topology-repair/V21_CONTRACT.sha256",
		"maintenance_freeze_sha256":     "specs/generic-causal-topology-repair/V21_EVALUATOR_FREEZE.json",
		"v20_evaluator_manifest_sha256": "specs/generic-analysis-model-solver-admission/V20_EVALUATOR.sha256",
		"v20_report_sha256":             "internal/capabilityfeedback/testdata/closed_loop_open_set_v20_generation_zero/report.json",
		"v18_report_sha256":             "internal/capabilityfeedback/testdata/closed_loop_open_set_v18_generation_one/report.json",
		"selected_population_sha256":    "specs/generic-causal-topology-repair/V21_PUBLIC_TOPOLOGY_POPULATION.json",
		"corpus_manifest_sha256":        "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus/manifest.json",
		"corpus_checksums_sha256":       "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus/CHECKSUMS.sha256",
		"toolchain_lock_sha256":         "toolchain/kicad-promotion.lock.json",
		"go_mod_sha256":                 "go.mod", "go_sum_sha256": "go.sum",
	}
	for key, path := range paths {
		if run[key] != fileHash(t, filepath.Join(root(), filepath.FromSlash(path))) {
			t.Fatalf("input %s drifted", key)
		}
	}
	corpus, err := capabilityexecutorv10.LoadPublicDiscovery(filepath.Join(root(), "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if err != nil || len(corpus.Cases) != 24 || corpus.ManifestSHA256 != run["corpus_manifest_sha256"] {
		t.Fatalf("public corpus authentication failed: %v", err)
	}
	freeze, err := os.ReadFile("FREEZE.sha256")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"RUN.json": true, "README.md": true, "run.sh": true, "evaluation_test.go": true}
	for _, line := range strings.Split(strings.TrimSpace(string(freeze)), "\n") {
		if len(line) < 67 || line[64:66] != "  " || !want[line[66:]] {
			t.Fatal("invalid freeze inventory")
		}
		if fileHash(t, line[66:]) != line[:64] {
			t.Fatalf("run freeze drifted: %s", line[66:])
		}
		delete(want, line[66:])
	}
	if len(want) != 0 {
		t.Fatal("incomplete run freeze")
	}
}

func readReport(t *testing.T, path string) (capabilitybaselinev10.Report, []json.RawMessage) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	report, err := authenticateReport(data)
	if err != nil {
		t.Fatalf("authenticate report: %v", err)
	}
	var raw struct {
		Cases []struct {
			Case json.RawMessage `json:"case"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	cases := make([]json.RawMessage, len(raw.Cases))
	for i := range raw.Cases {
		cases[i] = raw.Cases[i].Case
	}
	return report, cases
}

func authenticateReport(data []byte) (capabilitybaselinev10.Report, error) {
	var report capabilitybaselinev10.Report
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return report, fmt.Errorf("trailing report data")
	}
	if err := capabilitybaselinev10.Validate(report); err != nil {
		return report, err
	}
	for _, record := range report.Cases {
		validated, err := capabilitybaselinev10.ValidateCase(record)
		if err != nil || validated.Hash != record.Hash {
			return report, fmt.Errorf("case hash differs")
		}
	}
	return report, nil
}

func view(t *testing.T, raw json.RawMessage) caseView {
	t.Helper()
	var result caseView
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// Call only for an authenticated case: pass gates are enforced by the shared
// evidence validator, while this leaf is emitted only after V21 certification.
func qualifying(record caseView) bool {
	if record.Outcome == "pass" {
		return true
	}
	if record.Outcome != "exhausted" || len(record.Frontier) == 0 {
		return false
	}
	for _, leaf := range record.Frontier {
		if len(leaf.Path) == 0 {
			return false
		}
		last := leaf.Path[len(leaf.Path)-1]
		if last.Capability != "passing_behavioral_evidence" || last.Code != "OPEN_TOPOLOGY_NO_PASSING_GRAPH" {
			return false
		}
	}
	return true
}

func TestPublishedMaintenanceEvaluation(t *testing.T) {
	path := filepath.Join(root(), filepath.FromSlash(publication))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("maintenance evaluation has not been published")
	}
	TestMaintenanceFreeze(t)
	report, current := readReport(t, path)
	v20, previous := readReport(t, filepath.Join(root(), "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v20_generation_zero", "report.json"))
	v18, _ := readReport(t, filepath.Join(root(), "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v18_generation_one", "report.json"))
	if report.CaseCount != 24 || report.EvaluatorManifestSHA256 != "d3c9c6124d52cb5c59bd4a175d75f1c8fa2f4c9d9e4148d70979f62de06974d1" || report.CorpusManifestSHA256 != v20.CorpusManifestSHA256 {
		t.Fatal("maintenance report binding differs")
	}
	data, err := os.ReadFile(filepath.Join("..", "V21_PUBLIC_TOPOLOGY_POPULATION.json"))
	if err != nil {
		t.Fatal(err)
	}
	var population struct {
		Selected []struct {
			ID string `json:"id"`
		} `json:"selected_cases"`
	}
	if err := json.Unmarshal(data, &population); err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{}
	for _, record := range population.Selected {
		selected[record.ID] = true
	}
	if len(selected) != 8 {
		t.Fatal("selected population differs")
	}
	a := assessment{Schema: "kicadai.v21-maintenance-assessment.v1", ReportHash: report.Hash, CaseCount: 24, OutcomeCounts: report.OutcomeCounts, QualifyingCases: []string{}, QualifyingDomains: []string{}, Regressions: []string{}, TerminalFrontier: map[string]int{}, Experimental: true}
	domains := map[string]bool{}
	for i, evidence := range report.Cases {
		now, before := view(t, current[i]), view(t, previous[i])
		if now.ID != fmt.Sprintf("v10_case_%03d", i+1) || now.ID != before.ID || evidence.RequirementSHA256 != v20.Cases[i].RequirementSHA256 {
			t.Fatal("case order or requirement binding differs")
		}
		if now.Domain != before.Domain || now.Role != before.Role || now.CircuitRole != before.CircuitRole || now.Safety != before.Safety {
			a.Regressions = append(a.Regressions, now.ID+":metadata")
		}
		if !selected[now.ID] {
			if bytes.Equal(current[i], previous[i]) {
				a.IneligibleIdentical++
			} else {
				a.Regressions = append(a.Regressions, now.ID+":ineligible_bytes")
			}
		}
		if (before.Outcome == "pass" || before.Outcome == "unsafe") && now.Outcome != before.Outcome {
			a.Regressions = append(a.Regressions, now.ID+":v20_terminal")
		}
		if (v18.Cases[i].Case.Outcome == "pass" || v18.Cases[i].Case.Outcome == "unsafe") && now.Outcome != v18.Cases[i].Case.Outcome {
			a.Regressions = append(a.Regressions, now.ID+":v18_terminal")
		}
		if now.Outcome == "unsafe" && before.Outcome != "unsafe" {
			a.Regressions = append(a.Regressions, now.ID+":new_unsafe")
		}
		a.ReplayCount += len(evidence.ReplaySHA256)
		if now.Outcome == "pass" {
			a.Passes++
			a.Promotions += len(evidence.Promotions)
		}
		if selected[now.ID] && qualifying(now) {
			a.QualifyingCases = append(a.QualifyingCases, now.ID)
			domains[now.Domain] = true
		}
		for _, leaf := range now.Frontier {
			if len(leaf.Path) != 0 {
				last := leaf.Path[len(leaf.Path)-1]
				a.TerminalFrontier[last.Capability+"/"+last.Code]++
			}
		}
	}
	for domain := range domains {
		a.QualifyingDomains = append(a.QualifyingDomains, domain)
	}
	slices.Sort(a.QualifyingDomains)
	slices.Sort(a.Regressions)
	a.MaterialThresholdMet = len(a.QualifyingCases) >= 3 && len(domains) >= 2
	a.PreservationPassed = a.IneligibleIdentical == 16 && len(a.Regressions) == 0
	a.EvaluationPassed = a.MaterialThresholdMet && a.PreservationPassed
	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MAINTENANCE_ASSESSMENT %s", encoded)
}

func TestQualifyingFrontierDoesNotCountRenames(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"pass", `{"outcome":"pass"}`, true},
		{"empty", `{"outcome":"exhausted"}`, false},
		{"certificate", `{"outcome":"exhausted","frontier":[{"path":[{"capability":"passing_behavioral_evidence","code":"OPEN_TOPOLOGY_NO_PASSING_GRAPH"}]}]}`, true},
		{"bound", `{"outcome":"exhausted","frontier":[{"path":[{"capability":"bounded_topology_completion","code":"TOPOLOGY_COMPLETION_BOUND_EXHAUSTED"}]}]}`, false},
		{"wrong_capability", `{"outcome":"exhausted","frontier":[{"path":[{"capability":"causal_topology_repair","code":"OPEN_TOPOLOGY_NO_PASSING_GRAPH"}]}]}`, false},
		{"empty_path", `{"outcome":"exhausted","frontier":[{"path":[]}]}`, false},
		{"unsafe", `{"outcome":"unsafe"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := qualifying(view(t, []byte(tc.source))); got != tc.want {
				t.Fatalf("qualifying=%v want %v", got, tc.want)
			}
		})
	}
}

func TestReportAuthenticationRejectsTamper(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root(), "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v20_generation_zero", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := authenticateReport(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*capabilitybaselinev10.Report){
		func(r *capabilitybaselinev10.Report) { r.Hash = strings.Repeat("0", 64) },
		func(r *capabilitybaselinev10.Report) { r.Cases[0].Hash = strings.Repeat("0", 64) },
		func(r *capabilitybaselinev10.Report) { r.Cases[0].ReplaySHA256[1] = strings.Repeat("0", 64) },
	} {
		var copy capabilitybaselinev10.Report
		original, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := json.Unmarshal(original, &copy); err != nil {
			t.Fatal(err)
		}
		mutation(&copy)
		changed, marshalErr := json.Marshal(copy)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, err := authenticateReport(changed); err == nil {
			t.Fatal("tampered report admitted")
		}
	}
	if _, err := authenticateReport(append(data, []byte(`{}`)...)); err == nil {
		t.Fatal("trailing object admitted")
	}
}
