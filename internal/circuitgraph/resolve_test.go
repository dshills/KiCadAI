package circuitgraph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestResolveCheckedInExamplesAgainstCatalog(t *testing.T) {
	catalog := loadGraphCatalog(t)
	for _, name := range []string{"rc_filter.json", "transistor_switch.json", "usb_c_led_indicator_protected.json", "usb_c_bmp280_breakout.json"} {
		t.Run(name, func(t *testing.T) {
			document := loadGraphExample(t, name)
			resolver := NewResolver(ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
			first, issues := resolver.Resolve(context.Background(), document)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("resolve issues = %#v", issues)
			}
			second, issues := resolver.Resolve(context.Background(), document)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("second resolve issues = %#v", issues)
			}
			if first.ResolutionHash == "" || first.ResolutionHash != second.ResolutionHash {
				t.Fatalf("resolution hashes = %q, %q", first.ResolutionHash, second.ResolutionHash)
			}
			if len(first.Components) != len(document.Components) || len(first.Nets) != len(document.Nets) {
				t.Fatalf("resolved counts = components %d, nets %d", len(first.Components), len(first.Nets))
			}
			for _, net := range first.Nets {
				for _, endpoint := range net.Endpoints {
					if endpoint.Function == "" || len(endpoint.Bindings) == 0 {
						t.Fatalf("unresolved endpoint = %#v", endpoint)
					}
					for _, binding := range endpoint.Bindings {
						if binding.SymbolPin == "" || binding.Pad == "" {
							t.Fatalf("incomplete binding = %#v", binding)
						}
					}
				}
			}
		})
	}
}

func TestResolveNamedMultiUnitLM358Package(t *testing.T) {
	document := namedLM358Document()
	resolver := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"})
	first, issues := resolver.Resolve(context.Background(), document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	second, issues := resolver.Resolve(context.Background(), document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("second resolve issues = %#v", issues)
	}
	if first.ResolutionHash == "" || first.ResolutionHash != second.ResolutionHash {
		t.Fatalf("resolution hashes = %q, %q", first.ResolutionHash, second.ResolutionHash)
	}
	var amplifier ResolvedComponent
	for _, component := range first.Components {
		if component.ComponentID == "opamp.ti.lm358.soic8" {
			amplifier = component
		}
	}
	if amplifier.Instance.ID == "" || amplifier.FootprintID != "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm" {
		t.Fatalf("resolved amplifier = %#v", amplifier)
	}
	if len(amplifier.Units) != 3 || len(amplifier.Symbols) != 3 || len(amplifier.Functions) != 8 {
		t.Fatalf("resolved LM358 units/symbols/functions = %d/%d/%d", len(amplifier.Units), len(amplifier.Symbols), len(amplifier.Functions))
	}
	unitNumbers := map[string]int{}
	for _, unit := range amplifier.Units {
		unitNumbers[unit.ID] = unit.Unit
	}
	if unitNumbers["A"] != 1 || unitNumbers["B"] != 2 || unitNumbers["P"] != 3 {
		t.Fatalf("resolved LM358 unit numbers = %#v", unitNumbers)
	}
	for _, net := range first.Nets {
		for _, endpoint := range net.Endpoints {
			if endpoint.Intent.Component != "amplifier" {
				continue
			}
			if len(endpoint.Bindings) != 1 || endpoint.Bindings[0].UnitID != endpoint.Intent.Unit {
				t.Fatalf("unit-qualified endpoint = %#v", endpoint)
			}
		}
	}
}

func TestResolveNamedMultiUnitLM358FailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Document)
		code   reports.Code
	}{
		{
			name: "missing required power unit",
			mutate: func(document *Document) {
				document.Components[0].Units = document.Components[0].Units[:2]
				placements := document.Schematic.Placements[:0]
				for _, placement := range document.Schematic.Placements {
					if placement.Component != "amplifier" || placement.Unit != "P" {
						placements = append(placements, placement)
					}
				}
				document.Schematic.Placements = placements
				nets := document.Nets[:0]
				for _, net := range document.Nets {
					if net.Name != "5V" && net.Name != "GND" {
						nets = append(nets, net)
					}
				}
				document.Nets = nets
				document.PowerFlags = nil
			},
			code: CodeUnitInvalid,
		},
		{
			name: "unit absent from catalog",
			mutate: func(document *Document) {
				document.Components[0].Units[1].ID = "Q"
				for placementIndex := range document.Schematic.Placements {
					placement := &document.Schematic.Placements[placementIndex]
					if placement.Component == "amplifier" && placement.Unit == "B" {
						placement.Unit = "Q"
					}
				}
				for netIndex := range document.Nets {
					for endpointIndex := range document.Nets[netIndex].Endpoints {
						endpoint := &document.Nets[netIndex].Endpoints[endpointIndex]
						if endpoint.Component == "amplifier" && endpoint.Unit == "B" {
							endpoint.Unit = "Q"
						}
					}
				}
			},
			code: CodeUnitInvalid,
		},
		{
			name: "one physical pad on two nets",
			mutate: func(document *Document) {
				for netIndex := range document.Nets {
					if document.Nets[netIndex].Name == "A_IN" {
						document.Nets[netIndex].Endpoints = append(document.Nets[netIndex].Endpoints, Endpoint{Component: "amplifier", Unit: "A", SelectorKind: SelectorSymbolPin, Selector: "1"})
					}
				}
			},
			code: CodePinmapConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := namedLM358Document()
			tt.mutate(&document)
			_, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t)}).Resolve(context.Background(), document)
			assertGraphIssueCode(t, issues, tt.code)
		})
	}
}

