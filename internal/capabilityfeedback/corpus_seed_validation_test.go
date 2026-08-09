package capabilityfeedback

import (
	"fmt"
	"testing"

	"kicadai/internal/capabilityevaluation"
	ots "kicadai/internal/opentopologysynthesis"
)

func TestClosedLoopSeedRequirementsValidate(t *testing.T) {
	seeds := closedLoopCorpusSeeds()
	if len(seeds) != closedLoopCorpusSize {
		t.Fatalf("seed count = %d, want %d", len(seeds), closedLoopCorpusSize)
	}
	counts := map[string]int{}
	for index, seed := range seeds {
		wantID := fmt.Sprintf("case_%03d", index+1)
		if seed.ID != wantID {
			t.Errorf("seed %d ID = %q, want %q", index, seed.ID, wantID)
		}
		if issues := ots.Validate(closedLoopRequirement(seed)); len(issues) != 0 {
			t.Errorf("%s requirement issues: %#v", seed.ID, issues)
		}
		counts[string(seed.Role)+":"+string(seed.Domain)]++
	}
	for _, role := range []CorpusRole{RoleDiscovery, RoleHeldOut} {
		for _, domain := range []capabilityevaluation.Domain{
			capabilityevaluation.DomainAnalog,
			capabilityevaluation.DomainPower,
			capabilityevaluation.DomainDigital,
			capabilityevaluation.DomainMCU,
			capabilityevaluation.DomainSensor,
			capabilityevaluation.DomainMixedSignal,
		} {
			if got := counts[string(role)+":"+string(domain)]; got != 2 {
				t.Errorf("%s/%s seed count = %d, want 2", role, domain, got)
			}
		}
	}
}
