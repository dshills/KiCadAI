package writercorrectness

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCheckKiCadRoundTripSkipsWhenOptionalAndMissingCLI(t *testing.T) {
	check := CheckKiCadRoundTripEvidence(context.Background(), Target{PCBPath: "demo.kicad_pcb"}, Options{})
	if check.Status != CheckSkipped {
		t.Fatalf("status = %q, want skipped", check.Status)
	}
	if len(check.Issues) != 1 {
		t.Fatalf("issues = %#v", check.Issues)
	}
}

func TestCheckKiCadRoundTripFailsWhenRequiredAndMissingCLI(t *testing.T) {
	t.Setenv("PATH", "")
	check := CheckKiCadRoundTripEvidence(context.Background(), Target{PCBPath: "demo.kicad_pcb"}, Options{RequireKiCadRoundTrip: true})
	if check.Status != CheckFail {
		t.Fatalf("status = %q, want fail", check.Status)
	}
	if len(check.Issues) != 1 {
		t.Fatalf("issues = %#v", check.Issues)
	}
}

func TestCheckKiCadRoundTripRejectsNonExecutableCLI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kicad-cli")
	writeFile(t, path, "#!/bin/sh\nexit 0\n")

	check := CheckKiCadRoundTripEvidence(context.Background(), Target{PCBPath: "demo.kicad_pcb"}, Options{KiCadCLI: path})
	if check.Status != CheckSkipped {
		t.Fatalf("status = %q, want skipped when optional", check.Status)
	}
	if len(check.Issues) != 1 {
		t.Fatalf("issues = %#v", check.Issues)
	}
}