func TestResolveNamedMultiUnitRejectsInvalidCatalogUnitSets(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*components.ComponentRecord)
	}{
		{name: "duplicate named unit", mutate: func(record *components.ComponentRecord) { record.Symbols[1].UnitID = "A" }},
		{name: "mixed named and anonymous units", mutate: func(record *components.ComponentRecord) { record.Symbols[1].UnitID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := loadGraphCatalog(t)
			for index := range catalog.Records {
				if catalog.Records[index].ID == "opamp.ti.lm358.soic8" {
					tt.mutate(&catalog.Records[index])
				}
			}
			_, issues := NewResolver(ResolveOptions{Catalog: catalog}).Resolve(context.Background(), namedLM358Document())
			assertGraphIssueCode(t, issues, CodeUnitInvalid)
		})
	}
}

func TestResolveComponentUnitsDefensivelyRejectsDuplicateDeclarations(t *testing.T) {
	symbols := []components.SymbolBinding{{SymbolID: "Amplifier:Dual", Unit: 1, UnitID: "A", UnitType: components.SymbolUnitFunctional}}
	instance := Component{Units: []ComponentUnit{{ID: "A", Role: "first"}, {ID: "A", Role: "second"}}}
	_, _, issues := resolveComponentUnits("components[0]", instance, symbols)
	assertGraphIssueCode(t, issues, CodeUnitInvalid)
}

func TestResolveFailsClosedForUntrustedConstraints(t *testing.T) {
	base := minimalResolvedDocument()
	catalog := minimalResolvedCatalog()
	tests := []struct {
		name   string
		mutate func(*Document, *components.Catalog)
		code   reports.Code
	}{
		{name: "unknown component", mutate: func(document *Document, _ *components.Catalog) { document.Components[0].ComponentID = "missing" }, code: CodeComponentUnresolved},
		{name: "variant required", mutate: func(document *Document, catalog *components.Catalog) {
			document.Components[0].VariantID = ""
			document.Components[1].VariantID = ""
			catalog.Records[0].Packages = append(catalog.Records[0].Packages, catalog.Records[0].Packages[0])
			catalog.Records[0].Packages[1].ID = "other"
		}, code: CodeComponentAmbiguous},
		{name: "symbol mismatch", mutate: func(document *Document, _ *components.Catalog) {
			document.Components[0].Symbol = &LibraryConstraint{LibraryID: "Device:Wrong"}
		}, code: CodeSymbolMismatch},
		{name: "footprint mismatch", mutate: func(document *Document, _ *components.Catalog) {
			document.Components[0].Footprint = &LibraryConstraint{LibraryID: "Package:Wrong"}
		}, code: CodeFootprintMismatch},
		{name: "fabricated pin", mutate: func(document *Document, _ *components.Catalog) { document.Nets[0].Endpoints[0].Selector = "99" }, code: CodePinUnresolved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := Normalize(base)
			localCatalog := cloneCatalog(t, catalog)
			test.mutate(&document, localCatalog)
			_, issues := resolveDocument(context.Background(), document, ResolveOptions{Catalog: localCatalog})
			assertGraphIssueCode(t, issues, test.code)
		})
	}
}

