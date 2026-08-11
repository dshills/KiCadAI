package opentopologysynthesis

import (
	"bytes"
	"context"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

func TestNonlinearSwitchingRelationshipOperatorsProduceCompletePrimitiveGraphs(t *testing.T) {
	tests := []struct {
		file  string
		kinds map[string]int
	}{
		{"bounded_bipolar_transfer.json", map[string]int{"signal_diode": 2, "resistor": 2}},
		{"autonomous_square_wave_source.json", map[string]int{"opamp": 1, "capacitor": 1, "resistor": 5}},
		{"efficient_step_down_power.json", map[string]int{"synchronous_buck_regulator": 1, "inductor": 1, "capacitor": 2, "resistor": 2}},
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(nonlinearSwitchingCorpusRoot(), test.file))))
			if len(issues) != 0 {
				t.Fatalf("requirement issues = %#v", issues)
			}
			search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
			found := false
			for _, candidate := range search.Candidates {
				counts := map[string]int{}
				for _, instance := range candidate.Graph.Instances {
					counts[instance.Kind]++
				}
				matches := true
				for kind, minimum := range test.kinds {
					matches = matches && counts[kind] >= minimum
				}
				found = found || matches
			}
			if !found {
				t.Fatalf("relationship graph not found: status=%s rejections=%#v candidates=%d", search.Status, search.Rejections, len(search.Candidates))
			}
		})
	}
}

func TestStepDownRelationshipTiesEnableHighWithoutExternalControl(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(nonlinearSwitchingCorpusRoot(), "efficient_step_down_power.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	requirement.Requirements.Ports = slices.DeleteFunc(
		requirement.Requirements.Ports,
		func(port Port) bool { return port.ID == "enable" },
	)
	for index := range requirement.Requirements.OperatingCases {
		operatingCase := &requirement.Requirements.OperatingCases[index]
		operatingCase.Conditions = slices.DeleteFunc(
			operatingCase.Conditions,
			func(condition OperatingCondition) bool { return condition.Target == "enable" },
		)
		operatingCase.Events = slices.DeleteFunc(
			operatingCase.Events,
			func(event OperatingEvent) bool { return event.Target == "enable" },
		)
	}
	requirement = Normalize(requirement)
	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("uncontrolled step-down requirement issues = %#v", issues)
	}

	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	var selected CandidateGraph
	for _, candidate := range search.Candidates {
		for _, instance := range candidate.Graph.Instances {
			if instance.Kind != "synchronous_buck_regulator" {
				continue
			}
			terminals := topologyTerminalNodes(instance)
			if terminals["EN"] != terminals["PVIN"] || terminals["EN"] == "" {
				t.Fatalf("uncontrolled step-down enable = %q, input rail = %q", terminals["EN"], terminals["PVIN"])
			}
			selected = CloneGraph(candidate.Graph)
			break
		}
		if selected.Schema != "" {
			break
		}
	}
	if selected.Schema == "" {
		t.Fatalf("uncontrolled step-down graph not found: status=%s rejections=%#v candidates=%d", search.Status, search.Rejections, len(search.Candidates))
	}

	var seriesNode string
	selected, seriesNode = addInternalNode(selected, "power")
	rewired := false
	for instanceIndex := range selected.Instances {
		instance := &selected.Instances[instanceIndex]
		if instance.Kind != "inductor" {
			continue
		}
		for terminalIndex := range instance.Terminals {
			if instance.Terminals[terminalIndex].Node == "port_regulated_output" {
				instance.Terminals[terminalIndex].Node = seriesNode
				rewired = true
			}
		}
	}
	if !rewired {
		t.Fatal("selected step-down graph lacks a direct output-inductor connection")
	}
	seriesResistor := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", .1)
	if seriesResistor.Key == "" {
		t.Fatal("test inventory lacks a series resistor")
	}
	selected = AddPrimitive(
		selected,
		seriesResistor,
		seedPrimitiveValue(seriesResistor),
		topologyTwoTerminalPlacement(seriesNode, "port_regulated_output"),
	)
	index := newTopologyPowerEvidenceIndex(selected)
	if !topologyGraphHasRegulatedPowerTransferToOutput(selected, index, "regulated_output") {
		t.Fatal("step-down recognizer rejected a valid inductor path with a series sense element")
	}
}

