package componentonboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/simmodel"
	"kicadai/internal/writercorrectness"
)

func TestHeldOutCorpusOptionalKiCadPromotion(t *testing.T) {
	kicadCLI := strings.TrimSpace(os.Getenv("KICADAI_KICAD_CLI"))
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	footprintsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvFootprintsRoot))
	if kicadCLI == "" || symbolsRoot == "" || footprintsRoot == "" {
		t.Skip("installed-KiCad component onboarding promotion is optional")
	}
	if info, err := os.Stat(kicadCLI); err != nil || info.IsDir() {
		t.Skipf("KiCad CLI is unavailable: %s", kicadCLI)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	index, _ := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{
		SymbolsRoot: symbolsRoot, FootprintsRoot: footprintsRoot,
		TemplatesRoot: strings.TrimSpace(os.Getenv(libraryresolver.EnvTemplatesRoot)),
	}, libraryresolver.LoadOptions{})
	baseModels, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("load model provenance: %#v", diagnostics)
	}
	manifest := loadHeldOutManifest(t)
	for _, testCase := range manifest.Cases {
		testCase := testCase
		t.Run(testCase.ID, func(t *testing.T) {
			base, requirement, documents, extraction, _ := heldOutFixture(t, testCase)
			candidate, err := Onboard(ctx, requirement, documents, fixedExtractor{extraction}, base, index)
			if err != nil {
				t.Fatal(err)
			}
			environment, err := BuildEvaluationEnvironment(candidate, documents, base, baseModels, index)
			if err != nil {
				t.Fatal(err)
			}
			if environment.Status != StatusQuarantined {
				t.Fatalf("evaluation environment escaped quarantine: %q", environment.Status)
			}
			record := selectedCandidateRecord(t, candidate)
			request := physicalCandidateRequest(t, environment.Catalog, index, record, testCase.ID)
			runBase := t.TempDir()
			if artifactRoot := strings.TrimSpace(os.Getenv("KICADAI_COMPONENT_ONBOARDING_ARTIFACT_DIR")); artifactRoot != "" {
				runBase = filepath.Join(artifactRoot, testCase.ID)
				if err := os.RemoveAll(runBase); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(runBase, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			runRoots := [2]string{filepath.Join(runBase, "run-1"), filepath.Join(runBase, "run-2")}
			var runHashes [2]string
			var simulationHashes [2]string
			for runIndex, output := range runRoots {
				result := designworkflow.Create(ctx, request, designworkflow.CreateOptions{
					OutputDir: output, Overwrite: true, Seed: "component-onboarding-" + testCase.ID,
					Components:   designworkflow.ComponentSelectionOptions{Catalog: environment.Catalog},
					LibraryIndex: &index,
					Routing: designworkflow.RoutingOptions{
						Mode: routing.ModeTwoLayer, GridMM: 0.25, TraceWidthMM: 0.2, ClearanceMM: 0.1,
					},
					Validation: designworkflow.ValidationOptions{
						StrictZones: true, StrictUnrouted: true, RequireDRC: true,
						KiCadCLI: kicadCLI, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".evidence", "validation"),
					},
					KiCadChecks: designworkflow.KiCadCheckOptions{
						KiCadCLI: kicadCLI, Timeout: 2 * time.Minute,
						RequireERC: true, RequireDRC: true, KeepArtifacts: true,
						ArtifactDir: filepath.Join(output, ".evidence", "kicad"),
					},
					Writer: writercorrectness.Options{
						KiCadCLI: kicadCLI, RequireKiCadRoundTrip: true, StrictDiffs: true,
						KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".evidence", "writer"),
						LibraryIndex: index, HasLibraryIndex: true,
					},
				})
				if blocking := designworkflow.WorkflowIssues(result); reports.HasBlockingIssue(blocking) {
					t.Fatalf("physical promotion run %d blocked: %#v", runIndex+1, blocking)
				}
				for _, stage := range []designworkflow.StageName{
					designworkflow.StageRouting,
					designworkflow.StageWriterCorrect,
					designworkflow.StageValidation,
					designworkflow.StageKiCadChecks,
				} {
					if status := workflowStageStatus(result, stage); status != designworkflow.StageStatusOK {
						t.Fatalf("physical promotion run %d stage %s = %s: %#v", runIndex+1, stage, status, result.Stages)
					}
				}
				runHashes[runIndex] = hashKiCadProject(t, output)
				report, reportHash := evaluateCandidateSimulation(t, record, testCase.Analysis)
				simulationPath := filepath.Join(output, ".evidence", "simulation", "report.json")
				if err := os.MkdirAll(filepath.Dir(simulationPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(simulationPath, report, 0o600); err != nil {
					t.Fatal(err)
				}
				simulationHashes[runIndex] = reportHash
			}
			if runHashes[0] != runHashes[1] {
				t.Fatalf("physical project replay differs: %s != %s", runHashes[0], runHashes[1])
			}
			if simulationHashes[0] != simulationHashes[1] {
				t.Fatalf("simulation replay differs: %s != %s", simulationHashes[0], simulationHashes[1])
			}
			gates := make([]GateEvidence, 0, len(RequiredPromotionGates)*2)
			for _, gate := range RequiredPromotionGates {
				for run := 1; run <= 2; run++ {
					evidencePath := filepath.Join(runRoots[run-1], ".evidence")
					evidenceHash := runHashes[run-1]
					if gate == "simulation" {
						evidencePath = filepath.Join(evidencePath, "simulation", "report.json")
						evidenceHash = simulationHashes[run-1]
					}
					gates = append(gates, GateEvidence{
						Gate: gate, Run: run, Passed: true,
						EvidencePath: evidencePath, EvidenceSHA256: evidenceHash,
					})
				}
			}
			approval := Approval{
				CandidateHash: candidate.Hash, Decision: "approve", Reviewer: "physical-promotion-reviewer",
				ReviewRef:    "review://component-onboarding/" + testCase.ID,
				ReviewSHA256: hashText("physical-review-" + testCase.ID),
			}
			if _, overlay, err := Promote(candidate, documents, gates, approval, base, index); err != nil {
				t.Fatal(err)
			} else if overlay.Status != StatusSupported {
				t.Fatalf("physical promotion overlay status = %q", overlay.Status)
			}
		})
	}
}

func physicalCandidateRequest(
	t *testing.T,
	catalog *components.Catalog,
	index libraryresolver.LibraryIndex,
	record components.ComponentRecord,
	name string,
) designworkflow.Request {
	t.Helper()
	trueValue := true
	variant := record.Packages[0]
	symbol := record.Symbols[0]
	type pinBinding struct {
		function   string
		pin        string
		pad        string
		electrical string
	}
	pinsByFunction := map[string][]components.FunctionPin{}
	for _, pin := range symbol.FunctionPins {
		pinsByFunction[pin.Function] = append(pinsByFunction[pin.Function], pin)
	}
	var bindings []pinBinding
	for _, pad := range variant.PadFunctions {
		pins := pinsByFunction[pad.Function]
		if len(pins) == 0 {
			t.Fatalf("%s package function %s has no symbol pin", record.ID, pad.Function)
		}
		pin := pins[0]
		pinsByFunction[pad.Function] = pins[1:]
		electrical := pin.Electrical
		if electrical == "" {
			if librarySymbol, found := index.Symbols[symbol.SymbolID]; found {
				if pinIndex := slices.IndexFunc(librarySymbol.Pins, func(candidate libraryresolver.SymbolPin) bool {
					return candidate.Number == pin.SymbolPin
				}); pinIndex >= 0 {
					electrical = librarySymbol.Pins[pinIndex].ElectricalType
				}
			}
		}
		bindings = append(bindings, pinBinding{
			function: pad.Function, pin: pin.SymbolPin, pad: pad.Pad, electrical: electrical,
		})
	}
	slices.SortStableFunc(bindings, func(left, right pinBinding) int {
		return strings.Compare(left.pad, right.pad)
	})
	componentsList := []circuitgraph.Component{{
		ID: "device", Role: physicalComponentRole(record.Family), ComponentID: record.ID,
		VariantID: variant.ID, Population: circuitgraph.PopulationPopulate,
	}}
	var nets []circuitgraph.Net
	var flags []circuitgraph.PowerFlag
	var noConnects []circuitgraph.Endpoint
	for index, binding := range bindings {
		if binding.electrical == "no_connect" {
			noConnects = append(noConnects, circuitgraph.Endpoint{
				Component: "device", SelectorKind: circuitgraph.SelectorSymbolPin, Selector: binding.pin,
			})
			continue
		}
		connectorID := fmt.Sprintf("port_%02d", index+1)
		componentsList = append(componentsList, circuitgraph.Component{
			ID: connectorID, Role: circuitgraph.RoleConnector,
			ComponentID: "connector.pinheader.1x01.2_54mm", VariantID: "vertical",
			Population: circuitgraph.PopulationPopulate,
		})
		netName := fmt.Sprintf("PIN_%02d_%s", index+1, strings.ToUpper(binding.function))
		required := true
		role := circuitgraph.NetRoleSignal
		if strings.Contains(strings.ToUpper(binding.function), "GND") ||
			strings.Contains(strings.ToUpper(binding.function), "VSS") ||
			strings.Contains(strings.ToUpper(binding.function), "MINUS") {
			role = circuitgraph.NetRoleGround
		} else if strings.HasPrefix(strings.ToUpper(binding.function), "V") ||
			strings.Contains(strings.ToUpper(binding.function), "POWER") {
			role = circuitgraph.NetRolePower
		}
		nets = append(nets, circuitgraph.Net{
			Name: netName, Role: role, Required: &required,
			WidthMM: 0.2, ClearanceMM: 0.1, AllowedLayers: []string{"B.Cu", "F.Cu"},
			Endpoints: []circuitgraph.Endpoint{
				{Component: "device", SelectorKind: circuitgraph.SelectorSymbolPin, Selector: binding.pin},
				{Component: connectorID, SelectorKind: circuitgraph.SelectorFunction, Selector: "PIN_1"},
			},
		})
		if binding.electrical == libraryresolver.SymbolElectricalPowerIn {
			flags = append(flags, circuitgraph.PowerFlag{Net: netName})
		}
	}
	document := circuitgraph.Document{
		Schema: circuitgraph.SchemaID, Version: circuitgraph.Version,
		Project: circuitgraph.Project{
			Name: "onboard_" + name, Title: "Onboarding promotion " + name,
			Description: "Identity-neutral physical promotion of a quarantined component candidate.",
			Acceptance:  circuitgraph.AcceptanceERCDRC,
			Board:       circuitgraph.Board{WidthMM: 80, HeightMM: 60, Layers: 2, EdgeClearanceMM: 1},
		},
		Components: componentsList, Nets: nets, NoConnects: noConnects,
		PowerFlags: flags, Buses: []circuitgraph.Bus{},
		Schematic: circuitgraph.SchematicIntent{
			Flow: circuitgraph.FlowLeftToRight, Origin: circuitgraph.OriginCentered,
			Lanes:     circuitgraph.SchematicLanes{Power: circuitgraph.LaneTop, Signals: circuitgraph.LaneMiddle, Ground: circuitgraph.LaneBottom},
			Hierarchy: circuitgraph.HierarchyPolicy{Mode: "flat"},
			Rules: circuitgraph.SchematicRules{
				PositivePowerTop: &trueValue, GroundBottom: &trueValue, CenterOnPage: &trueValue,
				PreferLabelsForLongNets: &trueValue, AvoidWireCrossings: &trueValue,
				MinGroupSpacingMM: 10, MinComponentSpacingMM: 5,
			},
		},
		PCB: circuitgraph.PCBIntent{},
		Policy: circuitgraph.Policy{
			AllowReferenceAssignment: &trueValue, AllowValueNormalization: &trueValue,
			AllowLayoutInference: &trueValue, AllowSpacingAdjustment: &trueValue,
			AllowLabelInsertion: &trueValue, AllowPlacementAdjustment: &trueValue, AllowRouteRetry: &trueValue,
		},
	}
	symbols, footprints := circuitgraph.LibraryEvidenceFromIndex(index)
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{
		Catalog: catalog, CatalogID: "component-onboarding-evaluation",
		LibrarySymbols: symbols, LibraryFootprints: footprints, RequireLibraryEvidence: true,
	})
	resolved, resolveIssues := resolver.Resolve(context.Background(), document)
	if reports.HasBlockingIssue(resolveIssues) {
		t.Fatalf("resolve physical candidate graph: %#v", resolveIssues)
	}
	request, requestIssues := circuitgraph.ToDesignRequest(resolved)
	if reports.HasBlockingIssue(requestIssues) {
		t.Fatalf("lower physical candidate graph: %#v", requestIssues)
	}
	return request
}

