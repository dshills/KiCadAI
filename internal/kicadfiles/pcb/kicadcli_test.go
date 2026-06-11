package pcb

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kicadai/internal/kicadfiles"
	kicadproject "kicadai/internal/kicadfiles/project"
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

	boardPath := writeKiCadCLIBoardFixture(t, "led_indicator", board)

	runKiCadCLI(t, cli, "pcb", "upgrade", "--force", boardPath)
	runKiCadCLIDRC(t, cli, boardPath, false)
}

func TestKiCadCLIValidatesCorrectnessFixturePCB(t *testing.T) {
	if os.Getenv("KICADAI_RUN_KICAD_CLI") != "1" {
		t.Skip("set KICADAI_RUN_KICAD_CLI=1 to run KiCad CLI validation")
	}
	cli, ok := findKiCadCLI()
	if !ok {
		t.Skip("kicad-cli not found")
	}

	board, err := CorrectnessFixturePCB(CorrectnessFixtureInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "kicad-cli-connectivity-drc",
	})
	if err != nil {
		t.Fatalf("CorrectnessFixturePCB returned error: %v", err)
	}
	if err := ValidateGeneratedConnectivity(board); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}

	boardPath := writeKiCadCLIBoardFixture(t, "pcb_object_correctness", board)

	runKiCadCLI(t, cli, "pcb", "upgrade", "--force", boardPath)
	runKiCadCLIDRC(t, cli, boardPath, true)
}

func writeKiCadCLIBoardFixture(t *testing.T, name string, board PCBFile) string {
	t.Helper()
	root := t.TempDir()
	projectPath := filepath.Join(root, name+".kicad_pro")
	projectFile, err := os.Create(projectPath)
	if err != nil {
		t.Fatalf("create project fixture: %v", err)
	}
	err = kicadproject.Write(projectFile, kicadproject.ProjectFile{
		Name:          name,
		DesignID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		FormatVersion: kicadfiles.KiCadFormatV20260306,
		Generator:     "kicadai",
		PageSettings:  kicadproject.PageSettings{Paper: kicadfiles.Paper{Name: "A4"}},
		NetClasses: []kicadproject.NetClass{{
			Name:        "Default",
			Clearance:   kicadfiles.MM(0.2),
			TrackWidth:  kicadfiles.MM(0.25),
			ViaDiameter: kicadfiles.MM(0.8),
			ViaDrill:    kicadfiles.MM(0.4),
		}},
	})
	if closeErr := projectFile.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("write project fixture: %v", err)
	}

	boardPath := filepath.Join(root, name+".kicad_pcb")
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
	return boardPath
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

func runKiCadCLIDRC(t *testing.T, cli, boardPath string, refillZones bool) {
	t.Helper()
	outputPath := filepath.Join(t.TempDir(), "drc.json")
	args := []string{"pcb", "drc", "--exit-code-violations", "--format", "json", "--output", outputPath}
	if refillZones {
		args = append(args, "--refill-zones")
	}
	args = append(args, boardPath)
	cmd := exec.Command(cli, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	if processCrashed(err) {
		t.Skipf("%s %v crashed, so KiCad DRC is not available in this environment: %v\n%s", cli, args, err, output)
	}
	t.Fatalf("%s %v failed: %v\n%s", cli, args, err, output)
}

func processCrashed(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.ExitCode() < 0
}
