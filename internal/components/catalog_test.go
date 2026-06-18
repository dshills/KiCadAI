package components

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"kicadai/internal/reports"
)

func TestLoadCatalogEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: dir})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Diagnostics) != 1 || catalog.Diagnostics[0].Code != CodeCatalogEmpty {
		t.Fatalf("expected empty catalog diagnostic, got %+v", catalog.Diagnostics)
	}
}

func TestLoadCatalogRejectsRelativeParentTraversal(t *testing.T) {
	_, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: "../components"})
	if err == nil {
		t.Fatal("expected parent traversal catalog dir to fail")
	}
}

func TestLoadCatalogMergesDeterministically(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "b.json", `{
  "families": [{"id": "resistor", "name": "Resistor"}],
  "records": [`+validRecordJSON("resistor.generic.0805", "resistor", "0805")+`]
}`)
	writeCatalogFile(t, dir, "a.json", `{
  "families": [{"id": "capacitor", "name": "Capacitor"}],
  "records": [`+validRecordJSON("capacitor.generic.0805", "capacitor", "0805")+`]
}`)

	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: dir})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Records) != 2 {
		t.Fatalf("expected two records, got %d", len(catalog.Records))
	}
	if catalog.Families[0].ID != "capacitor" || catalog.Records[0].ID != "capacitor.generic.0805" {
		t.Fatalf("catalog merge order is not deterministic: %+v", catalog)
	}
	if reports.HasBlockingIssue(catalog.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %+v", catalog.Diagnostics)
	}
}

func TestCheckedInCatalogLoadsAndValidates(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	if len(catalog.Records) == 0 {
		t.Fatal("checked-in catalog has no records")
	}
	result := ValidateCatalog(catalog)
	if !result.OK {
		t.Fatalf("checked-in catalog validation failed: %+v", result.Issues)
	}
	coveredFamilies := catalogFamilyCoverage(catalog)
	for _, family := range catalog.Families {
		if !coveredFamilies[family.ID] {
			t.Fatalf("checked-in catalog missing family record for %s", family.ID)
		}
	}
}

func checkedInCatalogDir(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "data", "components")
}

func TestValidateCatalogDuplicateID(t *testing.T) {
	catalog := validCatalog()
	catalog.Records = append(catalog.Records, catalog.Records[0])
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected duplicate id to fail")
	}
	assertIssueCode(t, result.Issues, CodeDuplicateComponentID)
}

func catalogFamilyCoverage(catalog *Catalog) map[string]bool {
	covered := map[string]bool{}
	for _, record := range catalog.Records {
		covered[record.Family] = true
	}
	return covered
}

func TestValidateCatalogUnknownFamily(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Family = "unknown"
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected unknown family to fail")
	}
	assertIssueCode(t, result.Issues, CodeUnknownFamily)
}

func TestValidateCatalogDuplicateFamily(t *testing.T) {
	catalog := validCatalog()
	catalog.Families = append(catalog.Families, catalog.Families[0])
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected duplicate family to fail")
	}
	assertIssueCode(t, result.Issues, CodeInvalidComponentFamily)
}

func TestValidateCatalogMissingFootprint(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Packages[0].FootprintID = ""
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected missing footprint to fail")
	}
	assertIssueCode(t, result.Issues, CodeMissingFootprint)
}

func TestValidateCatalogInvalidConfidence(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Verification.Confidence = "maybe"
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected invalid confidence to fail")
	}
	assertIssueCode(t, result.Issues, CodeInvalidConfidence)
}

func TestValidateCatalogExtendedMetadata(t *testing.T) {
	catalog := validCatalog()
	record := &catalog.Records[0]
	record.Lifecycle = "active"
	record.Tolerances = []ToleranceConstraint{{Kind: "resistance", Max: "1", Unit: "%"}}
	record.Temperature = &TemperatureRange{Min: "-40", Max: "85", Unit: "C"}
	record.Companions = []CompanionRequirement{{
		ID:       "cap.input",
		Family:   "capacitor",
		Role:     "input_capacitor",
		Required: true,
	}}
	record.DeratingRules = []DeratingRule{{Kind: "voltage", Expression: "rated_voltage >= 2 * operating_voltage"}}
	record.PlacementHints = []PlacementHint{{Kind: "near", Target: "power_pin", Value: "2", Unit: "mm"}}
	record.RoutingHints = []RoutingHint{{Kind: "net_class", NetRole: "power", Value: "0.25", Unit: "mm"}}
	record.Properties = []SchematicProperty{{Name: "MPN", Value: "GENERIC-0805"}}
	record.Packages[0].MPN = "GENERIC-0805-PKG"
	record.Packages[0].Lifecycle = "preferred"
	record.Packages[0].HeightMM = 0.55

	result := ValidateCatalog(&catalog)
	if !result.OK {
		t.Fatalf("expected extended metadata to validate: %+v", result.Issues)
	}
}

func TestValidateCatalogInvalidExtendedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Catalog)
		code   reports.Code
	}{
		{
			name: "lifecycle",
			mutate: func(c *Catalog) {
				c.Records[0].Lifecycle = "shipping"
			},
			code: CodeInvalidLifecycle,
		},
		{
			name: "tolerance",
			mutate: func(c *Catalog) {
				c.Records[0].Tolerances = []ToleranceConstraint{{Kind: "resistance", Max: "one", Unit: "%"}}
			},
			code: CodeInvalidConstraint,
		},
		{
			name: "duplicate companion",
			mutate: func(c *Catalog) {
				c.Records[0].Companions = []CompanionRequirement{
					{ID: "c1", Role: "decoupling", Required: true},
					{ID: "c1", Role: "decoupling", Required: true},
				}
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "placement unit",
			mutate: func(c *Catalog) {
				c.Records[0].PlacementHints = []PlacementHint{{Kind: "near", Value: "2"}}
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "negative height",
			mutate: func(c *Catalog) {
				c.Records[0].Packages[0].HeightMM = -1
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "duplicate property",
			mutate: func(c *Catalog) {
				c.Records[0].Properties = []SchematicProperty{{Name: "MPN", Value: "a"}, {Name: "MPN", Value: "b"}}
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "untrimmed lifecycle",
			mutate: func(c *Catalog) {
				c.Records[0].Lifecycle = "active "
			},
			code: CodeInvalidLifecycle,
		},
		{
			name: "untrimmed property",
			mutate: func(c *Catalog) {
				c.Records[0].Properties = []SchematicProperty{{Name: " MPN", Value: "a"}}
			},
			code: CodeInvalidMetadata,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			tt.mutate(&catalog)
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, tt.code)
		})
	}
}

func TestValidateCatalogTemperaturePath(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Temperature = &TemperatureRange{Min: "cold", Max: "85", Unit: "C"}
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected invalid temperature to fail")
	}
	for _, issue := range result.Issues {
		if issue.Path == "records[0].temperature.min" {
			return
		}
	}
	t.Fatalf("expected temperature min path in %+v", result.Issues)
}

func validCatalog() Catalog {
	return Catalog{
		Version: CatalogVersion,
		Families: []FamilyDefinition{{
			ID:   "resistor",
			Name: "Resistor",
		}},
		Records: []ComponentRecord{{
			ID:      "resistor.generic.0805",
			Family:  "resistor",
			Name:    "Generic 0805 resistor",
			Generic: true,
			Values: []ValueConstraint{{
				Kind: "resistance",
				Typ:  "10k",
				Unit: "ohm",
			}},
			Symbols: []SymbolBinding{{
				SymbolID: "Device:R",
				FunctionPins: []FunctionPin{
					{Function: "A", SymbolPin: "1", Required: true},
					{Function: "B", SymbolPin: "2", Required: true},
				},
				Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
			}},
			Packages: []PackageVariant{{
				ID:          "0805",
				Name:        "0805",
				FootprintID: "Resistor_SMD:R_0805_2012Metric",
				PadFunctions: []PadFunction{
					{Function: "A", Pad: "1"},
					{Function: "B", Pad: "2"},
				},
				Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
			}},
			Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
		}},
	}
}

func validRecordJSON(id string, family string, variant string) string {
	return `{
  "id": "` + id + `",
  "family": "` + family + `",
  "name": "` + id + `",
  "generic": true,
  "values": [{"kind": "resistance", "typ": "10k", "unit": "ohm"}],
  "symbols": [{
    "symbol_id": "Device:R",
    "function_pins": [
      {"function": "A", "symbol_pin": "1", "required": true},
      {"function": "B", "symbol_pin": "2", "required": true}
    ],
    "verification": {"confidence": "rule_inferred"}
  }],
  "packages": [{
    "id": "` + variant + `",
    "name": "` + variant + `",
    "footprint_id": "Resistor_SMD:R_0805_2012Metric",
    "pad_functions": [
      {"function": "A", "pad": "1"},
      {"function": "B", "pad": "2"}
    ],
    "verification": {"confidence": "rule_inferred"}
  }],
  "verification": {"confidence": "rule_inferred"}
}`
}

func writeCatalogFile(t *testing.T, dir string, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write catalog file: %v", err)
	}
}

func assertIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("expected issue code %s in %+v", code, issues)
}
