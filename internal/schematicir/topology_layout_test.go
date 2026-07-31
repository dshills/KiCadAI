package schematicir

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

func TestEducationalExamplesInferConventionalTopologyWithoutPlacementRecipes(t *testing.T) {
	cases := []struct {
		directory string
		assert    func(*testing.T, map[string]Placement)
	}{
		{
			directory: "01_dc_voltage_source",
			assert: func(t *testing.T, placements map[string]Placement) {
				if placements["load"].Orientation != OrientationNormal {
					t.Fatalf("load placement = %#v, want vertical rail load", placements["load"])
				}
			},
		},
		{
			directory: "02_bjt_current_source",
			assert: func(t *testing.T, placements map[string]Placement) {
				if !stringSliceContains(placements["output_transistor"].RightOf, "reference_transistor") || !stringSliceContains(placements["output_transistor"].SameRowAs, "reference_transistor") {
					t.Fatalf("current mirror pair was not inferred: %#v", placements)
				}
				if !stringSliceContains(placements["rset"].Above, "reference_transistor") || !stringSliceContains(placements["load"].Above, "output_transistor") {
					t.Fatalf("current mirror collector loads were not inferred: %#v", placements)
				}
			},
		},
		{
			directory: "03_differential_amplifier",
			assert: func(t *testing.T, placements map[string]Placement) {
				if !stringSliceContains(placements["transistor_2"].RightOf, "transistor_1") || !stringSliceContains(placements["transistor_2"].SameRowAs, "transistor_1") {
					t.Fatalf("differential pair symmetry was not inferred: %#v", placements)
				}
				if got := placements["tail_resistor"].CenterBetween; len(got) != 2 || got[0] != "transistor_1" || got[1] != "transistor_2" {
					t.Fatalf("tail center relation = %#v, want transistor pair", got)
				}
				if placements["transistor_2"].Mirror != MirrorY {
					t.Fatalf("second transistor mirror = %q, want y", placements["transistor_2"].Mirror)
				}
			},
		},
		{
			directory: "04_rc_low_pass_filter",
			assert: func(t *testing.T, placements map[string]Placement) {
				if placements["series_resistor"].Orientation != OrientationRotated || placements["shunt_capacitor"].Orientation != OrientationNormal {
					t.Fatalf("RC orientations = series:%q shunt:%q", placements["series_resistor"].Orientation, placements["shunt_capacitor"].Orientation)
				}
				if len(placements["shunt_capacitor"].SameColumnAsPin) != 1 || placements["shunt_capacitor"].SameColumnAsPin[0] != "series_resistor.2" {
					t.Fatalf("shunt branch anchor = %#v", placements["shunt_capacitor"])
				}
			},
		},
		{
			directory: "05_voltage_divider",
			assert: func(t *testing.T, placements map[string]Placement) {
				if !stringSliceContains(placements["upper_resistor"].Above, "lower_resistor") || !stringSliceContains(placements["upper_resistor"].SameColumnAs, "lower_resistor") {
					t.Fatalf("vertical divider was not inferred: %#v", placements)
				}
				if len(placements["output"].SameRowAsPin) != 1 || placements["output"].SameRowAsPin[0] != "upper_resistor.2" {
					t.Fatalf("divider output tap = %#v", placements["output"])
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.directory, func(t *testing.T) {
			document := loadEducationalTopologyDocument(t, tc.directory)
			if len(document.Layout.Placements) != 0 {
				t.Fatalf("educational source has %d placement recipes; want graph-derived layout", len(document.Layout.Placements))
			}
			normalized := NormalizeLayoutIntent(document)
			tc.assert(t, placementsByTarget(normalized.Layout.Placements))
		})
	}
}

func TestTopologyAwareLayoutIsStableUnderComponentNetAndEndpointPermutation(t *testing.T) {
	for _, directory := range []string{"02_bjt_current_source", "03_differential_amplifier", "04_rc_low_pass_filter", "05_voltage_divider"} {
		t.Run(directory, func(t *testing.T) {
			document := loadEducationalTopologyDocument(t, directory)
			permuted := document
			permuted.Circuit.Components = reverseComponents(document.Circuit.Components)
			permuted.Circuit.Nets = reverseNets(document.Circuit.Nets)
			for index := range permuted.Circuit.Nets {
				permuted.Circuit.Nets[index].Connect = reverseEndpointRefs(permuted.Circuit.Nets[index].Connect)
			}

			first := LayoutDocument(document)
			second := LayoutDocument(permuted)
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("layout changed under input permutation: positions=%t components=%t connections=%t wires=%t labels=%t junctions=%t diagnostics=%t reports=%t", reflect.DeepEqual(topologyPositionSignature(first), topologyPositionSignature(second)), reflect.DeepEqual(first.Components, second.Components), reflect.DeepEqual(first.Connections, second.Connections), reflect.DeepEqual(first.Wires, second.Wires), reflect.DeepEqual(first.Labels, second.Labels), reflect.DeepEqual(first.Junctions, second.Junctions), reflect.DeepEqual(first.Diagnostics, second.Diagnostics), reflect.DeepEqual(first.Report, second.Report))
			}
		})
	}
}

func TestTopologyInferenceIsIdentityNeutralForRenamedDifferentialPair(t *testing.T) {
	document := loadEducationalTopologyDocument(t, "03_differential_amplifier")
	renames := map[string]string{
		"input_v1": "n01", "input_v2": "n02", "collector_load_1": "n03", "collector_load_2": "n04",
		"transistor_1": "n05", "transistor_2": "n06", "tail_resistor": "n07", "supply": "n08",
		"supply_flag": "n09", "output": "n10", "ground": "n11", "ground_flag": "n12",
	}
	document = renameTopologyComponents(document, renames)
	normalized := NormalizeLayoutIntent(document)
	placements := placementsByTarget(normalized.Layout.Placements)
	if got := placements["n07"].CenterBetween; len(got) != 2 || got[0] != "n05" || got[1] != "n06" {
		t.Fatalf("renamed tail center relation = %#v", got)
	}
	if !stringSliceContains(placements["n06"].RightOf, "n05") || placements["n06"].Mirror != MirrorY {
		t.Fatalf("renamed pair relation = %#v", placements["n06"])
	}
}

func TestDifferentialPairPrefersResistorOverBypassCapacitorForTail(t *testing.T) {
	document := loadEducationalTopologyDocument(t, "03_differential_amplifier")
	document.Circuit.Components = append(document.Circuit.Components, Component{
		ID: "aaa_tail_bypass", Ref: "C1", Role: ComponentRoleCapacitor,
		Symbol: "Device:C", Pins: []Pin{{Number: "1"}, {Number: "2"}},
	})
	for netIndex := range document.Circuit.Nets {
		switch document.Circuit.Nets[netIndex].Name {
		case "TAIL":
			document.Circuit.Nets[netIndex].Connect = append(document.Circuit.Nets[netIndex].Connect, "aaa_tail_bypass.1")
		case "GND":
			document.Circuit.Nets[netIndex].Connect = append(document.Circuit.Nets[netIndex].Connect, "aaa_tail_bypass.2")
		}
	}

	placements := placementsByTarget(NormalizeLayoutIntent(document).Layout.Placements)
	if got := placements["tail_resistor"].CenterBetween; len(got) != 2 || got[0] != "transistor_1" || got[1] != "transistor_2" {
		t.Fatalf("tail resistor center relation = %#v, want transistor pair", got)
	}
	if got := placements["aaa_tail_bypass"].CenterBetween; len(got) != 0 {
		t.Fatalf("bypass capacitor incorrectly selected as functional tail: %#v", got)
	}
}

func TestTopologyInferenceHandlesNovelInductorShuntResistorComposition(t *testing.T) {
	document := *NewDocument()
	document.Circuit.Components = []Component{
		{ID: "p01", Ref: "TP1", Role: ComponentRoleTestpoint, Symbol: "Connector:TestPoint", Pins: []Pin{{Number: "1"}}},
		{ID: "p02", Ref: "L1", Role: ComponentRoleInductor, Symbol: "Device:L", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "p03", Ref: "R1", Role: ComponentRoleResistor, Symbol: "Device:R", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "p04", Ref: "TP2", Role: ComponentRoleTestpoint, Symbol: "Connector:TestPoint", Pins: []Pin{{Number: "1"}}},
		{ID: "p05", Ref: "#PWR01", Role: ComponentRoleGroundSymbol, Symbol: "power:GND", Pins: []Pin{{Number: "1"}}},
	}
	document.Circuit.Nets = []Net{
		{Name: "N_A", Role: NetRoleSignal, Connect: []EndpointRef{"p01.1", "p02.1"}},
		{Name: "N_B", Role: NetRoleSignal, Connect: []EndpointRef{"p02.2", "p03.1", "p04.1"}},
		{Name: "N_C", Role: NetRoleGround, Connect: []EndpointRef{"p03.2", "p05.1"}},
	}

	placements := placementsByTarget(NormalizeLayoutIntent(document).Layout.Placements)
	if placements["p02"].Orientation != OrientationRotated || placements["p03"].Orientation != OrientationNormal {
		t.Fatalf("novel shunt orientations = series:%q shunt:%q", placements["p02"].Orientation, placements["p03"].Orientation)
	}
	if len(placements["p03"].SameColumnAsPin) != 1 || placements["p03"].SameColumnAsPin[0] != "p02.2" {
		t.Fatalf("novel shunt relation = %#v", placements["p03"])
	}
}

func TestEducationalTopologyLayoutWithKiCadLibrariesHasNoBlockingDiagnostics(t *testing.T) {
	symbolsRoot := os.Getenv(libraryresolver.EnvSymbolsRoot)
	footprintsRoot := os.Getenv(libraryresolver.EnvFootprintsRoot)
	if symbolsRoot == "" || footprintsRoot == "" {
		t.Skip("set " + libraryresolver.EnvSymbolsRoot + " and " + libraryresolver.EnvFootprintsRoot + " to run the KiCad-backed topology layout test")
	}
	index, _ := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{
		SymbolsRoot:    symbolsRoot,
		FootprintsRoot: footprintsRoot,
	}, libraryresolver.LoadOptions{})

	for _, directory := range []string{
		"01_dc_voltage_source",
		"02_bjt_current_source",
		"03_differential_amplifier",
		"04_rc_low_pass_filter",
		"05_voltage_divider",
	} {
		t.Run(directory, func(t *testing.T) {
			document := loadEducationalTopologyDocument(t, directory)
			result := LayoutDocumentWithLibraryIndex(document, &index)
			tx, transactionIssues := ToProjectTransactionWithLibraryIndex(document, &index)
			blocking := false
			for _, issue := range transactionIssues {
				if issue.Severity == "error" || issue.Severity == "blocked" {
					blocking = true
					t.Logf("%s %s %s: %s", issue.Severity, issue.Code, issue.Path, issue.Message)
				}
			}
			annotationCountByNet := map[string]int{}
			for _, connect := range decodeOperations[transactions.ConnectOperation](t, tx, transactions.OpConnect) {
				if connect.BendLabelAt != nil {
					annotationCountByNet[connect.NetName]++
				}
			}
			for netName, count := range annotationCountByNet {
				if count > 1 {
					t.Fatalf("net %s emitted %d route annotations; want at most one", netName, count)
				}
			}
			if result.Report.ErrorCount == 0 && result.Report.Passed && !blocking {
				return
			}
			for _, component := range result.Components {
				t.Logf("%s at (%d,%d) ref-text=(%d,%d) value-text=(%d,%d)", component.Ref, component.PlacedAt.X, component.PlacedAt.Y, component.ReferenceText.At.X, component.ReferenceText.At.Y, component.ValueText.At.X, component.ValueText.At.Y)
			}
			for _, wire := range result.Wires {
				if wire.NetName == "V1" {
					t.Logf("V1 wire (%d,%d)-(%d,%d)", wire.From.X, wire.From.Y, wire.To.X, wire.To.Y)
				}
			}
			for _, diagnostic := range result.Diagnostics {
				t.Logf("%s %s ref=%s net=%s: %s", diagnostic.Severity, diagnostic.Code, diagnostic.Ref, diagnostic.NetName, diagnostic.Message)
			}
			t.Fatalf("KiCad-backed layout report = %#v", result.Report)
		})
	}
}

