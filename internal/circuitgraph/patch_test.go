package circuitgraph

import (
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestDecodePatchStrict(t *testing.T) {
	patch, issues := DecodePatchStrict(strings.NewReader(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_endpoint","net":"N","endpoint":{"component":"r1","selector_kind":"symbol_pin","selector":"2"},"replacement":{"component":"r1","selector_kind":"symbol_pin","selector":"1"}}]}`))
	if reports.HasBlockingIssue(issues) || len(patch.Operations) != 1 {
		t.Fatalf("patch=%#v issues=%#v", patch, issues)
	}
}

func TestDecodePatchStrictRejectsUnsafeOperations(t *testing.T) {
	for _, input := range []string{
		`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_project"}]}`,
		`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_endpoint","net":"N","endpoint":{"component":"r1","selector_kind":"symbol_pin","selector":"1"},"replacement":{"component":"r2","selector_kind":"symbol_pin","selector":"1"}}]}`,
		`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_policy","policy":"require_drc","enabled":true}]}`,
	} {
		_, issues := DecodePatchStrict(strings.NewReader(input))
		if !reports.HasBlockingIssue(issues) || issues[0].Code != CodePatchInvalid {
			t.Fatalf("input=%s issues=%#v", input, issues)
		}
	}
}

func TestApplyPatchReturnsNormalizedCorrectedGraph(t *testing.T) {
	document, issues := DecodeStrict(strings.NewReader(`{"schema":"kicadai.circuit-graph.v1","version":1,"project":{"name":"demo","acceptance":"structural","board":{"width_mm":20,"height_mm":20,"layers":2}},"components":[{"id":"r1","reference":"R1","role":"resistor","component_id":"resistor.generic.0805","population":"populate"},{"id":"j1","reference":"J1","role":"connector","component_id":"connector.pinheader.1x02.2_54mm","variant_id":"vertical","population":"populate"}],"nets":[{"name":"N","role":"signal","required":true,"endpoints":[{"component":"j1","selector_kind":"symbol_pin","selector":"1"},{"component":"r1","selector_kind":"symbol_pin","selector":"999"}]}],"no_connects":[],"buses":[],"schematic":{"flow":"left_to_right","origin":"centered","groups":[{"id":"g","members":["j1","r1"],"rank":0}],"lanes":{"power":"top","signals":"middle","ground":"bottom"},"placements":[{"component":"j1","group":"g"},{"component":"r1","group":"g"}],"rules":{"positive_power_top":true,"ground_bottom":true,"center_on_page":true,"prefer_labels_for_long_nets":true,"avoid_wire_crossings":true,"min_group_spacing_mm":1,"min_component_spacing_mm":1},"hierarchy":{"mode":"flat"}},"pcb":{"regions":[{"id":"main","bounds":{"x_mm":0,"y_mm":0,"width_mm":20,"height_mm":20}}],"placements":[{"component":"j1","region":"main"},{"component":"r1","region":"main"}],"keepouts":[],"zones":[]},"policy":{"allow_reference_assignment":true,"allow_value_normalization":true,"allow_layout_inference":true,"allow_spacing_adjustment":true,"allow_label_insertion":true,"allow_placement_adjustment":true,"allow_route_retry":true}}`))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("graph issues=%#v", issues)
	}
	patch := PatchDocument{Schema: PatchSchemaID, Version: PatchVersion, Operations: []PatchOperation{{Op: "replace_endpoint", Net: "N", Endpoint: &Endpoint{Component: "r1", SelectorKind: SelectorSymbolPin, Selector: "999"}, Replacement: &Endpoint{Component: "r1", SelectorKind: SelectorSymbolPin, Selector: "1"}}}}
	corrected, issues := ApplyPatch(document, patch)
	if reports.HasBlockingIssue(issues) || corrected.Nets[0].Endpoints[1].Selector != "1" || document.Nets[0].Endpoints[1].Selector != "999" {
		t.Fatalf("corrected=%#v original=%#v issues=%#v", corrected, document, issues)
	}
}

func TestApplyPatchReplacesOnlyCatalogSelector(t *testing.T) {
	document, issues := DecodeStrict(strings.NewReader(`{"schema":"kicadai.circuit-graph.v1","version":1,"project":{"name":"demo","acceptance":"structural","board":{"width_mm":20,"height_mm":20,"layers":2}},"components":[{"id":"r1","reference":"R1","role":"resistor","component_id":"unsupported.component","population":"populate"},{"id":"r2","reference":"R2","role":"resistor","component_id":"resistor.generic.0805","population":"populate"}],"nets":[{"name":"N","role":"signal","required":true,"endpoints":[{"component":"r1","selector_kind":"function","selector":"A"},{"component":"r2","selector_kind":"function","selector":"A"}]}],"no_connects":[],"buses":[],"schematic":{"flow":"left_to_right","origin":"centered","groups":[{"id":"g","members":["r1","r2"],"rank":0}],"lanes":{"power":"top","signals":"middle","ground":"bottom"},"placements":[{"component":"r1","group":"g"},{"component":"r2","group":"g"}],"rules":{"positive_power_top":true,"ground_bottom":true,"center_on_page":true,"prefer_labels_for_long_nets":true,"avoid_wire_crossings":true,"min_group_spacing_mm":1,"min_component_spacing_mm":1},"hierarchy":{"mode":"flat"}},"pcb":{"regions":[{"id":"main","bounds":{"x_mm":0,"y_mm":0,"width_mm":20,"height_mm":20}}],"placements":[{"component":"r1","region":"main"},{"component":"r2","region":"main"}],"keepouts":[],"zones":[]},"policy":{"allow_reference_assignment":true,"allow_value_normalization":true,"allow_layout_inference":true,"allow_spacing_adjustment":true,"allow_label_insertion":true,"allow_placement_adjustment":true,"allow_route_retry":true}}`))
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	componentID := "resistor.generic.0805"
	corrected, issues := ApplyPatch(document, PatchDocument{Schema: PatchSchemaID, Version: PatchVersion, Operations: []PatchOperation{{Op: "replace_component", Component: "r1", ComponentPatch: &ComponentPatch{ComponentID: &componentID}}}})
	if reports.HasBlockingIssue(issues) || corrected.Components[0].ComponentID != componentID || corrected.Components[0].Reference != "R1" {
		t.Fatalf("corrected=%#v issues=%#v", corrected, issues)
	}
}