func physicalComponentRole(family string) circuitgraph.ComponentRole {
	switch family {
	case "bjt":
		return circuitgraph.RoleBJT
	case "regulator":
		return circuitgraph.RoleRegulator
	case "sensor":
		return circuitgraph.RoleSensor
	default:
		return circuitgraph.RoleIC
	}
}

func selectedCandidateRecord(t *testing.T, candidate Candidate) components.ComponentRecord {
	t.Helper()
	for _, proposal := range candidate.Proposals {
		if proposal.Record.ID == candidate.SelectedID {
			return proposal.Record
		}
	}
	t.Fatalf("selected candidate %s is absent", candidate.SelectedID)
	return components.ComponentRecord{}
}

func workflowStageStatus(result designworkflow.WorkflowResult, name designworkflow.StageName) designworkflow.StageStatus {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return stage.Status
		}
	}
	return ""
}

func evaluateCandidateSimulation(t *testing.T, record components.ComponentRecord, analysisKind string) ([]byte, string) {
	t.Helper()
	if len(record.Packages) == 0 || len(record.SimulationModels) == 0 {
		t.Fatalf("%s lacks simulation or package evidence", record.ID)
	}
	if record.SimulationModels[0].ModelID == simmodel.ModelLinearRegulatorIdealV1 {
		return evaluateLegacyRegulatorSimulation(t, record)
	}
	nodesByName := map[string]simmodel.NodeEvidence{}
	addNode := func(name, role string) {
		if _, found := nodesByName[name]; !found {
			nodesByName[name] = simmodel.NodeEvidence{Name: name, Role: role}
		}
	}
	var candidateConnections []simmodel.ConnectionEvidence
	seenFunctions := map[string]bool{}
	functionNets := map[string]string{}
	for _, pad := range record.Packages[0].PadFunctions {
		function := strings.ToUpper(strings.TrimSpace(pad.Function))
		if function == "" || function == "NC" || seenFunctions[function] {
			continue
		}
		seenFunctions[function] = true
		net, role := simulationFunctionNet(record.Family, function)
		functionNets[function] = net
		addNode(net, role)
		candidateConnections = append(candidateConnections, simmodel.ConnectionEvidence{Function: function, Net: net})
	}
	if len(candidateConnections) == 0 {
		t.Fatalf("%s has no simulatable pin functions", record.ID)
	}
	slices.SortStableFunc(candidateConnections, func(left, right simmodel.ConnectionEvidence) int {
		return strings.Compare(left.Function, right.Function)
	})
	candidate := simmodel.ComponentEvidence{
		InstanceID: "candidate", PhysicalComponent: record.ID, CatalogID: record.ID,
		Family: record.Family, ModelClaims: record.SimulationModels, Connections: candidateConnections,
	}
	componentsEvidence := []simmodel.ComponentEvidence{candidate}
	var excitations []simmodel.SourceExcitation
	assertionNode, assertionReference, assertionValue := "", "", 0.0
	sourceIndex, loadIndex := 0, 0
	sourcedPairs := map[string]bool{}
	addSource := func(node, reference string, value float64) {
		key := node + "\x00" + reference
		if sourcedPairs[key] {
			return
		}
		sourcedPairs[key] = true
		sourceIndex++
		id := fmt.Sprintf("source_%02d", sourceIndex)
		addNode(node, simulationNodeRole(node))
		addNode(reference, "ground")
		componentsEvidence = append(componentsEvidence, simmodel.ComponentEvidence{
			InstanceID: id, CatalogID: "evaluation.voltage_source", Family: "voltage_source",
			ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveVoltageSourceV1}},
			Connections: []simmodel.ConnectionEvidence{
				{Function: "POSITIVE", Net: node}, {Function: "NEGATIVE", Net: reference},
			},
		})
		excitations = append(excitations, simmodel.SourceExcitation{Component: id, DCValue: value})
		if value > assertionValue {
			assertionNode, assertionReference, assertionValue = node, reference, value
		}
	}
	addLoad := func(node, reference string, resistance float64) {
		if node == reference {
			return
		}
		loadIndex++
		id := fmt.Sprintf("load_%02d", loadIndex)
		addNode(node, simulationNodeRole(node))
		addNode(reference, "ground")
		componentsEvidence = append(componentsEvidence, simmodel.ComponentEvidence{
			InstanceID: id, CatalogID: "evaluation.resistor", Family: "resistor",
			ValueSI: resistance, HasValueSI: true,
			ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}},
			Connections: []simmodel.ConnectionEvidence{
				{Function: "A", Net: node}, {Function: "B", Net: reference},
			},
		})
	}
	functions := make([]string, 0, len(functionNets))
	for function := range functionNets {
		functions = append(functions, function)
	}
	slices.Sort(functions)
	for _, function := range functions {
		node := functionNets[function]
		if simulationGroundFunction(record.Family, function) {
			continue
		}
		reference := simulationReferenceNet(record.Family, function)
		switch {
		case simulationSupplyFunction(function):
			addSource(node, reference, simulationSupplyVoltage(record, function))
		case simulationInputFunction(record.Family, function):
			addSource(node, reference, simulationInputVoltage(record, function))
			addLoad(node, reference, 1_000_000)
		default:
			addLoad(node, reference, 100_000)
		}
	}
	if record.Family == "bjt" {
		addNode("HARNESS_VCC", "power")
		addSource("HARNESS_VCC", "GND", 5)
		if collector := functionNets["COLLECTOR"]; collector != "" {
			addLoad(collector, "HARNESS_VCC", 1_000)
		}
	}
	if assertionNode == "" {
		addSource("HARNESS_VCC", "GND", 3.3)
	}
	modelID, applicable, reason := simmodel.ApplicableGraphModelForAnalysis(componentsEvidence, analysisKind)
	if analysisKind == simmodel.AnalysisDCOperatingPoint {
		modelID, applicable, reason = simmodel.ApplicableGraphModel(componentsEvidence)
	}
	if !applicable {
		t.Fatalf("%s simulation model is not graph-applicable for %s: %s", record.ID, analysisKind, reason)
	}
	analysis := simmodel.Analysis{ID: "promotion", Kind: analysisKind, Excitations: excitations}
	assertion := simmodel.Assertion{
		AnalysisID: analysis.ID, Node: assertionNode, ReferenceNode: assertionReference,
		Quantity: simmodel.QuantityVoltageV, Min: assertionValue - 1e-6, Max: assertionValue + 1e-6,
	}
	if analysisKind == simmodel.AnalysisTransient {
		analysis.DurationS = 1e-6
		analysis.TimeStepS = 100e-9
		assertion.TimeS = analysis.DurationS
	}
	nodes := make([]simmodel.NodeEvidence, 0, len(nodesByName))
	for _, node := range nodesByName {
		nodes = append(nodes, node)
	}
	slices.SortStableFunc(nodes, func(left, right simmodel.NodeEvidence) int {
		return strings.Compare(left.Name, right.Name)
	})
	plan, diagnostics := simmodel.ResolveWithTopology(simmodel.Intent{
		ModelID: modelID, Analyses: []simmodel.Analysis{analysis}, Assertions: []simmodel.Assertion{assertion},
	}, "component-onboarding-evaluation", hashText(record.ID), componentsEvidence, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("%s simulation resolution: %#v", record.ID, diagnostics)
	}
	report, diagnostics := simmodel.Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("%s simulation report status=%q diagnostics=%#v", record.ID, report.Status, diagnostics)
	}
	return marshalSimulationReport(t, report)
}

