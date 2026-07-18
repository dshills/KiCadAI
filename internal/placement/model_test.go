package placement

import (
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestNormalizeRequestAppliesDefaults(t *testing.T) {
	req := minimalRequest()
	req.Rules = Rules{}
	req.Components[0].Side = ""
	req.Components[0].Rotation = RotationConstraint{}

	got := NormalizeRequest(req)
	if got.Rules.GridMM != 0.5 {
		t.Fatalf("GridMM = %v, want 0.5", got.Rules.GridMM)
	}
	if got.Rules.MaxCandidatesPerPart == 0 {
		t.Fatal("expected default candidate cap")
	}
	if got.Components[0].Side != SideTop {
		t.Fatalf("Side = %q, want top", got.Components[0].Side)
	}
	if len(got.Components[0].Rotation.AllowedDeg) != 4 {
		t.Fatalf("allowed rotations = %#v, want four defaults", got.Components[0].Rotation.AllowedDeg)
	}
}

func TestNormalizeRequestCapsMaxCandidates(t *testing.T) {
	req := minimalRequest()
	req.Rules.MaxCandidatesPerPart = maxCandidatesPerPartLimit + 1

	got := NormalizeRequest(req)
	if got.Rules.MaxCandidatesPerPart != maxCandidatesPerPartLimit {
		t.Fatalf("MaxCandidatesPerPart = %d, want capped %d", got.Rules.MaxCandidatesPerPart, maxCandidatesPerPartLimit)
	}
}

func TestNormalizeRequestDoesNotMutateInput(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Ref = " R1 "
	req.Components[0].Position = &Placement{XMM: 3, YMM: 4}
	fixedRotation := 90.0
	req.Components[0].Rotation.FixedDeg = &fixedRotation
	req.Groups = []Group{{ID: " g1 ", Components: []string{" C1 ", " R1 "}}}
	req.Groups[0].Anchor.At = &Point{XMM: 2, YMM: 2}
	req.Nets[0].Endpoints[0].Ref = " R1 "

	got := NormalizeRequest(req)
	if got.Components[0].Ref != "R1" {
		t.Fatalf("normalized ref = %q, want R1", got.Components[0].Ref)
	}
	if req.Components[0].Ref != " R1 " {
		t.Fatalf("NormalizeRequest mutated input component ref to %q", req.Components[0].Ref)
	}
	if req.Groups[0].Components[0] != " C1 " {
		t.Fatalf("NormalizeRequest mutated input group members: %#v", req.Groups[0].Components)
	}
	if req.Nets[0].Endpoints[0].Ref != " R1 " {
		t.Fatalf("NormalizeRequest mutated input endpoint ref to %q", req.Nets[0].Endpoints[0].Ref)
	}
	got.Components[0].Position.XMM = 99
	if req.Components[0].Position.XMM != 3 {
		t.Fatalf("NormalizeRequest reused input position pointer")
	}
	got.Groups[0].Anchor.At.XMM = 99
	if req.Groups[0].Anchor.At.XMM != 2 {
		t.Fatalf("NormalizeRequest reused input group anchor pointer")
	}
	*got.Components[0].Rotation.FixedDeg = 180
	if *req.Components[0].Rotation.FixedDeg != 90 {
		t.Fatalf("NormalizeRequest reused input fixed rotation pointer")
	}
}

func TestNormalizeRequestTrimsNestedIdentifiers(t *testing.T) {
	req := minimalRequest()
	req.Nets[0].Endpoints[0] = Endpoint{Ref: " R1 ", Pin: " 1 "}
	req.Groups = []Group{{ID: " analog ", Components: []string{" R1 "}, Anchor: GroupAnchor{Ref: " R1 "}}}
	req.Keepouts = []Keepout{{ID: " k1 "}}
	req.Components[0].Pads[0] = PadSummary{Name: " 1 ", Net: " N1 "}

	got := NormalizeRequest(req)
	if got.Nets[0].Endpoints[0] != (Endpoint{Ref: "R1", Pin: "1"}) {
		t.Fatalf("endpoint = %#v, want trimmed ref and pin", got.Nets[0].Endpoints[0])
	}
	if got.Groups[0].ID != "analog" || got.Groups[0].Anchor.Ref != "R1" || got.Groups[0].Components[0] != "R1" {
		t.Fatalf("group = %#v, want trimmed identifiers", got.Groups[0])
	}
	if got.Keepouts[0].ID != "k1" {
		t.Fatalf("keepout id = %q, want trimmed identifier", got.Keepouts[0].ID)
	}
	if got.Components[0].Pads[0].Name != "1" || got.Components[0].Pads[0].Net != "N1" {
		t.Fatalf("pad = %#v, want trimmed name and net", got.Components[0].Pads[0])
	}
}

func TestNormalizeRequestNormalizesAdvancedPlacementRules(t *testing.T) {
	req := minimalRequest()
	req.Nets = append(req.Nets, Net{Name: "VBUS", Role: NetPower, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}}})
	req.AdvancedRules = AdvancedPlacementRules{
		Thermal: []ThermalPlacementRule{{
			Refs:          []string{" R1 ", "r1"},
			Roles:         []string{" regulator "},
			KeepAwayRefs:  []string{" R1 "},
			ThermalRole:   " Heat_Source ",
			PreferredEdge: EdgeRight,
			Severity:      " ERROR ",
			Enforcement:   " Hard ",
		}},
		HighCurrent: []HighCurrentPlacementRule{{
			ID:         " power-path ",
			Nets:       []string{" VBUS "},
			NetRoles:   []NetRole{NetPower, NetPower},
			SourceRefs: []string{" R1 "},
			SinkRefs:   []string{" R1 "},
		}},
		CreepageClearance: []CreepageClearancePlacementRule{{
			ID:             " iso ",
			DomainA:        PlacementRuleDomain{Refs: []string{" R1 "}},
			DomainB:        PlacementRuleDomain{Nets: []string{" VBUS "}},
			MinClearanceMM: 2,
			BoardSides:     []string{" F.Cu ", "F.Cu"},
		}},
		DifferentialPair: []DifferentialPairPlacementRule{{
			ID:          " diff ",
			PositiveNet: " VBUS ",
			NegativeNet: " N1 ",
			SourceRefs:  []string{" R1 "},
			SinkRefs:    []string{" R1 "},
		}},
		ControlledImpedance: []ControlledImpedancePlacementRule{{
			ID:              " hs ",
			Nets:            []string{" VBUS "},
			PreferredLayers: []string{" F.Cu ", "F.Cu"},
			SourceRefs:      []string{" R1 "},
			SinkRefs:        []string{" R1 "},
		}},
	}

	got := NormalizeRequest(req)
	if got.AdvancedRules.Thermal[0].ID != "thermal-001" {
		t.Fatalf("thermal id = %q, want generated stable id", got.AdvancedRules.Thermal[0].ID)
	}
	if got.AdvancedRules.Thermal[0].Severity != AdvancedRuleSeverityError || got.AdvancedRules.Thermal[0].Enforcement != AdvancedRuleHard {
		t.Fatalf("thermal rule defaults = %#v", got.AdvancedRules.Thermal[0])
	}
	if got.AdvancedRules.Thermal[0].ThermalRole != ThermalRoleHeatSource {
		t.Fatalf("thermal role = %q, want heat_source", got.AdvancedRules.Thermal[0].ThermalRole)
	}
	if got.AdvancedRules.Thermal[0].Refs[0] != "R1" || len(got.AdvancedRules.Thermal[0].Refs) != 1 {
		t.Fatalf("thermal refs not normalized: %#v", got.AdvancedRules.Thermal[0].Refs)
	}
	if got.AdvancedRules.HighCurrent[0].ID != "power-path" || len(got.AdvancedRules.HighCurrent[0].NetRoles) != 1 {
		t.Fatalf("high-current rule not normalized: %#v", got.AdvancedRules.HighCurrent[0])
	}
	if got.AdvancedRules.CreepageClearance[0].DomainA.Refs[0] != "R1" || got.AdvancedRules.CreepageClearance[0].DomainB.Nets[0] != "VBUS" {
		t.Fatalf("clearance domains not normalized: %#v", got.AdvancedRules.CreepageClearance[0])
	}
	if got.AdvancedRules.ControlledImpedance[0].PreferredLayers[0] != "F.Cu" || len(got.AdvancedRules.ControlledImpedance[0].PreferredLayers) != 1 {
		t.Fatalf("preferred layers not normalized: %#v", got.AdvancedRules.ControlledImpedance[0].PreferredLayers)
	}
	if req.AdvancedRules.Thermal[0].Refs[0] != " R1 " {
		t.Fatalf("NormalizeRequest mutated input advanced refs: %#v", req.AdvancedRules.Thermal[0].Refs)
	}
}