func TestRegulatedPowerRecognizerAllowsSeriesOutputElement(t *testing.T) {
	graph := CandidateGraph{
		Schema: CandidateGraphSchema, Version: CandidateGraphVersion,
		Nodes: []GraphNode{
			{ID: "port_input", Scope: "external", SemanticKind: "port", SemanticID: "input", Role: "supply"},
			{ID: "port_ground", Scope: "external", SemanticKind: "port", SemanticID: "ground", Role: "reference"},
			{ID: "port_output", Scope: "external", SemanticKind: "port", SemanticID: "output", Role: "output"},
			{ID: "internal_filtered_input", Scope: "internal", Role: "power"},
			{ID: "internal_sensed_ground", Scope: "internal", Role: "reference"},
			{ID: "internal_sense", Scope: "internal", Role: "power"},
		},
		Instances: []GraphInstance{
			{ID: "regulator", Kind: "fixed_voltage_regulator", Terminals: []TerminalConnection{
				{Terminal: "VIN", Node: "internal_filtered_input"},
				{Terminal: "VOUT", Node: "internal_sense"},
				{Terminal: "GND", Node: "internal_sensed_ground"},
			}},
			{ID: "input_filter", Kind: "inductor", Terminals: topologyTwoTerminalPlacement("port_input", "internal_filtered_input")},
			{ID: "ground_sense", Kind: "resistor", Terminals: topologyTwoTerminalPlacement("port_ground", "internal_sensed_ground")},
			{ID: "sense", Kind: "resistor", Terminals: topologyTwoTerminalPlacement("internal_sense", "port_output")},
		},
	}
	index := newTopologyPowerEvidenceIndex(graph)
	if !topologyGraphHasRegulatedPowerTransferToOutput(graph, index, "output") {
		t.Fatal("regulated-power recognizer rejected valid series rail and output elements")
	}
}

func TestAutonomousPeriodicRelationshipUsesExternalTimingCapacitance(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(nonlinearSwitchingCorpusRoot(), "autonomous_square_wave_source.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	requirement.Requirements.Ports = append(requirement.Requirements.Ports, Port{
		ID:        "timing_terminal",
		Kind:      "analog_voltage",
		Direction: "sink",
		Domain:    "ground",
		Electrical: Electrical{
			MinVoltageV:          graphFloat(0),
			MaxVoltageV:          graphFloat(5.25),
			InputImpedanceMinOhm: graphFloat(1_000_000),
		},
	})
	for index := range requirement.Requirements.BehavioralRequirements {
		assertion := &requirement.Requirements.BehavioralRequirements[index]
		if assertion.Metric == "oscillation_frequency" {
			assertion.Excitation = &Observation{Kind: "port", ID: "timing_terminal"}
		}
	}
	for index := range requirement.Requirements.OperatingCases {
		requirement.Requirements.OperatingCases[index].Conditions = append(
			requirement.Requirements.OperatingCases[index].Conditions,
			OperatingCondition{
				Axis: "load_capacitance", Target: "timing_terminal",
				Min: 6e-9, Max: 8e-9, Unit: "F",
			},
		)
	}
	requirement = Normalize(requirement)
	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("external timing requirement issues = %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	var selected *TopologyCandidate
	for index := range search.Candidates {
		candidate := &search.Candidates[index]
		for _, graphInstance := range candidate.Graph.Instances {
			terminals := topologyTerminalNodes(graphInstance)
			if (graphInstance.Kind == "comparator" || graphInstance.Kind == "opamp") &&
				terminals["IN_MINUS"] == "port_timing_terminal" {
				selected = candidate
				break
			}
		}
		if selected != nil {
			break
		}
	}
	if selected == nil {
		t.Fatalf("external timing relationship graph not found: status=%s rejections=%#v candidates=%d", search.Status, search.Rejections, len(search.Candidates))
	}
	for _, graphInstance := range selected.Graph.Instances {
		if graphInstance.Kind == "capacitor" {
			t.Fatalf("external timing graph added an internal timing capacitor: %#v", selected.Graph)
		}
	}
	plan := BuildValueSearchPlan(requirement, selected.Graph, inventory, DefaultPolicy())
	if plan.Status != ValuePlanReady {
		t.Fatalf("external timing value plan = %s issues=%#v rejections=%#v", plan.Status, plan.Issues, plan.Rejections)
	}
	foundTimingResistance := false
	for _, domain := range plan.Domains {
		instance := selected.Graph.Instances[0]
		for _, candidateInstance := range selected.Graph.Instances {
			if candidateInstance.ID == domain.InstanceID {
				instance = candidateInstance
				break
			}
		}
		if instance.Kind != "resistor" || len(instance.Terminals) != 2 ||
			!((instance.Terminals[0].Node == "port_waveform_output" && instance.Terminals[1].Node == "port_timing_terminal") ||
				(instance.Terminals[1].Node == "port_waveform_output" && instance.Terminals[0].Node == "port_timing_terminal")) {
			continue
		}
		if len(domain.Candidates) == 0 || domain.Candidates[0].ValueSI == nil ||
			*domain.Candidates[0].ValueSI < 50_000 || *domain.Candidates[0].ValueSI > 200_000 {
			t.Fatalf("external timing resistance candidates = %#v", domain.Candidates)
		}
		foundTimingResistance = true
	}
	if !foundTimingResistance {
		t.Fatal("external timing graph lacks its derived charge-resistance domain")
	}
}