func evaluateLegacyRegulatorSimulation(t *testing.T, record components.ComponentRecord) ([]byte, string) {
	t.Helper()
	inputVoltage := simulationSupplyVoltage(record, "VIN")
	loadCurrentMA := 10.0
	outputVoltage := 0.0
	for _, parameter := range record.SimulationModels[0].Parameters {
		switch parameter.Name {
		case "max_load_current_ma":
			loadCurrentMA = math.Min(10, parameter.Value/10)
		case "output_voltage_v":
			outputVoltage = parameter.Value
		}
	}
	component := simmodel.ComponentEvidence{
		InstanceID: "candidate", PhysicalComponent: record.ID, CatalogID: record.ID,
		Family: record.Family, ModelClaims: record.SimulationModels,
	}
	plan, diagnostics := simmodel.Resolve(simmodel.Intent{
		ModelID:  simmodel.ModelLinearRegulatorIdealV1,
		Bindings: []simmodel.Binding{{Role: "regulator", Component: "candidate"}},
		Inputs: []simmodel.NamedValue{
			{Name: "input_voltage_v", Value: inputVoltage},
			{Name: "load_current_ma", Value: loadCurrentMA},
		},
		Assertions: []simmodel.Assertion{{
			Metric: "output_voltage_v", Min: outputVoltage - 1e-6, Max: outputVoltage + 1e-6,
		}},
	}, "component-onboarding-evaluation", hashText(record.ID), []simmodel.ComponentEvidence{component})
	if len(diagnostics) != 0 {
		t.Fatalf("%s legacy simulation resolution: %#v", record.ID, diagnostics)
	}
	report, diagnostics := simmodel.Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("%s legacy simulation report status=%q diagnostics=%#v", record.ID, report.Status, diagnostics)
	}
	return marshalSimulationReport(t, report)
}

