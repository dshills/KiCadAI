package placement

import (
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
	req.Groups = []Group{{ID: "analog", Components: []string{"R1"}}}

	issues := Validate(req)
	assertIssueContains(t, issues, "has group ID power but is listed in group analog")
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
