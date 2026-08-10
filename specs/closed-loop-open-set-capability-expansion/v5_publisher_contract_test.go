package closedloopopensetcontract

import (
	"path/filepath"
	"testing"
)

func TestVersionFiveCorpusPublisherIsFrozen(t *testing.T) {
	directory := v5ContractDirectory(t)
	repositoryRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	want := map[string]bool{
		"specs/closed-loop-open-set-capability-expansion/V5_QUARANTINE_PROTOCOL.md":     true,
		"specs/closed-loop-open-set-capability-expansion/V5_VALIDATOR.sha256":           true,
		"specs/closed-loop-open-set-capability-expansion/v5_publisher_contract_test.go": true,
		"go.mod":                             true,
		"go.sum":                             true,
		"cmd/kicadai-corpus-publish/main.go": true,
		"cmd/kicadai-corpus-publish/main_test.go":      true,
		"internal/corpuspublication/audit.go":          true,
		"internal/corpuspublication/checksum.go":       true,
		"internal/corpuspublication/checksum_test.go":  true,
		"internal/corpuspublication/filesystem.go":     true,
		"internal/corpuspublication/model.go":          true,
		"internal/corpuspublication/publish.go":        true,
		"internal/corpuspublication/publish_test.go":   true,
		"internal/corpuspublication/rename_darwin.go":  true,
		"internal/corpuspublication/rename_linux.go":   true,
		"internal/corpuspublication/rename_other.go":   true,
		"internal/corpuspublication/rename_windows.go": true,
		"internal/corpuspublication/seal.go":           true,
		"internal/corpuspublication/sync_unix.go":      true,
		"internal/corpuspublication/sync_windows.go":   true,
	}
	got := map[string]bool{}
	manifestPath := filepath.Join(directory, "V5_PUBLISHER.sha256")
	for _, name := range v5PacketManifestNamesAt(t, manifestPath, repositoryRoot) {
		if !want[name] || got[name] {
			t.Fatalf("V5 publisher manifest contains unexpected or duplicate entry %q", name)
		}
		got[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("V5 publisher manifest entries = %d, want %d", len(got), len(want))
	}
}
