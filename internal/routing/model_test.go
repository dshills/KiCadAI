package routing

import (
	"math"
	"testing"

	"kicadai/internal/pcbrules"
	"kicadai/internal/reports"
)

func TestNormalizeRequestAppliesDefaults(t *testing.T) {
	request := minimalRequest()
	request.Rules = Rules{}
	request.Strategy = Strategy{}
	request.Board.Layers = nil

	NormalizeRequest(&request)
	if request.Rules.GridMM != 0.25 {
		t.Fatalf("grid = %f, want default", request.Rules.GridMM)
	}
	if request.Rules.TraceWidthMM != 0.25 {
		t.Fatalf("trace width = %f, want default", request.Rules.TraceWidthMM)
	}
	if request.Rules.AllowVias == nil || !*request.Rules.AllowVias {
		t.Fatalf("AllowVias = false, want default true")
	}
	if request.Strategy.Mode != ModeTwoLayer {
		t.Fatalf("mode = %s, want two_layer", request.Strategy.Mode)
	}
	if len(request.Board.Layers) != 2 {
		t.Fatalf("layers = %d, want default two copper layers", len(request.Board.Layers))
	}
}

func TestNormalizeRequestPreservesTwoLayerWhenViasDisabled(t *testing.T) {
	request := minimalRequest()
	disabled := false
	request.Rules.AllowVias = &disabled
	request.Rules.AllowBackLayer = boolPtr(true)
	request.Strategy.Mode = ModeTwoLayer

	NormalizeRequest(&request)
	if request.Strategy.Mode != ModeTwoLayer {
		t.Fatalf("mode = %s, want two_layer when only vias are disabled", request.Strategy.Mode)
	}
	if request.Rules.AllowVias == nil || *request.Rules.AllowVias {
		t.Fatalf("AllowVias = %#v, want false", request.Rules.AllowVias)
	}
}

func TestValidateAcceptsMinimalRequest(t *testing.T) {
	request := minimalRequest()
	NormalizeRequest(&request)
	if issues := Validate(&request); len(issues) != 0 {
		t.Fatalf("Validate returned issues: %#v", issues)
	}
}

func TestValidateRejectsInvalidRules(t *testing.T) {
	request := minimalRequest()
	request.Board.WidthMM = 0
	request.Rules.GridMM = -1
	request.Rules.ViaDiameterMM = 0.3
	request.Rules.ViaDrillMM = 0.3

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "board.width_mm")
	assertIssuePath(t, issues, "rules.grid_mm")
	assertIssuePath(t, issues, "rules.via_drill_mm")
}

func TestValidateRejectsUnsupportedRipupRetryLimit(t *testing.T) {
	request := minimalRequest()
	request.Strategy.RipupRetryLimit = 1

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssueCode(t, issues, reports.CodeUnsupportedOperation)
	assertIssuePath(t, issues, "strategy.ripup_retry_limit")
}

func TestValidateRejectsDuplicateReferencesAndPads(t *testing.T) {
	request := minimalRequest()
	request.Components = append(request.Components, request.Components[0])
	duplicate := request.Components[0].Pads[0]
	duplicate.Net = "OTHER"
	request.Components[0].Pads = append(request.Components[0].Pads, duplicate)

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssueCode(t, issues, reports.CodeDuplicateReference)
	assertIssuePath(t, issues, "components[0].pads[1].name")
}

func TestValidateAllowsSameNetDuplicatePadAlias(t *testing.T) {
	request := minimalRequest()
	duplicate := request.Components[0].Pads[0]
	duplicate.Position = Point{XMM: 1.5, YMM: 0}
	request.Components[0].Pads = append(request.Components[0].Pads, duplicate)

	NormalizeRequest(&request)
	issues := Validate(&request)
	for _, issue := range issues {
		if issue.Path == "components[0].pads[1].name" {
			t.Fatalf("same-net duplicate pad alias should not fail validation: %#v", issues)
		}
	}
}

func TestValidateRejectsUnknownEndpointPad(t *testing.T) {
	request := minimalRequest()
	request.Nets[0].Endpoints[1].Pin = "99"

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "nets[0].endpoints[1]")
}

func TestValidateRejectsUnknownLayers(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Position.Layer = "In1.Cu"
	request.Components[0].Pads[0].Layers = []string{"Nope.Cu"}

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "components[0].position.layer")
	assertIssuePath(t, issues, "components[0].pads[0].layers[0]")
}

func TestValidateRejectsInvalidThroughHoleDrill(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Pads[0].Drill = nil

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "components[0].pads[0].drill")
}

func TestValidateRejectsInvalidObstacleShape(t *testing.T) {
	request := minimalRequest()
	request.Obstacles = []Obstacle{{Kind: ObstacleKeepout, Layer: "F.Cu"}}
	request.Existing = []ExistingCopper{{Kind: CopperSegment, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 2}, Max: Point{XMM: 1}}}}}

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "obstacles[0].geometry")
	assertIssuePath(t, issues, "existing[0].geometry.rect")
}

