package intentplanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
)

func TestAmplifierIntentFixturesDoNotClaimFabricationReady(t *testing.T) {
	for _, name := range []string{
		"amplifier_class_a_headphone.json",
		"amplifier_class_ab_headphone.json",
		"amplifier_module.json",
	} {
		t.Run(name, func(t *testing.T) {
			plan := planFixture(t, name)
			if plan.Status != PlanStatusPartial {
				t.Fatalf("status = %s, want partial; issues=%#v", plan.Status, plan.Issues)
			}
			if plan.GeneratedRequest == nil {
				t.Fatalf("generated request missing")
			}
			if plan.GeneratedRequest.Validation.Acceptance == designworkflow.AcceptanceFabricationCandidate {
				t.Fatalf("amplifier fixture unexpectedly requested fabrication-candidate validation")
			}
			if len(plan.KnownGaps) == 0 {
				t.Fatalf("amplifier fixture must carry verification gaps")
			}
		})
	}
}

func TestUnsupportedAmplifierPowerTopologyFailsClosed(t *testing.T) {
	plan := planFixture(t, "amplifier_low_voltage_power_blocked.json")
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if len(plan.Issues) == 0 {
		t.Fatalf("blocked fixture returned no issues")
	}
	if !goldenHasIssuePath(plan.Issues, "functions[0].family") {
		t.Fatalf("missing unsupported family issue: %#v", plan.Issues)
	}
	foundTopology := false
	for _, issue := range plan.Issues {
		if strings.Contains(issue.Message, "class_ab_power") {
			foundTopology = true
			break
		}
	}
	if !foundTopology {
		t.Fatalf("issue does not name unsupported topology: %#v", plan.Issues)
	}
}

func TestUnverifiedAmplifierFamilyBlocksFabricationCandidate(t *testing.T) {
	plan := Plan(Request{
		Version:    "0.1.0",
		Name:       "class_ab_fabrication_candidate",
		Kind:       IntentAmplifier,
		Acceptance: designworkflow.AcceptanceFabricationCandidate,
		Interfaces: []InterfaceIntent{
			{Kind: "analog", Connector: "audio_input", Voltage: "2V"},
		},
		Functions: []FunctionIntent{
			{Kind: "amplifier", Family: "class_ab_headphone", Params: map[string]any{"gain": 2}},
		},
	})
	if plan.Status != PlanStatusBlocked {
		t.Fatalf("status = %s, want blocked; issues=%#v", plan.Status, plan.Issues)
	}
	if !goldenHasIssuePath(plan.Issues, "functions[0].family") {
		t.Fatalf("missing fabrication blocker on amplifier family: %#v", plan.Issues)
	}
}

func planFixture(t *testing.T, name string) PlanResult {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "examples", "intent", name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	return Plan(request)
}
