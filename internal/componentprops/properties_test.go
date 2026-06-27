package componentprops

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

func TestMergeIdentityPropertiesAppendsHiddenMetadata(t *testing.T) {
	existing := []schematic.Property{{Name: "Tolerance", Value: "1%"}}
	merged, issues := MergeIdentityProperties(existing, Evidence{
		ComponentID:         "resistor.yageo.rc0805fr_0710kl.0805",
		VariantID:           "0805",
		ComponentRole:       "pullup",
		Manufacturer:        "Yageo",
		MPN:                 "RC0805FR-0710KL",
		ComponentConfidence: "verified",
	}, MergeOptions{Ref: "R1", Position: kicadfiles.Point{X: kicadfiles.MM(10)}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(merged) != 7 {
		t.Fatalf("property count = %d, want 7: %#v", len(merged), merged)
	}
	if merged[0].Name != "Tolerance" {
		t.Fatalf("custom property not preserved first: %#v", merged)
	}
	for _, property := range merged[1:] {
		if !property.Hidden || property.ShowName == nil || *property.ShowName || property.DoNotAutoplace == nil || !*property.DoNotAutoplace {
			t.Fatalf("identity property is not hidden metadata: %#v", property)
		}
	}
	if merged[1].Name != PropertyComponentID || merged[1].Value == "" {
		t.Fatalf("component id property = %#v", merged[1])
	}
	if merged[5].Name != PropertyMPN || merged[5].Value != "RC0805FR-0710KL" {
		t.Fatalf("mpn property = %#v", merged[5])
	}
}

func TestMergeIdentityPropertiesIsIdempotent(t *testing.T) {
	evidence := Evidence{
		ComponentID:         "capacitor.murata.grm21br71h104ka01l.0805",
		ComponentRole:       "decoupling",
		Manufacturer:        "Murata",
		MPN:                 "GRM21BR71H104KA01L",
		ComponentConfidence: "verified",
	}
	first, issues := MergeIdentityProperties(nil, evidence, MergeOptions{Ref: "C1"})
	if len(issues) != 0 {
		t.Fatalf("first issues = %#v", issues)
	}
	second, issues := MergeIdentityProperties(first, evidence, MergeOptions{Ref: "C1"})
	if len(issues) != 0 {
		t.Fatalf("second issues = %#v", issues)
	}
	if len(first) != len(second) {
		t.Fatalf("length changed from %d to %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name != second[i].Name || first[i].Value != second[i].Value {
			t.Fatalf("property %d changed from %#v to %#v", i, first[i], second[i])
		}
	}
}

func TestMergeIdentityPropertiesReportsGeneratedReplacement(t *testing.T) {
	merged, issues := MergeIdentityProperties([]schematic.Property{
		{Name: PropertyComponentID, Value: "old", Position: kicadfiles.Point{X: kicadfiles.MM(3)}},
	}, Evidence{ComponentID: "new", ComponentRole: "led", ComponentConfidence: "verified"}, MergeOptions{Ref: "D1", Path: "symbol.D1"})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityWarning || len(issues[0].Refs) != 1 || issues[0].Refs[0] != "D1" {
		t.Fatalf("issues = %#v", issues)
	}
	if merged[0].Name != PropertyComponentID || merged[0].Value != "new" {
		t.Fatalf("replacement not applied: %#v", merged)
	}
	if merged[0].Position.X != kicadfiles.MM(3) {
		t.Fatalf("replacement did not preserve property attributes: %#v", merged[0])
	}
}

func TestMergeIdentityPropertiesPreservesOwnedPropertiesOmittedFromEvidence(t *testing.T) {
	merged, issues := MergeIdentityProperties([]schematic.Property{
		{Name: PropertyManufacturer, Value: "Existing Manufacturer"},
	}, Evidence{ComponentID: "component", ComponentRole: "role", ComponentConfidence: "verified"}, MergeOptions{Ref: "R1"})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if merged[0].Name != PropertyManufacturer || merged[0].Value != "Existing Manufacturer" {
		t.Fatalf("omitted owned property was not preserved: %#v", merged)
	}
}

func TestMergeIdentityPropertiesCanBlockPreservationConflict(t *testing.T) {
	merged, issues := MergeIdentityProperties([]schematic.Property{
		{Name: PropertyComponentID, Value: "old"},
		{Name: "User", Value: "keep"},
	}, Evidence{ComponentID: "new", ComponentRole: "led", ComponentConfidence: "verified"}, MergeOptions{Ref: "D1", Policy: PolicyPreserveBlock})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityBlocked {
		t.Fatalf("issues = %#v", issues)
	}
	if len(merged) != 2 || merged[0].Value != "old" || merged[1].Name != "User" {
		t.Fatalf("preserve merge mutated properties: %#v", merged)
	}
}

func TestEvidencePropertyValuesOmitEmptyValues(t *testing.T) {
	values := Evidence{ComponentID: "component", Manufacturer: "  "}.PropertyValues()
	if values[PropertyComponentID] != "component" {
		t.Fatalf("component id missing: %#v", values)
	}
	if _, ok := values[PropertyManufacturer]; ok {
		t.Fatalf("empty manufacturer should be omitted: %#v", values)
	}
}