func TestResolveRequiresTrustedLibraryPinAndPadEvidence(t *testing.T) {
	document := minimalResolvedDocument()
	catalog := minimalResolvedCatalog()
	options := ResolveOptions{
		Catalog: catalog, RequireLibraryEvidence: true,
		LibrarySymbols: map[string]LibrarySymbolEvidence{
			"Amplifier:Dual": {LibraryID: "Amplifier:Dual", Pins: map[string]struct{}{"1": {}, "2": {}}},
		},
		LibraryFootprints: map[string]LibraryFootprintEvidence{
			"Package:Dual": {LibraryID: "Package:Dual", Pads: map[string]struct{}{"1": {}, "2": {}}},
		},
	}
	resolver := NewResolver(options)
	resolved, issues := resolver.Resolve(context.Background(), document)
	if reports.HasBlockingIssue(issues) || resolved.ResolutionHash == "" {
		t.Fatalf("trusted resolution issues = %#v", issues)
	}

	delete(options.LibrarySymbols["Amplifier:Dual"].Pins, "2")
	resolver = NewResolver(options)
	_, issues = resolver.Resolve(context.Background(), document)
	assertGraphIssueCode(t, issues, CodePinUnresolved)

	options.LibrarySymbols["Amplifier:Dual"].Pins["2"] = struct{}{}
	delete(options.LibraryFootprints["Package:Dual"].Pads, "2")
	resolver = NewResolver(options)
	_, issues = resolver.Resolve(context.Background(), document)
	assertGraphIssueCode(t, issues, CodePadUnresolved)
}

func TestResolveCollectsAllMultiUnitSymbolSources(t *testing.T) {
	document := minimalResolvedDocument()
	catalog := minimalResolvedCatalog()
	catalog.Records[0].Symbols[1].SymbolID = "Amplifier:DualB"
	resolved, issues := resolveDocument(context.Background(), document, ResolveOptions{
		Catalog: catalog, RequireLibraryEvidence: true,
		LibrarySymbols: map[string]LibrarySymbolEvidence{
			"Amplifier:Dual":  {Pins: map[string]struct{}{"1": {}}, Source: "symbols-a.kicad_sym"},
			"Amplifier:DualB": {Pins: map[string]struct{}{"2": {}}, Source: "symbols-b.kicad_sym"},
		},
		LibraryFootprints: map[string]LibraryFootprintEvidence{
			"Package:Dual": {Pads: map[string]struct{}{"1": {}, "2": {}}, Source: "dual.kicad_mod"},
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	want := []string{"symbols-a.kicad_sym", "symbols-b.kicad_sym"}
	if got := resolved.Components[0].SymbolSources; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("symbol sources = %#v", got)
	}
}

func TestResolveMultiUnitEndpointAndRequiredFunction(t *testing.T) {
	document := minimalResolvedDocument()
	resolved, issues := resolveDocument(context.Background(), document, ResolveOptions{Catalog: minimalResolvedCatalog()})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve issues = %#v", issues)
	}
	endpoint := resolved.Nets[0].Endpoints[0]
	if endpoint.Function != "OUT" || len(endpoint.Bindings) != 1 || endpoint.Bindings[0].Unit != 2 || endpoint.Bindings[0].SymbolPin != "2" {
		t.Fatalf("resolved endpoint = %#v", endpoint)
	}

	for index := range document.Nets[0].Endpoints {
		document.Nets[0].Endpoints[index].Unit = "A"
		document.Nets[0].Endpoints[index].Selector = "IN"
	}
	document.NoConnects = nil
	_, issues = resolveDocument(context.Background(), document, ResolveOptions{Catalog: minimalResolvedCatalog()})
	assertGraphIssueCode(t, issues, CodeRequiredPinOpen)
}

func TestResolveRejectsPinAssignedToNetAndNoConnect(t *testing.T) {
	document := minimalResolvedDocument()
	document.NoConnects = append(document.NoConnects, Endpoint{Component: "u1", Unit: "B", SelectorKind: SelectorSymbolPin, Selector: "2"})
	_, issues := resolveDocument(context.Background(), document, ResolveOptions{Catalog: minimalResolvedCatalog()})
	assertGraphIssueCode(t, issues, CodePinmapConflict)
}

func TestResolveRejectsAmbiguousUnsafeAndRatingDeficientSelection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Document, *components.Catalog)
		code   reports.Code
	}{
		{
			name: "ambiguous query",
			mutate: func(document *Document, catalog *components.Catalog) {
				for index := range document.Components {
					document.Components[index].ComponentID = ""
					document.Components[index].VariantID = ""
					document.Components[index].Query = &ComponentQuery{Family: "amplifier"}
				}
				other := catalog.Records[0]
				other.ID = "amplifier.dual.other"
				catalog.Records = append(catalog.Records, other)
			},
			code: CodeComponentAmbiguous,
		},
		{
			name: "unsafe confidence",
			mutate: func(_ *Document, catalog *components.Catalog) {
				catalog.Records[0].Verification.Confidence = components.ConfidencePlaceholder
			},
			code: CodeComponentUnresolved,
		},
		{
			name: "rating deficient",
			mutate: func(document *Document, _ *components.Catalog) {
				document.Components[0].RequiredRatings = []RequiredRating{{Kind: "supply_voltage", Value: "5", Unit: "V"}}
			},
			code: CodeComponentUnresolved,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := minimalResolvedDocument()
			catalog := minimalResolvedCatalog()
			test.mutate(&document, catalog)
			_, issues := resolveDocument(context.Background(), document, ResolveOptions{Catalog: catalog})
			assertGraphIssueCode(t, issues, test.code)
		})
	}
}