func TestNormalizeAdvancedPlacementRulesGeneratedIDsAreContentOrdered(t *testing.T) {
	base := minimalRequest()
	base.AdvancedRules.Thermal = []ThermalPlacementRule{
		{Refs: []string{"R2"}, ThermalRole: ThermalRoleHeatSource},
		{Refs: []string{"R1"}, ThermalRole: ThermalRoleThermalSensitive},
	}
	reversed := minimalRequest()
	reversed.AdvancedRules.Thermal = []ThermalPlacementRule{
		{Refs: []string{"R1"}, ThermalRole: ThermalRoleThermalSensitive},
		{Refs: []string{"R2"}, ThermalRole: ThermalRoleHeatSource},
	}

	got := NormalizeRequest(base)
	gotReversed := NormalizeRequest(reversed)
	if len(got.AdvancedRules.Thermal) != 2 || len(gotReversed.AdvancedRules.Thermal) != 2 {
		t.Fatalf("thermal rules missing after normalization: %#v %#v", got.AdvancedRules.Thermal, gotReversed.AdvancedRules.Thermal)
	}
	for i := range got.AdvancedRules.Thermal {
		if got.AdvancedRules.Thermal[i].ID != gotReversed.AdvancedRules.Thermal[i].ID || got.AdvancedRules.Thermal[i].Refs[0] != gotReversed.AdvancedRules.Thermal[i].Refs[0] {
			t.Fatalf("generated rule IDs depend on input order: %#v vs %#v", got.AdvancedRules.Thermal, gotReversed.AdvancedRules.Thermal)
		}
	}
}

