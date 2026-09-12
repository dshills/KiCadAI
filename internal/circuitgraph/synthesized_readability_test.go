package circuitgraph

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
)

func TestSynthesizedSchematicLayoutDoesNotPinEveryComponentToOneRank(t *testing.T) {
	document := Document{Components: []Component{{ID: "source", Role: RoleInputConnector}, {ID: "supply", Role: RoleRegulator}, {ID: "load", Role: RoleOutputConnector}}}
	intent := FunctionIntent{Constraints: SynthesisConstraints{MaxWidthMM: 100, MaxHeightMM: 100, PreferredComponentSpacingMM: 1}}
	if issues := deriveFunctionLayout(&document, intent, nil, nil); len(issues) != 0 {
		t.Fatal(issues)
	}
	if len(document.Schematic.Groups) != 1 || len(document.Schematic.Placements) != 3 {
		t.Fatal("legacy synthesis must remain unchanged until topology-v1 is selected")
	}
	applySynthesizedTopologyLayout(&document)
	if len(document.Schematic.Groups) != 0 || len(document.Schematic.Placements) != 0 {
		t.Error("synthesis must leave schematic groups and orientations to role/topology inference")
	}
	defaults := schematiclayout.DefaultRules(schematiclayout.ProfileStandard)
	if kicadfiles.MM(document.Schematic.Rules.MinComponentSpacingMM) != defaults.MinComponentSpacing || kicadfiles.MM(document.Schematic.Rules.MinGroupSpacingMM) != defaults.MinStageSpacing {
		t.Error("PCB spacing must not override standard schematic spacing")
	}
	if !document.Schematic.Rules.OrientEndpointLabels || !document.Schematic.Rules.ReserveTitleBlock {
		t.Error("generated schematics must orient labels away from bodies and reserve the title block")
	}
	want := []PCBPlacement{{Component: "source", Region: "main"}, {Component: "supply", Region: "main"}, {Component: "load", Region: "main"}}
	if !reflect.DeepEqual(document.PCB.Placements, want) {
		t.Fatalf("schematic inference changed PCB placements: %+v", document.PCB.Placements)
	}
}

func TestTopologyLayoutProfileIsExplicitAndElectricallyInvariant(t *testing.T) {
	b, err := os.ReadFile("testdata/function_corpus/atmega328p_isp_controller.json")
	if err != nil {
		t.Fatal(err)
	}
	input, issues := DecodeStrict(strings.NewReader(string(b)))
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	catalog := loadGraphCatalog(t)
	legacy := NewResolver(ResolveOptions{Catalog: catalog})
	native := NewResolver(ResolveOptions{Catalog: catalog, SchematicLayoutProfile: SchematicLayoutTopologyV1})
	a, _, issues := legacy.Synthesize(context.Background(), input)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	bdoc, report, issues := native.Synthesize(context.Background(), input)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	if reflect.DeepEqual(a.Schematic, bdoc.Schematic) {
		t.Fatal("opt-in did not change schematic layout")
	}
	bdoc.Schematic = a.Schematic
	if !reflect.DeepEqual(a, bdoc) {
		t.Fatal("layout profile changed electrical or PCB intent")
	}
	found := false
	for _, c := range report.DerivedConstraints {
		if c.Kind == "schematic_layout_profile" && c.Value == SchematicLayoutTopologyV1 {
			found = true
		}
	}
	if !found {
		t.Fatal("profile missing from synthesis evidence")
	}
	invalid := NewResolver(ResolveOptions{Catalog: catalog, SchematicLayoutProfile: "unknown"})
	if _, _, issues := invalid.Synthesize(context.Background(), input); !reports.HasBlockingIssue(issues) {
		t.Fatal("unknown profile did not fail closed")
	}
}
