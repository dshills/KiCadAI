package kiapi

import (
	"testing"

	commontypes "kicadai/internal/kiapi/gen/common/types"
)

func TestCapabilitiesForVersion(t *testing.T) {
	capabilities := CapabilitiesForVersion(&commontypes.KiCadVersion{
		Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0",
	})

	if capabilities.KiCadVersion != "9.1.0" {
		t.Fatalf("KiCadVersion = %q", capabilities.KiCadVersion)
	}
	if !capabilities.Supports(CapabilitySchematicRead) {
		t.Fatalf("expected schematic read support")
	}
	for _, missing := range []Capability{
		CapabilitySchematicWrite,
		CapabilitySymbolPlace,
		CapabilityWirePlace,
		CapabilityLabelPlace,
	} {
		if capabilities.Supports(missing) {
			t.Fatalf("expected %s to be missing", missing)
		}
	}
}