func TestSplitPadSelectorUsesRightmostDot(t *testing.T) {
	ref, pad, ok := splitPadSelector("U.SUB.1")
	if !ok || ref != "U.SUB" || pad != "1" {
		t.Fatalf("splitPadSelector = %q %q %v, want U.SUB 1 true", ref, pad, ok)
	}
	if _, _, ok := splitPadSelector("missing-dot"); ok {
		t.Fatal("splitPadSelector accepted malformed selector")
	}
}

func TestAdvancedPlacementRulesJSONRoundTrip(t *testing.T) {
	req := minimalRequest()
	req.AdvancedRules = AdvancedPlacementRules{
		Thermal: []ThermalPlacementRule{{
			ID:            "thermal-u1",
			Refs:          []string{"R1"},
			ThermalRole:   ThermalRoleHeatSource,
			Severity:      AdvancedRuleSeverityError,
			Enforcement:   AdvancedRuleHard,
			MinDistanceMM: 3,
		}},
		ControlledImpedance: []ControlledImpedancePlacementRule{{
			ID:                 "usb-hs",
			Nets:               []string{"N1"},
			PreferredLayers:    []string{"F.Cu"},
			MinCorridorWidthMM: 1.2,
			ReferencePlane:     "In1.Cu",
		}},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if decoded.AdvancedRules.Thermal[0].ID != "thermal-u1" || decoded.AdvancedRules.Thermal[0].ThermalRole != ThermalRoleHeatSource {
		t.Fatalf("thermal rule did not round trip: %#v", decoded.AdvancedRules.Thermal)
	}
	if decoded.AdvancedRules.ControlledImpedance[0].ReferencePlane != "In1.Cu" {
		t.Fatalf("controlled impedance rule did not round trip: %#v", decoded.AdvancedRules.ControlledImpedance)
	}
}

func TestNormalizeRequestHonorsOptionalKeepout(t *testing.T) {
	req := minimalRequest()
	req.Keepouts = []Keepout{{ID: "service-zone", Optional: true}}

	got := NormalizeRequest(req)
	if len(got.Keepouts) != 1 || !got.Keepouts[0].Optional {
		t.Fatalf("optional keepout normalized as required: %#v", got.Keepouts)
	}
}

func TestNormalizeRequestForcesTopWhenBackLayerDisabled(t *testing.T) {
	req := minimalRequest()
	req.Rules.AllowBackLayer = false
	req.Rules.PreferTopLayer = false
	req.Components[0].Side = SideAny

	got := NormalizeRequest(req)
	if got.Components[0].Side != SideTop {
		t.Fatalf("side = %q, want top when back layer is disabled", got.Components[0].Side)
	}
}

func TestNormalizeRequestPreservesAnySideWhenBackLayerAllowed(t *testing.T) {
	req := minimalRequest()
	req.Rules.AllowBackLayer = true
	req.Rules.PreferTopLayer = true
	req.Components[0].Side = SideAny

	got := NormalizeRequest(req)
	if got.Components[0].Side != SideAny {
		t.Fatalf("side = %q, want any when back layer is allowed", got.Components[0].Side)
	}
}

func TestValidateAcceptsMinimalRequest(t *testing.T) {
	issues := Validate(minimalRequest())
	if len(issues) != 0 {
		t.Fatalf("Validate returned issues: %#v", issues)
	}
}

func TestValidateAcceptsFootprintWithoutBounds(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Bounds = Bounds{}

	issues := Validate(req)
	if len(issues) != 0 {
		t.Fatalf("Validate returned issues for footprint-backed component: %#v", issues)
	}
}

func TestValidateRejectsInvalidBoard(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 10
	req.Board.HeightMM = 10
	req.Board.MarginMM = 5

	issues := Validate(req)
	assertIssueContains(t, issues, "board margin leaves no usable placement area")
}

func TestValidateRejectsInvalidKeepoutBounds(t *testing.T) {
	req := minimalRequest()
	req.Keepouts = []Keepout{{ID: "K1", Bounds: Rect{Min: Point{XMM: 4, YMM: 1}, Max: Point{XMM: 3, YMM: 2}}}}

	issues := Validate(req)
	assertIssueContains(t, issues, "keepout bounds min must not exceed max")
}

func TestNormalizeRequestConvertsMechanicalConstraintsToKeepouts(t *testing.T) {
	req := minimalRequest()
	req.Mechanical = []MechanicalConstraint{{
		ID:     " mh1 ",
		Kind:   "mounting_hole",
		Bounds: Rect{Min: Point{XMM: 1, YMM: 1}, Max: Point{XMM: 3, YMM: 3}},
		Layers: []string{"F.Cu"},
	}}

	got := NormalizeRequest(req)
	if len(got.Keepouts) != 1 || got.Keepouts[0].ID != "mh1" || got.Keepouts[0].Reason != "mounting_hole" || got.Keepouts[0].Optional {
		t.Fatalf("mechanical keepout not normalized: %#v", got.Keepouts)
	}
	again := NormalizeRequest(got)
	if len(again.Keepouts) != 1 {
		t.Fatalf("normalizing mechanical keepouts should be idempotent: %#v", again.Keepouts)
	}
}

func TestNormalizeRequestPreservesOptionalMechanicalKeepouts(t *testing.T) {
	req := minimalRequest()
	req.Mechanical = []MechanicalConstraint{{
		ID:       "optional-zone",
		Kind:     "service_area",
		Bounds:   Rect{Min: Point{XMM: 1, YMM: 1}, Max: Point{XMM: 3, YMM: 3}},
		Optional: true,
	}}

	got := NormalizeRequest(NormalizeRequest(req))
	if len(got.Keepouts) != 1 || !got.Keepouts[0].Optional {
		t.Fatalf("optional mechanical keepout not preserved across normalization: %#v", got.Keepouts)
	}
}

func TestValidateMechanicalConstraints(t *testing.T) {
	req := minimalRequest()
	req.Mechanical = []MechanicalConstraint{{
		Optional: true,
		Bounds:   Rect{Min: Point{XMM: 5, YMM: 5}, Max: Point{XMM: 1, YMM: 1}},
	}}

	issues := Validate(NormalizeRequest(req))
	assertIssueContains(t, issues, "mechanical constraint kind required")
	assertIssueContains(t, issues, "mechanical constraint bounds min must not exceed max")
	for _, issue := range issues {
		if strings.Contains(issue.Message, "mechanical constraint bounds") && issue.Severity != reports.SeverityError {
			t.Fatalf("invalid mechanical bounds severity = %q, want error: %#v", issue.Severity, issue)
		}
	}
}

func TestValidateAdvancedPlacementRules(t *testing.T) {
	req := minimalRequest()
	req.AdvancedRules = AdvancedPlacementRules{
		Thermal: []ThermalPlacementRule{{
			ID:            "thermal",
			ThermalRole:   "unknown",
			PreferredEdge: "diagonal",
			Refs:          []string{"MISSING"},
			MinDistanceMM: -1,
			Enforcement:   AdvancedRuleHard,
		}},
		HighCurrent: []HighCurrentPlacementRule{{
			ID:                   "current",
			Nets:                 []string{"MISSING_NET"},
			NetRoles:             []NetRole{"bad_role"},
			SourceRefs:           []string{"MISSING"},
			SourcePads:           []string{"MISSING.1"},
			SinkPads:             []string{"malformed"},
			CurrentEstimateA:     -0.1,
			MaxPreferredLengthMM: -1,
		}},
		CreepageClearance: []CreepageClearancePlacementRule{{
			ID:             "clearance",
			DomainA:        PlacementRuleDomain{Refs: []string{"MISSING"}, NetClasses: []string{"missing-class"}},
			MinClearanceMM: -1,
			MinCreepageMM:  -1,
		}},
		DifferentialPair: []DifferentialPairPlacementRule{{
			ID:          "diff",
			PositiveNet: "N1",
			NegativeNet: "N1",
			SourceRefs:  []string{"MISSING"},
			SourcePads:  []string{"R1.99"},
			MaxSkewMM:   -1,
		}},
		ControlledImpedance: []ControlledImpedancePlacementRule{{
			ID:                    "impedance",
			Nets:                  []string{"MISSING_NET"},
			NetRoles:              []NetRole{"bad_role"},
			SourceRefs:            []string{"MISSING"},
			SourcePads:            []string{"MISSING.1"},
			MinCorridorWidthMM:    -1,
			MaxViaCountPreference: -1,
		}},
	}

	issues := Validate(req)
	assertIssueContains(t, issues, "invalid thermal role")
	assertIssueContains(t, issues, "invalid preferred edge")
	assertIssueContains(t, issues, "thermal min distance must be non-negative")
	assertIssueContains(t, issues, "advanced placement rule references unknown net MISSING_NET")
	assertIssueContains(t, issues, "advanced placement rule references unknown net class missing-class")
	assertIssueContains(t, issues, "advanced placement rule pad selector references unknown component MISSING")
	assertIssueContains(t, issues, "advanced placement rule pad selector must use Ref.Pad format: malformed")
	assertIssueContains(t, issues, "advanced placement rule pad selector references unknown pad R1.99")
	assertIssueContains(t, issues, "invalid net role bad_role")
	assertIssueContains(t, issues, "clearance rule domain B required")
	assertIssueContains(t, issues, "differential-pair nets must be distinct")
	assertIssueContains(t, issues, "controlled-impedance corridor width must be non-negative")
}

func TestValidateAdvancedPlacementRuleSeverityFollowsEnforcement(t *testing.T) {
	req := minimalRequest()
	req.AdvancedRules.HighCurrent = []HighCurrentPlacementRule{{
		ID:          "soft-current",
		Enforcement: AdvancedRuleSoft,
	}}
	issues := Validate(req)
	assertIssueContains(t, issues, "high-current rule requires nets or net roles")
	for _, issue := range issues {
		if strings.Contains(issue.Message, "high-current rule requires nets") && issue.Severity != reports.SeverityWarning {
			t.Fatalf("soft advanced rule severity = %q, want warning: %#v", issue.Severity, issue)
		}
	}

	req.AdvancedRules.HighCurrent[0].Enforcement = AdvancedRuleHard
	issues = Validate(req)
	for _, issue := range issues {
		if strings.Contains(issue.Message, "high-current rule requires nets") && issue.Severity != reports.SeverityError {
			t.Fatalf("hard advanced rule severity = %q, want error: %#v", issue.Severity, issue)
		}
	}
}

func TestValidateAdvancedThermalPreferredRegion(t *testing.T) {
	req := minimalRequest()
	req.AdvancedRules.Thermal = []ThermalPlacementRule{{
		ID:              "thermal",
		Refs:            []string{"R1"},
		PreferredRegion: "power",
	}}
	if issues := Validate(req); len(issues) == 0 {
		t.Fatal("expected unknown preferred region issue")
	} else {
		assertIssueContains(t, issues, "thermal preferred region references unknown region power")
	}

	req.RegionRules = []RegionRule{{ID: "power-region", Region: "power", Refs: []string{"R1"}}}
	if issues := Validate(req); len(issues) != 0 {
		t.Fatalf("Validate returned issues for declared preferred region: %#v", issues)
	}
}

func TestValidateAdvancedPlacementRulesAcceptsCompleteRules(t *testing.T) {
	req := minimalRequest()
	req.Nets = []Net{
		{Name: "N1", Role: NetSignal, WidthClass: "signal", Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}}},
		{Name: "N2", Role: NetSignal, Endpoints: []Endpoint{{Ref: "R1", Pin: "2"}}},
		{Name: "VBUS", Role: NetPower, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}}},
	}
	req.AdvancedRules = AdvancedPlacementRules{
		Thermal: []ThermalPlacementRule{{
			ID:            "thermal",
			Refs:          []string{"R1"},
			KeepAwayRefs:  []string{"R1"},
			ThermalRole:   ThermalRoleRegulator,
			MinDistanceMM: 1,
		}},
		HighCurrent: []HighCurrentPlacementRule{{
			ID:         "current",
			Nets:       []string{"VBUS"},
			SourceRefs: []string{"R1"},
			SinkRefs:   []string{"R1"},
			SourcePads: []string{"R1.1"},
			SinkPads:   []string{"R1.2"},
		}},
		CreepageClearance: []CreepageClearancePlacementRule{{
			ID:             "clearance",
			DomainA:        PlacementRuleDomain{Refs: []string{"R1"}, NetClasses: []string{"signal"}},
			DomainB:        PlacementRuleDomain{Nets: []string{"VBUS"}},
			MinClearanceMM: 2,
		}},
		DifferentialPair: []DifferentialPairPlacementRule{{
			ID:          "diff",
			PositiveNet: "N1",
			NegativeNet: "N2",
			SourceRefs:  []string{"R1"},
			SinkRefs:    []string{"R1"},
			SourcePads:  []string{"R1.1"},
			SinkPads:    []string{"R1.2"},
		}},
		ControlledImpedance: []ControlledImpedancePlacementRule{{
			ID:         "impedance",
			Nets:       []string{"N1"},
			SourceRefs: []string{"R1"},
			SinkRefs:   []string{"R1"},
			SourcePads: []string{"R1.1"},
			SinkPads:   []string{"R1.2"},
		}},
	}

	if issues := Validate(req); len(issues) != 0 {
		t.Fatalf("Validate returned issues for complete advanced rules: %#v", issues)
	}
}

