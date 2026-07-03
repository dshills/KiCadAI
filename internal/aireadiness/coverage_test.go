package aireadiness

import "testing"

func TestSummarizeDomainReportsAmplifierCoverage(t *testing.T) {
	matrix := Matrix{Records: []Record{
		{ID: "amplifier.component.opamp", Domain: "amplifier", Category: CategoryComponent, Readiness: ReadinessDraft, NextTask: TaskAddComponent, ParallelGroup: ParallelGroupCatalogBlockExpansion},
		{ID: "amplifier.block.bias", Domain: "amplifier", Category: CategoryBlock, Readiness: ReadinessMissing, NextTask: TaskAddBlock, ParallelGroup: ParallelGroupCatalogBlockExpansion},
		{ID: "amplifier.layout.thermal", Domain: "amplifier", Category: CategoryLayout, Readiness: ReadinessDraft, NextTask: TaskVerifyLayout, ParallelGroup: ParallelGroupEngineHardening},
		{ID: "amplifier.validation.kicad", Domain: "amplifier", Category: CategoryValidation, Readiness: ReadinessMissing, NextTask: TaskCaptureKiCadEvidence, ParallelGroup: ParallelGroupFixturePromotion},
		{ID: "amplifier.documentation.limits", Domain: "amplifier", Category: CategoryDocumentation, Readiness: ReadinessDraft, NextTask: TaskWriteDocs},
		{ID: "power.component.regulator", Domain: "power", Category: CategoryComponent, Readiness: ReadinessCandidate, NextTask: TaskAddComponent},
	}}
	coverage := SummarizeDomain(matrix, "amplifier")
	if coverage.Total != 5 {
		t.Fatalf("amplifier total = %d, want 5", coverage.Total)
	}
	for _, category := range []Category{CategoryComponent, CategoryBlock, CategoryLayout, CategoryValidation, CategoryDocumentation} {
		if !coverageHasCategory(coverage, category) {
			t.Fatalf("amplifier coverage missing category %s: %#v", category, coverage.ByCategory)
		}
	}
	if !coverageHasTask(coverage, TaskAddComponent) || !coverageHasTask(coverage, TaskVerifyLayout) || !coverageHasTask(coverage, TaskCaptureKiCadEvidence) {
		t.Fatalf("amplifier coverage missing expected task families: %#v", coverage.NextTasks)
	}
	if !coverageHasParallelGroup(coverage, ParallelGroupCatalogBlockExpansion, 2) ||
		!coverageHasParallelGroup(coverage, ParallelGroupEngineHardening, 1) ||
		!coverageHasParallelGroup(coverage, ParallelGroupFixturePromotion, 1) ||
		!coverageHasParallelGroup(coverage, ParallelGroupUnassigned, 1) {
		t.Fatalf("amplifier coverage missing expected parallel groups: %#v", coverage.ByParallelGroup)
	}
}

func coverageHasCategory(coverage DomainCoverage, category Category) bool {
	for _, item := range coverage.ByCategory {
		if item.Category == category && item.Count > 0 {
			return true
		}
	}
	return false
}

func coverageHasTask(coverage DomainCoverage, task TaskType) bool {
	for _, item := range coverage.NextTasks {
		if item.Task == task && item.Count > 0 {
			return true
		}
	}
	return false
}

func coverageHasParallelGroup(coverage DomainCoverage, group ParallelGroup, count int) bool {
	for _, item := range coverage.ByParallelGroup {
		if item.ParallelGroup == group && item.Count == count {
			return true
		}
	}
	return false
}
