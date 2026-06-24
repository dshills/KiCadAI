package repair

import (
	"testing"

	"kicadai/internal/reports"
)

func TestNormalizePostApplyValidationsMapsAdapters(t *testing.T) {
	findings := NormalizePostApplyValidations([]PostApplyValidation{
		{
			Name: postValidatorBoardValidation,
			Issues: []reports.Issue{{
				Code:     reports.CodeDisconnectedPad,
				Severity: reports.SeverityError,
				Path:     "board",
				Message:  "pad is disconnected",
				Nets:     []string{"GND"},
			}},
		},
		{
			Name: postValidatorKiCadDRC,
			Issues: []reports.Issue{{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     "drc.json",
				Message:  "clearance warning",
			}},
		},
	})
	if len(findings) != 2 {
		t.Fatalf("findings = %d, want 2: %#v", len(findings), findings)
	}
	if findings[0].Source == "" || findings[0].Key == "" {
		t.Fatalf("finding missing normalized metadata: %#v", findings[0])
	}
	var sawBoard, sawDRC bool
	for _, finding := range findings {
		switch finding.Source {
		case FindingSourceBoard:
			sawBoard = true
			if finding.Category != FindingCategoryConnectivity || finding.Subject.Net != "GND" {
				t.Fatalf("board finding = %#v", finding)
			}
		case FindingSourceKiCadDRC:
			sawDRC = true
		}
	}
	if !sawBoard || !sawDRC {
		t.Fatalf("sources missing board=%v drc=%v findings=%#v", sawBoard, sawDRC, findings)
	}
}

func TestNormalizeStageIssuesUsesStageAsAdapter(t *testing.T) {
	findings := NormalizeStageIssues([]StageIssues{{
		Stage: "writer_correctness",
		Issues: []reports.Issue{{
			Code:     reports.CodeRoundTripDiff,
			Severity: reports.SeverityError,
			Message:  "round trip changed file",
		}},
	}})
	if len(findings) != 1 {
		t.Fatalf("findings = %#v", findings)
	}
	if findings[0].Source != FindingSourceWriter || findings[0].Adapter != "writer_correctness" || findings[0].Category != FindingCategoryRoundTrip {
		t.Fatalf("finding = %#v", findings[0])
	}
}