func TestValidateRejectsDuplicateReferences(t *testing.T) {
	req := minimalRequest()
	duplicate := req.Components[0]
	duplicate.Ref = "r1"
	req.Components = append(req.Components, duplicate)

	issues := Validate(req)
	assertIssueContains(t, issues, "duplicate component reference")
}

func TestValidateRejectsPartialBounds(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Bounds = Bounds{WidthMM: 2}

	issues := Validate(req)
	assertIssueContains(t, issues, "component bounds must be positive when provided")
}

func TestValidateRejectsUnknownGroupComponent(t *testing.T) {
	req := minimalRequest()
	req.Groups = []Group{{ID: "power", Components: []string{"R1", "C99"}}}

	issues := Validate(req)
	assertIssueContains(t, issues, "group references unknown component C99")
}

func TestValidateRejectsDuplicateGroupIDs(t *testing.T) {
	req := minimalRequest()
	req.Groups = []Group{
		{ID: "analog", Components: []string{"R1"}},
		{ID: "ANALOG", Components: []string{"R1"}},
	}

	issues := Validate(req)
	assertIssueContains(t, issues, "duplicate group ID")
}

func TestValidateRejectsConflictingComponentGroupID(t *testing.T) {
	req := minimalRequest()
	req.Components[0].GroupID = "power"
	req.Groups = []Group{{ID: "analog", Components: []string{"R1"}, TranslateAsUnit: true, Anchor: GroupAnchor{Ref: "R1"}}}

	issues := Validate(req)
	assertIssueContains(t, issues, "has group ID power but is listed in group analog")
}

