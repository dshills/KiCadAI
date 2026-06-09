//go:build integration

package design

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/kicadfiles"
)

func TestGeneratedLEDProjectKiCadCLIAvailable(t *testing.T) {
	if os.Getenv("KICAD_VALIDATE_GENERATED_FILES") != "1" {
		t.Skip("set KICAD_VALIDATE_GENERATED_FILES=1 to validate generated files with KiCad")
	}
	kicadCLI := os.Getenv("KICAD_CLI")
	if kicadCLI == "" {
		t.Fatal("KICAD_VALIDATE_GENERATED_FILES=1 requires KICAD_CLI to be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	name := "led_indicator"
	root := filepath.Join(t.TempDir(), name)
	design, err := LEDIndicatorDesign(LEDIndicatorInput{
		Name:       name,
		DesignID:   kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:       "integration",
		IncludePCB: true,
	})
	if err != nil {
		t.Fatalf("LEDIndicatorDesign returned error: %v", err)
	}
	if _, err := WriteProjectDirectory(root, design, WriteOptions{}); err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	projectPath := filepath.Join(root, name+".kicad_pro")
	schematicPath := filepath.Join(root, name+".kicad_sch")
	pcbPath := filepath.Join(root, name+".kicad_pcb")
	for _, path := range []string{projectPath, schematicPath, pcbPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated file missing before KiCad CLI smoke check: %s: %v", path, err)
		}
	}

	if output, err := exec.CommandContext(ctx, kicadCLI, "version").CombinedOutput(); err != nil {
		t.Fatalf("%s version failed: %v\n%s", kicadCLI, err, output)
	}
	netlistPath := filepath.Join(root, name+".net")
	if output, err := exec.CommandContext(ctx, kicadCLI, "sch", "export", "netlist", "--output", netlistPath, schematicPath).CombinedOutput(); err != nil {
		t.Fatalf("%s schematic netlist export failed: %v\n%s", kicadCLI, err, output)
	}
	if stat, err := os.Stat(netlistPath); err != nil || stat.Size() == 0 {
		t.Fatalf("exported netlist is missing or empty: %v", err)
	}
	if output, err := exec.CommandContext(ctx, kicadCLI, "pcb", "drc", "--exit-code-violations", "--output", filepath.Join(root, name+".drc.rpt"), pcbPath).CombinedOutput(); err != nil {
		t.Fatalf("%s PCB DRC failed: %v\n%s", kicadCLI, err, output)
	}
}
