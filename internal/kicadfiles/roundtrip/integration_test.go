package roundtrip

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
)

func TestKiCadRoundTripCheckedInGeneratedPCB(t *testing.T) {
	cli := requireKiCadCLI(t)
	path := repoPath(t, "examples", "07_generated_pcb", "generated_pcb.kicad_pcb")
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	result, err := RoundTripPCB(ctx, cli, path, OptionsFromEnv())
	if err != nil {
		t.Fatalf("RoundTripPCB returned error: %v\nresult=%#v", err, result)
	}
	if !result.Equal {
		t.Fatalf("round trip changed fixture: %s", firstResultDifference(result))
	}
}

func TestKiCadRoundTripGeneratedLEDPCB(t *testing.T) {
	cli := requireKiCadCLI(t)
	board, err := pcb.LEDIndicatorPCB(pcb.LEDIndicatorInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "roundtrip-integration",
	})
	if err != nil {
		t.Fatalf("LEDIndicatorPCB returned error: %v", err)
	}
	var out bytes.Buffer
	if err := pcb.Write(&out, board); err != nil {
		t.Fatalf("pcb.Write returned error: %v", err)
	}
	boardPath := filepath.Join(t.TempDir(), "generated.kicad_pcb")
	writeIntegrationTestFile(t, boardPath, out.String())
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	result, err := RoundTripPCB(ctx, cli, boardPath, OptionsFromEnv())
	if err != nil {
		t.Fatalf("RoundTripPCB returned error: %v\nresult=%#v", err, result)
	}
	if !result.Equal {
		t.Fatalf("round trip changed generated board: %s", firstResultDifference(result))
	}
}

func TestKiCadRoundTripCheckedInGeneratedSchematic(t *testing.T) {
	cli := requireKiCadCLI(t)
	path := repoPath(t, "examples", "01_led_indicator", "led_indicator.kicad_sch")
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	result, err := RoundTripSchematic(ctx, cli, path, OptionsFromEnv())
	if err != nil {
		t.Fatalf("RoundTripSchematic returned error: %v\nresult=%#v", err, result)
	}
	if !result.Equal {
		t.Fatalf("round trip changed fixture: %s", firstResultDifference(result))
	}
}

func TestKiCadRoundTripGeneratedLEDSchematic(t *testing.T) {
	cli := requireKiCadCLI(t)
	sch, err := schematic.LEDIndicatorSchematic(schematic.LEDIndicatorInput{
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "roundtrip-integration",
	})
	if err != nil {
		t.Fatalf("LEDIndicatorSchematic returned error: %v", err)
	}
	var out bytes.Buffer
	if err := schematic.Write(&out, sch); err != nil {
		t.Fatalf("schematic.Write returned error: %v", err)
	}
	schematicPath := filepath.Join(t.TempDir(), "generated.kicad_sch")
	writeIntegrationTestFile(t, schematicPath, out.String())
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	result, err := RoundTripSchematic(ctx, cli, schematicPath, OptionsFromEnv())
	if err != nil {
		t.Fatalf("RoundTripSchematic returned error: %v\nresult=%#v", err, result)
	}
	if !result.Equal {
		t.Fatalf("round trip changed generated schematic: %s", firstResultDifference(result))
	}
}

func TestKiCadRoundTripRejectsInvalidPCB(t *testing.T) {
	cli := requireKiCadCLI(t)
	boardPath := filepath.Join(t.TempDir(), "invalid.kicad_pcb")
	writeIntegrationTestFile(t, boardPath, `(kicad_pcb
  (version 20260206)
  (generator "pcbnew")
  (generator_version "10.0")
  (layers
    (1 "F.Mask" user)
  )
)
`)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := RoundTripPCB(ctx, cli, boardPath, OptionsFromEnv())
	if err == nil {
		t.Fatal("expected invalid PCB to fail")
	}
	if !strings.Contains(err.Error(), "upgrade failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func requireKiCadCLI(t *testing.T) KiCadCLI {
	t.Helper()
	if !EnabledFromEnv() {
		t.Skip("set KICADAI_RUN_KICAD_CLI=1 to run KiCad round-trip integration tests")
	}
	cli, err := DiscoverCLI()
	if err != nil {
		t.Fatalf("discover KiCad CLI: %v", err)
	}
	return cli
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return filepath.Join(append([]string{root}, parts...)...)
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("could not find repository root")
		}
		root = next
	}
}

func firstResultDifference(result Result) string {
	if len(result.Differences) == 0 {
		return "no structured difference"
	}
	return strings.TrimSpace(result.Differences[0].Message)
}

func writeIntegrationTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write integration test file: %v", err)
	}
}