func TestValidateAllowsOverlappingNonRigidGroupMembership(t *testing.T) {
	req := minimalRequest()
	req.Components[0].GroupID = "power"
	req.Groups = []Group{
		{ID: "power", Components: []string{"R1"}, KeepTogether: true},
		{ID: "analog", Components: []string{"R1"}, KeepTogether: true},
	}

	if issues := Validate(req); len(issues) != 0 {
		t.Fatalf("non-rigid overlapping membership issues = %#v", issues)
	}
}

func TestValidateRejectsUnknownNetEndpoint(t *testing.T) {
	req := minimalRequest()
	req.Nets = []Net{{Name: "N1", Endpoints: []Endpoint{{Ref: "R99", Pin: "1"}}}}

	issues := Validate(req)
	assertIssueContains(t, issues, "net endpoint references unknown component R99")
}

func TestValidateRejectsUnknownEndpointPinWhenPadsKnown(t *testing.T) {
	req := minimalRequest()
	req.Nets = []Net{{Name: "N1", Endpoints: []Endpoint{{Ref: "R1", Pin: "3"}}}}

	issues := Validate(req)
	assertIssueContains(t, issues, "pin 3 not found in component R1")
}

func TestValidateRejectsDuplicateNetNames(t *testing.T) {
	req := minimalRequest()
	req.Nets = []Net{
		{Name: "N1", Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}}},
		{Name: "n1", Endpoints: []Endpoint{{Ref: "R1", Pin: "2"}}},
	}

	issues := Validate(req)
	assertIssueContains(t, issues, "duplicate net name")
}

