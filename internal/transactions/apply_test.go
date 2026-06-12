package transactions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyBuildsSimpleProject(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1","x_mm":-2.54},{"number":"2","x_mm":2.54}]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`)
	result := Apply(tx, ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	if len(result.Artifacts) != 3 {
		t.Fatalf("expected artifacts, got %#v", result.Artifacts)
	}
	for _, name := range []string{"demo.kicad_pro", "demo.kicad_sch", "demo.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func TestApplyRequiresCreateProject(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"write_project"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[0].op" {
		t.Fatalf("expected create_project issue: %#v", result.Issues)
	}
}

func TestApplyRejectsLateCreateProject(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"write_project"},{"op":"create_project","name":"demo"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[0].op" {
		t.Fatalf("expected first create_project issue: %#v", result.Issues)
	}
}

func TestApplyRejectsWriteProjectOutputOverride(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"create_project","name":"demo"},{"op":"write_project","output_dir":"elsewhere"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[1]" {
		t.Fatalf("expected output override issue: %#v", result.Issues)
	}
}

func TestDeterministicDesignUUIDDependsOnSeed(t *testing.T) {
	a := deterministicDesignUUID("demo", "a")
	b := deterministicDesignUUID("demo", "b")
	if a == b || a == "" || b == "" {
		t.Fatalf("unexpected deterministic UUIDs: %q %q", a, b)
	}
}

func TestApplyStopsOnOperationFailure(t *testing.T) {
	tx := mustParse(t, `{"operations":[{"op":"create_project","name":"demo"},{"op":"assign_footprint","ref":"R1","footprint_id":"Device:R"}]}`)
	result := Apply(tx, ApplyOptions{OutputDir: t.TempDir()})
	if len(result.Issues) == 0 || result.Issues[0].Path != "operations[1]" {
		t.Fatalf("expected operation index issue: %#v", result.Issues)
	}
}
