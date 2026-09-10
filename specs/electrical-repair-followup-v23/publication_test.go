package electricalfollowup

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	diag "kicadai/internal/electricaldiagnostics"
	ot "kicadai/internal/opentopologysynthesis"
)

func TestResidualPublication(t *testing.T) {
	manifest, err := os.ReadFile("EVIDENCE.sha256")
	if err != nil {
		t.Fatal(err)
	}
	manifestHash := sha256.Sum256(manifest)
	if hex.EncodeToString(manifestHash[:]) != "8434b4f9a397e0a45c05145970d1e50a0c48e404bdfeed23b9202921fac2c6b6" {
		t.Fatal("frozen evidence manifest differs")
	}
	wantPaths := map[string]bool{"RESIDUAL_REPORT.md": true, "evidence/source-commit.txt": true, "evidence/diagnostic.sha256": true, "evidence/go-environment.json": true, "evidence/progress.log": true, "evidence/resource-usage.log": true}
	for _, id := range []string{"v10_case_004", "v10_case_017", "v10_case_018", "v10_case_021"} {
		wantPaths["evidence/"+id+".diagnostic.json"] = true
		wantPaths["evidence/"+id+".metrics.json"] = true
	}
	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		fields := strings.Split(line, "  ")
		if len(fields) != 2 || !wantPaths[fields[1]] {
			t.Fatal("invalid, duplicate, or unexpected evidence entry")
		}
		info, err := os.Lstat(fields[1])
		if err != nil || !info.Mode().IsRegular() {
			t.Fatal("evidence is not a regular file")
		}
		data, err := os.ReadFile(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != fields[0] {
			t.Fatalf("evidence digest differs: %s", fields[1])
		}
		delete(wantPaths, fields[1])
	}
	if len(wantPaths) != 0 {
		t.Fatal("missing evidence entries")
	}
	entries, err := os.ReadDir("evidence")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 13 {
		t.Fatal("unexpected evidence inventory")
	}
	for _, tc := range []struct {
		id                           string
		trials, simulations, corners int
	}{{"v10_case_004", 0, 0, 0}, {"v10_case_017", 114, 128, 638}, {"v10_case_018", 4, 6, 20}, {"v10_case_021", 0, 0, 0}} {
		var source sidecar
		readJSON(t, filepath.Join("../../internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/replays", tc.id, "replay-1/ELECTRICAL_REPAIR.json"), &source)
		var published struct {
			Schema          string            `json:"schema"`
			Case            string            `json:"case"`
			SourceTraceHash string            `json:"source_trace_sha256"`
			RepairHash      string            `json:"repair_sha256"`
			InitialFailure  *diag.Failure     `json:"initial_failure"`
			StopReason      string            `json:"v22_stop_reason"`
			Trials          []trialDiagnostic `json:"trials"`
			Error           string            `json:"error,omitempty"`
		}
		readJSON(t, "evidence/"+tc.id+".diagnostic.json", &published)
		if published.Schema != "kicadai.residual-electrical-diagnostic.v23" || published.Case != tc.id || published.Error != "" || source.Repair == nil || published.RepairHash != source.Repair.Hash || published.StopReason != source.Repair.StopReason || len(published.Trials) != tc.trials {
			t.Fatal("residual publication differs")
		}
		phase := "initial"
		if tc.id == "v10_case_018" || tc.id == "v10_case_021" {
			phase = "recovery-1"
		}
		var prior diag.Synthesis
		readJSON(t, filepath.Join("../post-topology-electrical-blockers/evidence", phase, "results", tc.id+".trace.json"), &prior)
		if len(prior.CertifiedCandidates) != 1 || published.SourceTraceHash != prior.Hash || !reflect.DeepEqual(published.InitialFailure, prior.CertifiedCandidates[0].Evaluation.FirstFailure) {
			t.Fatal("retained initial failure binding differs")
		}
		simulations, corners := 0, 0
		for i, trial := range published.Trials {
			if !reflect.DeepEqual(trial.Trial, source.Repair.Trials[i]) || trial.Evaluation.EvaluationHash != trial.Trial.EvaluationHash {
				t.Fatal("recorded trial identity differs")
			}
			unsigned := trial.Evaluation
			unsigned.Hash = ""
			if digest(unsigned) != trial.Evaluation.Hash || trial.Evaluation.FirstFailure == nil {
				t.Fatal("diagnostic projection authentication failed")
			}
			policy := ot.DefaultPolicy()
			policy.MaxCandidateSimulations = 128 - simulations
			policy.MaxCornerEvaluations = 4096 - corners
			if trial.Policy != policy {
				t.Fatal("recorded remaining policy differs")
			}
			simulations += max(1, trial.Evaluation.Consumption.CandidateSimulations)
			corners += trial.Evaluation.Consumption.CornerEvaluations
		}
		if simulations != tc.simulations || corners != tc.corners {
			t.Fatal("numerical consumption differs")
		}
		var metrics struct {
			Invocations   int   `json:"evaluation_invocations"`
			Authenticated int   `json:"authenticated_trials"`
			Nanoseconds   int64 `json:"elapsed_nanoseconds"`
		}
		readJSON(t, "evidence/"+tc.id+".metrics.json", &metrics)
		if metrics.Invocations != tc.trials || metrics.Authenticated != tc.trials || metrics.Nanoseconds <= 0 {
			t.Fatal("diagnostic execution count differs")
		}
	}
}
