package components

import (
	"context"
	"testing"
)

func TestV18CatalogExtensionIsExplicit(t *testing.T) {
	legacy, err := LoadCatalog(context.Background(), LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := LookupRecord(legacy, "opamp.ti.tlv9061idbvr.sot23_5"); found {
		t.Fatal("legacy catalog unexpectedly contains V18-only component")
	}

	v18, err := LoadCatalogV18(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if issues := ValidateCatalog(v18).Issues; len(issues) != 0 {
		t.Fatalf("V18 catalog invalid: %+v", issues)
	}
	record, found := LookupRecord(v18, "opamp.ti.tlv9061idbvr.sot23_5")
	if !found {
		t.Fatal("V18 catalog is missing its reviewed low-voltage op-amp")
	}
	if record.OpAmp == nil || !record.OpAmp.FabricationProof {
		t.Fatal("V18 op-amp lacks reviewed fabrication evidence")
	}
}
