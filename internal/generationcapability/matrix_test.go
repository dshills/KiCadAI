package generationcapability

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/components"
)

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

func TestBuildDocumentIncludesGenericCatalogContract(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	document, err := BuildDocument(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(document.GenericGraphContract) || len(document.Capabilities) == 0 {
		t.Fatalf("document = %#v", document)
	}
	context, err := ProviderCapabilityContext(catalog, 0)
	if err != nil || !json.Valid([]byte(context)) {
		t.Fatalf("provider context = %q, %v", context, err)
	}
	if _, err := ProviderCapabilityContext(catalog, 1); err == nil {
		t.Fatal("expected capability size limit failure")
	}
}
