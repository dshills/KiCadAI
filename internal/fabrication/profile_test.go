package fabrication

import (
	"testing"

	"kicadai/internal/reports"
)

func TestLookupManufacturerProfileLoadsGenericAssembly(t *testing.T) {
	profile, ok := LookupManufacturerProfile(GenericAssemblyProfileID)
	if !ok {
		t.Fatal("generic assembly profile not found")
	}
	if profile.DisplayName != "Generic Assembly" || !profile.AllowGenericPassives || profile.ExactMPNPolicy != ExactMPNRequireAssemblyCritical {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestValidateManufacturerProfileRejectsUnknownProfile(t *testing.T) {
	issues := ValidateManufacturerProfile(ManufacturerProfile{ID: "missing_profile"}, ReportData{})
	if len(issues) != 1 || issues[0].Code != reports.CodeInvalidArgument || !issues[0].Blocking() {
		t.Fatalf("issues = %#v, want blocking unknown profile issue", issues)
	}
}

func TestValidateManufacturerProfileAllowsCompleteCustomProfile(t *testing.T) {
	profile := ManufacturerProfile{
		ID:                   "custom_profile",
		DisplayName:          "Custom Profile",
		AcceptedSides:        []string{"top"},
		ExactMPNPolicy:       ExactMPNRequireAll,
		AllowGenericPassives: false,
	}
	issues := ValidateManufacturerProfile(profile, ReportData{
		BOM: []BOMRow{{References: []string{"U1"}, Manufacturer: "Example", MPN: "ABC-1"}},
		CPL: []CPLRow{{Reference: "U1", NormalizedSide: "top"}},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want complete custom profile accepted", issues)
	}
}

func TestValidateManufacturerProfileRequiresExactMPNForActiveParts(t *testing.T) {
	profile, _ := LookupManufacturerProfile(GenericAssemblyProfileID)
	issues := ValidateManufacturerProfile(profile, ReportData{BOM: []BOMRow{{
		References:     []string{"U1"},
		Value:          "MCU",
		ComponentClass: "active",
		FootprintID:    "Package:QFN",
	}}})
	if !hasIssuePath(issues, "profile.generic_assembly.bom.U1") {
		t.Fatalf("issues = %#v, want exact MPN issue", issues)
	}
}

func TestValidateManufacturerProfileAllowsGenericPassives(t *testing.T) {
	profile, _ := LookupManufacturerProfile(GenericAssemblyProfileID)
	issues := ValidateManufacturerProfile(profile, ReportData{BOM: []BOMRow{{
		References:     []string{"R1"},
		Value:          "10k",
		ComponentClass: "passive",
		FootprintID:    "Resistor_SMD:R_0603",
	}}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want generic passive allowed", issues)
	}
}

func TestValidateManufacturerProfileWarnsForGenericPassivePolicy(t *testing.T) {
	profile := ManufacturerProfile{
		ID:                   "warn_passives",
		DisplayName:          "Warn Passives",
		AcceptedSides:        []string{"top", "bottom"},
		ExactMPNPolicy:       ExactMPNWarnGenericPassives,
		AllowGenericPassives: true,
	}
	issues := ValidateManufacturerProfile(profile, ReportData{BOM: []BOMRow{{
		References:     []string{"R1"},
		Value:          "10k",
		ComponentClass: "passive",
	}}})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("issues = %#v, want warning for generic passive", issues)
	}
}

func TestValidateManufacturerProfileChecksAcceptedSides(t *testing.T) {
	profile, _ := LookupManufacturerProfile(GenericAssemblyProfileID)
	issues := ValidateManufacturerProfile(profile, ReportData{CPL: []CPLRow{
		{Reference: "U1", NormalizedSide: "top"},
		{Reference: "U2", NormalizedSide: "left"},
		{Reference: "U3", NormalizedSide: "unknown"},
	}})
	if !hasIssuePath(issues, "profile.generic_assembly.cpl.U2.side") || !hasIssuePath(issues, "profile.generic_assembly.cpl.U3.side") {
		t.Fatalf("issues = %#v, want unsupported and unknown side issues", issues)
	}
}
