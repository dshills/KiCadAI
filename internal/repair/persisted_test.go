package repair

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/inspect"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestApplyPersistedBundleReplaysOutlineRepair(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	initial := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output})
	if len(initial.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", initial.Issues)
	}
	result := ApplyPersistedBundle(output, Bundle{
		Schema:      BundleSchemaV1,
		ProjectRoot: output,
		ProjectName: "demo",
		Generated:   true,
		Transaction: &tx,
		StageIssues: []StageIssues{{Stage: "validation", Issues: []reports.Issue{{
			Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing outline",
		}}}},
		RepairOptions: Options{Enabled: true, AllowOutlineGeneration: true},
	}, PersistedApplyOptions{
		Execute:        true,
		Overwrite:      true,
		Board:          &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
		InspectProject: cleanInspection,
	})
	if result.Status != StatusRepaired || len(result.Issues) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if !hasOperation(result.Transaction, transactions.OpSetBoardOutline) {
		t.Fatalf("repaired transaction missing outline: %#v", result.Transaction.Operations)
	}
	data, err := os.ReadFile(filepath.Join(output, "demo.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Edge.Cuts") {
		t.Fatalf("persisted PCB missing outline:\n%s", data)
	}
}

func TestApplyPersistedBundleAssignsFootprintBeforeReplay(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpAddSymbol, transactions.AddSymbolOperation{Op: transactions.OpAddSymbol, Ref: "R1", LibraryID: "Device:R", At: transactions.Point{XMM: 10, YMM: 10}, Pins: []transactions.PinSpec{{Number: "1"}, {Number: "2"}}}, "R1"),
		mustRepairOperation(t, transactions.OpAssignFootprint, transactions.AssignFootprintOperation{Op: transactions.OpAssignFootprint, Ref: "R1", FootprintID: "Resistor_SMD:R_0603_1608Metric"}, "R1"),
		mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{Op: transactions.OpPlaceFootprint, Ref: "R1", FootprintID: "Resistor_SMD:R_0603_1608Metric", At: transactions.Point{XMM: 20, YMM: 20}}, "R1"),
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	initial := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output})
	if len(initial.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", initial.Issues)
	}
	result := ApplyPersistedBundle(output, Bundle{
		Schema:      BundleSchemaV1,
		ProjectRoot: output,
		ProjectName: "demo",
		Generated:   true,
		Transaction: &tx,
		StageIssues: []StageIssues{{Stage: "validation", Issues: []reports.Issue{{
			Code: reports.CodeMissingFootprint, Severity: reports.SeverityError, Message: "missing verified footprint", Refs: []string{"R1"},
		}}}},
		RepairOptions: Options{Enabled: true, AllowFootprintAssignment: true},
	}, PersistedApplyOptions{
		Execute:   true,
		Overwrite: true,
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
		InspectProject: cleanInspection,
	})
	if result.Status != StatusRepaired || len(result.Issues) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if !hasOperation(result.Transaction, transactions.OpAssignFootprint) {
		t.Fatalf("repaired transaction missing assign_footprint: %#v", result.Transaction.Operations)
	}
	data, err := os.ReadFile(filepath.Join(output, "demo.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Resistor_SMD:R_0805_2012Metric") {
		t.Fatalf("persisted schematic missing footprint assignment:\n%s", data)
	}
}

func TestApplyPersistedBundleBlocksWithoutOverwrite(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	initial := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output})
	if len(initial.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", initial.Issues)
	}
	result := ApplyPersistedBundle(output, Bundle{
		Schema:        BundleSchemaV1,
		Generated:     true,
		Transaction:   &tx,
		RepairOptions: Options{Enabled: true},
	}, PersistedApplyOptions{Execute: true, InspectProject: cleanInspection})
	if result.Status != StatusBlocked || !containsIssue(result.Issues, "overwrite") {
		t.Fatalf("expected overwrite block, got %#v", result)
	}
}

func TestApplyPersistedBundleBlocksWithoutExecute(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	if initial := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output}); len(initial.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", initial.Issues)
	}
	result := ApplyPersistedBundle(output, Bundle{
		Schema:        BundleSchemaV1,
		Generated:     true,
		Transaction:   &tx,
		RepairOptions: Options{Enabled: true},
	}, PersistedApplyOptions{Overwrite: true, InspectProject: cleanInspection})
	if result.Status != StatusBlocked || !containsIssue(result.Issues, "execute") {
		t.Fatalf("expected execute block, got %#v", result)
	}
}

func TestApplyPersistedBundleBlocksInvalidTransactionBeforeWrite(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpAddSymbol, transactions.AddSymbolOperation{Op: transactions.OpAddSymbol, Ref: "", LibraryID: "Device:R", At: transactions.Point{XMM: 10, YMM: 10}}, ""),
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	result := ApplyPersistedBundle(output, Bundle{
		Schema:      BundleSchemaV1,
		ProjectRoot: output,
		Generated:   true,
		Transaction: &tx,
		StageIssues: []StageIssues{{Stage: "validation", Issues: []reports.Issue{{
			Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing outline",
		}}}},
		RepairOptions: Options{Enabled: true, AllowOutlineGeneration: true},
	}, PersistedApplyOptions{
		Execute:        true,
		Overwrite:      true,
		Board:          &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
		InspectProject: cleanInspection,
	})
	if result.Status != StatusBlocked || len(result.Apply.Artifacts) != 0 {
		t.Fatalf("expected validation block before apply, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(output, "demo.kicad_pro")); !os.IsNotExist(err) {
		t.Fatalf("project was written despite invalid transaction: %v", err)
	}
}

