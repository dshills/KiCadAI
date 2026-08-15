package modelprovenance

import "testing"

func TestV18RegistryExtensionIsExplicit(t *testing.T) {
	legacy, diagnostics := LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("load legacy registry: %+v", diagnostics)
	}
	if _, found := Lookup(legacy, "opamp.ti.tlv9061idbvr.sot23_5", "mna_opamp_single_pole_v1"); found {
		t.Fatal("legacy registry unexpectedly contains V18-only provenance")
	}

	v18, diagnostics := LoadV18()
	if len(diagnostics) != 0 {
		t.Fatalf("load V18 registry: %+v", diagnostics)
	}
	if _, found := Lookup(v18, "opamp.ti.tlv9061idbvr.sot23_5", "mna_opamp_single_pole_v1"); !found {
		t.Fatal("V18 registry is missing the reviewed model binding")
	}
	legacyHash, err := Hash(legacy)
	if err != nil {
		t.Fatal(err)
	}
	v18Hash, err := Hash(v18)
	if err != nil {
		t.Fatal(err)
	}
	if legacyHash == v18Hash {
		t.Fatal("V18 registry extension did not create a distinct authenticated snapshot")
	}
}
