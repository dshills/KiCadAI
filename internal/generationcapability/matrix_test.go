package generationcapability

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/aiprovider"
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
	providerContext, err := ProviderCapabilityContext(catalog, 0)
	if err != nil || !json.Valid([]byte(providerContext)) {
		t.Fatalf("provider context = %q, %v", providerContext, err)
	}
	var providerDocument Document
	if err := json.Unmarshal([]byte(providerContext), &providerDocument); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(document, providerDocument) {
		t.Fatalf("provider document differs from CLI document\nCLI: %#v\nprovider: %#v", document, providerDocument)
	}
	if _, err := ProviderCapabilityContext(catalog, 1); err == nil {
		t.Fatal("expected capability size limit failure")
	}
}

func TestCheckedInCatalogFitsProviderCapabilityLimitWithoutOmissions(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	providerContext, err := ProviderCapabilityContext(catalog, aiprovider.MaxCapabilityBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(providerContext) > aiprovider.MaxCapabilityBytes {
		t.Fatalf("provider context is %d bytes, limit is %d", len(providerContext), aiprovider.MaxCapabilityBytes)
	}

	var document Document
	if err := json.Unmarshal([]byte(providerContext), &document); err != nil {
		t.Fatal(err)
	}
	var graphContract struct {
		Components []struct {
			ID  string `json:"id"`
			MPN string `json:"mpn"`
		} `json:"components"`
	}
	if err := json.Unmarshal(document.GenericGraphContract, &graphContract); err != nil {
		t.Fatal(err)
	}
	if len(graphContract.Components) != len(catalog.Records) {
		t.Fatalf("provider component count = %d, catalog record count = %d", len(graphContract.Components), len(catalog.Records))
	}
	for index, record := range catalog.Records {
		got := graphContract.Components[index]
		if got.ID != record.ID || got.MPN != record.MPN {
			t.Fatalf("provider component %d = %#v, want id=%q mpn=%q", index, got, record.ID, record.MPN)
		}
	}
}
