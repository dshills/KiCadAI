package fabrication

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRealKiCadCLIFabricationPlotSmoke(t *testing.T) {
	cliPath := os.Getenv("KICADAI_REAL_KICAD_CLI")
	if cliPath == "" {
		t.Skip("set KICADAI_REAL_KICAD_CLI to run real KiCad fabrication plotting smoke test")
	}
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result := ExportPackage(ctx, root, Options{
		Execute:   true,
		Overwrite: true,
		KiCadCLI:  cliPath,
		CLIPolicy: CLIPolicyRequired,
	})
	if result.Summary.Gerber != EvidencePass {
		t.Fatalf("gerber evidence = %s, issues = %#v", result.Summary.Gerber, result.Issues)
	}
	if result.Summary.Drill != EvidencePass {
		t.Fatalf("drill evidence = %s, issues = %#v", result.Summary.Drill, result.Issues)
	}
	for _, rel := range []string{"gerbers", "drill"} {
		entries, err := os.ReadDir(filepath.Join(root, "fabrication", rel))
		if err != nil {
			t.Fatalf("%s output missing: %v", rel, err)
		}
		if len(entries) == 0 {
			t.Fatalf("%s output is empty", rel)
		}
	}
}
