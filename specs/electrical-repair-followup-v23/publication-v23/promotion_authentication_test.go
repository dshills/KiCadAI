package v23publication

import (
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
	ot "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
)

func TestPromotionAuthenticationPreservesPassAndFailureBoundaries(t *testing.T) {
	synthesis, project := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, mode := range []string{"pass", "failure", "failure_without_diagnostic", "nonterminal", "missing_project", "synthesis", "unmeasured", "pass_not_claimed", "failure_claimed_pass"} {
		t.Run(mode, func(t *testing.T) {
			promotion := ot.PhysicalPromotionResult{SynthesisHash: synthesis, ProjectHash: project, Status: ot.PhysicalPromotionPassed, ReplayIdentical: true,
				Runs: []ot.PhysicalPromotionRun{{Number: 1, ProjectHash: project}, {Number: 2, ProjectHash: project}}}
			expected := &capabilitybaselinev10.PromotionEvidence{ProjectSHA256: project}
			elapsed := int64(1)
			switch mode {
			case "failure", "failure_without_diagnostic", "failure_claimed_pass":
				promotion.Status = ot.PhysicalPromotionFailed
				promotion.ReplayIdentical = false
				promotion.Runs = nil
				if mode != "failure_without_diagnostic" {
					promotion.Issues = []reports.Issue{{Code: "test.physical_failure", Message: "synthetic physical gate refusal"}}
				}
				if mode != "failure_claimed_pass" {
					expected = nil
				}
			case "nonterminal":
				promotion.Status = "pending"
				expected = nil
			case "missing_project":
				promotion.Runs = promotion.Runs[:1]
			case "synthesis":
				promotion.SynthesisHash = strings.Repeat("c", 64)
			case "unmeasured":
				elapsed = 0
			case "pass_not_claimed":
				expected = nil
			}
			hash, err := valueDigest(promotionIdentity(promotion))
			if err != nil {
				t.Fatal(err)
			}
			promotion.Hash = hash
			if expected != nil {
				expected.RunSHA256 = hash
			}
			err = authenticatePromotion(promotion, synthesis, expected, elapsed)
			if (err == nil) != (mode == "pass" || mode == "failure") {
				t.Fatalf("mode %s: %v", mode, err)
			}
		})
	}
}
