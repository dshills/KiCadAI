package boardfamily

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogAndSchemaUseFamilyProfiles(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 2 {
		t.Fatal("expected two distinct families")
	}
	schema := SelectionSchema()
	properties := schema["properties"].(map[string]any)
	variants := properties["configuration"].(map[string]any)["anyOf"].([]any)
	if len(variants) != 3 {
		t.Fatal("need two family objects plus null")
	}
	for i, family := range catalog {
		if _, err := Check(family.DefaultConfiguration); err != nil {
			t.Fatal(err)
		}
		if family.DefaultConfiguration.Family != family.ID || len(family.Capabilities) == 0 || len(family.Unsupported) == 0 || len(family.Conditions) == 0 {
			t.Fatal("incomplete catalog", family)
		}
		if !reflect.DeepEqual(family.Profiles, ProfilesFor(family.ID)) {
			t.Fatal("catalog profile drift")
		}
		cfg := variants[i].(map[string]any)
		props := cfg["properties"].(map[string]any)
		if cfg["additionalProperties"] != false || len(cfg["required"].([]string)) != len(props) {
			t.Fatal("configuration schema is not strict")
		}
		if !reflect.DeepEqual(props["family"].(map[string]any)["enum"], []string{family.ID}) {
			t.Fatal("schema family drift")
		}
		var ids []string
		for _, p := range family.Profiles {
			ids = append(ids, p.ID)
		}
		if !reflect.DeepEqual(props["profile"].(map[string]any)["enum"], ids) {
			t.Fatal("schema admitted another family's profiles")
		}
		if !strings.Contains(LanguageContext, family.ID) {
			t.Fatal("language contract missing family")
		}
	}
	catalog[0].Profiles[0].MaxBusPF = 999
	catalog[1].Conditions[0] = "mutated"
	if Catalog()[0].Profiles[0].MaxBusPF != 200 || Catalog()[1].Conditions[0] == "mutated" {
		t.Fatal("catalog exposes shared mutable data")
	}
}

func TestTwoFamilyDecisionAdmission(t *testing.T) {
	for _, family := range Catalog() {
		for _, profile := range family.Profiles {
			c := family.DefaultConfiguration
			c.Profile, c.TotalBusCapacitancePF = profile.ID, profile.MaxBusPF
			prompt := "Use " + family.ID + " with the " + profile.ID + " profile."
			d := Decision{"supported", "Selected the requested family and its declared profile limits.", []Clause{{prompt, "supported", "Explicit family/profile."}}, &c}
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			got, err := DecodeDecision(prompt, b)
			if err != nil || got.Configuration == nil || *got.Configuration != c {
				t.Fatalf("admission changed selection: %+v %v", got, err)
			}
		}
	}
	for _, bad := range []Config{
		{"1", FamilySHT31, "low_current", 3.2, 3.4, 1000, 10, 35, 50},
		{"1", FamilySHT31, "standard", 3.2, 3.4, 1000, 10, 35, 200},
		{"1", "combined_pressure_humidity", "standard", 3.2, 3.4, 1000, 10, 35, 70},
	} {
		d := Decision{"supported", "Incorrect model selection.", []Clause{{"Use the requested board.", "supported", "Incorrect model claim."}}, &bad}
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := DecodeDecision(d.Clauses[0].Text, b); err == nil && got.Configuration != nil {
			t.Fatal("invalid two-family selection escaped", bad)
		}
	}
}
