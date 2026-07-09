package schematicir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/evaluate"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

func TestSchematicIRWritesReadableProject(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		projectName string
	}{
		{name: "LED indicator", fileName: "led_indicator.json", projectName: "led_indicator"},
		{name: "USB-C LED indicator", fileName: "usb_c_led_indicator.json", projectName: "usb_c_led_indicator"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testSchematicIRWritesReadableProject(t, tc.fileName, tc.projectName)
		})
	}
}

func testSchematicIRWritesReadableProject(t *testing.T, fileName string, projectName string) {
	t.Helper()
	document := loadExampleDocument(t, fileName)
	tx, issues := ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("project transaction issues: %+v", issues)
	}
	if tx.Operations[len(tx.Operations)-1].Op != transactions.OpWriteProject {
		t.Fatalf("last operation = %s, want %s", tx.Operations[len(tx.Operations)-1].Op, transactions.OpWriteProject)
	}
	var writeOp transactions.WriteProjectOperation
	if err := json.Unmarshal(tx.Operations[len(tx.Operations)-1].Raw, &writeOp); err != nil {
		t.Fatalf("decode write_project operation: %v", err)
	}
	if !writeOp.SchematicOnly {
		t.Fatalf("write_project schematic_only = false")
	}

	outputDir := filepath.Join(t.TempDir(), projectName)
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("apply issues: %+v", apply.Issues)
	}
	projectPath := filepath.Join(outputDir, projectName+".kicad_pro")
	schematicPath := filepath.Join(outputDir, projectName+".kicad_sch")
	for _, path := range []string{projectPath, schematicPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, projectName+".kicad_pcb")); err == nil {
		t.Fatal("schematic IR project write emitted a PCB file")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat generated PCB file: %v", err)
	}

	report, err := evaluate.Schematic(schematicPath)
	if err != nil {
		t.Fatalf("evaluate generated schematic: %v", err)
	}
	if check := schematicIRCheckByName(report.Checks, "schematic_validation"); check.Status != evaluate.CheckPassed {
		t.Fatalf("schematic_validation check = %#v", check)
	}
	if check := schematicIRCheckByName(report.Checks, "schematic_electrical"); check.Status != evaluate.CheckPassed {
		t.Fatalf("schematic_electrical check = %#v", check)
	}

	generated, err := schematic.ReadFile(schematicPath)
	if err != nil {
		t.Fatalf("read generated schematic: %v", err)
	}
	request, layoutResult := schematiclayout.AdaptSchematic(&generated)
	layoutResult = schematiclayout.Validate(layoutResult, request)
	readability := schematiclayout.BuildReport(layoutResult, schematiclayout.ProfileStandard)
	if !readability.Passed {
		t.Fatalf("readability report failed: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
	if readability.DiagonalWireCount != 0 || readability.ErrorCount != 0 {
		t.Fatalf("unexpected readability counts: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
	if readability.WarningCount != 0 || len(readability.OverlapCounts) != 0 {
		t.Fatalf("generated schematic has readability warnings: %#v diagnostics=%#v", readability, layoutResult.Diagnostics)
	}
}

func loadExampleDocument(t *testing.T, name string) Document {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "schematic-ir", name)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open example %s: %v", name, err)
	}
	defer file.Close()
	document, issues := DecodeStrict(file)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode example %s issues: %+v", name, issues)
	}
	if issues := Validate(document); reports.HasBlockingIssue(issues) {
		t.Fatalf("validate example %s issues: %+v", name, issues)
	}
	return document
}

func schematicIRCheckByName(checks []evaluate.CheckResult, name string) evaluate.CheckResult {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return evaluate.CheckResult{}
}
