package electricalevaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityexecutorv10"
	"kicadai/internal/opentopologysynthesis"
)

const publication = "internal/capabilityfeedback/testdata/closed_loop_open_set_v23_public_1/report.json"
const predecessor = "internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/report.json"

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

func checkSeal(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if len(line) < 67 || line[64:66] != "  " || seen[line[66:]] {
			t.Fatalf("invalid seal %s", path)
		}
		name := line[66:]
		if fileHash(t, filepath.Join(filepath.Dir(path), name)) != line[:64] {
			t.Fatalf("sealed input differs: %s", name)
		}
		seen[name] = true
	}
	return seen
}

func TestV23Freeze(t *testing.T) {
	for _, path := range []string{"FREEZE.sha256", "EVALUATOR.sha256", "../REPAIR_IMPLEMENTATION.sha256", "../ADAPTER_IMPLEMENTATION.sha256", "../SOLVER_IMPLEMENTATION.sha256", "../CORRECTION_SCOPE.sha256", "../EVIDENCE.sha256", "../DIAGNOSTIC.sha256", "../../post-topology-electrical-blockers/V22_IMPLEMENTATION.sha256", "../../post-topology-electrical-blockers/EVIDENCE.sha256", "../../post-topology-electrical-blockers/DIAGNOSTIC.sha256", "../../post-topology-electrical-blockers/public-evaluation-v22/EVALUATOR.sha256", "../../post-topology-electrical-blockers/public-evaluation-v22/FREEZE.sha256", "../../generic-causal-topology-repair/V21_CONTRACT.sha256", "../../generic-causal-topology-repair/V21_EVALUATOR.sha256", "../../generic-analysis-model-solver-admission/V20_CONTRACT.sha256", "../../generic-analysis-model-solver-admission/V20_EVALUATOR.sha256"} {
		checkSeal(t, path)
	}
	const v22PublicationSeal = "internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/SHA256SUMS"
	if fileHash(t, filepath.Join(root(), v22PublicationSeal)) != "12f5e026bda420fc6a663f2d464986282aeff25eb8df356d9dee1ffecbcf612a" {
		t.Fatal("immutable V22 publication seal changed")
	}
	checkSeal(t, filepath.Join(root(), v22PublicationSeal))
	var run struct {
		Schema         string                                          `json:"schema"`
		Implementation string                                          `json:"implementation_commit"`
		GoVersion      string                                          `json:"go_version"`
		Platform       string                                          `json:"platform"`
		CGO            string                                          `json:"cgo_enabled"`
		GoEnv          string                                          `json:"goenv"`
		GoWork         string                                          `json:"gowork"`
		GoFlags        string                                          `json:"goflags"`
		GoExperiment   string                                          `json:"goexperiment"`
		KiCad          string                                          `json:"kicad_version"`
		Cases          int                                             `json:"cases"`
		Replays        int                                             `json:"replays_per_case"`
		Parallel       int                                             `json:"parallel_cases"`
		Selected       int                                             `json:"selected_cases"`
		Preserved      int                                             `json:"preserved_cases"`
		Limits         opentopologysynthesis.ElectricalRepairLimitsV22 `json:"electrical_limits"`
		Timeout        string                                          `json:"timeout"`
		Additional     int                                             `json:"required_additional_complete_passes"`
		PredecessorSHA string                                          `json:"predecessor_report_sha256"`
		Publication    string                                          `json:"publication_root"`
		HeldOut        bool                                            `json:"held_out_access"`
		Resume         bool                                            `json:"resume"`
		Retries        bool                                            `json:"outcome_driven_retries"`
		Admission      bool                                            `json:"automatic_v1_admission"`
	}
	data, err := os.ReadFile("RUN.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	if run.Schema != "kicadai.public-electrical-evaluation.v23" || run.Implementation != "9413c881ace95065b62d4cdaa31ee532f0b7cf87" || run.GoVersion != "go1.26.8" || run.Platform != "darwin/arm64" || run.KiCad != "10.0.3" || run.Timeout != "6h" || run.CGO != "1" || run.GoEnv != "off" || run.GoWork != "off" || run.GoFlags != "" || run.GoExperiment != "" {
		t.Fatal("frozen identity or environment differs")
	}
	if run.Cases != 24 || run.Replays != 2 || run.Parallel != 1 || run.Selected != 4 || run.Preserved != 20 || run.Additional != 1 || run.Limits != opentopologysynthesis.DefaultElectricalRepairLimitsV22() || run.Limits != (opentopologysynthesis.ElectricalRepairLimitsV22{MaxDepth: 4, BeamWidth: 8, MaxBindingWork: 4096, MaxEvaluations: 128, MaxCorners: 4096}) {
		t.Fatal("frozen count limits differ")
	}
	if run.HeldOut || run.Resume || run.Retries || run.Admission || run.Publication != filepath.ToSlash(filepath.Dir(publication)) || run.PredecessorSHA != fileHash(t, filepath.Join(root(), predecessor)) {
		t.Fatal("publication or preservation boundary differs")
	}
	corpus, err := capabilityexecutorv10.LoadPublicDiscovery(filepath.Join(root(), "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus"))
	if err != nil || len(corpus.Cases) != 24 || corpus.ManifestSHA256 != "0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94" {
		t.Fatalf("public corpus authentication: %v", err)
	}
	selected := selection(t)
	if !reflect.DeepEqual(selected, map[string]bool{"v10_case_004": true, "v10_case_017": true, "v10_case_018": true, "v10_case_021": true}) {
		t.Fatal("diagnostic population changed")
	}
}

