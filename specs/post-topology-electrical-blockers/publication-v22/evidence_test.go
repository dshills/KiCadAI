// Package v22publication audits retained evidence without executing synthesis.
// It is separate from the frozen evaluator and cannot change its outcomes.
package v22publication

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
)

const repository = "../../.."
const publication = repository + "/internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1"

type sidecar struct {
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

func authenticateSidecar(record sidecar, selected bool, requirementHash string) error {
	if record.Schema != "kicadai.public-electrical-replay.v22" || record.Selected != selected || !validDigest(record.PredecessorHash) || !validDigest(record.SynthesisHash) {
		return fmt.Errorf("sidecar identity differs")
	}
	if !selected {
		if record.Repair != nil || record.ElectricalHash != "" || record.PredecessorHash != record.SynthesisHash {
			return fmt.Errorf("unselected case contains successor evidence")
		}
		return nil
	}
	repairHash := ""
	if record.Repair == nil && record.PredecessorHash != record.SynthesisHash {
		return fmt.Errorf("missing repair for changed synthesis")
	}
	if record.Repair != nil {
		repair := *record.Repair
		repairHash, repair.Hash = repair.Hash, ""
		got, err := valueDigest(repair)
		if err != nil || got != repairHash || repair.Schema != "kicadai.electrical-repair.v22" || repair.RequirementHash != requirementHash || repair.Limits != ot.DefaultElectricalRepairLimitsV22() {
			return fmt.Errorf("repair identity, requirement, or frozen limits differ")
		}
		if repair.Selected == nil && record.PredecessorHash != record.SynthesisHash {
			return fmt.Errorf("nonpassing repair changed predecessor synthesis")
		}
	}
	got, err := valueDigest(struct{ Schema, PredecessorHash, SynthesisHash, RepairHash string }{
		"kicadai.electrical-synthesis.v22", record.PredecessorHash, record.SynthesisHash, repairHash,
	})
	if err != nil || got != record.ElectricalHash {
		return fmt.Errorf("electrical wrapper identity differs")
	}
	return nil
}

func auditCases(t *testing.T, evidenceRoot string, records []capabilitybaselinev10.CaseEvidence) {
	t.Helper()
	var predecessor capabilitybaselinev10.Report
	readJSON(t, repository+"/internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json", &predecessor)
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
	data, err := os.ReadFile("../V22_PUBLIC_POPULATION.json")
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
				if digest(rootBytes) != record.ReplayRootSHA256[replay-1] || marker.Schema != "kicadai.closed-loop-open-set-clean-root.v22" || marker.Version != 22 || marker.CaseID != record.Case.ID || marker.Replay != replay || marker.CorpusManifestSHA256 != predecessor.CorpusManifestSHA256 || marker.RequirementSHA256 != record.RequirementSHA256 || marker.EnvironmentSHA256 != record.EnvironmentSHA256 || marker.EvaluatorManifestSHA256 != record.EvaluatorManifestSHA256 {
					t.Fatal("clean-root commitment differs")
				}
				var electrical sidecar
				payload := readJSON(t, filepath.Join(root, "ELECTRICAL_REPAIR.json"), &electrical)
				if err := authenticateSidecar(electrical, selected[record.Case.ID], requirementHash); err != nil {
					t.Fatal(err)
				}
				if replay == 2 && !bytes.Equal(priorSidecar, payload) {
					t.Fatal("electrical sidecar differs across replay roots")
				}
				priorSidecar = payload
				var metrics replayMetrics
				readJSON(t, filepath.Join(root, "METRICS.json"), &metrics)
				if metrics.Schema != "kicadai.public-replay-metrics.v22" || metrics.SynthesisNanoseconds <= 0 || metrics.HashNanoseconds <= 0 || metrics.PromotionNanoseconds < 0 || metrics.CanonicalSynthesisBytes <= 0 || metrics.SynthesisSpoolBytes != 0 {
					t.Fatal("invalid replay measurement")
				}
				canonicalBytes += metrics.CanonicalSynthesisBytes
				if record.Case.Outcome == "pass" {
					var promotion ot.PhysicalPromotionResult
					readJSON(t, filepath.Join(root, "PROMOTION.json"), &promotion)
					got, err := valueDigest(promotionIdentity(promotion))
					if err != nil || got != promotion.Hash || promotion.Hash != record.Promotions[replay-1].RunSHA256 || promotion.ProjectHash != record.Promotions[replay-1].ProjectSHA256 || promotion.SynthesisHash != electrical.SynthesisHash || !promotion.ReplayIdentical || promotion.Status != ot.PhysicalPromotionPassed || metrics.PromotionNanoseconds <= 0 {
						t.Fatal("passing promotion evidence differs")
					}
				}
			}
		})
	}
	if !t.Failed() {
		t.Logf("authenticated_cases=%d replays=%d canonical_synthesis_bytes=%d synthesis_spool_bytes=0", len(records), 2*len(records), canonicalBytes)
	}
}

