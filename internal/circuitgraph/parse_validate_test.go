package circuitgraph

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestDecodeStrictValidGraph(t *testing.T) {
	document := validTestDocument()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	if decoded.Schema != SchemaID || decoded.Version != Version || decoded.Project.Name != "test_graph" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestDecodeStrictRejectsUnknownAndTrailingJSON(t *testing.T) {
	data, err := json.Marshal(validTestDocument())
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(data, []byte(`"version":1`), []byte(`"version":1,"unknown":true`), 1)
	if _, issues := DecodeStrict(bytes.NewReader(unknown)); len(issues) != 1 || !strings.Contains(issues[0].Message, "unknown field") {
		t.Fatalf("unknown issues = %#v", issues)
	}
	trailing := append(append([]byte(nil), data...), []byte(` {}`)...)
	if _, issues := DecodeStrict(bytes.NewReader(trailing)); len(issues) != 1 || !strings.Contains(issues[0].Message, "trailing") {
		t.Fatalf("trailing issues = %#v", issues)
	}
}

func TestDecodeStrictRejectsOversizedDocument(t *testing.T) {
	data := bytes.Repeat([]byte(" "), MaxDocumentBytes+1)
	if _, issues := DecodeStrict(bytes.NewReader(data)); len(issues) != 1 || issues[0].Code != CodeLimitExceeded {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestNormalizeIsDeterministicAndDoesNotMutateCaller(t *testing.T) {
	document := validTestDocument()
	document.Components[0], document.Components[1] = document.Components[1], document.Components[0]
	document.Nets[0].Endpoints[0], document.Nets[0].Endpoints[1] = document.Nets[0].Endpoints[1], document.Nets[0].Endpoints[0]
	document.PowerFlags = []PowerFlag{{Net: "PWR"}, {Net: "GND"}}
	document.Nets = append(document.Nets, Net{Name: "PWR", Role: NetRolePowerPos, Required: document.Nets[0].Required, Endpoints: append([]Endpoint(nil), document.Nets[0].Endpoints...)})
	document.Nets[2].Endpoints[0].Selector = "3"
	document.Nets[2].Endpoints[1].Selector = "3"
	original, _ := json.Marshal(document)
	first := Normalize(document)
	second := Normalize(first)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("normalization is not idempotent\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.Components[0].ID != "j1" || first.Components[1].ID != "r1" {
		t.Fatalf("component order = %#v", first.Components)
	}
	if got := []string{first.PowerFlags[0].Net, first.PowerFlags[1].Net}; !reflect.DeepEqual(got, []string{"GND", "PWR"}) {
		t.Fatalf("power flag order = %#v", got)
	}
	after, _ := json.Marshal(document)
	if !bytes.Equal(original, after) {
		t.Fatal("Normalize mutated caller")
	}
}

func TestValidateRejectsUnsafeGraphCases(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Document)
		code string
		path string
	}{
		{name: "project traversal", edit: func(document *Document) { document.Project.Name = "../bad" }, code: string(CodeSchemaInvalid), path: "project.name"},
		{name: "invalid layers", edit: func(document *Document) { document.Project.Board.Layers = 3 }, code: string(CodeSchemaInvalid), path: "project.board.layers"},
		{name: "duplicate component", edit: func(document *Document) { document.Components[1].ID = document.Components[0].ID }, code: string(CodeComponentDuplicate), path: "components[1].id"},
		{name: "selection union", edit: func(document *Document) { document.Components[0].Query = &ComponentQuery{Family: "connector"} }, code: string(CodeComponentSelectionInvalid), path: "components[0]"},
		{name: "unknown endpoint", edit: func(document *Document) { document.Nets[0].Endpoints[0].Component = "missing" }, code: string(CodeNetInvalid), path: "nets[0].endpoints[0].component"},
		{name: "duplicate endpoint", edit: func(document *Document) { document.Nets[1].Endpoints[0] = document.Nets[0].Endpoints[0] }, code: string(CodeEndpointDuplicate), path: "nets[1].endpoints[0]"},
		{name: "connected no-connect", edit: func(document *Document) { document.NoConnects = []Endpoint{document.Nets[0].Endpoints[0]} }, code: string(CodeEndpointDuplicate), path: "no_connects[0]"},
		{name: "unknown power flag net", edit: func(document *Document) { document.PowerFlags = []PowerFlag{{Net: "MISSING"}} }, code: string(CodePowerFlagInvalid), path: "power_flags[0].net"},
		{name: "empty power flag net", edit: func(document *Document) { document.PowerFlags = []PowerFlag{{}} }, code: string(CodePowerFlagInvalid), path: "power_flags[0].net"},
		{name: "duplicate power flag", edit: func(document *Document) { document.PowerFlags = []PowerFlag{{Net: "GND"}, {Net: "GND"}} }, code: string(CodePowerFlagInvalid), path: "power_flags[1].net"},
		{name: "signal power flag", edit: func(document *Document) { document.PowerFlags = []PowerFlag{{Net: "IN"}} }, code: string(CodePowerFlagInvalid), path: "power_flags[0].net"},
		{name: "too many power flags", edit: func(document *Document) { document.PowerFlags = make([]PowerFlag, MaxPowerFlags+1) }, code: string(CodeLimitExceeded), path: "power_flags"},
		{name: "reserved power flag reference", edit: func(document *Document) { document.Components[0].Reference = "#FLG01" }, code: string(CodePowerFlagInvalid), path: "components[0].reference"},
		{name: "unknown group member", edit: func(document *Document) {
			document.Schematic.Groups[0].Members = append(document.Schematic.Groups[0].Members, "missing")
		}, code: string(CodeLayoutUnsupported), path: "schematic.groups[0].members[2]"},
		{name: "region out of bounds", edit: func(document *Document) { document.PCB.Regions[0].Bounds.XMM = 39 }, code: string(CodePCBConstraintInvalid), path: "pcb.regions[0].bounds"},
		{name: "implicit policy", edit: func(document *Document) { document.Policy.AllowRouteRetry = nil }, code: string(CodeRepairForbidden), path: "policy.allow_route_retry"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := validTestDocument()
			test.edit(&document)
			issues := Validate(document)
			if !hasIssue(issues, test.code, test.path) {
				t.Fatalf("issues = %#v, want code=%s path=%s", issues, test.code, test.path)
			}
		})
	}
}

