package repair

import (
	"context"
	"testing"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func TestBuiltInPostApplyValidatorsRegistersEnabledValidators(t *testing.T) {
	tests := []struct {
		name  string
		opts  PostValidationOptions
		types []string
	}{
		{name: "none", opts: PostValidationOptions{}},
		{name: "writer", opts: PostValidationOptions{WriterCorrectness: true}, types: []string{"writer"}},
		{name: "board", opts: PostValidationOptions{BoardValidation: true}, types: []string{"board"}},
		{name: "erc", opts: PostValidationOptions{KiCadERC: true}, types: []string{"erc"}},
		{name: "drc", opts: PostValidationOptions{KiCadDRC: true}, types: []string{"drc"}},
		{name: "roundtrip", opts: PostValidationOptions{RoundTrip: true}, types: []string{"roundtrip"}},
		{name: "both", opts: PostValidationOptions{WriterCorrectness: true, BoardValidation: true}, types: []string{"writer", "board"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validators := BuiltInPostApplyValidators(test.opts)
			if len(validators) != len(test.types) {
				t.Fatalf("validators = %d, want %d", len(validators), len(test.types))
			}
			counts := validatorTypeCounts(validators)
			for _, want := range test.types {
				if counts[want] != 1 {
					t.Fatalf("validator count for %q = %d, want 1; all counts=%+v", want, counts[want], counts)
				}
			}
		})
	}
}

func validatorTypeCounts(validators []PostApplyValidator) map[string]int {
	counts := map[string]int{}
	for _, validator := range validators {
		switch validator := validator.(type) {
		case WriterCorrectnessValidator:
			counts["writer"]++
		case BoardValidationValidator:
			counts["board"]++
		case KiCadCheckValidator:
			if validator.Kind == checks.CheckKindDRC {
				counts["drc"]++
			} else {
				counts["erc"]++
			}
		case RoundTripValidator:
			counts["roundtrip"]++
		}
	}
	return counts
}

func TestWriterCorrectnessPostValidatorRequiresTarget(t *testing.T) {
	validation := WriterCorrectnessValidator{}.ValidatePostApply(context.Background(), PostApplyValidationContext{})
	if len(validation.Issues) != 1 || validation.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("validation issues = %+v", validation.Issues)
	}
	if validation.Name != postValidatorWriterCorrectness {
		t.Fatalf("validation name = %q", validation.Name)
	}
}

func TestWriterCorrectnessPostValidatorHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	validation := WriterCorrectnessValidator{}.ValidatePostApply(ctx, PostApplyValidationContext{OutputDir: t.TempDir()})
	if len(validation.Issues) != 1 || validation.Issues[0].Code != reports.CodeOperationCanceled {
		t.Fatalf("validation issues = %+v", validation.Issues)
	}
}

func TestBoardValidationPostValidatorRequiresTarget(t *testing.T) {
	validation := BoardValidationValidator{}.ValidatePostApply(context.Background(), PostApplyValidationContext{})
	if len(validation.Issues) != 1 || validation.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("validation issues = %+v", validation.Issues)
	}
	if validation.Name != postValidatorBoardValidation {
		t.Fatalf("validation name = %q", validation.Name)
	}
}

func TestBoardValidationOptionsMapPostValidationOptions(t *testing.T) {
	opts := boardValidationOptions(PostValidationOptions{
		StrictZones:             true,
		StrictUnrouted:          true,
		AllowMissingKiCadChecks: true,
		KiCadCLI:                "/bin/kicad-cli",
		KeepArtifacts:           true,
		ArtifactDir:             "artifacts",
	})
	if !opts.StrictZones {
		t.Fatalf("StrictZones = false, want true")
	}
	if !opts.StrictUnrouted {
		t.Fatalf("StrictUnrouted = false, want true")
	}
	if opts.RequireDRC {
		t.Fatalf("RequireDRC = true, want false")
	}
	if !opts.AllowMissingDRC {
		t.Fatalf("AllowMissingDRC = false, want true")
	}
	if opts.KiCadCLI != "/bin/kicad-cli" {
		t.Fatalf("KiCadCLI = %q, want /bin/kicad-cli", opts.KiCadCLI)
	}
	if opts.ArtifactDir != "artifacts" {
		t.Fatalf("ArtifactDir = %q, want artifacts", opts.ArtifactDir)
	}
	if !opts.KeepArtifacts {
		t.Fatalf("KeepArtifacts = false, want true")
	}
}

func TestBoardValidationOptionsRequireDRCOverridesMissingKiCadAllowance(t *testing.T) {
	opts := boardValidationOptions(PostValidationOptions{
		KiCadDRC:                true,
		AllowMissingKiCadChecks: true,
	})
	if !opts.RequireDRC || opts.AllowMissingDRC {
		t.Fatalf("required DRC must not allow missing DRC: %+v", opts)
	}
}

func TestKiCadUnavailableIssueCanBeWarningWhenMissingChecksAllowed(t *testing.T) {
	issue := kicadUnavailableIssue(assertErr("missing"), PostValidationOptions{AllowMissingKiCadChecks: true}, postValidatorKiCadERC)
	if issue.Code != reports.CodeSkippedExternalTool || issue.Severity != reports.SeverityWarning {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestKiCadUnavailableIssueBlocksWhenRequired(t *testing.T) {
	issue := kicadUnavailableIssue(assertErr("missing"), PostValidationOptions{RequireKiCadERC: true, AllowMissingKiCadChecks: true}, postValidatorKiCadERC)
	if issue.Code != reports.CodeSkippedExternalTool || issue.Severity != reports.SeverityError {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestKiCadCheckIssuesAndArtifactsConvertsFindings(t *testing.T) {
	result := checks.CheckResult{
		Kind:       checks.CheckKindERC,
		ReportPath: "artifacts/erc.json",
		Findings: []checks.CheckFinding{{
			Severity:   "warning",
			Rule:       "power_pin_not_driven",
			Message:    "power pin not driven",
			File:       "demo.kicad_sch",
			References: []string{"U1"},
			Net:        "VCC",
			Nets:       []string{"VCC", "GND"},
		}},
	}
	issues, artifacts := kicadCheckIssuesAndArtifacts("demo.kicad_sch", result, nil)
	if len(issues) != 1 || issues[0].Severity != reports.SeverityWarning || len(issues[0].Nets) != 2 {
		t.Fatalf("issues = %+v", issues)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != reports.ArtifactERCReport {
		t.Fatalf("artifacts = %+v", artifacts)
	}
}

func TestKiCadCheckIssuesAndArtifactsUsesTargetPathWhenResultPathEmpty(t *testing.T) {
	issues, _ := kicadCheckIssuesAndArtifacts("demo.kicad_pcb", checks.CheckResult{}, assertErr("failed"))
	if len(issues) != 1 || issues[0].Path != "demo.kicad_pcb" {
		t.Fatalf("issues = %+v", issues)
	}
}

type assertErr string

func (err assertErr) Error() string {
	return string(err)
}
