package opentopologysynthesis

import "testing"

func TestV18InputImpedanceProbeChangesOnlyItsAssertionCases(t *testing.T) {
	requirement := testV18Case005(t)
	probeRequirement, originalByProbe := v18InputImpedanceProbeRequirement(requirement)
	if len(originalByProbe) != 2 {
		t.Fatalf("V18 probe case count = %d, want 2", len(originalByProbe))
	}
	for _, assertion := range probeRequirement.Requirements.BehavioralRequirements {
		if assertion.ID == "measurement_loading" {
			for _, caseID := range assertion.OperatingCases {
				if _, found := originalByProbe[caseID]; !found {
					t.Fatalf("input-impedance assertion retained unprobed case %q", caseID)
				}
			}
			continue
		}
		for _, caseID := range assertion.OperatingCases {
			if _, found := originalByProbe[caseID]; found {
				t.Fatalf("non-impedance assertion %q was rebound to probe case %q", assertion.ID, caseID)
			}
		}
	}
	if probe, found := v18BoundedNonzeroProbe(0, 2.5); !found || probe != 1e-3 {
		t.Fatalf("bounded positive probe = %.12g, found=%t", probe, found)
	}
	if _, found := v18BoundedNonzeroProbe(0, 0); found {
		t.Fatal("fixed zero incorrectly received a nonzero probe")
	}
}
