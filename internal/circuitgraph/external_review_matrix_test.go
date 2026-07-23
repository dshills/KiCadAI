package circuitgraph

import (
	"os"
	"testing"
)

func TestExternalReviewMatrixCircuitGraph(t *testing.T) {
	if os.Getenv("KICADAI_RUN_EXTERNAL_REVIEW_MATRIX") != "1" {
		t.Skip("run through make review-matrix")
	}
	t.Run("function corpus replay", TestFrozenFunctionLevelCorpusOfflineWorkflowAndReplay)
	t.Run("multi unit resolution", TestResolveNamedMultiUnitLM358Package)
	t.Run("public function example", TestPublicFunctionLevelExampleCreatesOffline)
	t.Run("promoted fixture equivalence", TestPublicFunctionLevelExampleMatchesPromotedFixture)
	t.Run("invalid function parameter", TestFunctionCapabilityValidationAggregatesIndependentMistakes)
}