func TestBoundedBipolarRelationshipRejectsUnityMinimumGainWithoutInvalidResistance(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(nonlinearSwitchingCorpusRoot(), "bounded_bipolar_transfer.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	for index := range requirement.Requirements.BehavioralRequirements {
		assertion := &requirement.Requirements.BehavioralRequirements[index]
		if assertion.ID == "linear_accuracy" {
			unity := 1.0
			assertion.Min = &unity
		}
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	for _, rejection := range search.Rejections {
		if rejection.Code == "relationship_gap" {
			for _, sample := range rejection.Samples {
				if strings.Contains(sample, "no series resistance satisfying clamp current and passband gain") {
					return
				}
			}
		}
	}
	t.Fatalf("unity-minimum-gain requirement did not fail closed at the resistance interval: status=%s rejections=%#v", search.Status, search.Rejections)
}

func TestStepDownOutputCapacitorSelectionRequiresReviewedESRRippleMargin(t *testing.T) {
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	selected := topologyStepDownOutputCapacitor(inventory.Primitives, 10e-6, .1, 500_000, .05)
	if selected.Key == "" {
		t.Fatal("no output capacitor satisfies the reviewed capacitance-plus-ESR ripple envelope")
	}
	esr, _, _, found := primitiveModelParameterRange(
		selected, simmodel.PrimitiveCapacitorTransientV1, "series_resistance_ohm",
	)
	capacitance := primitiveSeedValueOrZero(selected)
	ripple := .1 * (1/(8*500_000*capacitance) + esr)
	if !found || ripple > .05 {
		t.Fatalf("selected output capacitor %s has ESR=%g found=%t capacitance=%g and ripple=%g Vpp", selected.Key, esr, found, capacitance, ripple)
	}
}

func TestStepDownValuePlanPreservesOutputCapacitorESRRippleMargin(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(nonlinearSwitchingCorpusRoot(), "efficient_step_down_power.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	inventoryByKey := primitiveInventoryByKey(inventory)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	for _, topology := range search.Candidates {
		for _, instance := range topology.Graph.Instances {
			if instance.Kind != "capacitor" || len(instance.Terminals) != 2 {
				continue
			}
			outputReference := false
			for _, node := range topology.Graph.Nodes {
				if node.ID == instance.Terminals[0].Node && node.Role == "output" {
					outputReference = true
				}
				if node.ID == instance.Terminals[1].Node && node.Role == "output" {
					outputReference = true
				}
			}
			if !outputReference {
				continue
			}
			plan := BuildValueSearchPlan(requirement, topology.Graph, inventory, DefaultPolicy())
			for _, domain := range plan.Domains {
				if domain.InstanceID != instance.ID {
					continue
				}
				for _, candidate := range domain.Candidates {
					variant := inventoryByKey[candidate.PrimitiveKey]
					if !stepDownOutputCapacitorVariantAllowed(requirement, topology.Graph, instance, variant, inventoryByKey) {
						t.Fatalf("value plan admitted output capacitor outside reviewed ripple envelope: %#v", candidate)
					}
				}
				return
			}
		}
	}
	t.Fatalf("step-down output-capacitor value domain absent: status=%s rejections=%#v", search.Status, search.Rejections)
}

func TestIntegratedReferenceRegulatorDerivesCatalogFeedbackDivider(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(nonlinearSwitchingCorpusRoot(), "efficient_step_down_power.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("step-down requirement issues = %#v", issues)
	}
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	for _, candidate := range search.Candidates {
		hasBuck := false
		for _, instance := range candidate.Graph.Instances {
			hasBuck = hasBuck || primitiveModelParameter(
				primitiveInventoryByKey(inventory)[instance.PrimitiveKey],
				simmodel.PrimitiveSynchronousBuckRegulatorV1,
				"reference_voltage_v",
			) > 0
		}
		if !hasBuck {
			continue
		}
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, DefaultPolicy())
		upper, lower := 0.0, 0.0
		series := map[string]float64{}
		for _, domain := range plan.Domains {
			for _, scale := range domain.AnalyticScales {
				switch {
				case strings.HasPrefix(scale.ID, "topology:regulated_feedback_upper:"):
					upper = scale.ValueSI
				case strings.HasPrefix(scale.ID, "topology:regulated_feedback_lower:"):
					lower = scale.ValueSI
				case strings.HasPrefix(scale.ID, "topology:regulated_feedback_series:"):
					series[strings.TrimPrefix(scale.ID, "topology:regulated_feedback_series:")] = scale.ValueSI
				}
			}
		}
		if (upper <= 0 || lower <= 0) && len(series) == 3 {
			references := topologyNodesByRole(candidate.Graph, "reference")
			for _, controller := range candidate.Graph.Instances {
				if primitiveModelParameter(
					primitiveInventoryByKey(inventory)[controller.PrimitiveKey],
					simmodel.PrimitiveSynchronousBuckRegulatorV1,
					"reference_voltage_v",
				) <= 0 || len(references) != 1 {
					continue
				}
				feedback := topologyTerminalNodes(controller)["FB"]
				for _, resistor := range candidate.Graph.Instances {
					value, found := series[resistor.ID]
					if !found || len(resistor.Terminals) != 2 {
						continue
					}
					first, second := resistor.Terminals[0].Node, resistor.Terminals[1].Node
					if first == feedback && second == references[0] || first == references[0] && second == feedback {
						lower = value
					} else {
						upper += value
					}
				}
				break
			}
		}
		if upper <= 0 || lower <= 0 {
			t.Fatalf("integrated-reference feedback scales absent: %#v", plan.Domains)
		}
		reference, referenceMinimum, referenceMaximum := 0.0, 0.0, 0.0
		for _, instance := range candidate.Graph.Instances {
			nominal, minimum, maximum, found := primitiveModelParameterRange(
				primitiveInventoryByKey(inventory)[instance.PrimitiveKey],
				simmodel.PrimitiveSynchronousBuckRegulatorV1,
				"reference_voltage_v",
			)
			if found {
				reference, referenceMinimum, referenceMaximum = nominal, minimum, maximum
			}
		}
		output := reference * (1 + upper/lower)
		if output < 4.85 || output > 5.15 {
			t.Fatalf("derived divider produces %.9g V from reference %.9g V: upper=%.9g lower=%.9g", output, reference, upper, lower)
		}
		upperPrimitive := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", upper)
		lowerPrimitive := topologyPrimitiveClosestValue(inventory.Primitives, "resistor", lower)
		upperTolerance, upperProven := primitiveTolerancePercent(upperPrimitive, "resistance")
		lowerTolerance, lowerProven := primitiveTolerancePercent(lowerPrimitive, "resistance")
		if !upperProven || !lowerProven {
			t.Fatal("derived feedback pair lacks tolerance evidence")
		}
		minimumOutput := referenceMinimum * (1 + upper*(1-upperTolerance/100)/(lower*(1+lowerTolerance/100)))
		maximumOutput := referenceMaximum * (1 + upper*(1+upperTolerance/100)/(lower*(1-lowerTolerance/100)))
		if minimumOutput < 4.85 || maximumOutput > 5.15 {
			t.Fatalf("derived divider corners %.9g..%.9g V escape 4.85..5.15 V", minimumOutput, maximumOutput)
		}
		return
	}
	t.Fatal("synchronous buck candidate absent")
}