func TestResolveVerifiedAlias(t *testing.T) {
	document := minimalResolvedDocument()
	catalog := minimalResolvedCatalog()
	catalog.Records[0].Symbols[1].FunctionPins[0].Aliases = []string{"OUTPUT"}
	catalog.Records[0].Packages[0].PadFunctions[1].Aliases = []string{"OUTPUT"}
	for index := range document.Nets[0].Endpoints {
		document.Nets[0].Endpoints[index].SelectorKind = SelectorAlias
		document.Nets[0].Endpoints[index].Selector = "output"
	}
	resolved, issues := resolveDocument(context.Background(), document, ResolveOptions{Catalog: catalog})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("resolve alias issues = %#v", issues)
	}
	if got := resolved.Nets[0].Endpoints[0].Function; got != "OUT" {
		t.Fatalf("resolved alias function = %q", got)
	}
}

func TestResolveRequiresUnitForRepeatedFunctionAcrossUnits(t *testing.T) {
	document := minimalResolvedDocument()
	catalog := minimalResolvedCatalog()
	catalog.Records[0].Symbols[0].FunctionPins = append(catalog.Records[0].Symbols[0].FunctionPins,
		components.FunctionPin{Function: "OUT", SymbolPin: "3", Required: true})
	catalog.Records[0].Packages[0].PadFunctions = append(catalog.Records[0].Packages[0].PadFunctions,
		components.PadFunction{Function: "OUT", Pad: "3"})
	document.Nets[0].Endpoints[0].Unit = ""
	document.NoConnects = append(document.NoConnects, Endpoint{Component: "u1", Unit: "A", SelectorKind: SelectorSymbolPin, Selector: "3"})
	_, issues := resolveDocument(context.Background(), document, ResolveOptions{Catalog: catalog})
	assertGraphIssueCode(t, issues, CodePinmapConflict)
}

func TestResolveRequiresEveryPhysicalPinOfRequiredFunction(t *testing.T) {
	document := minimalResolvedDocument()
	catalog := minimalResolvedCatalog()
	catalog.Records[0].Symbols[1].FunctionPins = append(catalog.Records[0].Symbols[1].FunctionPins,
		components.FunctionPin{Function: "OUT", SymbolPin: "4", Required: true})
	catalog.Records[0].Packages[0].PadFunctions = append(catalog.Records[0].Packages[0].PadFunctions,
		components.PadFunction{Function: "OUT", Pad: "4"})
	document.Nets[0].Endpoints[0].SelectorKind = SelectorSymbolPin
	document.Nets[0].Endpoints[0].Selector = "2"
	_, issues := resolveDocument(context.Background(), document, ResolveOptions{Catalog: catalog})
	assertGraphIssueCode(t, issues, CodeRequiredPinOpen)
}