func topologyPositionSignature(result schematiclayout.Result) map[string]kicadfiles.Point {
	positions := make(map[string]kicadfiles.Point, len(result.Components))
	for _, component := range result.Components {
		positions[component.Ref] = component.PlacedAt
	}
	return positions
}

func TestTopologyTerminalKindUsesDeclaredBJTTerminalOrder(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		symbol string
		want   []string
	}{
		{symbol: "Transistor_BJT:Q_NPN_CBE", want: []string{"collector", "base", "emitter"}},
		{symbol: "Transistor_BJT:Q_NPN_EBC", want: []string{"emitter", "base", "collector"}},
		{symbol: "Transistor_BJT:Q_PNP_BEC", want: []string{"base", "emitter", "collector"}},
	} {
		component := Component{Role: ComponentRoleTransistor, Symbol: testCase.symbol}
		for index, want := range testCase.want {
			pin := Pin{Number: fmt.Sprintf("%d", index+1)}
			if got := topologyTerminalKind(component, pin); got != want {
				t.Fatalf("topologyTerminalKind(%s, %s) = %q, want %q", testCase.symbol, pin.Number, got, want)
			}
		}
	}

	named := Component{Role: ComponentRoleTransistor, Symbol: "Transistor_BJT:Q_NPN"}
	if got := topologyTerminalKind(named, Pin{Number: "1", Name: "Emitter"}); got != "emitter" {
		t.Fatalf("named emitter terminal = %q, want emitter", got)
	}
}

