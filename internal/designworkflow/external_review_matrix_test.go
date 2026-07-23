package designworkflow

import (
	"os"
	"testing"
)

func TestExternalReviewMatrixAmplifier(t *testing.T) {
	if os.Getenv("KICADAI_RUN_EXTERNAL_REVIEW_MATRIX") != "1" {
		t.Skip("run through make review-matrix")
	}
	t.Run("declared acceptance", TestAmplifierDesignFixturesPlanToDeclaredAcceptance)
	t.Run("placement and routing", TestClassABHeadphoneFixturePCBPlacementRoutingEvidence)
}
