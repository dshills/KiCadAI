package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles/checks"
)

func TestRunKiCadChecksSkipsWhenNotRequired(t *testing.T) {
	result := RunKiCadChecks(context.Background(), &Request{}, &ProjectWriteResult{}, KiCadCheckOptions{})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestRunKiCadChecksBlocksWhenRequiredCLIMissing(t *testing.T) {
	request := Request{Validation: ValidationSpec{RequireDRC: true}}
	write := ProjectWriteResult{}

	result := RunKiCadChecks(context.Background(), &request, &write, KiCadCheckOptions{KiCadCLI: filepath.Join(t.TempDir(), "missing-kicad-cli")})
	if result.Stage.Status != StageStatusBlocked {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestRunKiCadChecksWithFakeCLIPasses(t *testing.T) {
	request, write := writeValidationFixture(t)
	request.Validation.RequireERC = true
	request.Validation.RequireDRC = true
	cli := fakeKiCadCheckCLI(t)

	result := RunKiCadChecks(context.Background(), &request, &write, KiCadCheckOptions{KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: t.TempDir()})
	if result.Stage.Status != StageStatusOK {
		t.Fatalf("stage = %#v", result.Stage)
	}
	if result.ERC.Status == "" || result.DRC.Status == "" {
		t.Fatalf("checks did not run: %#v", result)
	}
	if _, ok := result.Stage.Summary[promotionKiCadERCSummaryKey]; !ok {
		t.Fatalf("ERC result missing from stage summary: %#v", result.Stage.Summary)
	}
	if _, ok := result.Stage.Summary[promotionKiCadDRCSummaryKey]; !ok {
		t.Fatalf("DRC result missing from stage summary: %#v", result.Stage.Summary)
	}
	if len(result.Stage.Artifacts) != 2 {
		t.Fatalf("artifacts = %#v", result.Stage.Artifacts)
	}
	if result.ERC.ProjectContext != "full" || result.DRC.ProjectContext != "full" {
		t.Fatalf("checks did not use project context: ERC=%q DRC=%q", result.ERC.ProjectContext, result.DRC.ProjectContext)
	}
}

func TestKiCadCheckTargetPrefersProjectRoot(t *testing.T) {
	write := ProjectWriteResult{Inspection: inspect.ProjectSummary{
		Root:      "/tmp/demo",
		Schematic: &inspect.SchematicSummary{Path: "/tmp/demo/demo.kicad_sch"},
		PCB:       &inspect.PCBSummary{Path: "/tmp/demo/demo.kicad_pcb"},
	}}

	if got := kicadCheckTargetFromWrite(&write, checks.CheckKindERC); got != "/tmp/demo" {
		t.Fatalf("ERC target = %q, want project root", got)
	}
	if got := kicadCheckTargetFromWrite(&write, checks.CheckKindDRC); got != "/tmp/demo" {
		t.Fatalf("DRC target = %q, want project root", got)
	}
}

func fakeKiCadCheckCLI(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fake kicad-cli is not portable to Windows")
	}
	path := filepath.Join(t.TempDir(), "kicad-cli")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "10.0.0"
  exit 0
fi
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    shift
    out="$1"
  fi
  shift
done
if [ -n "$out" ]; then
  printf '{"violations":[],"sheets":[],"coordinate_units":"mm"}' > "$out"
fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