func TestResolveFunctionsRejectsAmbiguousFallbackPads(t *testing.T) {
	symbols := []components.SymbolBinding{{
		SymbolID: "Device:Multi", Unit: 1,
		FunctionPins: []components.FunctionPin{{Function: "IO", SymbolPin: "X"}, {Function: "IO", SymbolPin: "Y"}},
	}}
	variant := components.PackageVariant{PadFunctions: []components.PadFunction{{Function: "io", Pad: "A"}, {Function: "io", Pad: "X"}}}
	_, issues := resolveFunctions("components[0]", symbols, variant)
	assertGraphIssueCode(t, issues, CodePinmapConflict)
}

func TestResolveFunctionsAllowsOneToOneLogicalBinding(t *testing.T) {
	symbols := []components.SymbolBinding{{SymbolID: "Device:One", FunctionPins: []components.FunctionPin{{Function: "IO", SymbolPin: "A"}}}}
	variant := components.PackageVariant{PadFunctions: []components.PadFunction{{Function: "io", Pad: "1"}}}
	functions, issues := resolveFunctions("components[0]", symbols, variant)
	if reports.HasBlockingIssue(issues) || len(functions) != 1 || functions[0].SymbolPin != "A" || functions[0].Pad != "1" {
		t.Fatalf("one-to-one logical binding = %#v, issues = %#v", functions, issues)
	}
}

func TestResolveFunctionsRejectsCaseCollisionWithinSource(t *testing.T) {
	symbols := []components.SymbolBinding{{SymbolID: "Device:Collision", FunctionPins: []components.FunctionPin{
		{Function: "SS", SymbolPin: "1"}, {Function: "ss", SymbolPin: "2"},
	}}}
	variant := components.PackageVariant{PadFunctions: []components.PadFunction{{Function: "SS", Pad: "1"}, {Function: "SS", Pad: "2"}}}
	_, issues := resolveFunctions("components[0]", symbols, variant)
	assertGraphIssueCode(t, issues, CodePinmapConflict)
}

func TestResolveFunctionsRejectsDifferentMultiUnitPinsOnOnePad(t *testing.T) {
	symbols := []components.SymbolBinding{
		{SymbolID: "Device:Shared", Unit: 1, FunctionPins: []components.FunctionPin{{Function: "COMMON", SymbolPin: "A"}}},
		{SymbolID: "Device:Shared", Unit: 2, FunctionPins: []components.FunctionPin{{Function: "COMMON", SymbolPin: "B"}}},
	}
	variant := components.PackageVariant{PadFunctions: []components.PadFunction{{Function: "COMMON", Pad: "1"}}}
	_, issues := resolveFunctions("components[0]", symbols, variant)
	assertGraphIssueCode(t, issues, CodePinmapConflict)
}

func TestResolveFunctionsAllowsSharedPhysicalPinAcrossUnits(t *testing.T) {
	symbols := []components.SymbolBinding{
		{SymbolID: "Device:Shared", Unit: 1, FunctionPins: []components.FunctionPin{{Function: "COMMON", SymbolPin: "1"}}},
		{SymbolID: "Device:Shared", Unit: 2, FunctionPins: []components.FunctionPin{{Function: "COMMON", SymbolPin: "1"}}},
	}
	variant := components.PackageVariant{PadFunctions: []components.PadFunction{{Function: "COMMON", Pad: "1"}}}
	functions, issues := resolveFunctions("components[0]", symbols, variant)
	if reports.HasBlockingIssue(issues) || len(functions) != 2 {
		t.Fatalf("shared physical pin = %#v, issues = %#v", functions, issues)
	}
}

func TestResolverCatalogHashIgnoresTopLevelCatalogOrder(t *testing.T) {
	first := minimalResolvedCatalog()
	other := first.Records[0]
	other.ID = "amplifier.dual.other"
	first.Records = append(first.Records, other)
	first.Families = append(first.Families, components.FamilyDefinition{ID: "other", Name: "Other"})
	second := cloneCatalog(t, first)
	second.Records[0], second.Records[1] = second.Records[1], second.Records[0]
	second.Families[0], second.Families[1] = second.Families[1], second.Families[0]
	if left, right := NewResolver(ResolveOptions{Catalog: first}).catalogHash, NewResolver(ResolveOptions{Catalog: second}).catalogHash; left == "" || left != right {
		t.Fatalf("catalog hashes = %q, %q", left, right)
	}
}