func TestPublishedV22ReplayEvidence(t *testing.T) {
	if _, err := os.Stat(filepath.Join(publication, "report.json")); os.IsNotExist(err) {
		t.Skip("V22 public report not published yet")
	}
	var report capabilitybaselinev10.Report
	readJSON(t, filepath.Join(publication, "report.json"), &report)
	if err := capabilitybaselinev10.Validate(report); err != nil || report.CaseCount != 24 {
		t.Fatalf("invalid complete report: %v", err)
	}
	auditCases(t, filepath.Join(publication, "replays"), report.Cases)
}

func TestCompletedV22ScratchEvidence(t *testing.T) {
	root := os.Getenv("KICADAI_V22_AUDIT_SCRATCH")
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
		repair := ot.ElectricalRepairResultV22{Schema: "kicadai.electrical-repair.v22", RequirementHash: requirement, Status: ot.RepairSearchUnsupported, StopReason: "critical_failure", Limits: ot.DefaultElectricalRepairLimitsV22(), Trials: []ot.ElectricalRepairTrialV22{}}
		repair.Hash = mustDigest(t, repair)
		record := sidecar{Schema: "kicadai.public-electrical-replay.v22", Selected: true, PredecessorHash: strings.Repeat("b", 64), SynthesisHash: strings.Repeat("b", 64), Repair: &repair}
		record.ElectricalHash = mustDigest(t, struct{ Schema, PredecessorHash, SynthesisHash, RepairHash string }{"kicadai.electrical-synthesis.v22", record.PredecessorHash, record.SynthesisHash, repair.Hash})
		return record
	}
	if err := authenticateSidecar(makeRecord(t), true, requirement); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*testing.T, *sidecar){
		"wrapper":       func(_ *testing.T, s *sidecar) { s.ElectricalHash = strings.Repeat("0", 64) },
		"requirement":   func(_ *testing.T, s *sidecar) { s.Repair.RequirementHash = strings.Repeat("c", 64) },
		"repair":        func(_ *testing.T, s *sidecar) { s.Repair.BindingWork++ },
		"selection":     func(_ *testing.T, s *sidecar) { s.Selected = false },
		"missing":       func(_ *testing.T, s *sidecar) { s.Repair = nil },
		"synthesis":     func(_ *testing.T, s *sidecar) { s.SynthesisHash = strings.Repeat("d", 64) },
		"digest_format": func(_ *testing.T, s *sidecar) { s.PredecessorHash = "invalid" },
		"rehashed_limits": func(t *testing.T, s *sidecar) {
			s.Repair.Limits.MaxDepth++
			s.Repair.Hash = ""
			s.Repair.Hash = mustDigest(t, *s.Repair)
			s.ElectricalHash = mustDigest(t, struct{ Schema, PredecessorHash, SynthesisHash, RepairHash string }{"kicadai.electrical-synthesis.v22", s.PredecessorHash, s.SynthesisHash, s.Repair.Hash})
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := makeRecord(t)
			mutate(t, &record)
			if err := authenticateSidecar(record, true, requirement); err == nil {
				t.Fatal("tampered sidecar accepted")
			}
		})
	}
}