func marshalSimulationReport(t *testing.T, report simmodel.Report) ([]byte, string) {
	t.Helper()
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	return body, hashBytes(body)
}

func simulationFunctionNet(family, function string) (string, string) {
	switch {
	case family == "sensor" && strings.HasPrefix(function, "VDD"):
		return "VDD", "power"
	case family == "isolated_converter" && function == "VIN_MINUS":
		return "GND_IN", "ground"
	case family == "isolated_converter" && function == "VOUT_MINUS":
		return "GND_OUT", "ground"
	case simulationGroundFunction(family, function):
		return "GND", "ground"
	default:
		return function, simulationNodeRole(function)
	}
}

func simulationNodeRole(function string) string {
	if simulationSupplyFunction(function) || function == "HARNESS_VCC" {
		return "power"
	}
	return "signal"
}

func simulationGroundFunction(family, function string) bool {
	switch function {
	case "GND", "AGND", "DGND", "PGND", "VSS", "V_MINUS":
		return true
	case "VIN_MINUS", "VOUT_MINUS":
		return family == "isolated_converter"
	case "EMITTER":
		return family == "bjt"
	default:
		return false
	}
}

func simulationReferenceNet(family, function string) string {
	if family == "isolated_converter" {
		if strings.HasPrefix(function, "VIN_") {
			return "GND_IN"
		}
		if strings.HasPrefix(function, "VOUT_") {
			return "GND_OUT"
		}
	}
	return "GND"
}