func TestSynchronousBuckPhysicalSupportUsesLocalizedSMDParts(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(nonlinearSwitchingCorpusRoot(), "efficient_step_down_power.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("step-down requirement issues = %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	policy.MaxCandidateSimulations = 4096
	policy.MaxCornerEvaluations = 16384
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	run := Synthesize(context.Background(), requirement, inventory, environment, policy)
	if run.Report.Status != StatusPassed || run.SelectedGraph == nil || run.Physical == nil || run.Physical.Status != PhysicalLoweringReady {
		diagnoses := []Diagnosis{}
		for _, candidate := range run.Candidates {
			for _, evaluation := range candidate.Evaluations {
				diagnoses = append(diagnoses, evaluation.Diagnoses...)
				if len(diagnoses) >= 8 {
					break
				}
			}
			if len(diagnoses) >= 8 {
				break
			}
		}
		if len(diagnoses) > 8 {
			diagnoses = diagnoses[:8]
		}
		t.Fatalf("buck synthesis status=%s stop=%s diagnostics=%#v diagnoses=%#v physical=%#v", run.Report.Status, run.Report.StopReason, run.Report.Diagnostics, diagnoses, run.Physical)
	}
	controller := ""
	for _, instance := range run.SelectedGraph.Instances {
		if instance.Kind == "synchronous_buck_regulator" {
			controller = instance.ID
			break
		}
	}
	if controller == "" {
		t.Fatal("selected step-down graph lacks its synchronous-buck controller")
	}
	placements := map[string]float64{}
	for _, placement := range run.Physical.Document.PCB.Placements {
		if placement.Near == controller {
			placements[placement.Component] = placement.MaxDistanceMM
		}
	}
	supportCount := 0
	for _, binding := range run.Physical.Bindings {
		if binding.Kind != "model_support" {
			continue
		}
		supportCount++
		distance := placements[binding.Component]
		if distance <= 0 || distance > 4 {
			t.Fatalf("model support %s placement distance = %.9g, want a positive model-backed bound no greater than 4 mm", binding.SemanticID, distance)
		}
		record, found := components.LookupRecord(environment.Catalog, binding.CatalogID)
		if !found {
			t.Fatalf("model support %s selected missing catalog record %s", binding.SemanticID, binding.CatalogID)
		}
		packageType, footprint := "", ""
		for _, variant := range record.Packages {
			if variant.ID == binding.VariantID {
				packageType, footprint = strings.ToLower(variant.PackageType), strings.ToLower(variant.FootprintID)
				break
			}
		}
		if packageType == "" || strings.Contains(packageType, "tht") || !strings.Contains(footprint, "_smd:") {
			t.Fatalf("model support %s selected non-SMD package %s/%s (%s)", binding.SemanticID, binding.CatalogID, binding.VariantID, footprint)
		}
	}
	if supportCount < 2 {
		t.Fatalf("model-backed controller support bindings = %d, want at least bootstrap and local bypass", supportCount)
	}
	if !slices.ContainsFunc(run.Physical.Document.PowerFlags, func(flag circuitgraph.PowerFlag) bool {
		return flag.Net == "REGULATED_OUTPUT"
	}) {
		t.Fatalf("generated power output lacks ERC drive evidence: %#v", run.Physical.Document.PowerFlags)
	}
}

func TestDynamicPulseMeasurementUsesDeclaredFrequencyAndEventPhase(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(nonlinearSwitchingCorpusRoot(), "controlled_pulse_power_stage.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("controlled-pulse requirement issues = %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("controlled-pulse initial graph issues = %#v", graphIssues)
	}
	var assertion BehavioralAssertion
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.ID == "pulse_duty_transfer" {
			assertion = candidate
		}
	}
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "nominal_pulses" {
			operatingCase = candidate
		}
	}
	if assertion.ID == "" || operatingCase.ID == "" {
		t.Fatal("controlled-pulse corpus case lacks its duty assertion or nominal operating case")
	}
	corner := operatingCaseCorners(operatingCase)[0]
	duration := dynamicDurationForRequirement(requirement, assertion, operatingCase)
	if math.Abs(duration-.00051) > 1e-12 {
		t.Fatalf("periodic duration = %.12g, want ten periods after the 10 us event", duration)
	}
	found := false
	for _, excitation := range simulationExcitations(requirement, assertion, operatingCase, corner, graph) {
		if excitation.PulsePeriodS == 0 {
			continue
		}
		found = true
		if math.Abs(excitation.PulsePeriodS-50e-6) > 1e-15 ||
			math.Abs(excitation.PulseWidthS-25e-6) > 1e-15 ||
			math.Abs(excitation.PulseDelayS-10e-6) > 1e-15 ||
			excitation.SineFrequencyHz != 0 {
			t.Fatalf("periodic excitation = %#v", excitation)
		}
	}
	if !found {
		t.Fatal("declared PWM frequency did not produce a periodic source excitation")
	}
}

func TestAutonomousPeriodicMeasurementsDeriveZeroEnergySupplyStartup(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(nonlinearSwitchingCorpusRoot(), "autonomous_square_wave_source.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("autonomous requirement issues = %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("autonomous initial graph issues = %#v", graphIssues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.ID == "output_span" {
			assertion = candidate
			break
		}
	}
	operatingCase := requirement.Requirements.OperatingCases[1]
	corner := operatingCaseCorners(operatingCase)[0]
	analysis := simmodel.Analysis{
		ID: "span", Kind: simmodel.AnalysisTransient, DurationS: .01, TimeStepS: 1e-5,
		Excitations: simulationExcitations(requirement, assertion, operatingCase, corner, graph),
	}
	addAutonomousStartupEvents(&analysis, requirement, assertion, graph)
	found := false
	for _, event := range analysis.SourceValueEvents {
		if event.Component == "source_port_power" && event.TriggerTimeS == 0 && event.Initial == 0 && event.Applied > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("autonomous startup events = %#v", analysis.SourceValueEvents)
	}
}
