package aireadiness

import (
	"path/filepath"
	"testing"
)

func TestLoadDirValidatesCheckedInMatrix(t *testing.T) {
	matrix, err := LoadDir(filepath.Join("..", "..", "data", "ai-readiness"))
	if err != nil {
		t.Fatalf("load matrix: %v", err)
	}
	if len(matrix.Records) == 0 {
		t.Fatal("matrix has no records")
	}
	for _, record := range matrix.Records {
		if record.ID == "" || record.NextTask == "" {
			t.Fatalf("incomplete record: %#v", record)
		}
	}
}

func TestValidateRejectsDuplicateIDs(t *testing.T) {
	record := Record{
		ID:             "generic.component.duplicate",
		Category:       CategoryComponent,
		Domain:         "generic",
		Title:          "Duplicate",
		Readiness:      ReadinessMissing,
		Blocker:        "missing",
		EvidenceNeeded: []string{"evidence"},
		NextTask:       TaskAddComponent,
	}
	if err := Validate(Matrix{Records: []Record{record, record}}); err == nil {
		t.Fatal("expected duplicate ID validation error")
	}
}

func TestValidateRejectsUnexpectedIDShape(t *testing.T) {
	record := Record{
		ID:             "generic.component.too.many",
		Category:       CategoryComponent,
		Domain:         "generic",
		Title:          "Bad ID",
		Readiness:      ReadinessMissing,
		Blocker:        "missing",
		EvidenceNeeded: []string{"evidence"},
		NextTask:       TaskAddComponent,
	}
	if err := Validate(Matrix{Records: []Record{record}}); err == nil {
		t.Fatal("expected invalid ID shape")
	}
}

func TestValidateRejectsIDMetadataMismatch(t *testing.T) {
	record := Record{
		ID:             "amplifier.component.mismatch",
		Category:       CategoryBlock,
		Domain:         "power",
		Title:          "Mismatch",
		Readiness:      ReadinessMissing,
		Blocker:        "missing",
		EvidenceNeeded: []string{"evidence"},
		NextTask:       TaskAddBlock,
	}
	if err := Validate(Matrix{Records: []Record{record}}); err == nil {
		t.Fatal("expected ID metadata mismatch validation error")
	}
}

func TestValidateRejectsVerifiedWithoutEvidence(t *testing.T) {
	record := Record{
		ID:        "generic.component.verified",
		Category:  CategoryComponent,
		Domain:    "generic",
		Title:     "Verified",
		Readiness: ReadinessVerified,
		NextTask:  TaskCaptureKiCadEvidence,
	}
	if err := Validate(Matrix{Records: []Record{record}}); err == nil {
		t.Fatal("expected verified record evidence validation error")
	}
}

func TestAmplifierRequirementsCoveredByMatrix(t *testing.T) {
	root := filepath.Join("..", "..", "data", "ai-readiness")
	matrix, err := LoadDir(root)
	if err != nil {
		t.Fatalf("load matrix: %v", err)
	}
	requirement, err := LoadRequirement(filepath.Join(root, "requirements", "amplifier.json"))
	if err != nil {
		t.Fatalf("load requirement: %v", err)
	}
	if err := ValidateRequirements(matrix, requirement); err != nil {
		t.Fatalf("requirements not covered: %v", err)
	}
}
