package closedloopopensetcontract

import (
	"path/filepath"
	"testing"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev6"
)

func TestVersionSixOutcomeBlindValidatorIsFrozen(t *testing.T) {
	directory := v6ContractDirectory(t)
	want := map[string]bool{
		"specs/closed-loop-open-set-capability-expansion/V6_QUARANTINE_PROTOCOL.md":             true,
		"specs/closed-loop-open-set-capability-expansion/V6_HISTORICAL_COMMITMENTS.json":        true,
		"specs/closed-loop-open-set-capability-expansion/v6-authoring-packet/PACKET_SET.sha256": true,
		"specs/closed-loop-open-set-capability-expansion/v6_author_packet_test.go":              true,
		"specs/closed-loop-open-set-capability-expansion/v6_historical_commitments_test.go":     true,
		"specs/closed-loop-open-set-capability-expansion/v6_validator_contract_test.go":         true,
		"cmd/kicadai-corpus-validate-v6/main.go":                                                true,
		"cmd/kicadai-corpus-validate-v6/main_test.go":                                           true,
		"internal/corpusfreezev6/history.go":                                                    true,
		"internal/corpusfreezev6/history_test.go":                                               true,
		"internal/corpusfreezev6/policy.go":                                                     true,
		"internal/corpusfreezev6/policy_test.go":                                                true,
		"internal/corpusfreezev6/validate.go":                                                   true,
		"internal/corpusfreezev6/validate_test.go":                                              true,
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
	manifestPath := filepath.Join(directory, "V6_VALIDATOR.sha256")
	for _, name := range historicalManifestNames(t, manifestPath) {
		if !want[name] || got[name] {
			t.Fatalf("V6 validator manifest contains unexpected or duplicate entry %q", name)
		}
		got[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("V6 validator manifest entries = %d, want %d", len(got), len(want))
	}
}

func TestVersionSixFrozenPacketAndHistoryLoadThroughCustodianBoundary(t *testing.T) {
	directory := v6ContractDirectory(t)
	policy := corpusfreezev6.Policy()
	packet, err := corpusfreeze.LoadPacket(filepath.Join(directory, "v6-authoring-packet"), policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Assignments) != 3 || packet.Binding.PacketSetSHA256 != policy.PacketSetSHA256 {
		t.Fatal("V6 packet did not preserve the exact three-author binding")
	}
	history, err := corpusfreezev6.LoadHistoricalCommitments(filepath.Join(directory, "V6_HISTORICAL_COMMITMENTS.json"))
	if err != nil {
		t.Fatal(err)
	}
	if history.Base.SourceSHA256 != policy.HistoricalCommitmentsSHA256 || len(history.Base.RawSHA256) != 132 ||
		len(history.Base.NeutralSemanticSHA256) != 60 || len(history.NormalizedSemanticSHA256) != 36 {
		t.Fatal("V6 historical commitment binding is incomplete")
	}
}