func TestValidateRejectsDuplicateNetAndUnknownNetClass(t *testing.T) {
	request := minimalRequest()
	request.Nets = append(request.Nets, request.Nets[0])
	request.Nets[0].Class = "fast"

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "nets[0].class")
	assertIssuePath(t, issues, "nets[1].name")
}

func TestValidateRejectsNonFiniteCoordinates(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Position.XMM = math.NaN()
	request.Components[0].Pads[0].Position.YMM = math.Inf(1)

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "components[0].position")
	assertIssuePath(t, issues, "components[0].pads[0].position")
}

func TestValidateRejectsInvalidNetClassParameters(t *testing.T) {
	request := minimalRequest()
	request.Rules.NetClasses = map[string]NetClass{
		"bad": {TraceWidthMM: -1, ViaDiameterMM: 0.4, ViaDrillMM: 0.4},
	}
	request.Nets[0].Class = "bad"

	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "rules.net_classes[bad].trace_width_mm")
	assertIssuePath(t, issues, "rules.net_classes[bad].via_drill_mm")
}

func TestResolveNetRuleFromSetUsesOverrideWithoutRebuildingRequest(t *testing.T) {
	request := minimalRequest()
	request.Rules.NetClasses = map[string]NetClass{
		"POWER": {TraceWidthMM: 0.5},
	}
	request.Rules.NetOverrides = map[string]NetRule{
		"SIG": {ClassName: "POWER", MaxViasPerNet: 1},
	}
	NormalizeRequest(&request)

	rule, issues := ResolveNetRuleFromSet(toPCBRules(request.Rules, request.Strategy), request.Nets[0])
	if len(issues) != 0 {
		t.Fatalf("ResolveNetRuleFromSet issues = %#v", issues)
	}
	if rule.ClassName != "POWER" || rule.TraceWidthMM != 0.5 || rule.MaxViasPerNet != 1 {
		t.Fatalf("resolved rule = %#v", rule)
	}
	if pcbrules.PairKey("B", "A") != "A|B" {
		t.Fatalf("PairKey normalization changed")
	}
}

func TestValidateRejectsInvalidDifferentialPairMetadata(t *testing.T) {
	request := minimalRequest()
	request.Rules.DifferentialPairs = []pcbrules.CoupledNetGroup{{
		ID:      "USB",
		Mode:    pcbrules.DifferentialPairMode,
		Members: []string{"USB_P"},
	}}
	NormalizeRequest(&request)
	issues := Validate(&request)
	assertIssuePath(t, issues, "rules.differential_pairs[0].members")
}

func minimalRequest() Request {
	return Request{
		Board: Board{
			WidthMM:  30,
			HeightMM: 20,
			Layers: []Layer{
				{Name: "F.Cu", Kind: LayerCopper, Routable: true},
				{Name: "B.Cu", Kind: LayerCopper, Routable: true},
			},
		},
		Components: []Component{
			{
				Ref:      "J1",
				Position: Placement{XMM: 5, YMM: 10, Layer: "F.Cu"},
				Pads: []Pad{{
					Ref:      "J1",
					Name:     "1",
					Net:      "SIG",
					Position: Point{},
					Shape:    PadCircle,
					Type:     PadThroughHole,
					Size:     Size{WidthMM: 1, HeightMM: 1},
					Drill:    &Drill{DiameterMM: 0.5},
					Layers:   []string{"F.Cu", "B.Cu"},
				}},
			},
			{
				Ref:      "J2",
				Position: Placement{XMM: 20, YMM: 10, Layer: "F.Cu"},
				Pads: []Pad{{
					Ref:      "J2",
					Name:     "1",
					Net:      "SIG",
					Position: Point{},
					Shape:    PadCircle,
					Type:     PadThroughHole,
					Size:     Size{WidthMM: 1, HeightMM: 1},
					Drill:    &Drill{DiameterMM: 0.5},
					Layers:   []string{"F.Cu", "B.Cu"},
				}},
			},
		},
		Nets: []Net{{
			Name: "SIG",
			Endpoints: []Endpoint{
				{Ref: "J1", Pin: "1"},
				{Ref: "J2", Pin: "1"},
			},
		}},
		Rules:    DefaultRules(),
		Strategy: Strategy{Mode: ModeTwoLayer, TreatZonesAs: ZoneObstacle},
	}
}

func assertIssuePath(t *testing.T, issues []reports.Issue, path string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Path == path {
			return
		}
	}
	t.Fatalf("missing issue path %q in %#v", path, issues)
}

func assertIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("missing issue code %q in %#v", code, issues)
}