func selection(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("../V23_PUBLIC_POPULATION.json")
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
	result := map[string]bool{}
	for _, c := range population.Selected {
		result[c.ID] = true
	}
	return result
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
	for _, c := range report.Cases {
		validated, err := capabilitybaselinev10.ValidateCase(c)
		if err != nil || validated.Hash != c.Hash {
			return report, fmt.Errorf("case authentication differs")
		}
	}
	return report, nil
}

func readReport(t *testing.T, path string) capabilitybaselinev10.Report {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root(), path))
	if err != nil {
		t.Fatal(err)
	}
	report, err := authenticateReport(data)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestPublishedV23Evaluation(t *testing.T) {
	if _, err := os.Stat(filepath.Join(root(), publication)); os.IsNotExist(err) {
		t.Skip("V23 public evaluation not yet published")
	}
	TestV23Freeze(t)
	report := readReport(t, publication)
	prior := readReport(t, predecessor)
	if report.CaseCount != 24 || report.CorpusManifestSHA256 != prior.CorpusManifestSHA256 || report.EnvironmentSHA256 != prior.EnvironmentSHA256 || report.EvaluatorManifestSHA256 != fileHash(t, "EVALUATOR.sha256") {
		t.Fatal("successor report binding differs")
	}
	v18 := readReport(t, "internal/capabilityfeedback/testdata/closed_loop_open_set_v18_generation_one/report.json")
	v20 := readReport(t, "internal/capabilityfeedback/testdata/closed_loop_open_set_v20_generation_zero/report.json")
	v21 := readReport(t, "internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json")
	selected := selection(t)
	additional, regressions := []string{}, []string{}
	preserved, passes, promotions, replays := 0, 0, 0, 0
	frontierBefore, frontierAfter := map[string]int{}, map[string]int{}
	for i, now := range report.Cases {
		before := prior.Cases[i]
		if now.Case.ID != fmt.Sprintf("v10_case_%03d", i+1) || now.Case.ID != before.Case.ID || now.RequirementSHA256 != before.RequirementSHA256 {
			t.Fatal("case order or requirement changed")
		}
		if now.Case.ReportingDomain != before.Case.ReportingDomain || now.Case.CircuitRole != before.Case.CircuitRole || now.Case.Role != before.Case.Role || now.Case.SafetyImpact != before.Case.SafetyImpact {
			regressions = append(regressions, now.Case.ID+":metadata")
		}
		if !selected[now.Case.ID] {
			if reflect.DeepEqual(now.Case, before.Case) && reflect.DeepEqual(now.ReplaySHA256, before.ReplaySHA256) && reflect.DeepEqual(now.Gates, before.Gates) {
				preserved++
			} else {
				regressions = append(regressions, now.Case.ID+":preserved_evidence")
			}
		}
		for _, historical := range []capabilitybaselinev10.CaseEvidence{before, v18.Cases[i], v20.Cases[i], v21.Cases[i]} {
			if (historical.Case.Outcome == "pass" || historical.Case.Outcome == "unsafe") && now.Case.Outcome != historical.Case.Outcome {
				regressions = append(regressions, now.Case.ID+":historical_terminal")
			}
		}
		if now.Case.Outcome == "unsafe" && before.Case.Outcome != "unsafe" {
			regressions = append(regressions, now.Case.ID+":new_unsafe")
		}
		if now.Case.Outcome == "pass" {
			passes++
			promotions += len(now.Promotions)
			if before.Case.Outcome != "pass" {
				additional = append(additional, now.Case.ID)
			} else if now.Promotions[0].ProjectSHA256 != before.Promotions[0].ProjectSHA256 {
				regressions = append(regressions, now.Case.ID+":project_replay")
			}
		}
		replays += len(now.ReplaySHA256)
		for _, gap := range before.Case.Frontier {
			for _, leaf := range gap.Path {
				frontierBefore[leaf.Capability+"/"+leaf.Code]++
			}
		}
		for _, gap := range now.Case.Frontier {
			for _, leaf := range gap.Path {
				frontierAfter[leaf.Capability+"/"+leaf.Code]++
			}
		}
	}
	assessment := map[string]any{"schema": "kicadai.public-electrical-assessment.v23", "report_hash": report.Hash, "cases": 24, "replays": replays, "passes": passes, "installed_kicad_promotions": promotions, "additional_complete_passes": additional, "preserved_cases": preserved, "regressions": regressions, "terminal_frontier_before": frontierBefore, "terminal_frontier_after": frontierAfter, "outcome_counts": report.OutcomeCounts, "preservation_passed": preserved == 20 && len(regressions) == 0, "electrical_improvement_passed": len(additional) >= 1 && preserved == 20 && len(regressions) == 0, "experimental": true}
	data, err := json.Marshal(assessment)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("V23_ASSESSMENT %s", data)
	if len(regressions) != 0 || preserved != 20 {
		t.Error("V23 preservation failed; retain and publish the negative assessment")
	}
}

func TestV23ReportAuthenticationRejectsTampering(t *testing.T) {
	prior := readReport(t, predecessor)
	for _, mutate := range []func(*capabilitybaselinev10.Report){
		func(r *capabilitybaselinev10.Report) { r.Hash = strings.Repeat("0", 64) },
		func(r *capabilitybaselinev10.Report) { r.Cases[0].Hash = strings.Repeat("0", 64) },
		func(r *capabilitybaselinev10.Report) { r.Cases[0].ReplaySHA256[1] = strings.Repeat("0", 64) },
	} {
		encoded, err := json.Marshal(prior)
		if err != nil {
			t.Fatal(err)
		}
		var copyReport capabilitybaselinev10.Report
		if err := json.Unmarshal(encoded, &copyReport); err != nil {
			t.Fatal(err)
		}
		mutate(&copyReport)
		encoded, err = json.Marshal(copyReport)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := authenticateReport(encoded); err == nil {
			t.Fatal("tampered report accepted")
		}
	}
}