func TestValidateRejectsBottomWithoutBackLayer(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Side = SideBottom
	req.Rules.AllowBackLayer = false

	issues := Validate(req)
	assertIssueContains(t, issues, "bottom placement requires AllowBackLayer")
}

func TestValidateRejectsInvalidEnumValues(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Side = SideConstraint("front")
	req.Components[0].Edge = EdgeConstraint("middle")
	req.Components[0].Bounds.Source = BoundsSource("guessed")
	req.Nets[0].Role = NetRole("fast")

	issues := Validate(req)
	assertIssueContains(t, issues, "invalid side constraint front")
	assertIssueContains(t, issues, "invalid edge constraint middle")
	assertIssueContains(t, issues, "invalid bounds source guessed")
	assertIssueContains(t, issues, "invalid net role fast")
}

func TestValidateRejectsInvalidRotation(t *testing.T) {
	req := minimalRequest()
	rotation := 45.0
	req.Components[0].Rotation.FixedDeg = &rotation

	issues := Validate(req)
	assertIssueContains(t, issues, "fixed rotation must be one of")
}

func TestValidateAcceptsEquivalentRightAngleRotations(t *testing.T) {
	req := minimalRequest()
	fixed := -90.0
	req.Components[0].Rotation.FixedDeg = &fixed
	req.Components[0].Rotation.AllowedDeg = []float64{0, 360, 450}

	issues := Validate(req)
	if len(issues) != 0 {
		t.Fatalf("Validate returned issues for equivalent right-angle rotations: %#v", issues)
	}
}

