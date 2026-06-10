package pcb

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestKiCadCLIValidatesGeneratedLEDPCB(t *testing.T) {
	if os.Getenv("KICADAI_RUN_KICAD_CLI") != "1" {
		t.Skip("set KICADAI_RUN_KICAD_CLI=1 to run KiCad CLI validation")
	}
	cli, ok := findKiCadCLI()
	if !ok {
		t.Skip("kicad-cli not found")
	}

	board, err := LEDIndicatorPCB(LEDIndicatorInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "kicad-cli-validation",
	})
	if err != nil {
		t.Fatalf("LEDIndicatorPCB returned error: %v", err)
	}

	boardPath := filepath.Join(t.TempDir(), "led_indicator.kicad_pcb")
	file, err := os.Create(boardPath)
	if err != nil {
		t.Fatalf("create board fixture: %v", err)
	}
	if err := Write(file, board); err != nil {
		_ = file.Close()
		t.Fatalf("Write returned error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close board fixture: %v", err)
	}

	runKiCadCLI(t, cli, "pcb", "upgrade", "--force", boardPath)
	runKiCadCLI(t, cli, "pcb", "drc", "--exit-code-violations", boardPath)
}

func findKiCadCLI() (string, bool) {
	if path, err := exec.LookPath("kicad-cli"); err == nil {
		return path, true
	}
	candidates := []string{
		"/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli",
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func runKiCadCLI(t *testing.T, cli string, args ...string) {
	t.Helper()
	cmd := exec.Command(cli, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", cli, args, err, output)
	}
}