func TestParameterValueRejectsStructuredObjectsAndMixedArrays(t *testing.T) {
	for _, input := range []string{`{"x":1}`, `["ok",1]`, `null`} {
		var value ParameterValue
		if err := json.Unmarshal([]byte(input), &value); err == nil {
			t.Fatalf("input %s unexpectedly decoded as %#v", input, value)
		}
	}
	for _, input := range []string{`"10k"`, `5`, `true`, `["F.Cu","B.Cu"]`} {
		var value ParameterValue
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Fatalf("input %s: %v", input, err)
		}
	}
}

func TestValidateEndpointOwnershipIncludesSymbolUnit(t *testing.T) {
	document := validTestDocument()
	document.Nets[0].Endpoints[0] = Endpoint{Component: "j1", Unit: "A", SelectorKind: SelectorSymbolPin, Selector: "1"}
	document.Nets[1].Endpoints[0] = Endpoint{Component: "j1", Unit: "B", SelectorKind: SelectorSymbolPin, Selector: "1"}
	for _, issue := range Validate(document) {
		if issue.Code == CodeEndpointDuplicate {
			t.Fatalf("distinct symbol units collided: %#v", issue)
		}
	}
}

func hasIssue(issues []reports.Issue, code, path string) bool {
	for _, issue := range issues {
		if string(issue.Code) == code && issue.Path == path {
			return true
		}
	}
	return false
}

func validTestDocument() Document {
	trueValue := true
	falseValue := false
	return Document{
		Schema:  SchemaID,
		Version: Version,
		Project: Project{
			Name: "test_graph", Title: "Test Graph", Description: "Strict graph fixture",
			Acceptance: AcceptanceStructural,
			Board:      Board{WidthMM: 40, HeightMM: 25, Layers: 2, EdgeClearanceMM: 0.25},
		},
		Components: []Component{
			{ID: "j1", Reference: "J1", Role: RoleInputConnector, ComponentID: "connector.pinheader.1x02.2_54mm", VariantID: "vertical", Population: PopulationPopulate},
			{ID: "r1", Reference: "R1", Role: RoleResistor, Query: &ComponentQuery{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k", MinimumConfidence: "library_derived"}, Value: "10k", Population: PopulationPopulate},
		},
		Nets: []Net{
			{Name: "IN", Role: NetRoleSignal, Required: &trueValue, Endpoints: []Endpoint{{Component: "j1", SelectorKind: SelectorSymbolPin, Selector: "1"}, {Component: "r1", SelectorKind: SelectorSymbolPin, Selector: "1"}}},
			{Name: "GND", Role: NetRoleGround, Required: &trueValue, Endpoints: []Endpoint{{Component: "j1", SelectorKind: SelectorSymbolPin, Selector: "2"}, {Component: "r1", SelectorKind: SelectorSymbolPin, Selector: "2"}}},
		},
		NoConnects: []Endpoint{}, PowerFlags: []PowerFlag{}, Buses: []Bus{},
		Schematic: SchematicIntent{
			Flow: FlowLeftToRight, Origin: OriginCentered,
			Groups:     []SchematicGroup{{ID: "signal", Role: "processing_stage", Members: []string{"j1", "r1"}, Rank: 0}},
			Lanes:      SchematicLanes{Power: LaneTop, Signals: LaneMiddle, Ground: LaneBottom},
			Placements: []SchematicPlacement{{Component: "j1", Group: "signal"}, {Component: "r1", Group: "signal", RightOf: "j1"}},
			Rules:      SchematicRules{PositivePowerTop: &trueValue, GroundBottom: &trueValue, CenterOnPage: &trueValue, PreferLabelsForLongNets: &trueValue, AvoidWireCrossings: &trueValue, MinGroupSpacingMM: 12.7, MinComponentSpacingMM: 7.62},
			Hierarchy:  HierarchyPolicy{Mode: "flat"},
		},
		PCB: PCBIntent{
			Regions:    []PCBRegion{{ID: "main", Bounds: Bounds{XMM: 2, YMM: 2, WidthMM: 36, HeightMM: 21}}},
			Placements: []PCBPlacement{{Component: "j1", Region: "main", Edge: SideLeft}, {Component: "r1", Region: "main", Near: "j1", MaxDistanceMM: 15}},
			Keepouts:   []PCBKeepout{}, Zones: []PCBZone{},
		},
		Policy: Policy{AllowReferenceAssignment: &trueValue, AllowValueNormalization: &trueValue, AllowLayoutInference: &trueValue, AllowSpacingAdjustment: &trueValue, AllowLabelInsertion: &trueValue, AllowPlacementAdjustment: &trueValue, AllowRouteRetry: &falseValue},
	}
}