func TestNormalizeRequestNormalizesIntentRules(t *testing.T) {
	req := minimalRequest()
	req.Components = append(req.Components, Component{
		Ref:         " C1 ",
		FootprintID: "Capacitor_SMD:C_0805_2012Metric",
		Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
	})
	req.ProximityRules = []ProximityRule{{
		Role:       IntentDecoupling,
		AnchorRef:  " R1 ",
		TargetRefs: []string{" C1 ", "c1"},
		AnchorPins: []string{" 1 ", "1"},
		TargetPins: []string{" 2 "},
	}}
	req.RegionRules = []RegionRule{{
		Region:   " analog ",
		Refs:     []string{" C1 ", "R1", "r1"},
		NetRoles: []NetRole{NetAnalog},
		Preferred: Rect{
			Min: Point{XMM: 0, YMM: 0},
			Max: Point{XMM: 10, YMM: 10},
		},
	}}

	got := NormalizeRequest(req)
	if got.ProximityRules[0].ID != "proximity-001" || got.ProximityRules[0].AnchorRef != "R1" {
		t.Fatalf("proximity rule not normalized: %#v", got.ProximityRules[0])
	}
	if got.ProximityRules[0].Weight != 1 || len(got.ProximityRules[0].TargetRefs) != 1 || got.ProximityRules[0].TargetRefs[0] != "C1" {
		t.Fatalf("proximity rule targets/weight not normalized: %#v", got.ProximityRules[0])
	}
	if got.RegionRules[0].ID != "region-001" || got.RegionRules[0].Region != "analog" {
		t.Fatalf("region rule not normalized: %#v", got.RegionRules[0])
	}
	if len(got.RegionRules[0].Refs) != 2 {
		t.Fatalf("region refs not deduplicated: %#v", got.RegionRules[0].Refs)
	}
}

