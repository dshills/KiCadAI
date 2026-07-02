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

func TestCheckedInCatalogRegulatorSliceEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}

	regulator := requireCatalogRecord(t, catalog, "regulator.linear.ams1117_3v3.sot223")
	if regulator.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("regulator confidence = %q", regulator.Verification.Confidence)
	}
	requireRatingMax(t, regulator, "input_voltage", "12", "V")
	requireRatingMax(t, regulator, "output_current", "800", "mA")
	requireValueTyp(t, regulator, "output_voltage", "3.3", "V")
	for _, role := range []string{"input_capacitor", "output_capacitor"} {
		requireCompanionRole(t, regulator, role)
	}
	requireSymbolFunctions(t, regulator, "Regulator_Linear:AMS1117-3.3", []string{"GND", "VOUT", "VIN"})
	requirePackagePads(t, regulator, "sot223", []string{"GND", "VOUT", "VIN"})

	ap2112 := requireCatalogRecord(t, catalog, "regulator.linear.ap2112k_3v3.sot23_5")
	if ap2112.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("AP2112K confidence = %q", ap2112.Verification.Confidence)
	}
	requireRatingMinMax(t, ap2112, "input_voltage", "3.8", "6", "V")
	requireRatingMax(t, ap2112, "output_current", "600", "mA")
	requireRatingMax(t, ap2112, "enable_voltage", "6", "V")
	requireRatingMax(t, ap2112, "enable_voltage_abs_max", "6.5", "V")
	requireRatingMax(t, ap2112, "power_dissipation_max", "250", "mW")
	requireValueTyp(t, ap2112, "output_voltage", "3.3", "V")
	requireValueMax(t, ap2112, "dropout_voltage", "400", "mV")
	requireValueTyp(t, ap2112, "headroom_margin", "100", "mV")
	for _, role := range []string{"input_capacitor", "output_capacitor"} {
		requireCompanionRole(t, ap2112, role)
	}
	requireSymbolFunctions(t, ap2112, "Regulator_Linear:AP2112K-3.3", []string{"VIN", "GND", "EN", "NC", "VOUT"})
	requirePackagePads(t, ap2112, "sot23_5", []string{"VIN", "GND", "EN", "NC", "VOUT"})
	requireDeratingRule(t, ap2112, "thermal")
	requireDeratingRule(t, ap2112, "enable_voltage")
	requireDeratingRule(t, ap2112, "capacitor_stability")
	requireRegulatorStability(t, regulator, "esr_window_required", true)
	requireRegulatorStability(t, ap2112, "ceramic_stable", true)

	capacitor := requireCatalogRecord(t, catalog, "capacitor.ceramic.0805")
	if capacitor.Verification.Confidence != ConfidenceRuleInferred {
		t.Fatalf("capacitor confidence = %q", capacitor.Verification.Confidence)
	}
	requireValueMax(t, capacitor, "capacitance", "100u", "F")
	requireRatingMax(t, capacitor, "voltage", "25", "V")
	requireToleranceMax(t, capacitor, "capacitance", "20", "%")
	requireSymbolFunctions(t, capacitor, "Device:C", []string{"A", "B"})
	requirePackagePads(t, capacitor, "0805", []string{"A", "B"})
	requireCapacitorEvidence(t, capacitor, "unknown", true)

	requireCapacitorEvidence(t, requireCatalogRecord(t, catalog, "capacitor.murata.grm21br71h104ka01l.0805"), "X7R", true)
	requireCapacitorEvidence(t, requireCatalogRecord(t, catalog, "capacitor.murata.grm188r71h104ka93d.0603"), "X7R", true)
	requireCapacitorEvidence(t, requireCatalogRecord(t, catalog, "capacitor.murata.grm21br61a106ke19l.0805"), "X5R", true)
}

func checkedInCatalogDir(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "data", "components")
}

func requireCatalogRecord(t *testing.T, catalog *Catalog, id string) *ComponentRecord {
	t.Helper()
	for index := range catalog.Records {
		if catalog.Records[index].ID == id {
			return &catalog.Records[index]
		}
	}
	t.Fatalf("catalog missing record %s", id)
	return nil
}

func requireRatingMax(t *testing.T, record *ComponentRecord, kind, max, unit string) {
	t.Helper()
	for _, rating := range record.Ratings {
		if rating.Kind == kind && rating.Unit == unit && rating.Max == max {
			return
		}
	}
	t.Fatalf("%s missing max rating %s=%s%s: %+v", record.ID, kind, max, unit, record.Ratings)
}

func requireRatingMinMax(t *testing.T, record *ComponentRecord, kind, min, max, unit string) {
	t.Helper()
	for _, rating := range record.Ratings {
		if rating.Kind == kind && rating.Unit == unit && rating.Min == min && rating.Max == max {
			return
		}
	}
	t.Fatalf("%s missing min/max rating %s=%s..%s%s: %+v", record.ID, kind, min, max, unit, record.Ratings)
}

