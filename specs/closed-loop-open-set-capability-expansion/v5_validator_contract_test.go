package closedloopopensetcontract

import (
	"path/filepath"
	"testing"
)

func TestVersionFiveOutcomeBlindValidatorIsFrozen(t *testing.T) {
	directory := v5ContractDirectory(t)
	repositoryRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	want := map[string]bool{
		"specs/closed-loop-open-set-capability-expansion/V5_QUARANTINE_PROTOCOL.md":             true,
		"specs/closed-loop-open-set-capability-expansion/v5-authoring-packet/PACKET_SET.sha256": true,
		"specs/closed-loop-open-set-capability-expansion/v5_authoring_packet_test.go":           true,
		"specs/closed-loop-open-set-capability-expansion/v5_validator_contract_test.go":         true,
		"internal/corpusfreeze/model.go":                                                        true,
		"internal/corpusfreeze/policy.go":                                                       true,
		"internal/corpusfreeze/decode.go":                                                       true,
		"internal/corpusfreeze/normalize.go":                                                    true,
		"internal/corpusfreeze/diversity.go":                                                    true,
		"internal/corpusfreeze/validate.go":                                                     true,
		"internal/corpusfreeze/validate_test.go":                                                true,
	}
	got := map[string]bool{}
	manifestPath := filepath.Join(directory, "V5_VALIDATOR.sha256")
	for _, name := range v5PacketManifestNamesAt(t, manifestPath, repositoryRoot) {
		if !want[name] || got[name] {
			t.Fatalf("V5 validator manifest contains unexpected or duplicate entry %q", name)
		}
		got[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("V5 validator manifest entries = %d, want %d", len(got), len(want))
	}
}
