package closedloopopensetcontract

import (
	"path/filepath"
	"testing"

	"kicadai/internal/corpusfreeze"
)

func TestVersionFiveOutcomeBlindValidatorIsFrozen(t *testing.T) {
	directory := v5ContractDirectory(t)
	want := map[string]bool{
		"specs/closed-loop-open-set-capability-expansion/V5_QUARANTINE_PROTOCOL.md":             true,
		"specs/closed-loop-open-set-capability-expansion/V5_HISTORICAL_COMMITMENTS.json":        true,
		"specs/closed-loop-open-set-capability-expansion/v5-authoring-packet/PACKET_SET.sha256": true,
		"specs/closed-loop-open-set-capability-expansion/v5_authoring_packet_test.go":           true,
		"specs/closed-loop-open-set-capability-expansion/v5_validator_contract_test.go":         true,
		"cmd/kicadai-corpus-validate/main.go":                                                   true,
		"cmd/kicadai-corpus-validate/main_test.go":                                              true,
		"internal/corpusfreeze/model.go":                                                        true,
		"internal/corpusfreeze/policy.go":                                                       true,
		"internal/corpusfreeze/decode.go":                                                       true,
		"internal/corpusfreeze/normalize.go":                                                    true,
		"internal/corpusfreeze/diversity.go":                                                    true,
		"internal/corpusfreeze/validate.go":                                                     true,
		"internal/corpusfreeze/validate_test.go":                                                true,
		"internal/corpusfreeze/load.go":                                                         true,
		"internal/corpusfreeze/load_test.go":                                                    true,
		"internal/corpusfreeze/secure_open_unix.go":                                             true,
		"internal/corpusfreeze/secure_open_windows.go":                                          true,
		"internal/corpusfreeze/history.go":                                                      true,
		"internal/corpusfreeze/history_test.go":                                                 true,
		"internal/corpusfreeze/output.go":                                                       true,
		"internal/corpusfreeze/output_test.go":                                                  true,
		"internal/opentopologysynthesis/model.go":                                               true,
		"internal/opentopologysynthesis/model_test.go":                                          true,
		"internal/opentopologysynthesis/decode.go":                                              true,
		"internal/opentopologysynthesis/normalize.go":                                           true,
		"internal/opentopologysynthesis/validate.go":                                            true,
		"internal/reports/issue.go":                                                             true,
		"internal/reports/diagnostics.go":                                                       true,
	}
	got := map[string]bool{}
	manifestPath := filepath.Join(directory, "V5_VALIDATOR.sha256")
	for _, name := range historicalManifestNames(t, manifestPath) {
		if !want[name] || got[name] {
			t.Fatalf("V5 validator manifest contains unexpected or duplicate entry %q", name)
		}
		got[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("V5 validator manifest entries = %d, want %d", len(got), len(want))
	}
}

func TestVersionFiveFrozenPacketLoadsThroughCustodianBoundary(t *testing.T) {
	directory := v5ContractDirectory(t)
	packet, err := corpusfreeze.LoadPacket(filepath.Join(directory, "v5-authoring-packet"), corpusfreeze.V5Policy())
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Assignments) != 3 || len(packet.Binding.AuthorPacketSHA256) != 3 || len(packet.Binding.AssignmentSHA256) != 3 {
		t.Fatalf("V5 packet binding sizes = %d/%d/%d, want 3 each", len(packet.Assignments), len(packet.Binding.AuthorPacketSHA256), len(packet.Binding.AssignmentSHA256))
	}
	if got, want := packet.Binding.PacketSetSHA256, corpusfreeze.V5Policy().PacketSetSHA256; got != want {
		t.Fatalf("V5 packet-set binding = %q, want %q", got, want)
	}
}

func TestVersionFiveHistoricalCommitmentsAreSanitizedAndComplete(t *testing.T) {
	directory := v5ContractDirectory(t)
	path := filepath.Join(directory, "V5_HISTORICAL_COMMITMENTS.json")
	commitments, err := corpusfreeze.LoadHistoricalCommitments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(commitments.RawSHA256) != 96 || len(commitments.NeutralSemanticSHA256) != 24 {
		t.Fatalf("V5 historical commitment counts = %d/%d, want 96/24", len(commitments.RawSHA256), len(commitments.NeutralSemanticSHA256))
	}
	if got, want := commitments.SourceSHA256, corpusfreeze.V5Policy().HistoricalCommitmentsSHA256; got != want || got != v5FileSHA256(t, path) {
		t.Fatalf("V5 historical commitment binding = %q, want %q", got, want)
	}
}