func TestTopologyPowerNetNameDoesNotTreatDifferentialPlusAsPower(t *testing.T) {
	t.Parallel()

	index := &topologyLayoutIndex{nets: []Net{
		{Name: "D+", Role: NetRoleSignal},
		{Name: "USB+", Role: NetRoleSignal},
		{Name: "+12V", Role: NetRoleSignal},
		{Name: "VCC_3V3", Role: NetRoleSignal},
	}}
	want := []bool{false, false, true, true}
	for netIndex, expected := range want {
		if got := index.isPowerNet(netIndex); got != expected {
			t.Fatalf("isPowerNet(%q) = %t, want %t", index.nets[netIndex].Name, got, expected)
		}
	}
}

func loadEducationalTopologyDocument(t *testing.T, directory string) Document {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "educational", directory, "source.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	document, issues := DecodeStrict(file)
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("close %s: %v", path, closeErr)
	}
	if len(issues) != 0 {
		t.Fatalf("decode %s: %#v", path, issues)
	}
	return document
}

func reverseComponents(source []Component) []Component {
	result := append([]Component(nil), source...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func reverseNets(source []Net) []Net {
	result := append([]Net(nil), source...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func reverseEndpointRefs(source []EndpointRef) []EndpointRef {
	result := append([]EndpointRef(nil), source...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func renameTopologyComponents(document Document, renames map[string]string) Document {
	for index := range document.Circuit.Components {
		if renamed := renames[document.Circuit.Components[index].ID]; renamed != "" {
			document.Circuit.Components[index].ID = renamed
		}
	}
	for netIndex := range document.Circuit.Nets {
		for endpointIndex, endpoint := range document.Circuit.Nets[netIndex].Connect {
			componentID, pin, ok := endpoint.Split()
			if ok && renames[componentID] != "" {
				document.Circuit.Nets[netIndex].Connect[endpointIndex] = EndpointRef(renames[componentID] + "." + pin)
			}
		}
	}
	for groupIndex := range document.Layout.Groups {
		for memberIndex, member := range document.Layout.Groups[groupIndex].Members {
			if renamed := renames[member]; renamed != "" {
				document.Layout.Groups[groupIndex].Members[memberIndex] = renamed
			}
		}
	}
	return document
}
