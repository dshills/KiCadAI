package compositionlowering

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/schematiclayout"
)

// Produces only a diagnostic transaction, not a KiCad project. Every input
// except the separately versioned drawing profile comes from recorded evidence.
func TestFunctionalPowerLocalityRecordedLayoutDiagnostic(t *testing.T) {
	root, out := os.Getenv("KICADAI_LOCAL_WIRING_DIAGNOSTIC_INPUT"), os.Getenv("KICADAI_LOCAL_WIRING_DIAGNOSTIC_TRANSACTION")
	if root == "" || out == "" {
		t.Skip("requires explicit recorded input and exclusive diagnostic output")
	}
	read := func(path string, value any) {
		t.Helper()
		f, err := os.Open(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := json.NewDecoder(f).Decode(value); err != nil {
			t.Fatal(err)
		}
	}
	var request designworkflow.Request
	var index libraryresolver.LibraryIndex
	read("controller_adc_100ma/workflow_request.json", &request)
	read("library_index.json", &index)
	request.ExplicitCircuit.Schematic.Layout.FunctionalProfile = schematiclayout.FunctionalOwnershipV6
	layout := schematicir.LayoutDocumentWithLibraryIndex(request.ExplicitCircuit.Schematic, &index)
	layoutFile, err := os.OpenFile(out+".layout.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(layoutFile).Encode(layout); err != nil {
		layoutFile.Close()
		t.Fatal(err)
	}
	if err := layoutFile.Close(); err != nil {
		t.Fatal(err)
	}
	tx, issues := schematicir.ToTransactionWithLibraryIndex(request.ExplicitCircuit.Schematic, &index)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(tx); err != nil {
		t.Fatal(err)
	}
	t.Logf("diagnostic transaction only: %s; operations=%d; issues=%v", out, len(tx.Operations), issues)
}