func requireValueTyp(t *testing.T, record *ComponentRecord, kind, typ, unit string) {
	t.Helper()
	for _, value := range record.Values {
		if value.Kind == kind && value.Unit == unit && value.Typ == typ {
			return
		}
	}
	t.Fatalf("%s missing typ value %s=%s%s: %+v", record.ID, kind, typ, unit, record.Values)
}

func requireValueMax(t *testing.T, record *ComponentRecord, kind, max, unit string) {
	t.Helper()
	for _, value := range record.Values {
		if value.Kind == kind && value.Unit == unit && value.Max == max {
			return
		}
	}
	t.Fatalf("%s missing max value %s=%s%s: %+v", record.ID, kind, max, unit, record.Values)
}

func requireToleranceMax(t *testing.T, record *ComponentRecord, kind, max, unit string) {
	t.Helper()
	for _, tolerance := range record.Tolerances {
		if tolerance.Kind == kind && tolerance.Unit == unit && tolerance.Max == max {
			return
		}
	}
	t.Fatalf("%s missing max tolerance %s=%s%s: %+v", record.ID, kind, max, unit, record.Tolerances)
}

func requireCompanionRole(t *testing.T, record *ComponentRecord, role string) {
	t.Helper()
	for _, companion := range record.Companions {
		if companion.Role == role && companion.Required {
			return
		}
	}
	t.Fatalf("%s missing required companion role %s: %+v", record.ID, role, record.Companions)
}

func requireDeratingRule(t *testing.T, record *ComponentRecord, kind string) {
	t.Helper()
	for _, rule := range record.DeratingRules {
		if rule.Kind == kind {
			return
		}
	}
	t.Fatalf("%s missing derating rule %s: %+v", record.ID, kind, record.DeratingRules)
}

func requireRegulatorStability(t *testing.T, record *ComponentRecord, kind string, blocksFabrication bool) {
	t.Helper()
	if record.Regulator == nil || record.Regulator.OutputCapacitor == nil {
		t.Fatalf("%s missing regulator output-capacitor evidence", record.ID)
	}
	stability := record.Regulator.OutputCapacitor
	if stability.Kind != kind {
		t.Fatalf("%s stability kind = %q, want %q", record.ID, stability.Kind, kind)
	}
	if stability.FabricationCandidateBlocks != blocksFabrication {
		t.Fatalf("%s fabrication block = %t, want %t", record.ID, stability.FabricationCandidateBlocks, blocksFabrication)
	}
}

func requireCapacitorEvidence(t *testing.T, record *ComponentRecord, dielectric string, blocksFabrication bool) {
	t.Helper()
	if record.Capacitor == nil {
		t.Fatalf("%s missing capacitor evidence", record.ID)
	}
	if record.Capacitor.Dielectric != dielectric {
		t.Fatalf("%s dielectric = %q, want %q", record.ID, record.Capacitor.Dielectric, dielectric)
	}
	if record.Capacitor.FabricationCandidateBlocks != blocksFabrication {
		t.Fatalf("%s fabrication block = %t, want %t", record.ID, record.Capacitor.FabricationCandidateBlocks, blocksFabrication)
	}
}

func requireSymbolFunctions(t *testing.T, record *ComponentRecord, symbolID string, functions []string) {
	t.Helper()
	for _, symbol := range record.Symbols {
		if symbol.SymbolID != symbolID {
			continue
		}
		for _, function := range functions {
			if !symbolHasFunction(symbol, function) {
				t.Fatalf("%s symbol %s missing function %s: %+v", record.ID, symbolID, function, symbol.FunctionPins)
			}
		}
		return
	}
	t.Fatalf("%s missing symbol %s", record.ID, symbolID)
}

func requirePackagePads(t *testing.T, record *ComponentRecord, packageID string, functions []string) {
	t.Helper()
	for _, pkg := range record.Packages {
		if pkg.ID != packageID {
			continue
		}
		for _, function := range functions {
			if !packageHasPadFunction(pkg, function) {
				t.Fatalf("%s package %s missing pad function %s: %+v", record.ID, packageID, function, pkg.PadFunctions)
			}
		}
		return
	}
	t.Fatalf("%s missing package %s", record.ID, packageID)
}

func symbolHasFunction(symbol SymbolBinding, function string) bool {
	for _, pin := range symbol.FunctionPins {
		if pin.Function == function && pin.SymbolPin != "" {
			return true
		}
	}
	return false
}

func packageHasPadFunction(pkg PackageVariant, function string) bool {
	for _, pad := range pkg.PadFunctions {
		if pad.Function == function && pad.Pad != "" {
			return true
		}
	}
	return false
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

func TestValidateCatalogEquivalenceMetadata(t *testing.T) {
	catalog := validCatalog()
	catalog.Records = append(catalog.Records, catalog.Records[0])
	catalog.Records[0].ID = "resistor.yageo.10k.0805"
	catalog.Records[0].Generic = false
	catalog.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
	catalog.Records[1].ID = "resistor.generic.10k.0805"
	catalog.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceFallback}

	result := ValidateCatalog(&catalog)
	if !result.OK {
		t.Fatalf("expected equivalence metadata to validate: %+v", result.Issues)
	}
}

func TestValidateCatalogInvalidEquivalenceMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Catalog)
	}{
		{
			name: "missing group",
			mutate: func(c *Catalog) {
				c.Records[0].Equivalence = &EquivalenceMetadata{Role: EquivalencePreferred}
			},
		},
		{
			name: "invalid role",
			mutate: func(c *Catalog) {
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: "maybe"}
			},
		},
		{
			name: "multiple preferred",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.a.0805"
				c.Records[1].ID = "resistor.b.0805"
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
			},
		},
		{
			name: "missing preferred",
			mutate: func(c *Catalog) {
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
		},
		{
			name: "incompatible values",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.10k.0805"
				c.Records[1].ID = "resistor.1k.0805"
				c.Records[1].Values = []ValueConstraint{{Kind: "resistance", Typ: "1k", Unit: "ohm"}}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
		},
		{
			name: "incompatible package",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.0805"
				c.Records[1].ID = "resistor.0603"
				c.Records[1].Packages = []PackageVariant{{
					ID:          "0603",
					Name:        "0603",
					FootprintID: "Resistor_SMD:R_0603_1608Metric",
					PadFunctions: []PadFunction{
						{Function: "A", Pad: "1"},
						{Function: "B", Pad: "2"},
					},
					Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
				}}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
			},
		},
		{
			name: "incompatible pad map",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.a.0805"
				c.Records[1].ID = "resistor.b.0805"
				c.Records[1].Packages = []PackageVariant{c.Records[1].Packages[0]}
				c.Records[1].Packages[0].PadFunctions = []PadFunction{
					{Function: "A", Pad: "2"},
					{Function: "B", Pad: "1"},
				}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
		},
		{
			name: "incompatible rating",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.a.0805"
				c.Records[1].ID = "resistor.b.0805"
				c.Records[0].Ratings = []RatingConstraint{{Kind: "power", Max: "125", Unit: "mW"}}
				c.Records[1].Ratings = []RatingConstraint{{Kind: "power", Max: "63", Unit: "mW"}}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
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
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
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

func TestValidateCatalogRegulatorEvidenceRejectsMalformedStability(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(record *ComponentRecord)
		path   string
	}{
		{
			name: "invalid stability kind",
			mutate: func(record *ComponentRecord) {
				record.Regulator = &RegulatorEvidence{OutputCapacitor: &RegulatorCapacitorStability{
					Kind:            "magic",
					MinCapacitance:  "10u",
					CapacitanceUnit: "F",
				}}
			},
			path: "records[0].regulator_evidence.output_capacitor.kind",
		},
		{
			name: "missing required capacitance",
			mutate: func(record *ComponentRecord) {
				record.Regulator = &RegulatorEvidence{OutputCapacitor: &RegulatorCapacitorStability{
					Kind: "ceramic_stable",
				}}
			},
			path: "records[0].regulator_evidence.output_capacitor.min_capacitance",
		},
		{
			name: "ESR minimum greater than maximum",
			mutate: func(record *ComponentRecord) {
				record.Regulator = &RegulatorEvidence{OutputCapacitor: &RegulatorCapacitorStability{
					Kind:            "esr_window_required",
					MinCapacitance:  "10u",
					CapacitanceUnit: "F",
					ESRMin:          "2",
					ESRMax:          "0.5",
					ESRUnit:         "ohm",
				}}
			},
			path: "records[0].regulator_evidence.output_capacitor.esr_min",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			tt.mutate(&catalog.Records[0])
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
}

func TestValidateCatalogCapacitorEvidenceRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(record *ComponentRecord)
		path   string
	}{
		{
			name: "invalid nominal capacitance",
			mutate: func(record *ComponentRecord) {
				record.Capacitor = &CapacitorEvidence{
					NominalCapacitance: "many",
					CapacitanceUnit:    "F",
				}
			},
			path: "records[0].capacitor_evidence.nominal_capacitance",
		},
		{
			name: "invalid voltage rating",
			mutate: func(record *ComponentRecord) {
				record.Capacitor = &CapacitorEvidence{
					VoltageRating: "high",
					VoltageUnit:   "V",
				}
			},
			path: "records[0].capacitor_evidence.voltage_rating",
		},
		{
			name: "generic fabrication proof",
			mutate: func(record *ComponentRecord) {
				record.Capacitor = &CapacitorEvidence{
					NominalCapacitance: "100n",
					CapacitanceUnit:    "F",
					FabricationProof:   true,
				}
			},
			path: "records[0].capacitor_evidence.fabrication_proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			catalog.Records[0].Family = "capacitor"
			catalog.Families = append(catalog.Families, FamilyDefinition{ID: "capacitor", Name: "Capacitor"})
			tt.mutate(&catalog.Records[0])
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
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

func assertIssuePath(t *testing.T, issues []reports.Issue, path string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Path == path {
			return
		}
	}
	t.Fatalf("expected issue path %s in %+v", path, issues)
}
