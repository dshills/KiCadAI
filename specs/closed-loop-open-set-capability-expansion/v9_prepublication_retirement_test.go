package closedloopopensetcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"testing"
)

type v9RetirementAssignment struct {
	Role         string `json:"role"`
	CircuitRole  string `json:"circuit_role"`
	SafetyImpact string `json:"safety_impact"`
}

func TestVersionNineRetiredOnFrozenAssignmentValidatorInconsistency(t *testing.T) {
	directory := v9BaselinePublisherContractDirectory(t)
	data := v9BaselinePublisherReadFile(t, filepath.Join(directory, "V9_PREPUBLICATION_RETIREMENT.json"))
	var retirement struct {
		Schema                                 string   `json:"schema"`
		Version                                int      `json:"version"`
		Stage                                  string   `json:"stage"`
		InfrastructureCommit                   string   `json:"infrastructure_commit"`
		ContractManifestSHA256                 string   `json:"contract_manifest_sha256"`
		AuthorPacketManifestSHA256             string   `json:"author_packet_manifest_sha256"`
		PacketSetSHA256                        string   `json:"packet_set_sha256"`
		HistoricalCommitmentsSHA256            string   `json:"historical_commitments_sha256"`
		ValidatorCommit                        string   `json:"validator_commit"`
		Reason                                 string   `json:"reason"`
		ValidatorError                         string   `json:"validator_error"`
		DiscoveryMissingHighSafetyCircuitRoles []string `json:"discovery_missing_high_safety_circuit_roles"`
		HeldOutMissingHighSafetyCircuitRoles   []string `json:"held_out_missing_high_safety_circuit_roles"`
		CorpusPublished                        bool     `json:"corpus_published"`
		BaselineStarted                        bool     `json:"baseline_started"`
		HeldOutSourceKeyCreated                bool     `json:"held_out_source_key_created"`
		HeldOutSourceOpened                    bool     `json:"held_out_source_opened"`
		HeldOutBaselineKeyCreated              bool     `json:"held_out_baseline_key_created"`
		HeldOutBaselineOpened                  bool     `json:"held_out_baseline_opened"`
		AuthorQuarantinesRetained              bool     `json:"author_quarantines_retained"`
		TerminalState                          string   `json:"terminal_state"`
		Hash                                   string   `json:"hash"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&retirement); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("V9 retirement contains trailing JSON")
	}
	if retirement.Schema != "kicadai.closed-loop-open-set-prepublication-retirement.v9" || retirement.Version != 9 ||
		retirement.Stage != "aggregate_validation" || retirement.Reason != "frozen_assignment_validator_inconsistent" ||
		retirement.ValidatorError != "V9_CIRCUIT_ROLE_BALANCE" || retirement.TerminalState != "permanently_retired" {
		t.Fatalf("invalid V9 retirement state: %+v", retirement)
	}
	if retirement.InfrastructureCommit != "d4fac116d1da7f9e345d615214ab1b7c09c27b53" ||
		retirement.ValidatorCommit != "a4c93d5f1d2c7c54093b4275824f7dec7a6e4493" ||
		retirement.ContractManifestSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, "V9_CONTRACT.sha256")) ||
		retirement.AuthorPacketManifestSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, "V9_AUTHOR_PACKET.sha256")) ||
		retirement.PacketSetSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, "v9-authoring-packet", "PACKET_SET.sha256")) ||
		retirement.HistoricalCommitmentsSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, "V9_HISTORICAL_COMMITMENTS.json")) {
		t.Fatal("V9 retirement bindings do not reproduce")
	}
	if retirement.CorpusPublished || retirement.BaselineStarted || retirement.HeldOutSourceKeyCreated || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineKeyCreated || retirement.HeldOutBaselineOpened || !retirement.AuthorQuarantinesRetained {
		t.Fatal("V9 retirement crossed its prepublication boundary")
	}
	discoveryMissing, heldOutMissing := v9MissingHighSafetyCircuitRoles(t, directory)
	if !slices.Equal(retirement.DiscoveryMissingHighSafetyCircuitRoles, discoveryMissing) || !slices.Equal(retirement.HeldOutMissingHighSafetyCircuitRoles, heldOutMissing) {
		t.Fatalf("V9 retirement assignment evidence does not reproduce: discovery=%v held_out=%v", discoveryMissing, heldOutMissing)
	}
	var canonical map[string]any
	if err := json.Unmarshal(data, &canonical); err != nil {
		t.Fatal(err)
	}
	delete(canonical, "hash")
	canonicalBytes, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonicalBytes)
	if got := hex.EncodeToString(digest[:]); got != retirement.Hash {
		t.Fatalf("V9 retirement hash = %s, want %s", retirement.Hash, got)
	}
}

func v9MissingHighSafetyCircuitRoles(t *testing.T, directory string) ([]string, []string) {
	t.Helper()
	wantRoles := []string{"amplification_conditioning", "conversion_regulation", "interface_control", "protection_supervision", "sensing_measurement", "source_bias"}
	seen := map[string]map[string]bool{"discovery": {}, "held_out": {}}
	for author := 1; author <= 6; author++ {
		var assignment struct {
			Entries []v9RetirementAssignment `json:"entries"`
		}
		path := filepath.Join(directory, "v9-authoring-packet", "assignments", fmt.Sprintf("author_%d.json", author))
		if err := json.Unmarshal(v9BaselinePublisherReadFile(t, path), &assignment); err != nil {
			t.Fatal(err)
		}
		for _, entry := range assignment.Entries {
			roleEvidence, exists := seen[entry.Role]
			if !exists {
				t.Fatalf("V9 assignment contains unknown role %q", entry.Role)
			}
			if entry.SafetyImpact == "safety_relevant" || entry.SafetyImpact == "safety_critical" {
				roleEvidence[entry.CircuitRole] = true
			}
		}
	}
	missing := func(role string) []string {
		var result []string
		for _, circuitRole := range wantRoles {
			if !seen[role][circuitRole] {
				result = append(result, circuitRole)
			}
		}
		return result
	}
	return missing("discovery"), missing("held_out")
}
