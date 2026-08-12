package corpusfreezev8

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestFrozenPolicyAndPacket(t *testing.T) {
	policy := FrozenPolicy()
	if policy.Version != 8 || len(policy.AuthorSlots) != 6 || policy.CasesPerAuthor != 6 ||
		len(policy.Domains) != 6 || len(policy.CircuitRoles) != 6 || len(policy.SafetyImpacts) != 4 ||
		policy.PacketSetSHA256 != PacketSetSHA256 || policy.HistoricalCommitmentsSHA256 != HistoricalCommitmentsSHA256 {
		t.Fatalf("unexpected V8 policy: %+v", policy)
	}
	root := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "v8-authoring-packet")
	packet, err := LoadPacket(root, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Assignments) != 6 || len(packet.Binding.AuthorPacketSHA256) != 6 || len(packet.Binding.AssignmentSHA256) != 6 {
		t.Fatalf("unexpected packet shape: %+v", packet.Binding)
	}
}

func TestV8StrictShapes(t *testing.T) {
	assignment := []byte(`{"schema":"kicadai.closed-loop-open-set-author-assignment.v8","version":8,"author_slot":"author_1","entries":[{"id":"x","role":"discovery","domain":"analog_signal_path","circuit_role":"source_bias","safety_impact":"non_safety","source_id":"s","requirement_file":"discovery/request_001.json"}]}`)
	if _, err := decodeAssignment(assignment); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(assignment, []byte(`,"circuit_role":"source_bias"`), nil, 1),
		bytes.Replace(assignment, []byte(`"circuit_role":"source_bias"`), []byte(`"circuit_role":"source_bias","unknown":true`), 1),
	} {
		if _, err := decodeAssignment(invalid); err == nil {
			t.Fatal("invalid assignment shape was accepted")
		}
	}

	templatePath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "v8-authoring-packet", "AUTHORSHIP_TEMPLATE.json")
	template, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	var authorship Authorship
	if err := decodeStrict(template, &authorship); err != nil {
		t.Fatal(err)
	}
	if !authorship.Attestations.allTrue() {
		t.Fatal("frozen V8 template attestations are incomplete")
	}
	old := bytes.Replace(template, []byte(`"no_synthesis_simulation_classification_ranking_or_feasibility"`), []byte(`"no_synthesis_simulation_classification_or_feasibility"`), 1)
	if _, err := decodeAuthorship(old); err == nil {
		t.Fatal("legacy authorship attestation was accepted as V8")
	}
}

func TestV8HistoricalCommitmentsIncludePublishedV7WithoutOpeningHeldOut(t *testing.T) {
	root := repositoryRoot(t)
	historyPath := filepath.Join(root, "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if history.Base.SourceSHA256 != HistoricalCommitmentsSHA256 || len(history.Base.RawSHA256) != 204 ||
		len(history.Base.NeutralSemanticSHA256) != 132 || len(history.NormalizedSemanticSHA256) != 108 {
		t.Fatal("unexpected V8 historical commitments")
	}

	manifestPath := filepath.Join(root, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v7_corpus", "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			StablePath string `json:"stable_path"`
			Sealed     bool   `json:"sealed"`
			Neutral    string `json:"neutral_semantic_sha256"`
			Normalized string `json:"normalized_semantic_sha256"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range manifest.Entries {
		if entry.Sealed {
			continue
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(entry.StablePath)))
		if err != nil {
			t.Fatal(err)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s has %d issues", entry.StablePath, len(issues))
		}
		neutral, normalized, err := semanticHashes(requirement)
		if err != nil {
			t.Fatal(err)
		}
		if neutral != entry.Neutral || normalized != entry.Normalized {
			t.Fatalf("semantic hash mismatch for %s", entry.StablePath)
		}
		checked++
	}
	if checked != 18 {
		t.Fatalf("checked %d public V7 discovery cases", checked)
	}
}

func TestV8ReportFailsClosed(t *testing.T) {
	if _, err := (Report{}).MarshalJSONStable(); err == nil {
		t.Fatal("empty report was accepted")
	}
}
