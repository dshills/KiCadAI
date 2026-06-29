package designworkflow

import "testing"

func TestSummarizePromotionFixturesGroupsReadinessDeterministically(t *testing.T) {
	summary := SummarizePromotionFixtures([]PromotionFixture{{
		ID:                "sensor",
		Tier:              "block-composition",
		DeclaredReadiness: PromotionReadinessExpectedFail,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		RequireDRC:        true,
		KnownGaps:         []string{"route completion", "  ", "KiCad DRC"},
	}, {
		ID:                "led",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		RequireDRC:        true,
	}, {
		ID:                "blocked_amp",
		Tier:              "fabrication",
		DeclaredReadiness: PromotionReadinessBlocked,
		Acceptance:        AcceptanceFabricationCandidate,
		KnownGaps:         []string{"fabrication evidence"},
	}})
	if summary.Total != 3 {
		t.Fatalf("total = %d, want 3", summary.Total)
	}
	if got := len(summary.ByReadiness); got != 3 {
		t.Fatalf("group count = %d, want 3", got)
	}
	if summary.ByReadiness[0].Readiness != PromotionReadinessCandidate {
		t.Fatalf("first readiness = %q, want candidate", summary.ByReadiness[0].Readiness)
	}
	if summary.ByReadiness[1].Readiness != PromotionReadinessExpectedFail {
		t.Fatalf("second readiness = %q, want expected_fail", summary.ByReadiness[1].Readiness)
	}
	if got := summary.ByReadiness[1].Fixtures[0].KnownGapCount; got != 2 {
		t.Fatalf("known gap count = %d, want 2", got)
	}
	if !summary.ByReadiness[0].Fixtures[0].RequireERC || !summary.ByReadiness[0].Fixtures[0].RequireDRC {
		t.Fatalf("required KiCad evidence not preserved: %#v", summary.ByReadiness[0].Fixtures[0])
	}
}
