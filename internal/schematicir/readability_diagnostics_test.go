package schematicir

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/libraryresolver"
)

// Opt-in diagnostic only: inspect a serialized request through the adapter.
// Runtime-only Inferred group flags are not present in JSON, so this is a
// serialized-request replay audit, not a claim about the in-memory native run.
// It never modifies the request or project and is not a visual pass gate.
func TestSerializedReadabilityRequestDiagnostics(t *testing.T) {
	root := os.Getenv("KICADAI_READABILITY_DIAGNOSTICS")
	if root == "" {
		t.Skip("requires retained native request root")
	}
	roots, _ := libraryresolver.ResolveRoots()
	index, _ := libraryresolver.Load(context.Background(), roots, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 {
		t.Fatal("native libraries unavailable")
	}
	for _, name := range []string{"standalone_regulator", "controller_adc_100ma"} {
		data, err := os.ReadFile(filepath.Join(root, name, "workflow_request.json"))
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			Explicit struct {
				Schematic Document `json:"schematic"`
			} `json:"explicit_circuit"`
		}
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		state, issues := newAdapterState(NormalizeLayoutIntent(request.Explicit.Schematic), &index)
		if state == nil {
			t.Fatal(issues)
		}
		data, err = json.MarshalIndent(state.layoutResult, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(root, name, "layout-analysis.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.Write(append(data, '\n'))
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("write=%v close=%v", writeErr, closeErr)
		}
	}
}