func TestResolverUsesTrustedPrecomputedCatalogHash(t *testing.T) {
	want := strings.Repeat("a", sha256HexLength)
	resolver := NewResolver(ResolveOptions{Catalog: minimalResolvedCatalog(), CatalogHash: want})
	if resolver.catalogHash != want {
		t.Fatalf("catalog hash = %q", resolver.catalogHash)
	}
}

func TestResolveRejectsPowerFlagOnInternalPowerOutput(t *testing.T) {
	document := loadGraphExample(t, "usb_c_bmp280_breakout.json")
	document.PowerFlags = []PowerFlag{{Net: "VCC_3v3"}}
	_, issues := NewResolver(ResolveOptions{Catalog: loadGraphCatalog(t), CatalogID: "checked-in"}).Resolve(context.Background(), document)
	if !hasIssue(issues, string(CodePowerFlagInvalid), "power_flags[0].net") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateResolvedPowerFlagsRejectsMissingResolvedNet(t *testing.T) {
	issues := validateResolvedPowerFlags([]PowerFlag{{Net: "MISSING"}}, nil)
	if !hasIssue(issues, string(CodePowerFlagInvalid), "power_flags[0].net") {
		t.Fatalf("issues = %#v", issues)
	}
}

const sha256HexLength = 64

func TestNilResolverAndCatalogFailClosed(t *testing.T) {
	document := minimalResolvedDocument()
	var nilResolver *Resolver
	_, issues := nilResolver.Resolve(context.Background(), document)
	assertGraphIssueCode(t, issues, CodeComponentUnresolved)
	_, issues = NewResolver(ResolveOptions{}).Resolve(context.Background(), document)
	assertGraphIssueCode(t, issues, CodeComponentUnresolved)
}

func namedLM358Document() Document {
	trueValue := true
	falseValue := false
	return Document{
		Schema: SchemaID, Version: Version,
		Project: Project{Name: "named_lm358", Acceptance: AcceptanceERCDRC, Board: Board{WidthMM: 50, HeightMM: 35, Layers: 2, EdgeClearanceMM: 0.5}},
		Components: []Component{
			{ID: "amplifier", Reference: "U1", Role: RoleIC, Units: []ComponentUnit{{ID: "A", Role: "reference_buffer"}, {ID: "B", Role: "gain_stage"}, {ID: "P", Role: "power"}}, ComponentID: "opamp.ti.lm358.soic8", VariantID: "soic8", Population: PopulationPopulate},
			{ID: "power", Reference: "J1", Role: RoleInputConnector, ComponentID: "connector.pinheader.1x02.2_54mm", VariantID: "vertical", Population: PopulationPopulate},
			{ID: "signal", Reference: "J2", Role: RoleInputConnector, ComponentID: "connector.pinheader.1x02.2_54mm", VariantID: "vertical", Population: PopulationPopulate},
		},
		Nets: []Net{
			{Name: "5V", Role: NetRolePowerPos, Required: &trueValue, Endpoints: []Endpoint{{Component: "power", SelectorKind: SelectorFunction, Selector: "PIN_1"}, {Component: "amplifier", Unit: "P", SelectorKind: SelectorFunction, Selector: "V_PLUS"}}},
			{Name: "GND", Role: NetRoleGround, Required: &trueValue, Endpoints: []Endpoint{{Component: "power", SelectorKind: SelectorFunction, Selector: "PIN_2"}, {Component: "amplifier", Unit: "P", SelectorKind: SelectorFunction, Selector: "V_MINUS"}}},
			{Name: "A_IN", Role: NetRoleSignal, Required: &trueValue, Endpoints: []Endpoint{{Component: "signal", SelectorKind: SelectorFunction, Selector: "PIN_1"}, {Component: "amplifier", Unit: "A", SelectorKind: SelectorFunction, Selector: "IN_PLUS"}}},
			{Name: "A_OUT", Role: NetRoleSignal, Required: &trueValue, Endpoints: []Endpoint{{Component: "amplifier", Unit: "A", SelectorKind: SelectorFunction, Selector: "OUT"}, {Component: "amplifier", Unit: "A", SelectorKind: SelectorFunction, Selector: "IN_MINUS"}}},
			{Name: "B_IN", Role: NetRoleSignal, Required: &trueValue, Endpoints: []Endpoint{{Component: "signal", SelectorKind: SelectorFunction, Selector: "PIN_2"}, {Component: "amplifier", Unit: "B", SelectorKind: SelectorFunction, Selector: "IN_PLUS"}}},
			{Name: "B_OUT", Role: NetRoleSignal, Required: &trueValue, Endpoints: []Endpoint{{Component: "amplifier", Unit: "B", SelectorKind: SelectorFunction, Selector: "OUT"}, {Component: "amplifier", Unit: "B", SelectorKind: SelectorFunction, Selector: "IN_MINUS"}}},
		},
		PowerFlags: []PowerFlag{{Net: "5V"}, {Net: "GND"}}, NoConnects: []Endpoint{}, Buses: []Bus{},
		Schematic: SchematicIntent{
			Flow: FlowLeftToRight, Origin: OriginCentered,
			Groups:     []SchematicGroup{{ID: "analog", Role: "processing_stage", Members: []string{"amplifier", "power", "signal"}, Rank: 0}},
			Lanes:      SchematicLanes{Power: LaneTop, Signals: LaneMiddle, Ground: LaneBottom},
			Placements: []SchematicPlacement{{Component: "amplifier", Group: "analog"}, {Component: "amplifier", Unit: "A", Group: "analog"}, {Component: "amplifier", Unit: "B", Group: "analog"}, {Component: "amplifier", Unit: "P", Group: "analog"}, {Component: "power", Group: "analog"}, {Component: "signal", Group: "analog", RightOf: "power"}},
			Rules:      SchematicRules{PositivePowerTop: &trueValue, GroundBottom: &trueValue, CenterOnPage: &trueValue, PreferLabelsForLongNets: &trueValue, AvoidWireCrossings: &trueValue, MinGroupSpacingMM: 12.7, MinComponentSpacingMM: 7.62},
			Hierarchy:  HierarchyPolicy{Mode: "flat"},
		},
		PCB: PCBIntent{
			Regions:    []PCBRegion{{ID: "main", Bounds: Bounds{XMM: 2, YMM: 2, WidthMM: 46, HeightMM: 31}}},
			Placements: []PCBPlacement{{Component: "amplifier", Region: "main"}, {Component: "power", Region: "main"}, {Component: "signal", Region: "main"}},
			Keepouts:   []PCBKeepout{}, Zones: []PCBZone{},
		},
		Policy: Policy{AllowReferenceAssignment: &trueValue, AllowValueNormalization: &trueValue, AllowLayoutInference: &trueValue, AllowSpacingAdjustment: &trueValue, AllowLabelInsertion: &trueValue, AllowPlacementAdjustment: &trueValue, AllowRouteRetry: &falseValue},
	}
}

func minimalResolvedDocument() Document {
	return Document{
		Schema: SchemaID, Version: Version,
		Project: Project{Name: "multi_unit", Acceptance: AcceptanceStructural, Board: Board{WidthMM: 30, HeightMM: 20, Layers: 2}},
		Components: []Component{
			{ID: "u1", Role: RoleIC, ComponentID: "amplifier.dual", VariantID: "soic8", RequiredFunctions: []string{"out"}, Population: PopulationPopulate},
			{ID: "u2", Role: RoleIC, ComponentID: "amplifier.dual", VariantID: "soic8", RequiredFunctions: []string{"out"}, Population: PopulationPopulate},
		},
		Nets: []Net{{Name: "OUT", Role: NetRoleSignal, Required: graphBool(true), Endpoints: []Endpoint{
			{Component: "u1", Unit: "B", SelectorKind: SelectorFunction, Selector: "OUT"},
			{Component: "u2", Unit: "B", SelectorKind: SelectorFunction, Selector: "OUT"},
		}}},
		NoConnects: []Endpoint{
			{Component: "u1", Unit: "A", SelectorKind: SelectorFunction, Selector: "IN"},
			{Component: "u2", Unit: "A", SelectorKind: SelectorFunction, Selector: "IN"},
		},
		Schematic: SchematicIntent{
			Flow: FlowLeftToRight, Origin: OriginCentered,
			Groups:     []SchematicGroup{{ID: "main", Members: []string{"u1", "u2"}}},
			Lanes:      SchematicLanes{Power: LaneTop, Signals: LaneMiddle, Ground: LaneBottom},
			Placements: []SchematicPlacement{{Component: "u1", Group: "main"}, {Component: "u2", Group: "main", RightOf: "u1"}},
			Rules: SchematicRules{
				PositivePowerTop: graphBool(true), GroundBottom: graphBool(true), CenterOnPage: graphBool(true),
				PreferLabelsForLongNets: graphBool(true), AvoidWireCrossings: graphBool(true),
				MinGroupSpacingMM: 10, MinComponentSpacingMM: 5,
			},
			Hierarchy: HierarchyPolicy{Mode: "flat"},
		},
		PCB: PCBIntent{
			Regions:    []PCBRegion{{ID: "main", Bounds: Bounds{XMM: 1, YMM: 1, WidthMM: 28, HeightMM: 18}}},
			Placements: []PCBPlacement{{Component: "u1", Region: "main"}, {Component: "u2", Region: "main", Near: "u1"}},
		},
		Policy: Policy{
			AllowReferenceAssignment: graphBool(true), AllowValueNormalization: graphBool(true), AllowLayoutInference: graphBool(true),
			AllowSpacingAdjustment: graphBool(true), AllowLabelInsertion: graphBool(true), AllowPlacementAdjustment: graphBool(true), AllowRouteRetry: graphBool(true),
		},
	}
}

func graphBool(value bool) *bool { return &value }

func resolveDocument(ctx context.Context, document Document, options ResolveOptions) (ResolvedDocument, []reports.Issue) {
	return NewResolver(options).Resolve(ctx, document)
}

func minimalResolvedCatalog() *components.Catalog {
	return &components.Catalog{
		Version:  components.CatalogVersion,
		Families: []components.FamilyDefinition{{ID: "amplifier", Name: "Amplifier"}},
		Records: []components.ComponentRecord{{
			ID: "amplifier.dual", Family: "amplifier", Name: "Dual amplifier",
			Symbols: []components.SymbolBinding{
				{SymbolID: "Amplifier:Dual", Unit: 1, FunctionPins: []components.FunctionPin{{Function: "IN", SymbolPin: "1", Required: true}}, Verification: components.VerificationRecord{Confidence: components.ConfidenceVerified}},
				{SymbolID: "Amplifier:Dual", Unit: 2, FunctionPins: []components.FunctionPin{{Function: "OUT", SymbolPin: "2", Required: true}}, Verification: components.VerificationRecord{Confidence: components.ConfidenceVerified}},
			},
			Packages: []components.PackageVariant{{
				ID: "soic8", FootprintID: "Package:Dual", PinMapID: "dual",
				PadFunctions: []components.PadFunction{{Function: "IN", Pad: "1"}, {Function: "OUT", Pad: "2"}},
				Verification: components.VerificationRecord{Confidence: components.ConfidenceVerified},
			}},
			Verification: components.VerificationRecord{Confidence: components.ConfidenceVerified},
		}},
	}
}

func loadGraphExample(t *testing.T, name string) Document {
	t.Helper()
	file, err := os.Open(filepath.Join(circuitGraphExamplesRoot(t), name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	document, issues := DecodeStrict(file)
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	return document
}

func loadGraphCatalog(t *testing.T) *components.Catalog {
	t.Helper()
	root := filepath.Clean(filepath.Join(circuitGraphExamplesRoot(t), "..", ".."))
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{CatalogDir: filepath.Join(root, "data", "components")})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func cloneCatalog(t *testing.T, catalog *components.Catalog) *components.Catalog {
	t.Helper()
	clone := &components.Catalog{
		Version: catalog.Version, GeneratedAt: catalog.GeneratedAt,
		Records:     append([]components.ComponentRecord(nil), catalog.Records...),
		Families:    append([]components.FamilyDefinition(nil), catalog.Families...),
		Diagnostics: append([]reports.Issue(nil), catalog.Diagnostics...),
	}
	for index := range clone.Records {
		clone.Records[index].Packages = append([]components.PackageVariant(nil), catalog.Records[index].Packages...)
	}
	return clone
}

func assertGraphIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("issues %#v do not contain %s", issues, code)
}
