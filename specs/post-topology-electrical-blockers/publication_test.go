package posttopologyelectrical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/electricaldiagnostics"
	ot "kicadai/internal/opentopologysynthesis"
)

func TestPublishedEvidenceInventory(t *testing.T) {
	data, err := os.ReadFile("EVIDENCE.sha256")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if len(line) < 67 || line[64:66] != "  " || seen[line[66:]] {
			t.Fatal("invalid publication checksum")
		}
		path := line[66:]
		seen[path] = true
		if fileHash(t, path) != line[:64] {
			t.Fatalf("publication bytes differ: %s", path)
		}
	}
	if len(seen) != 19 {
		t.Fatalf("publication inventory count %d", len(seen))
	}
}

func publicationJSON(t *testing.T, path string, target any) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
	return data
}

func publicationHash(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestPublishedRootCauseEvidence(t *testing.T) {
	root := filepath.Join("..", "..")
	var baseline capabilitybaselinev10.Report
	publicationJSON(t, filepath.Join(root, "internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json"), &baseline)
	references := map[string]capabilitybaselinev10.CaseEvidence{}
	for _, record := range baseline.Cases {
		references[record.Case.ID] = record
	}
	var totalTrace int
	var totalRaw int64
	for _, tc := range []struct {
		id, batch, assertion string
		stage                electricaldiagnostics.Stage
		attempt, operations  int
	}{
		{"v10_case_004", "initial", "closed_loop_phase_margin", electricaldiagnostics.StagePreparation, 1, 0},
		// Preserve the original label. A separate test verifies the stronger
		// report-bound assertion diagnosis used by ROOT_CAUSE.md.
		{"v10_case_017", "initial", "a_voltage_excitation_level", electricaldiagnostics.StageSolver, 1, 1},
		{"v10_case_018", "recovery-1", "a_output_noise", electricaldiagnostics.StageAssertion, 1, 0},
		{"v10_case_021", "recovery-1", "high_output_inactive_level", electricaldiagnostics.StageSolver, 2, 1},
	} {
		t.Run(tc.id, func(t *testing.T) {
			base := filepath.Join("evidence", tc.batch, "results", tc.id)
			var trace electricaldiagnostics.Synthesis
			data := publicationJSON(t, base+".trace.json", &trace)
			var measurement struct {
				CaseID            string  `json:"case_id"`
				Requirement       string  `json:"requirement_sha256"`
				ExpectedReplay    string  `json:"expected_replay_sha256"`
				Replay            string  `json:"replay_sha256"`
				Matches           bool    `json:"replay_matches"`
				Trace             string  `json:"trace_file_sha256"`
				Bytes             int     `json:"trace_bytes"`
				Raw               int64   `json:"raw_serialized_bytes"`
				SynthesisSeconds  float64 `json:"synthesis_seconds"`
				ProjectionSeconds float64 `json:"projection_seconds"`
			}
			publicationJSON(t, base+".measurements.json", &measurement)
			reference := references[tc.id]
			if len(reference.ReplaySHA256) != 2 || reference.ReplaySHA256[0] != reference.ReplaySHA256[1] {
				t.Fatal("historical replay identity unavailable")
			}
			if measurement.CaseID != tc.id || !measurement.Matches || measurement.Requirement != reference.RequirementSHA256 || measurement.ExpectedReplay != reference.ReplaySHA256[0] || measurement.Replay != measurement.ExpectedReplay || trace.ReplayHash != measurement.Replay {
				t.Fatal("diagnostic does not match exact historical public replay")
			}
			if fileHash(t, base+".trace.json") != measurement.Trace || len(data) != measurement.Bytes || measurement.Raw != trace.RawSerializedBytes || measurement.Raw <= int64(len(data)) || measurement.SynthesisSeconds <= 0 || measurement.ProjectionSeconds <= 0 {
				t.Fatal("measured trace identity, size, or timing differs")
			}
			unsigned := trace
			unsigned.Hash = ""
			if publicationHash(t, unsigned) != trace.Hash || len(trace.CertifiedCandidates) != 1 || trace.Status == ot.StatusPassed {
				t.Fatal("invalid or unexpectedly passing trace")
			}
			source, err := os.ReadFile(filepath.Join(root, "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus/discovery", tc.id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			sourceDigest := sha256.Sum256(source)
			if hex.EncodeToString(sourceDigest[:]) != measurement.Requirement {
				t.Fatal("public source file bytes differ from historical requirement identity")
			}
			requirement, issues := ot.DecodeStrict(bytes.NewReader(source))
			requirementHash, err := ot.CanonicalHash(requirement)
			if err != nil || len(issues) != 0 || requirementHash != trace.RequirementHash {
				t.Fatal("public requirement binding differs")
			}
			candidate := trace.CertifiedCandidates[0]
			certificate := candidate.Certificate
			certificate.Hash = ""
			graphHash, err := ot.GraphHash(candidate.Graph)
			if err != nil || publicationHash(t, certificate) != candidate.Certificate.Hash || !certificate.Complete || certificate.Contradictory || len(certificate.Obligations) != 0 || graphHash != certificate.GraphHash || certificate.RequirementHash != requirementHash || candidate.OperationCount != tc.operations {
				t.Fatal("structural certificate differs")
			}
			evaluation := candidate.Evaluation
			evaluation.Hash = ""
			if publicationHash(t, evaluation) != candidate.Evaluation.Hash || evaluation.InventoryHash != certificate.InventoryHash || evaluation.GraphHash != graphHash || evaluation.RequirementHash != requirementHash {
				t.Fatal("exact certificate/evaluation provenance differs")
			}
			failure := evaluation.FirstFailure
			if failure == nil || failure.Attempt != tc.attempt || failure.Stage != tc.stage || failure.Assertion != tc.assertion {
				t.Fatal("first failed gate differs")
			}
			totalTrace += len(data)
			totalRaw += trace.RawSerializedBytes
		})
	}
	if totalTrace != 49186 || totalRaw != 5753208442 {
		t.Fatalf("report size totals differ: %d / %d", totalTrace, totalRaw)
	}
}

func TestPublishedLegacyAssertionInterpretation(t *testing.T) {
	var trace electricaldiagnostics.Synthesis
	publicationJSON(t, "evidence/initial/results/v10_case_017.trace.json", &trace)
	evaluation := trace.CertifiedCandidates[0].Evaluation
	failure := evaluation.FirstFailure
	for _, diagnosis := range evaluation.Diagnoses {
		if diagnosis.Code == "assertion_below_minimum" && diagnosis.EvidenceHash == failure.ReportHash && diagnosis.RequirementID == failure.Assertion && diagnosis.OperatingCase == failure.OperatingCase+"/"+failure.Corner && diagnosis.Analysis == failure.Analysis && diagnosis.Metric == failure.Metric && diagnosis.Actual != nil && failure.Actual != nil && diagnosis.RequiredMin != nil && failure.Minimum != nil && *diagnosis.Actual == *failure.Actual && *diagnosis.RequiredMin == *failure.Minimum && *failure.Actual < *failure.Minimum {
			return
		}
	}
	t.Fatal("report lacks exact numerical assertion evidence")
}
