package generationcapability

import "testing"

func TestGenericCircuitCapabilityIsExplicit(t *testing.T) {
	capability, ok := Lookup(ProfileGenericCircuit)
	if !ok {
		t.Fatal("generic circuit capability is missing")
	}
	if capability.Kind != KindGeneric || capability.InputContract == "" {
		t.Fatalf("generic capability = %#v", capability)
	}
	if len(capability.RequiredEvidence) == 0 || len(capability.Limitations) == 0 {
		t.Fatalf("generic capability lacks evidence or limitations: %#v", capability)
	}
}

func TestCapabilityLookupFailsClosedAndReturnsCopies(t *testing.T) {
	if _, ok := Lookup("unknown-profile"); ok {
		t.Fatal("unknown profile unexpectedly has a capability")
	}
	first := All()
	first[0].Supports[0] = "mutated"
	second := All()
	if second[0].Supports[0] == "mutated" {
		t.Fatal("capability matrix exposed mutable shared state")
	}
	lookup, ok := Lookup(ProfileGenericCircuit)
	if !ok {
		t.Fatal("generic circuit capability is missing")
	}
	lookup.RequiredEvidence[0] = "mutated"
	again, ok := Lookup(ProfileGenericCircuit)
	if !ok || again.RequiredEvidence[0] == "mutated" {
		t.Fatal("capability lookup exposed mutable shared state")
	}
}