func simulationSupplyFunction(function string) bool {
	return function == "VIN" || function == "VIN_PLUS" || function == "V_PLUS" ||
		strings.HasPrefix(function, "VCC") || strings.HasPrefix(function, "VDD")
}

func simulationInputFunction(family, function string) bool {
	if family == "bjt" {
		return function == "BASE"
	}
	if strings.HasPrefix(function, "IN") || function == "OE" || function == "EN" ||
		function == "ENABLE" || function == "CS" || function == "CSB" ||
		function == "SCL" || function == "SDA" || function == "SDO" ||
		function == "CLK" || function == "RESET" {
		return true
	}
	return strings.HasPrefix(function, "A") && len(function) > 1
}

func simulationSupplyVoltage(record components.ComponentRecord, function string) float64 {
	parameters := map[string]float64{}
	for _, model := range record.SimulationModels {
		for _, parameter := range model.Parameters {
			parameters[parameter.Name] = parameter.Value
		}
	}
	midpoint := func(minimumName, maximumName string) (float64, bool) {
		minimum, hasMinimum := parameters[minimumName]
		maximum, hasMaximum := parameters[maximumName]
		return (minimum + maximum) / 2, hasMinimum && hasMaximum && maximum >= minimum
	}
	var value float64
	var found bool
	switch function {
	case "VCCA":
		value, found = midpoint("vcca_min_v", "vcca_max_v")
	case "VCCB":
		value, found = midpoint("vccb_min_v", "vccb_max_v")
	case "VIN", "VIN_PLUS":
		value, found = midpoint("input_min_v", "input_max_v")
		if !found {
			if output, outputFound := parameters["output_voltage_v"]; outputFound {
				value = output + parameters["min_headroom_v"] + .5
				found = true
			}
		}
	default:
		value, found = midpoint("supply_min_v", "supply_max_v")
		if !found {
			value, found = midpoint("minimum_supply_voltage_v", "maximum_supply_voltage_v")
		}
	}
	if !found || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 3.3
	}
	return value
}

func simulationInputVoltage(record components.ComponentRecord, function string) float64 {
	switch {
	case record.Family == "bjt" && function == "BASE":
		return .65
	case function == "OE" || function == "EN" || function == "ENABLE":
		return 3.3
	case function == "IN_PLUS" || function == "IN_MINUS":
		return .5
	default:
		return 0
	}
}

func hashKiCadProject(t *testing.T, root string) string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".evidence" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".kicad_pcb", ".kicad_pro", ".kicad_sch":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	hash := sha256.New()
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(body)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
