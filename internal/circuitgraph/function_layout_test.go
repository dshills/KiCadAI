package circuitgraph

import (
	"testing"

	"kicadai/internal/components"
)

func TestSynthesisPhysicalEnvelopeIncludesPackageConstraints(t *testing.T) {
	records := map[string]components.ComponentRecord{
		"radio": {
			ID: "radio",
			Packages: []components.PackageVariant{{
				ID:           "module",
				DimensionsMM: &components.Bounds{Width: 18, Height: 25.5},
				Constraints:  []components.PhysicalConstraint{{Kind: "antenna_keepout", Value: "48x21.44", Unit: "mm"}},
			}},
		},
	}
	width, height := synthesisPhysicalEnvelope([]Component{{ComponentID: "radio", VariantID: "module"}}, records)
	if width != 48 || height != 25.5 {
		t.Fatalf("physical envelope = %.2fx%.2f, want 48x25.5", width, height)
	}
}

func TestDeriveFunctionLayoutAddsNegativePowerLaneOnlyForNegativeRails(t *testing.T) {
	constraints := SynthesisConstraints{MaxWidthMM: 100, MaxHeightMM: 100}
	withNegative := Document{Nets: []Net{{Name: "VEE", Role: NetRolePowerNeg}}}
	if issues := deriveFunctionLayout(&withNegative, FunctionIntent{Constraints: constraints}, nil, nil); len(issues) != 0 {
		t.Fatalf("derive split-supply layout: %#v", issues)
	}
	if withNegative.Schematic.Lanes.PowerNegative == nil || *withNegative.Schematic.Lanes.PowerNegative != LaneLower {
		t.Fatalf("negative power lane = %#v, want lower", withNegative.Schematic.Lanes.PowerNegative)
	}

	withoutNegative := Document{Nets: []Net{{Name: "VCC", Role: NetRolePowerPos}}}
	if issues := deriveFunctionLayout(&withoutNegative, FunctionIntent{Constraints: constraints}, nil, nil); len(issues) != 0 {
		t.Fatalf("derive single-supply layout: %#v", issues)
	}
	if withoutNegative.Schematic.Lanes.PowerNegative != nil {
		t.Fatalf("single-supply negative power lane = %#v, want omitted", withoutNegative.Schematic.Lanes.PowerNegative)
	}
}

func TestPhysicalConstraintDimensionsMMRejectsNonDimensionalEvidence(t *testing.T) {
	if _, _, ok := physicalConstraintDimensionsMM(components.PhysicalConstraint{Value: "48x21.44", Unit: "mil"}); ok {
		t.Fatal("non-mm physical evidence was interpreted as millimetres")
	}
	if _, _, ok := physicalConstraintDimensionsMM(components.PhysicalConstraint{Value: "not-a-size", Unit: "mm"}); ok {
		t.Fatal("non-dimensional physical evidence was accepted")
	}
}

func TestTieParentFunctionToLevelRejectsAmbiguousSupplyDomains(t *testing.T) {
	document := Document{Nets: []Net{
		{Name: "3V3", Role: NetRolePowerPos, Endpoints: []Endpoint{{Component: "u1", SelectorKind: SelectorFunction, Selector: "VDD"}}},
		{Name: "1V8", Role: NetRolePowerPos, Endpoints: []Endpoint{{Component: "u1", SelectorKind: SelectorFunction, Selector: "VDDIO"}}},
	}}
	component := ResolvedComponent{
		Instance: Component{ID: "u1"},
		Functions: []ResolvedFunction{
			{Function: "MODE"},
			{Function: "VDD", Electrical: "power_in"},
			{Function: "VDDIO", Electrical: "power_in"},
		},
	}
	connected := map[string]bool{}
	issue := tieParentFunctionToLevel(&document, component, "MODE", "high", "", connected)
	if issue == nil || issue.Code != CodeSynthesisPowerDomainInvalid {
		t.Fatalf("ambiguous high tie issue = %#v", issue)
	}
	if connected["u1\x00MODE"] || len(document.Nets[0].Endpoints) != 1 || len(document.Nets[1].Endpoints) != 1 {
		t.Fatalf("ambiguous tie mutated graph: nets=%#v connected=%#v", document.Nets, connected)
	}
}
