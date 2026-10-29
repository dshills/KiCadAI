package capabilityexplorer

import (
	"bytes"
	"kicadai/internal/generationcapability"
	"os"
	"path/filepath"
	"testing"
)

func TestExplorerRegistryCoverageAndGeneratedDocs(t *testing.T) {
	inventory := Build()
	seen := map[string]bool{}
	for _, entry := range inventory.Entries {
		if seen[entry.ID] || len(entry.Limits) == 0 || len(entry.Tools) == 0 || len(entry.Commands) == 0 || entry.EvidenceLevel == "" || len(entry.Limitations) == 0 {
			t.Fatalf("incomplete entry %+v", entry)
		}
		seen[entry.ID] = true
	}
	for _, profile := range generationcapability.All() {
		if !seen["generation/"+profile.ProfileID] {
			t.Fatal("profile missing", profile.ProfileID)
		}
	}
	for _, id := range []string{"divider", "rc-filter", "numerical-conformance", "reproduction", "guided-clarification"} {
		if _, ok := Lookup(id); !ok {
			t.Fatal("missing", id)
		}
	}
	root := filepath.Join("..", "..", "docs", "capability-explorer.md")
	if raw, err := os.ReadFile(root); err != nil || !bytes.Equal(raw, Markdown()) {
		t.Fatalf("generated capability docs drift: %v; regenerate through capability docs", err)
	}
}
