package amplifiers

import (
	"testing"

	"kicadai/internal/blocks"
)

func TestBuiltinAmplifierLayoutsCarryTypedPolicy(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	for _, test := range []struct {
		blockID string
		profile LayoutProfile
	}{
		{blockID: "class_a_voltage_stage", profile: LayoutProfileClassALine},
		{blockID: "class_ab_output_stage", profile: LayoutProfileClassABHeadphone},
		{blockID: "opamp_gain_stage", profile: LayoutProfileOpAmpFeedback},
	} {
		definition, ok := registry.GetBlock(test.blockID)
		if !ok {
			t.Fatalf("missing block %s", test.blockID)
		}
		evidence := ValidateLayoutPolicy(test.profile, definition.PCBRealization)
		if !evidence.OK() {
			t.Fatalf("%s layout evidence = %#v", test.blockID, evidence)
		}
	}
}

func TestLayoutPolicyFailsClosedOnUnquantifiedConstraint(t *testing.T) {
	realization := &blocks.PCBRealization{Constraints: []blocks.PCBConstraint{
		{ID: "return", Kind: "return_topology", Category: blocks.PCBConstraintReturnTopology, NetTemplate: "agnd", AppliesTo: []string{"a", "b"}},
		{ID: "width", Kind: "route_width", Category: blocks.PCBConstraintCurrentPath, NetTemplate: "out"},
		{ID: "thermal", Kind: "max_spacing", Category: blocks.PCBConstraintThermalCoupling, AppliesTo: []string{"q1", "q2"}, MaxLengthMM: 5},
		{ID: "symmetry", Kind: "max_spacing", Category: blocks.PCBConstraintDeviceSymmetry, AppliesTo: []string{"q1", "q2"}, MaxLengthMM: 4},
	}}
	evidence := ValidateLayoutPolicy(LayoutProfileClassABHeadphone, realization)
	if evidence.OK() || len(evidence.InvalidCategories) != 1 || evidence.InvalidCategories[0] != blocks.PCBConstraintCurrentPath {
		t.Fatalf("evidence = %#v, want unquantified current path blocked", evidence)
	}
}
