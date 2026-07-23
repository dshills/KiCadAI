package placement

import (
	"os"
	"testing"
)

func TestExternalReviewMatrixPlacement(t *testing.T) {
	if os.Getenv("KICADAI_RUN_EXTERNAL_REVIEW_MATRIX") != "1" {
		t.Skip("run through make review-matrix")
	}
	t.Run("translated fixed group", TestPlaceTranslatableFixedGroupBeforeRejectingAuthoredCoordinates)
	t.Run("obstacle translation", TestPreserveRelativeGroupPlacementTranslatesClusterAroundObstacle)
	t.Run("deterministic search", TestRelativeGroupSetRecordsDeterministicSearchEvidence)
	t.Run("infeasible group", TestPlaceTranslatableGroupFailureIsAtomic)
}