func TestValidateIntentRules(t *testing.T) {
	req := minimalRequest()
	req.Components = append(req.Components, Component{
		Ref:         "C1",
		FootprintID: "Capacitor_SMD:C_0805_2012Metric",
		Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
	})
	req.ProximityRules = []ProximityRule{{
		ID:            "decouple",
		Role:          IntentDecoupling,
		AnchorRef:     "R1",
		TargetRefs:    []string{"C1"},
		MaxDistanceMM: 5,
		Required:      true,
	}}
	req.RegionRules = []RegionRule{{
		ID:       "analog",
		Region:   "analog",
		Refs:     []string{"R1"},
		NetRoles: []NetRole{NetAnalog},
		Preferred: Rect{
			Min: Point{XMM: 0, YMM: 0},
			Max: Point{XMM: 10, YMM: 10},
		},
		Required: true,
	}}

	issues := Validate(NormalizeRequest(req))
	if len(issues) != 0 {
		t.Fatalf("Validate returned intent rule issues: %#v", issues)
	}
}

func TestValidateRequiredIntentRuleBlocksUnknownRefs(t *testing.T) {
	req := minimalRequest()
	req.ProximityRules = []ProximityRule{{
		ID:         "bad",
		Role:       IntentDecoupling,
		AnchorRef:  "U99",
		TargetRefs: []string{"C99"},
		Required:   true,
	}}

	issues := Validate(NormalizeRequest(req))
	assertIssueContains(t, issues, "proximity rule anchor references unknown component U99")
	assertIssueContains(t, issues, "proximity rule target references unknown component C99")
	for _, issue := range issues {
		if strings.Contains(issue.Message, "proximity rule") && issue.Severity != reports.SeverityError {
			t.Fatalf("required proximity issue severity = %q, want error: %#v", issue.Severity, issue)
		}
	}
}

func TestValidateProximityRulePins(t *testing.T) {
	req := minimalRequest()
	req.Components = append(req.Components, Component{
		Ref:         "C1",
		FootprintID: "Capacitor_SMD:C_0805_2012Metric",
		Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
		Pads:        []PadSummary{{Name: "1"}, {Name: "2"}},
	})
	req.ProximityRules = []ProximityRule{{
		ID:         "bad-pins",
		Role:       IntentDecoupling,
		AnchorRef:  "R1",
		TargetRefs: []string{"C1"},
		AnchorPins: []string{"9"},
		TargetPins: []string{"8"},
		Required:   true,
	}}

	issues := Validate(NormalizeRequest(req))
	assertIssueContains(t, issues, "proximity rule anchor pin 9 not found in component R1")
	assertIssueContains(t, issues, "proximity rule target pin 8 not found in component C1")
}

func TestValidateOptionalIntentRuleWarns(t *testing.T) {
	req := minimalRequest()
	req.RegionRules = []RegionRule{{
		ID:       "optional-region",
		Region:   "analog",
		Refs:     []string{"C99"},
		Required: false,
	}}

	issues := Validate(NormalizeRequest(req))
	assertIssueContains(t, issues, "region rule references unknown component C99")
	for _, issue := range issues {
		if strings.Contains(issue.Message, "region rule") && issue.Severity != reports.SeverityWarning {
			t.Fatalf("optional region issue severity = %q, want warning: %#v", issue.Severity, issue)
		}
	}
}

func minimalRequest() Request {
	return Request{
		Board: BoardPlacementArea{WidthMM: 40, HeightMM: 25, MarginMM: 1},
		Components: []Component{{
			Ref:         "R1",
			FootprintID: "Resistor_SMD:R_0805_2012Metric",
			Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
			Pads:        []PadSummary{{Name: "1"}, {Name: "2"}},
		}},
		Nets: []Net{{Name: "N1", Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}}}},
	}
}

func assertIssueContains(t *testing.T, issues []reports.Issue, want string) {
	t.Helper()
	for _, issue := range issues {
		if strings.Contains(issue.Message, want) {
			return
		}
	}
	t.Fatalf("missing issue containing %q in %#v", want, issues)
}
