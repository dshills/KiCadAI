package repair

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"kicadai/internal/repairloop"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestGeneratedOutputCrossStageRepairsPreserveUserFilesAndReplay(t *testing.T) {
	transaction, err := transactions.Parse([]byte(`{"operations":[
	  {"op":"create_project","name":"cross_stage_output"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1","x_mm":-2.54},{"number":"2","x_mm":2.54}]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	authoritative, generatedPaths, err := GenerateCrossStageProject(context.Background(), transaction, transactions.ApplyOptions{Seed: "cross-stage-output"})
	if err != nil {
		t.Fatal(err)
	}
	schematicPath := ""
	for _, file := range authoritative.Files {
		if filepath.Ext(file.Path) == ".kicad_sch" {
			schematicPath = file.Path
			break
		}
	}
	if schematicPath == "" {
		t.Fatal("authoritative project lacks a schematic")
	}

	for _, test := range []struct {
		stage   repairloop.CrossStage
		code    reports.Code
		reenter repairloop.CrossStage
	}{
		{stage: repairloop.CrossStageSchematic, code: reports.CodeValidationFailed, reenter: repairloop.CrossStageSchematic},
		{stage: repairloop.CrossStageERC, code: reports.CodeKiCadCLIFailed, reenter: repairloop.CrossStageSchematic},
		{stage: repairloop.CrossStageWriter, code: reports.CodeValidationFailed, reenter: repairloop.CrossStageWriter},
		{stage: repairloop.CrossStageRoundTrip, code: reports.CodeRoundTripDiff, reenter: repairloop.CrossStageWriter},
	} {
		t.Run(string(test.stage), func(t *testing.T) {
			run := func() (repairloop.CrossStageReport, CrossStageGeneratedProject) {
				current := cloneCrossStageGeneratedProject(authoritative)
				for index := range current.Files {
					if current.Files[index].Path == schematicPath {
						current.Files[index].Data = append(current.Files[index].Data, []byte("\noutput-layer fault\n")...)
					}
				}
				current.Files = append(current.Files, CrossStageGeneratedFile{Path: "notes/lesson.txt", Data: []byte("user-owned educational notes\n")})
				validate := func(_ context.Context, _ repairloop.CrossStage, project CrossStageGeneratedProject) (CrossStageTransactionEvidence, error) {
					stage := CrossStageTransactionStage{Stage: test.stage}
					actual, found := crossStageGeneratedFile(project, schematicPath)
					expected, _ := crossStageGeneratedFile(authoritative, schematicPath)
					if !found || !bytes.Equal(actual.Data, expected.Data) {
						stage.Issues = []reports.Issue{{
							Code: test.code, Severity: reports.SeverityBlocked, Path: schematicPath,
							Message: "test-only fault description is not repair authorization",
						}}
					}
					return CrossStageTransactionEvidence{Stages: []CrossStageTransactionStage{stage}}, nil
				}
				target, err := NewCrossStageGeneratedTarget(CrossStageGeneratedTargetOptions{
					Transaction: transaction, Apply: transactions.ApplyOptions{Seed: "cross-stage-output"},
					Project: current, GeneratedPaths: generatedPaths,
					RequiredStages: []repairloop.CrossStage{test.stage}, Validate: validate,
				})
				if err != nil {
					t.Fatal(err)
				}
				report, err := repairloop.RunCrossStageRepair(context.Background(), target, repairloop.DefaultCrossStagePolicy())
				if err != nil {
					t.Fatal(err)
				}
				if err := repairloop.ValidateCrossStageReport(report); err != nil {
					t.Fatal(err)
				}
				return report, target.Project()
			}

			first, firstProject := run()
			second, secondProject := run()
			if first.Status != repairloop.CrossStageStatusPassed || first.Consumption.CommittedRepairs != 1 || len(first.Trials) != 1 {
				t.Fatalf("stage %s report=%#v", test.stage, first)
			}
			trial := first.Trials[0]
			if !trial.Confirmed || trial.Proposal.ReenterStage != test.reenter {
				t.Fatalf("stage %s trial=%#v", test.stage, trial)
			}
			firstJSON, _ := json.Marshal(first)
			secondJSON, _ := json.Marshal(second)
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatalf("stage %s report replay differs", test.stage)
			}
			firstProjectJSON, _ := json.Marshal(firstProject)
			secondProjectJSON, _ := json.Marshal(secondProject)
			if !bytes.Equal(firstProjectJSON, secondProjectJSON) {
				t.Fatalf("stage %s project replay differs", test.stage)
			}
			notes, found := crossStageGeneratedFile(firstProject, "notes/lesson.txt")
			if !found || string(notes.Data) != "user-owned educational notes\n" {
				t.Fatalf("stage %s user file changed: %#v", test.stage, notes)
			}
			schematic, found := crossStageGeneratedFile(firstProject, schematicPath)
			expected, _ := crossStageGeneratedFile(authoritative, schematicPath)
			if !found || !bytes.Equal(schematic.Data, expected.Data) {
				t.Fatalf("stage %s schematic was not regenerated", test.stage)
			}
		})
	}
}

func TestGeneratedOutputReentryValidationFailureDoesNotPublishProject(t *testing.T) {
	transaction, err := transactions.Parse([]byte(`{"operations":[
	  {"op":"create_project","name":"atomic_output"},
	  {"op":"write_project"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	project, generatedPaths, err := GenerateCrossStageProject(context.Background(), transaction, transactions.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	project.Files = append(project.Files, CrossStageGeneratedFile{Path: "notes/lesson.txt", Data: []byte("preserve me\n")})
	failValidation := false
	validate := func(context.Context, repairloop.CrossStage, CrossStageGeneratedProject) (CrossStageTransactionEvidence, error) {
		if failValidation {
			return CrossStageTransactionEvidence{}, errors.New("validation failed")
		}
		return CrossStageTransactionEvidence{Stages: []CrossStageTransactionStage{{
			Stage:  repairloop.CrossStageWriter,
			Issues: []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked}},
		}}}, nil
	}
	target, err := NewCrossStageGeneratedTarget(CrossStageGeneratedTargetOptions{
		Transaction:    transaction,
		Project:        project,
		GeneratedPaths: generatedPaths,
		RequiredStages: []repairloop.CrossStage{
			repairloop.CrossStageWriter,
		},
		Validate: validate,
	})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := target.Diagnose(context.Background())
	if err != nil || len(diagnostics) != 1 {
		t.Fatalf("diagnostics=%#v err=%v", diagnostics, err)
	}
	proposals, err := target.Propose(context.Background(), diagnostics[0])
	if err != nil || len(proposals) != 1 {
		t.Fatalf("proposals=%#v err=%v", proposals, err)
	}
	if err := target.Apply(context.Background(), proposals[0]); err != nil {
		t.Fatal(err)
	}
	failValidation = true
	before, _ := json.Marshal(target.Project())
	if err := target.Reenter(context.Background(), repairloop.CrossStageWriter); err == nil {
		t.Fatal("expected generated project validation to fail")
	}
	after, _ := json.Marshal(target.Project())
	if !bytes.Equal(before, after) {
		t.Fatal("failed generated project validation published partial state")
	}
}