func TestApplyPersistedBundleRemovesStaleGeneratedFiles(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	if initial := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output}); len(initial.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", initial.Issues)
	}
	stale := filepath.Join(output, "old_sheet.kicad_sch")
	userSheet := filepath.Join(output, "user_sheet.kicad_sch")
	keep := filepath.Join(output, "notes.txt")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userSheet, []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Write(output, manifest.Manifest{
		ProjectName: "demo",
		Artifacts:   []reports.Artifact{{Kind: reports.ArtifactSchematic, Path: stale}},
	}); err != nil {
		t.Fatal(err)
	}
	result := ApplyPersistedBundle(output, Bundle{
		Schema:        BundleSchemaV1,
		Generated:     true,
		Transaction:   &tx,
		RepairOptions: Options{Enabled: true},
	}, PersistedApplyOptions{Execute: true, Overwrite: true, InspectProject: cleanInspection})
	if result.Status != StatusRepaired || len(result.Issues) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale generated file was not removed: %v", err)
	}
	if _, err := os.Stat(userSheet); err != nil {
		t.Fatalf("user-created KiCad file should remain: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated file should remain: %v", err)
	}
}

func TestApplyPersistedBundlePostValidationWarningIsPartial(t *testing.T) {
	output, bundle := persistedOutlineFixture(t)
	result := ApplyPersistedBundle(output, bundle, PersistedApplyOptions{
		Execute:        true,
		Overwrite:      true,
		Board:          &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
		InspectProject: cleanInspection,
		PostValidators: []PostApplyValidator{PostApplyValidatorFunc(func(context.Context, PostApplyValidationContext) PostApplyValidation {
			return PostApplyValidation{Name: "writer", Issues: []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Message: "non-blocking diff"}}}
		})},
	})
	if result.Status != StatusPartial || len(result.Validation) != 2 {
		t.Fatalf("expected partial validation result, got %#v", result)
	}
}

func TestApplyPersistedBundlePostValidationRepeatedBlockingIsBlocked(t *testing.T) {
	output, bundle := persistedOutlineFixture(t)
	result := ApplyPersistedBundle(output, bundle, PersistedApplyOptions{
		Execute:        true,
		Overwrite:      true,
		Board:          &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
		InspectProject: cleanInspection,
		PostValidators: []PostApplyValidator{PostApplyValidatorFunc(func(context.Context, PostApplyValidationContext) PostApplyValidation {
			return PostApplyValidation{Name: "board", Issues: []reports.Issue{{Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing outline"}}}
		})},
	})
	if result.Status != StatusBlocked {
		t.Fatalf("expected blocked validation result, got %#v", result)
	}
}

func TestApplyPersistedBundlePostValidationWorsenedIssueCountBlocks(t *testing.T) {
	output, bundle := persistedOutlineFixture(t)
	result := ApplyPersistedBundle(output, bundle, PersistedApplyOptions{
		Execute:        true,
		Overwrite:      true,
		Board:          &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
		InspectProject: cleanInspection,
		PostValidators: []PostApplyValidator{PostApplyValidatorFunc(func(context.Context, PostApplyValidationContext) PostApplyValidation {
			return PostApplyValidation{Name: "board", Issues: []reports.Issue{
				{Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing outline"},
				{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Message: "disconnected"},
			}}
		})},
	})
	if result.Status != StatusBlocked {
		t.Fatalf("expected blocked validation result, got %#v", result)
	}
}

func TestApplyPersistedBundleSkippedOptionalValidatorDoesNotBlock(t *testing.T) {
	output, bundle := persistedOutlineFixture(t)
	result := ApplyPersistedBundle(output, bundle, PersistedApplyOptions{
		Execute:        true,
		Overwrite:      true,
		Board:          &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
		InspectProject: cleanInspection,
		PostValidators: []PostApplyValidator{nil},
	})
	if result.Status != StatusRepaired || len(result.Validation) != 2 || !result.Validation[1].Skipped {
		t.Fatalf("expected skipped optional validator, got %#v", result)
	}
}

func persistedOutlineFixture(t *testing.T) (string, Bundle) {
	t.Helper()
	output := filepath.Join(t.TempDir(), "demo")
	tx := persistedBaseTransaction(t, "demo",
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	)
	initial := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output})
	if len(initial.Issues) != 0 {
		t.Fatalf("initial apply issues: %#v", initial.Issues)
	}
	return output, Bundle{
		Schema:      BundleSchemaV1,
		ProjectRoot: output,
		ProjectName: "demo",
		Generated:   true,
		Transaction: &tx,
		StageIssues: []StageIssues{{Stage: "validation", Issues: []reports.Issue{{
			Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing outline",
		}}}},
		RepairOptions: Options{Enabled: true, AllowOutlineGeneration: true},
	}
}

func persistedBaseTransaction(t *testing.T, name string, ops ...transactions.Operation) transactions.Transaction {
	t.Helper()
	create := mustRepairOperation(t, transactions.OpCreateProject, transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: name}, "")
	all := append([]transactions.Operation{create}, ops...)
	return transactions.Transaction{Operations: all}
}

func cleanInspection(string) (inspect.ProjectSummary, error) {
	return inspect.ProjectSummary{}, nil
}

func hasOperation(tx transactions.Transaction, kind transactions.OperationKind) bool {
	for _, op := range tx.Operations {
		if op.Op == kind {
			return true
		}
	}
	return false
}

func containsIssue(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}
