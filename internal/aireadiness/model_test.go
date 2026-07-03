package aireadiness

import (
	"path/filepath"
	"strings"
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

func TestValidateRejectsUnsupportedParallelGroup(t *testing.T) {
	record := validReadinessRecord("generic.component.part")
	record.ParallelGroup = ParallelGroup("unknown")
	if err := Validate(Matrix{Records: []Record{record}}); err == nil {
		t.Fatal("expected unsupported parallel_group validation error")
	}
}

func TestValidateAcceptsExplicitUnassignedParallelGroup(t *testing.T) {
	record := validReadinessRecord("generic.component.part")
	record.ParallelGroup = ParallelGroupUnassigned
	if err := Validate(Matrix{Records: []Record{record}}); err != nil {
		t.Fatalf("expected explicit unassigned parallel_group to pass: %v", err)
	}
}

func TestValidateRejectsUnknownDependency(t *testing.T) {
	record := validReadinessRecord("generic.component.part")
	record.DependsOn = []string{"generic.component.missing"}
	if err := Validate(Matrix{Records: []Record{record}}); err == nil {
		t.Fatal("expected unknown dependency validation error")
	}
}

func TestValidateRejectsSelfDependency(t *testing.T) {
	record := validReadinessRecord("generic.component.part")
	record.DependsOn = []string{record.ID}
	if err := Validate(Matrix{Records: []Record{record}}); err == nil {
		t.Fatal("expected self dependency validation error")
	}
}

func TestValidateRejectsUnsortedDependencies(t *testing.T) {
	record := validReadinessRecord("generic.component.part")
	depA := validReadinessRecord("generic.component.alpha")
	depB := validReadinessRecord("generic.component.beta")
	record.DependsOn = []string{depB.ID, depA.ID}
	if err := Validate(Matrix{Records: []Record{record, depA, depB}}); err == nil {
		t.Fatal("expected unsorted dependency validation error")
	}
}

func TestValidateRejectsDependencyCycle(t *testing.T) {
	first := validReadinessRecord("generic.component.first")
	second := validReadinessRecord("generic.component.second")
	first.DependsOn = []string{second.ID}
	second.DependsOn = []string{first.ID}
	if err := Validate(Matrix{Records: []Record{first, second}}); err == nil {
		t.Fatal("expected dependency cycle validation error")
	}
}

func TestValidateRejectsVerifiedRecordWithUnverifiedDependency(t *testing.T) {
	dependency := validReadinessRecord("generic.component.dependency")
	record := validVerifiedRecord("generic.component.done")
	record.DependsOn = []string{dependency.ID}
	if err := Validate(Matrix{Records: []Record{dependency, record}}); err == nil {
		t.Fatal("expected verified dependency validation error")
	}
}

func TestValidateAcceptsVerifiedRecordWithVerifiedDependency(t *testing.T) {
	dependency := validVerifiedRecord("generic.component.dependency")
	record := validVerifiedRecord("generic.component.done")
	record.DependsOn = []string{dependency.ID}
	if err := Validate(Matrix{Records: []Record{dependency, record}}); err != nil {
		t.Fatalf("expected verified dependency to pass: %v", err)
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

func validReadinessRecord(id string) Record {
	parts := strings.Split(id, ".")
	domain := ""
	category := Category("")
	if len(parts) >= 2 {
		domain = parts[0]
		category = Category(parts[1])
	}
	return Record{
		ID:             id,
		Category:       category,
		Domain:         domain,
		Title:          "Valid",
		Readiness:      ReadinessMissing,
		Blocker:        "missing",
		EvidenceNeeded: []string{"evidence"},
		NextTask:       TaskAddComponent,
	}
}

func validVerifiedRecord(id string) Record {
	record := validReadinessRecord(id)
	record.Readiness = ReadinessVerified
	record.Blocker = ""
	record.EvidenceNeeded = nil
	record.Evidence = []Evidence{{Kind: "test", Description: "verified"}}
	return record
}
