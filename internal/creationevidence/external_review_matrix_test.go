package creationevidence

import (
	"os"
	"testing"
)

func TestExternalReviewMatrixCreationEvidence(t *testing.T) {
	if os.Getenv("KICADAI_RUN_EXTERNAL_REVIEW_MATRIX") != "1" {
		t.Skip("run through make review-matrix")
	}
	t.Run("write failure preserves evidence", TestWriteFailurePreservesPreviousCoreEvidence)
	t.Run("typed deterministic evidence", TestWriteProducesTypedDeterministicCoreEvidence)
}
