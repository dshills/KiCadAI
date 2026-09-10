// Package v23publication audits retained evidence without executing synthesis.
// It is separate from the frozen evaluator and cannot change its outcomes.
package v23publication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
	ot "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/simmodel"
)

const repository = "../../.."
const publication = repository + "/internal/capabilityfeedback/testdata/closed_loop_open_set_v23_public_1"

type sidecar struct {
	Schema           string                        `json:"schema"`
	Selected         bool                          `json:"selected"`
	PredecessorHash  string                        `json:"predecessor_sha256"`
	SynthesisHash    string                        `json:"synthesis_sha256"`
	ElectricalHash   string                        `json:"electrical_sha256,omitempty"`
	SolverPolicyID   string                        `json:"solver_policy_id,omitempty"`
	SolverPolicyHash string                        `json:"solver_policy_sha256,omitempty"`
	Repair           *ot.ElectricalRepairResultV23 `json:"repair,omitempty"`
}

// V22 and V23 start from the same frozen predecessor, but retain different
// repair result schemas. Decode the historical record using its original type.
type predecessorSidecar struct {
	Schema          string                        `json:"schema"`
	Selected        bool                          `json:"selected"`
	PredecessorHash string                        `json:"predecessor_sha256"`
	SynthesisHash   string                        `json:"synthesis_sha256"`
	ElectricalHash  string                        `json:"electrical_sha256,omitempty"`
	Repair          *ot.ElectricalRepairResultV22 `json:"repair,omitempty"`
}

