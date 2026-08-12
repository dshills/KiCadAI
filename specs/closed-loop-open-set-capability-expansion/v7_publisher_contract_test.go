package closedloopopensetcontract

import (
	"path/filepath"
	"testing"
)

func TestVersionSevenCorpusPublisherIsFrozen(t *testing.T) {
	directory := v7ContractDirectory(t)
	repositoryRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	want := map[string]bool{
		"specs/closed-loop-open-set-capability-expansion/V7_PUBLICATION_PROTOCOL.md":    true,
		"specs/closed-loop-open-set-capability-expansion/V7_CONTRACT.sha256":            true,
		"specs/closed-loop-open-set-capability-expansion/V7_AUTHOR_PACKET.sha256":       true,
		"specs/closed-loop-open-set-capability-expansion/V7_VALIDATOR.sha256":           true,
		"specs/closed-loop-open-set-capability-expansion/V7_VALIDATOR_CONTRACT.sha256":  true,
		"specs/closed-loop-open-set-capability-expansion/V7_VALIDATOR_REFREEZE.md":      true,
		"specs/closed-loop-open-set-capability-expansion/v7_publisher_contract_test.go": true,
		"go.mod":                                true,
		"go.sum":                                true,
		"cmd/kicadai-corpus-publish-v7/main.go": true,
		"cmd/kicadai-corpus-publish-v7/main_test.go":     true,
		"internal/corpuspublication/audit.go":            true,
		"internal/corpuspublication/checksum.go":         true,
		"internal/corpuspublication/checksum_test.go":    true,
		"internal/corpuspublication/filesystem.go":       true,
		"internal/corpuspublication/model.go":            true,
		"internal/corpuspublication/publish.go":          true,
		"internal/corpuspublication/publish_test.go":     true,
		"internal/corpuspublication/rename_darwin.go":    true,
		"internal/corpuspublication/rename_linux.go":     true,
		"internal/corpuspublication/rename_other.go":     true,
		"internal/corpuspublication/rename_windows.go":   true,
		"internal/corpuspublication/seal.go":             true,
		"internal/corpuspublication/sync_unix.go":        true,
		"internal/corpuspublication/sync_windows.go":     true,
		"internal/corpuspublication/v6.go":               true,
		"internal/corpuspublication/v6_checksum.go":      true,
		"internal/corpuspublication/v6_checksum_test.go": true,
		"internal/corpuspublication/v7.go":               true,
		"internal/corpuspublication/v7_audit.go":         true,
		"internal/corpuspublication/v7_test.go":          true,
	}
	got := map[string]bool{}
	manifestPath := filepath.Join(directory, "V7_PUBLISHER.sha256")
	for _, name := range v5PacketManifestNamesAt(t, manifestPath, repositoryRoot) {
		if !want[name] || got[name] {
			t.Fatalf("V7 publisher manifest contains unexpected or duplicate entry %q", name)
		}
		got[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("V7 publisher manifest entries = %d, want %d", len(got), len(want))
	}
}

func TestVersionSevenPublisherRootManifest(t *testing.T) {
	v7VerifyPacketChecksumManifest(t, v7ContractDirectory(t), "V7_PUBLISHER_CONTRACT.sha256", []string{
		"V7_PUBLISHER.sha256",
		"v7_publisher_contract_test.go",
	})
}