type replayMetrics struct {
	Schema                  string `json:"schema"`
	SynthesisNanoseconds    int64  `json:"synthesis_nanoseconds"`
	HashNanoseconds         int64  `json:"hash_nanoseconds"`
	PromotionNanoseconds    int64  `json:"promotion_nanoseconds"`
	CanonicalSynthesisBytes int64  `json:"canonical_synthesis_bytes"`
	SynthesisSpoolBytes     int64  `json:"synthesis_spool_bytes"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func valueDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func readJSON(t *testing.T, path string, target any) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("trailing data in %s", path)
	}
	return data
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func authenticateSidecar(record sidecar, selected bool, requirementHash, predecessorHash string) error {
	if record.Schema != "kicadai.public-electrical-replay.v23" || record.Selected != selected || !validDigest(record.PredecessorHash) || !validDigest(record.SynthesisHash) {
		return fmt.Errorf("sidecar identity differs")
	}
	if !validDigest(predecessorHash) || record.PredecessorHash != predecessorHash {
		return fmt.Errorf("sidecar predecessor differs from frozen V22 evidence")
	}
	if !selected {
		if record.Repair != nil || record.ElectricalHash != "" || record.SolverPolicyID != "" || record.SolverPolicyHash != "" || record.PredecessorHash != record.SynthesisHash {
			return fmt.Errorf("unselected case contains successor evidence")
		}
		return nil
	}
	if record.SolverPolicyID != simmodel.SolverIDV23 || record.SolverPolicyHash != simmodel.SolverSHA256V23() {
		return fmt.Errorf("selected solver policy differs from frozen V23 execution")
	}
	repairHash := ""
	if record.Repair == nil && record.PredecessorHash != record.SynthesisHash {
		return fmt.Errorf("missing repair for changed synthesis")
	}
	if record.Repair != nil {
		repair := *record.Repair
		repairHash, repair.Hash = repair.Hash, ""
		got, err := valueDigest(repair)
		if err != nil {
			return fmt.Errorf("marshal repair identity: %w", err)
		}
		if got != repairHash || repair.Schema != "kicadai.electrical-repair.v23" || repair.RequirementHash != requirementHash || repair.Limits != ot.DefaultElectricalRepairLimitsV22() || repair.SolverPolicyID != record.SolverPolicyID || repair.SolverPolicyHash != record.SolverPolicyHash {
			return fmt.Errorf("repair identity, requirement, or frozen limits differ")
		}
		if repair.Selected == nil && record.PredecessorHash != record.SynthesisHash {
			return fmt.Errorf("nonpassing repair changed predecessor synthesis")
		}
	}
	got, err := valueDigest(synthesisIdentity(record, repairHash))
	if err != nil {
		return fmt.Errorf("marshal electrical wrapper identity: %w", err)
	}
	if got != record.ElectricalHash {
		return fmt.Errorf("electrical wrapper identity differs")
	}
	return nil
}

// Mirrors the frozen synthesis wrapper, not a second numerical evaluation.
func synthesisIdentity(record sidecar, repairHash string) any {
	return struct{ Schema, SolverPolicyID, SolverPolicyHash, PredecessorHash, SynthesisHash, RepairHash string }{
		"kicadai.electrical-synthesis.v23", record.SolverPolicyID, record.SolverPolicyHash, record.PredecessorHash, record.SynthesisHash, repairHash,
	}
}

func auditCases(t *testing.T, evidenceRoot string, records []capabilitybaselinev10.CaseEvidence) {
	t.Helper()
	var predecessor capabilitybaselinev10.Report
	readJSON(t, repository+"/internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/report.json", &predecessor)
	if err := capabilitybaselinev10.Validate(predecessor); err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || len(records) > len(predecessor.Cases) {
		t.Fatal("invalid public case count")
	}
	var population struct {
		Selected []struct {
			ID string `json:"id"`
		} `json:"selected_cases"`
	}
	// The frozen population has other fields; only this projection is needed.
	data, err := os.ReadFile("../V23_PUBLIC_POPULATION.json")
	if err != nil {
		t.Fatalf("read frozen public selection: %v", err)
	}
	if err := json.Unmarshal(data, &population); err != nil {
		t.Fatalf("decode frozen public selection: %v", err)
	}
	selected := map[string]bool{}
	for _, entry := range population.Selected {
		selected[entry.ID] = true
	}
	var canonicalBytes int64
	for index, record := range records {
		// Authenticate the frozen identity before using it in any local path.
		if record.Case.ID != predecessor.Cases[index].Case.ID || record.RequirementSHA256 != predecessor.Cases[index].RequirementSHA256 {
			t.Fatal("public case identity or order differs from the predecessor")
		}
		t.Run(record.Case.ID, func(t *testing.T) {
			validated, err := capabilitybaselinev10.ValidateCase(record)
			if err != nil || validated.Hash != record.Hash {
				t.Fatal("case evidence authentication failed")
			}
			source, err := os.ReadFile(filepath.Join(repository, "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus/discovery", record.Case.ID+".json"))
			if err != nil || digest(source) != record.RequirementSHA256 {
				t.Fatal("public requirement file identity differs")
			}
			requirement, issues := ot.DecodeStrict(bytes.NewReader(source))
			requirementHash, err := ot.CanonicalHash(requirement)
			if err != nil || len(issues) != 0 {
				t.Fatal("public requirement is invalid")
			}
			var priorSidecar []byte
			priorPromotionHash := ""
			for replay := 1; replay <= 2; replay++ {
				root := filepath.Join(evidenceRoot, record.Case.ID, fmt.Sprintf("replay-%d", replay))
				var marker struct {
					Schema                  string `json:"schema"`
					Version                 int    `json:"version"`
					CaseID                  string `json:"case_id"`
					Replay                  int    `json:"replay"`
					CorpusManifestSHA256    string `json:"corpus_manifest_sha256"`
					RequirementSHA256       string `json:"requirement_sha256"`
					EnvironmentSHA256       string `json:"environment_sha256"`
					EvaluatorManifestSHA256 string `json:"evaluator_manifest_sha256"`
				}
				rootBytes := readJSON(t, filepath.Join(root, "CLEAN_ROOT.json"), &marker)
				if digest(rootBytes) != record.ReplayRootSHA256[replay-1] || marker.Schema != "kicadai.closed-loop-open-set-clean-root.v23" || marker.Version != 23 || marker.CaseID != record.Case.ID || marker.Replay != replay || marker.CorpusManifestSHA256 != predecessor.CorpusManifestSHA256 || marker.RequirementSHA256 != record.RequirementSHA256 || marker.EnvironmentSHA256 != record.EnvironmentSHA256 || marker.EvaluatorManifestSHA256 != record.EvaluatorManifestSHA256 {
					t.Fatal("clean-root commitment differs")
				}
				var electrical sidecar
				payload := readJSON(t, filepath.Join(root, "ELECTRICAL_REPAIR.json"), &electrical)
				var historical predecessorSidecar
				readJSON(t, filepath.Join(repository, "internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/replays", record.Case.ID, fmt.Sprintf("replay-%d", replay), "ELECTRICAL_REPAIR.json"), &historical)
				if historical.Schema != "kicadai.public-electrical-replay.v22" || historical.Selected != selected[record.Case.ID] || !validDigest(historical.PredecessorHash) {
					t.Fatal("frozen V22 predecessor evidence differs")
				}
				if err := authenticateSidecar(electrical, selected[record.Case.ID], requirementHash, historical.PredecessorHash); err != nil {
					t.Fatal(err)
				}
				if replay == 2 && !bytes.Equal(priorSidecar, payload) {
					t.Fatal("electrical sidecar differs across replay roots")
				}
				priorSidecar = payload
				var metrics replayMetrics
				readJSON(t, filepath.Join(root, "METRICS.json"), &metrics)
				if metrics.Schema != "kicadai.public-replay-metrics.v23" || metrics.SynthesisNanoseconds <= 0 || metrics.HashNanoseconds <= 0 || metrics.PromotionNanoseconds < 0 || metrics.CanonicalSynthesisBytes <= 0 || metrics.SynthesisSpoolBytes != 0 {
					t.Fatal("invalid replay measurement")
				}
				canonicalBytes += metrics.CanonicalSynthesisBytes
				if record.Case.Outcome == "pass" || metrics.PromotionNanoseconds > 0 {
					var promotion ot.PhysicalPromotionResult
					readJSON(t, filepath.Join(root, "PROMOTION.json"), &promotion)
					var expected *capabilitybaselinev10.PromotionEvidence
					if record.Case.Outcome == "pass" {
						expected = &record.Promotions[replay-1]
					}
					if err := authenticatePromotion(promotion, electrical.SynthesisHash, expected, metrics.PromotionNanoseconds); err != nil {
						t.Fatal(err)
					}
					if replay == 2 && priorPromotionHash != promotion.Hash {
						t.Fatal("physical promotion differs across outer replays")
					}
					priorPromotionHash = promotion.Hash
				} else if priorPromotionHash != "" {
					t.Fatal("physical promotion missing in second replay")
				}
			}
		})
	}
	if !t.Failed() {
		t.Logf("authenticated_cases=%d replays=%d canonical_synthesis_bytes=%d synthesis_spool_bytes=0", len(records), 2*len(records), canonicalBytes)
	}
}

func authenticatePromotion(promotion ot.PhysicalPromotionResult, synthesisHash string, expected *capabilitybaselinev10.PromotionEvidence, elapsed int64) error {
	got, err := valueDigest(promotionIdentity(promotion))
	if err != nil {
		return fmt.Errorf("marshal physical promotion identity: %w", err)
	}
	if got != promotion.Hash || promotion.SynthesisHash != synthesisHash || elapsed <= 0 {
		return fmt.Errorf("physical promotion identity or measurement differs")
	}
	if expected == nil {
		if promotion.Status != ot.PhysicalPromotionFailed || len(promotion.Issues) == 0 {
			return fmt.Errorf("nonpassing promotion lacks terminal failure evidence")
		}
		return nil
	}
	if promotion.Hash != expected.RunSHA256 || promotion.ProjectHash != expected.ProjectSHA256 || !promotion.ReplayIdentical || promotion.Status != ot.PhysicalPromotionPassed || len(promotion.Runs) != 2 {
		return fmt.Errorf("passing promotion evidence differs")
	}
	for i, run := range promotion.Runs {
		if run.Number != i+1 || run.ProjectHash != promotion.ProjectHash {
			return fmt.Errorf("passing promotion lacks two identical clean projects")
		}
	}
	return nil
}

func TestPublishedV23ReplayEvidence(t *testing.T) {
	var report capabilitybaselinev10.Report
	readJSON(t, filepath.Join(publication, "report.json"), &report)
	if err := capabilitybaselinev10.Validate(report); err != nil || report.CaseCount != 24 {
		t.Fatalf("invalid complete report: %v", err)
	}
	auditCases(t, filepath.Join(publication, "replays"), report.Cases)
}

func TestCompletedV23ScratchEvidence(t *testing.T) {
	root := os.Getenv("KICADAI_V23_AUDIT_SCRATCH")
	if root == "" {
		t.Skip("optional read-only audit of completed scratch checkpoints")
	}
	paths, err := filepath.Glob(filepath.Join(root, "checkpoints", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatal("no completed checkpoints")
	}
	records := make([]capabilitybaselinev10.CaseEvidence, 0, len(paths))
	for _, path := range paths {
		var record capabilitybaselinev10.CaseEvidence
		readJSON(t, path, &record)
		records = append(records, record)
	}
	auditCases(t, root, records)
}

func TestSidecarAuthenticationRejectsTampering(t *testing.T) {
	requirement := strings.Repeat("a", 64)
	predecessor := strings.Repeat("b", 64)
	mustDigest := func(t *testing.T, value any) string {
		t.Helper()
		hash, err := valueDigest(value)
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	makeRecord := func(t *testing.T) sidecar {
		t.Helper()
		repair := ot.ElectricalRepairResultV23{Schema: "kicadai.electrical-repair.v23", SolverPolicyID: simmodel.SolverIDV23, SolverPolicyHash: simmodel.SolverSHA256V23(), RequirementHash: requirement, Status: ot.RepairSearchUnsupported, StopReason: "critical_failure", Limits: ot.DefaultElectricalRepairLimitsV22(), Trials: []ot.ElectricalRepairTrialV23{}}
		repair.Hash = mustDigest(t, repair)
		record := sidecar{Schema: "kicadai.public-electrical-replay.v23", Selected: true, SolverPolicyID: simmodel.SolverIDV23, SolverPolicyHash: simmodel.SolverSHA256V23(), PredecessorHash: predecessor, SynthesisHash: predecessor, Repair: &repair}
		record.ElectricalHash = mustDigest(t, synthesisIdentity(record, repair.Hash))
		return record
	}
	if err := authenticateSidecar(makeRecord(t), true, requirement, predecessor); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*testing.T, *sidecar){
		"wrapper":         func(_ *testing.T, s *sidecar) { s.ElectricalHash = strings.Repeat("0", 64) },
		"requirement":     func(_ *testing.T, s *sidecar) { s.Repair.RequirementHash = strings.Repeat("c", 64) },
		"repair":          func(_ *testing.T, s *sidecar) { s.Repair.BindingWork++ },
		"selection":       func(_ *testing.T, s *sidecar) { s.Selected = false },
		"missing":         func(_ *testing.T, s *sidecar) { s.Repair = nil },
		"synthesis":       func(_ *testing.T, s *sidecar) { s.SynthesisHash = strings.Repeat("d", 64) },
		"digest_format":   func(_ *testing.T, s *sidecar) { s.PredecessorHash = "invalid" },
		"solver_identity": func(_ *testing.T, s *sidecar) { s.SolverPolicyID = "historical" },
		"solver_digest":   func(_ *testing.T, s *sidecar) { s.SolverPolicyHash = strings.Repeat("d", 64) },
		"rehashed_predecessor": func(t *testing.T, s *sidecar) {
			s.PredecessorHash, s.SynthesisHash = strings.Repeat("d", 64), strings.Repeat("d", 64)
			s.ElectricalHash = mustDigest(t, synthesisIdentity(*s, s.Repair.Hash))
		},
		"rehashed_solver": func(t *testing.T, s *sidecar) {
			s.SolverPolicyHash = strings.Repeat("d", 64)
			s.Repair.SolverPolicyHash = s.SolverPolicyHash
			s.Repair.Hash = ""
			s.Repair.Hash = mustDigest(t, *s.Repair)
			s.ElectricalHash = mustDigest(t, synthesisIdentity(*s, s.Repair.Hash))
		},
		"rehashed_limits": func(t *testing.T, s *sidecar) {
			s.Repair.Limits.MaxDepth++
			s.Repair.Hash = ""
			s.Repair.Hash = mustDigest(t, *s.Repair)
			s.ElectricalHash = mustDigest(t, synthesisIdentity(*s, s.Repair.Hash))
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := makeRecord(t)
			mutate(t, &record)
			if err := authenticateSidecar(record, true, requirement, predecessor); err == nil {
				t.Fatal("tampered sidecar accepted")
			}
		})
	}
}

func TestHistoricalSidecarCannotClaimV23Solver(t *testing.T) {
	record := sidecar{Schema: "kicadai.public-electrical-replay.v23", PredecessorHash: strings.Repeat("b", 64), SynthesisHash: strings.Repeat("b", 64)}
	if err := authenticateSidecar(record, false, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	record.SolverPolicyID, record.SolverPolicyHash = simmodel.SolverIDV23, simmodel.SolverSHA256V23()
	if err := authenticateSidecar(record, false, strings.Repeat("a", 64), strings.Repeat("b", 64)); err == nil {
		t.Fatal("unselected historical execution claimed the new solver policy")
	}
}

func TestHistoricalSidecarCannotReplaceFrozenPredecessor(t *testing.T) {
	record := sidecar{Schema: "kicadai.public-electrical-replay.v23", PredecessorHash: strings.Repeat("c", 64), SynthesisHash: strings.Repeat("c", 64)}
	for _, expected := range []string{strings.Repeat("b", 64), "invalid"} {
		if err := authenticateSidecar(record, false, strings.Repeat("a", 64), expected); err == nil {
			t.Fatal("unselected historical execution accepted a substituted predecessor")
		}
	}
}
