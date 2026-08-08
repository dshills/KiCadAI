package opentopologysynthesis

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/simmodel"
)

func TestPhysicalSchematicIntentUsesTopologyDerivedCoreRanks(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "vin", SemanticID: "vin", Scope: "external", Role: "input"},
			{ID: "sense", Scope: "internal", Role: "feedback"},
			{ID: "out", SemanticID: "out", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{
			{ID: "controller", Kind: "opamp"},
			{ID: "pass", Kind: "nmos"},
			{ID: "sense_resistor", Kind: "resistor"},
		},
	}
	intent := physicalSchematicIntent(graph)
	groupByComponent := map[string]string{}
	nearByComponent := map[string]string{}
	for _, placement := range intent.Placements {
		if _, exists := groupByComponent[placement.Component]; exists {
			t.Fatalf("duplicate placement for %s: %#v", placement.Component, intent.Placements)
		}
		groupByComponent[placement.Component] = placement.Group
		nearByComponent[placement.Component] = placement.Near
		if placement.Mirror != "" {
			t.Fatalf("core mirror was fixed before topology layout: %#v", placement)
		}
	}
	ranks := physicalTopologyRanks(graph)
	for _, component := range []string{"controller", "pass", "sense_resistor"} {
		wantGroup := fmt.Sprintf("topology_rank_%03d", ranks[component])
		if groupByComponent[component] != wantGroup || nearByComponent[component] != "" {
			t.Fatalf("%s topology group = %q, near = %q; want %q and no named near chain", component, groupByComponent[component], nearByComponent[component], wantGroup)
		}
	}
	groupRanks := map[string]int{}
	for _, group := range intent.Groups {
		groupRanks[group.ID] = group.Rank
	}
	for _, component := range []string{"controller", "pass", "sense_resistor"} {
		group := groupByComponent[component]
		if groupRanks[group] != ranks[component] {
			t.Fatalf("%s group rank = %d, want topology rank %d", component, groupRanks[group], ranks[component])
		}
	}
	if groupByComponent["interface_out"] != "external_outputs" || groupRanks["external_outputs"] != 4 {
		t.Fatalf("output connector boundary placement = %#v, ranks=%#v", groupByComponent, groupRanks)
	}
	if intent.Rules.MinComponentSpacingMM > 10.16 {
		t.Fatalf("synthesized component spacing is not compact: %v", intent.Rules.MinComponentSpacingMM)
	}
	if intent.Rules.PreferLabelsForLongNets == nil || *intent.Rules.PreferLabelsForLongNets {
		t.Fatal("synthesized local nets should prefer continuous conductors")
	}
	if intent.Rules.MaxAuxiliaryPerRank != 2 {
		t.Fatalf("auxiliary components per rank = %d, want 2", intent.Rules.MaxAuxiliaryPerRank)
	}
	if !intent.Rules.ReserveTitleBlock {
		t.Fatal("synthesized schematics must reserve the standard title block")
	}
	if !intent.Rules.OrientEndpointLabels {
		t.Fatal("synthesized endpoint labels must face away from component bodies")
	}
	if intent.Hierarchy.Mode != "auto" || intent.Hierarchy.MaxComponentsPerSheet != 3 {
		t.Fatalf("functional hierarchy policy = %#v, want automatic grouping with the largest derived stage kept intact", intent.Hierarchy)
	}
}

func TestPhysicalBoardUsesFourLayersOnlyForDenseFabricationCandidates(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", Role: "input"},
			{ID: "middle", Scope: "internal", Role: "signal"},
			{ID: "output", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{
			{ID: "first", Terminals: []TerminalConnection{{Node: "input"}, {Node: "middle"}}},
			{ID: "second", Terminals: []TerminalConnection{{Node: "middle"}, {Node: "output"}}},
			{ID: "bias", Terminals: []TerminalConnection{{Node: "middle"}}},
			{ID: "protection", Terminals: []TerminalConnection{{Node: "output"}}},
		},
	}
	requirement := Requirement{
		Requirements: Requirements{Constraints: BoardLimits{MaxWidthMM: 80, MaxHeightMM: 60}},
		Acceptance: Acceptance{
			RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true,
			RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true,
		},
	}
	if got := physicalBoard(requirement, graph, PrimitiveInventory{}).Layers; got != 4 {
		t.Fatalf("fabrication multi-stage layers = %d, want 4", got)
	}
	requirement.Acceptance.RequireStrictDRC = false
	if got := physicalBoard(requirement, graph, PrimitiveInventory{}).Layers; got != 2 {
		t.Fatalf("non-fabrication layers = %d, want 2", got)
	}
	graph.Instances = graph.Instances[:3]
	requirement.Acceptance.RequireStrictDRC = true
	if got := physicalBoard(requirement, graph, PrimitiveInventory{}).Layers; got != 2 {
		t.Fatalf("small graph layers = %d, want 2", got)
	}
}

func TestPhysicalMultilayerIntentReservesReferenceAndPowerLayers(t *testing.T) {
	nets := []circuitgraph.Net{
		{Name: "0V", Role: circuitgraph.NetRoleGround},
		{Name: "VCC", Role: circuitgraph.NetRolePowerPos},
		{Name: "SIGNAL", Role: circuitgraph.NetRoleSignal},
	}
	intent := physicalPCBIntent(circuitgraph.Board{WidthMM: 60, HeightMM: 40, Layers: 4}, nil, nets, circuitgraph.SchematicIntent{}, nil)
	if len(intent.Zones) != 2 || intent.Zones[0].Net != "0V" || intent.Zones[0].Layers[0] != "In1.Cu" ||
		intent.Zones[1].Net != "VCC" || intent.Zones[1].Layers[0] != "In2.Cu" {
		t.Fatalf("multilayer zones = %#v", intent.Zones)
	}
	for _, test := range []struct {
		role   circuitgraph.NetRole
		layers []string
		prefer string
	}{
		{circuitgraph.NetRoleGround, []string{"F.Cu", "In1.Cu", "B.Cu"}, "In1.Cu"},
		{circuitgraph.NetRolePowerPos, []string{"F.Cu", "In2.Cu", "B.Cu"}, "In2.Cu"},
		{circuitgraph.NetRoleSignal, []string{"F.Cu", "B.Cu"}, "F.Cu"},
	} {
		layers, prefer := physicalNetLayerIntent(4, test.role)
		if fmt.Sprint(layers) != fmt.Sprint(test.layers) || prefer != test.prefer {
			t.Fatalf("role %s layer intent = %v prefer %q, want %v prefer %q", test.role, layers, prefer, test.layers, test.prefer)
		}
	}
	physicalApplyReturnPathIntent(4, nets)
	if nets[0].ReturnNet != "" || nets[1].ReturnNet != "0V" || nets[2].ReturnNet != "0V" ||
		nets[1].ReturnPathMaxDistanceMM != physicalReferencePlaneBudgetMM ||
		nets[2].ReturnPathMaxDistanceMM != physicalReferencePlaneBudgetMM {
		t.Fatalf("four-layer return-path intent = %#v", nets)
	}
	twoLayer := append([]circuitgraph.Net(nil), nets...)
	for index := range twoLayer {
		twoLayer[index].ReturnNet = ""
		twoLayer[index].ReturnPathMaxDistanceMM = 0
	}
	physicalApplyReturnPathIntent(2, twoLayer)
	if twoLayer[1].ReturnNet != "" || twoLayer[2].ReturnNet != "" {
		t.Fatalf("two-layer circuit acquired multilayer return-path intent: %#v", twoLayer)
	}
	explicit := []circuitgraph.Net{
		{Name: "0V", Role: circuitgraph.NetRoleGround},
		{Name: "CLOCK", Role: circuitgraph.NetRoleClock, ReturnNet: "CHASSIS", ReturnPathMaxDistanceMM: .4},
	}
	physicalApplyReturnPathIntent(4, explicit)
	if explicit[1].ReturnNet != "CHASSIS" || explicit[1].ReturnPathMaxDistanceMM != .4 {
		t.Fatalf("explicit return-path constraint was overwritten: %#v", explicit[1])
	}
}

func TestPhysicalPrimaryPowerPlaneNetUsesElectricalDemandAndStableTies(t *testing.T) {
	nets := []circuitgraph.Net{
		{Name: "CONTROL", Role: circuitgraph.NetRolePower, CurrentMA: 50, Endpoints: make([]circuitgraph.Endpoint, 5)},
		{Name: "LOAD", Role: circuitgraph.NetRolePowerPos, CurrentMA: 2_000, Endpoints: make([]circuitgraph.Endpoint, 2)},
		{Name: "AUXILIARY", Role: circuitgraph.NetRolePower, CurrentMA: 50, Endpoints: make([]circuitgraph.Endpoint, 3)},
		{Name: "GROUND", Role: circuitgraph.NetRoleGround, CurrentMA: 10_000, Endpoints: make([]circuitgraph.Endpoint, 20)},
	}
	if got := physicalPrimaryPowerPlaneNet(nets); got != "CONTROL" {
		t.Fatalf("primary power plane = %q, want highest-fanout CONTROL domain", got)
	}
	nets[1].Endpoints = make([]circuitgraph.Endpoint, 5)
	if got := physicalPrimaryPowerPlaneNet(nets); got != "LOAD" {
		t.Fatalf("primary power plane = %q, want highest-current LOAD fanout tie", got)
	}
	nets[1].CurrentMA = 50
	nets[0].Endpoints = nets[0].Endpoints[:3]
	nets[1].Endpoints = nets[1].Endpoints[:3]
	if got := physicalPrimaryPowerPlaneNet(nets); got != "AUXILIARY" {
		t.Fatalf("primary power plane = %q, want lexical AUXILIARY tie-break", got)
	}
}

func TestPhysicalFunctionalRegionBoundsRetainPackageWidthsWhenDense(t *testing.T) {
	board := circuitgraph.Board{WidthMM: 20, HeightMM: 30}
	seeds := []*physicalRegionSeed{
		{role: physicalRegionInput, weight: 8},
		{role: physicalRegionControl, weight: 10},
		{role: physicalRegionPower, weight: 9},
		{role: physicalRegionOutput, weight: 8},
	}
	bounds := physicalFunctionalRegionBounds(board, seeds)
	if len(bounds) != len(seeds) {
		t.Fatalf("dense functional bounds = %#v", bounds)
	}
	overlap := false
	for index, bound := range bounds {
		if bound.WidthMM+1e-12 < seeds[index].weight || bound.XMM < 0 ||
			bound.XMM+bound.WidthMM > board.WidthMM+1e-12 || bound.HeightMM != board.HeightMM {
			t.Fatalf("dense functional bound %d lost capacity: %#v seed=%#v", index, bound, seeds[index])
		}
		if index > 0 {
			if bound.XMM < bounds[index-1].XMM {
				t.Fatalf("dense functional centers lost ordering: %#v", bounds)
			}
			overlap = overlap || bound.XMM < bounds[index-1].XMM+bounds[index-1].WidthMM
		}
	}
	if !overlap {
		t.Fatalf("dense functional regions did not use bounded overlap: %#v", bounds)
	}
}

func TestPhysicalPCBIntentDerivesDeterministicFunctionalAndThermalPlacement(t *testing.T) {
	if role := physicalFunctionalRole(circuitgraph.Component{}, "EXTERNAL_INPUTS", "", 0); role != physicalRegionInput {
		t.Fatalf("case-insensitive external input role = %q", role)
	}
	if order := physicalFunctionalRoleOrder("future_functional_role"); order != 25 {
		t.Fatalf("unknown functional role order = %d, want middle-of-flow rank 25", order)
	}
	junctionToCase, junctionToAmbient := 1.0, 62.0
	freeAirRecord := components.ComponentRecord{PowerSemiconductor: &components.PowerSemiconductorEvidence{
		JunctionToCaseCPerW: &junctionToCase, JunctionToAmbientCPerW: &junctionToAmbient,
	}}
	if physicalRequiresExternalThermalPath(freeAirRecord) {
		t.Fatal("junction-to-case capability alone became an external thermal-path requirement")
	}
	freeAirRecord.ID = "test.free_air_power_device"
	pathID, pathCPerW, found := physicalIntrinsicAmbientThermalPath(freeAirRecord)
	if !found || pathID != "test.free_air_power_device.power_semiconductor_junction_to_ambient" || pathCPerW != junctionToAmbient {
		t.Fatalf("intrinsic ambient thermal path = %q %.3f found=%t", pathID, pathCPerW, found)
	}
	caseReferencedRecord := freeAirRecord
	caseReferencedRecord.SimulationModels = []simmodel.CatalogEvidence{{
		ModelID: "case_referenced", ThermalModel: &simmodel.ThermalRCNetwork{Reference: "junction_to_case"},
	}}
	if !physicalRequiresExternalThermalPath(caseReferencedRecord) {
		t.Fatal("junction-to-case simulation boundary was incorrectly closed by unrelated ambient evidence")
	}
	freeAirRecord.PlacementHints = []components.PlacementHint{{Kind: "thermal_edge", Target: "heatsink"}}
	if !physicalRequiresExternalThermalPath(freeAirRecord) {
		t.Fatal("catalog thermal-edge requirement was ignored when junction-to-ambient evidence also exists")
	}
	tokens := physicalSemanticTokens("3V3 TO220 SOT-23")
	if !tokens["3v3"] || !tokens["to220"] || !tokens["sot"] || !tokens["23"] {
		t.Fatalf("alphanumeric semantic tokens = %#v", tokens)
	}
	if edge := physicalRegionBoardEdge(
		circuitgraph.PCBRegion{ID: "power", Role: physicalRegionPower, Bounds: circuitgraph.Bounds{XMM: 10, WidthMM: 20, HeightMM: 40}},
		circuitgraph.Board{WidthMM: 50, HeightMM: 40}, "heat_a",
	); edge != circuitgraph.SideBottom {
		t.Fatalf("central thermal edge = %q, want bottom", edge)
	}
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalog.Records = append(catalog.Records, components.ComponentRecord{
		ID: "test.nonfinite_dimensions",
		Packages: []components.PackageVariant{{
			ID: "bad", DimensionsMM: &components.Bounds{Width: math.NaN(), Height: math.Inf(1)},
		}},
	})
	components.RebuildCatalogIndexes(catalog)
	to252Weight := physicalRegionWeight([]circuitgraph.Component{{
		ID: "power_switch", ComponentID: "mosfet.onsemi.rfd16n05lsm.to252", VariantID: "to252_3",
	}}, catalog)
	if minimum := 6.73 + 2*physicalRegionPackagePaddingMM; to252Weight < minimum {
		t.Fatalf("TO-252 functional-region weight = %.3f, want at least bilateral package span %.3f", to252Weight, minimum)
	}
	if width, height := physicalPackageDimensions(circuitgraph.Component{
		ComponentID: "test.nonfinite_dimensions", VariantID: "bad",
	}, catalog); width != physicalDefaultPackageWidthMM || height != physicalDefaultPackageHeightMM {
		t.Fatalf("non-finite package fallback = %vx%v", width, height)
	}
	board := circuitgraph.Board{WidthMM: 80, HeightMM: 60, Layers: 4, EdgeClearanceMM: .25}
	staleGroupRegions, staleGroupMapping := physicalFunctionalRegions(board, []circuitgraph.Component{{
		ID: "orphan_sense", Role: circuitgraph.RoleResistor, Usage: "current_sense",
	}}, circuitgraph.SchematicIntent{Placements: []circuitgraph.SchematicPlacement{{
		Component: "orphan_sense", Group: "missing_group",
	}}}, catalog)
	if len(staleGroupRegions) != 1 || staleGroupMapping["orphan_sense"] != "functional_sensing" {
		t.Fatalf("stale group fallback regions=%#v mapping=%#v", staleGroupRegions, staleGroupMapping)
	}
	componentList := []circuitgraph.Component{
		{ID: "input", Role: circuitgraph.RoleInputConnector, Usage: "command_input"},
		{ID: "input_fuse", Role: circuitgraph.RoleFuse, Usage: "supply_protection"},
		{ID: "sense", Role: circuitgraph.RoleResistor, Usage: "current_sense"},
		{ID: "current_clamp", Role: circuitgraph.RoleResistor, Usage: "current_clamp"},
		{ID: "controller", Role: circuitgraph.RoleIC, Usage: "error_amplifier"},
		{ID: "clamp", Role: circuitgraph.RoleTVS, Usage: "output_clamp"},
		{ID: "pass", Role: circuitgraph.RoleBJT, Usage: "power_output", ComponentID: "bjt.onsemi.njw0302g.to3p", VariantID: "to3p_3"},
		{ID: "output_tvs", Role: circuitgraph.RoleTVS, Usage: "load_protection"},
		{ID: "output", Role: circuitgraph.RoleOutputConnector, Usage: "load_output"},
	}
	schematic := circuitgraph.SchematicIntent{
		Groups: []circuitgraph.SchematicGroup{
			{ID: "external_inputs", Role: "input_boundary", Members: []string{"input"}, Rank: 0},
			{ID: "input_protection_stage", Role: "processing_stage", Members: []string{"input_fuse"}, Rank: 1},
			{ID: "sense_stage", Role: "processing_stage", Members: []string{"sense", "current_clamp"}, Rank: 1},
			{ID: "control_stage", Role: "processing_stage", Members: []string{"controller", "clamp"}, Rank: 2},
			{ID: "power_stage", Role: "processing_stage", Members: []string{"pass", "output_tvs"}, Rank: 3},
			{ID: "external_outputs", Role: "output_boundary", Members: []string{"output"}, Rank: 4},
		},
	}
	first := physicalPCBIntent(board, componentList, nil, schematic, catalog)
	second := physicalPCBIntent(board, componentList, nil, schematic, catalog)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("functional placement is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	thermalCohort := make([]circuitgraph.Component, 0, 6)
	thermalMembers := make([]string, 0, 6)
	for index := 0; index < 6; index++ {
		id := fmt.Sprintf("parallel_heat_%d", index)
		thermalMembers = append(thermalMembers, id)
		thermalCohort = append(thermalCohort, circuitgraph.Component{
			ID: id, Role: circuitgraph.RoleBJT, Usage: "power_output",
			ComponentID: "bjt.onsemi.njw0302g.to3p", VariantID: "to3p_3",
		})
	}
	cohortComponents := append([]circuitgraph.Component{
		{ID: "cohort_input", Role: circuitgraph.RoleInputConnector},
		{ID: "cohort_output", Role: circuitgraph.RoleOutputConnector},
	}, thermalCohort...)
	cohortIntent := physicalPCBIntent(
		board,
		cohortComponents,
		nil,
		circuitgraph.SchematicIntent{Groups: []circuitgraph.SchematicGroup{
			{ID: "external_inputs", Role: "input_boundary", Members: []string{"cohort_input"}, Rank: 0},
			{ID: "power_stage", Role: "processing_stage", Members: thermalMembers, Rank: 3},
			{ID: "external_outputs", Role: "output_boundary", Members: []string{"cohort_output"}, Rank: 4},
		}},
		catalog,
	)
	edgeCounts := map[circuitgraph.Side]int{}
	for _, placement := range cohortIntent.Placements {
		if !strings.HasPrefix(placement.Component, "parallel_heat_") {
			continue
		}
		edgeCounts[placement.Edge]++
	}
	if edgeCounts[circuitgraph.SideTop] != 3 || edgeCounts[circuitgraph.SideBottom] != 3 {
		t.Fatalf("capacity-aware thermal cohort edges = %#v, want balanced top/bottom access", edgeCounts)
	}
	wantRoles := []string{
		physicalRegionInput, physicalRegionInputProtection, physicalRegionSensing, physicalRegionControl,
		physicalRegionProtection, physicalRegionPower, physicalRegionOutputProtection, physicalRegionOutput,
	}
	gotRoles := make([]string, len(first.Regions))
	previousEnd := 0.0
	for index, region := range first.Regions {
		gotRoles[index] = region.Role
		if region.Bounds.XMM < previousEnd || region.Bounds.WidthMM <= 0 || region.Bounds.HeightMM != board.HeightMM ||
			region.Bounds.XMM+region.Bounds.WidthMM > board.WidthMM+1e-9 {
			t.Fatalf("invalid functional region geometry after x=%v: %#v", previousEnd, region)
		}
		previousEnd = region.Bounds.XMM + region.Bounds.WidthMM
	}
	if !reflect.DeepEqual(gotRoles, wantRoles) {
		t.Fatalf("functional region roles = %v, want %v", gotRoles, wantRoles)
	}
	placements := map[string]circuitgraph.PCBPlacement{}
	for _, placement := range first.Placements {
		placements[placement.Component] = placement
	}
	if placements["input"].Region != "functional_input_interface" || placements["input"].Edge != circuitgraph.SideLeft ||
		placements["output"].Region != "functional_output_interface" || placements["output"].Edge != circuitgraph.SideRight {
		t.Fatalf("interface placement = input %#v output %#v", placements["input"], placements["output"])
	}
	if placements["input_fuse"].Region != "functional_input_protection" ||
		placements["current_clamp"].Region != "functional_sensing" ||
		placements["output_tvs"].Region != "functional_output_protection" {
		t.Fatalf("rank-aware protection/current-clamp regions = fuse %#v clamp %#v tvs %#v", placements["input_fuse"], placements["current_clamp"], placements["output_tvs"])
	}
	thermalPlacement := placements["pass"]
	if thermalPlacement.Region != "functional_power" || thermalPlacement.Edge != circuitgraph.SideTop || thermalPlacement.Priority != 110 {
		t.Fatalf("thermal component placement = %#v", thermalPlacement)
	}
	if nonthermal := placements["controller"]; nonthermal.Edge != "" || nonthermal.Priority != 80 {
		t.Fatalf("unrelated component acquired thermal constraints: %#v", nonthermal)
	}
	document := circuitgraph.Document{Project: circuitgraph.Project{Board: board}, Components: componentList, PCB: first}
	evidence, issues := physicalPlacementEvidence(document, catalog)
	if len(issues) != 0 {
		t.Fatalf("placement evidence issues = %#v", issues)
	}
	thermalEvidence := PhysicalPlacementEvidence{}
	for _, entry := range evidence {
		if entry.Kind == "thermal_placement" && entry.Component == "pass" {
			thermalEvidence = entry
		}
	}
	if thermalEvidence.Region != "functional_power" || thermalEvidence.Edge != circuitgraph.SideTop ||
		thermalEvidence.Role != "power_switch" || thermalEvidence.PackageType != "to3p_3" ||
		thermalEvidence.ThermalPathID != "thermal_path.wakefield.641k_120_5" || math.Abs(thermalEvidence.ThermalPathCPerW-2.45) > 1e-12 ||
		thermalEvidence.MinimumClearanceMM <= 0 ||
		!thermalEvidence.BoardEdgeRequired || !thermalEvidence.PreferThermalCopper || thermalEvidence.EvidenceSHA == "" || thermalEvidence.Rationale == "" {
		t.Fatalf("catalog-backed thermal evidence = %#v", thermalEvidence)
	}
	smallBoardDecision, ok := physicalThermalPlacement(
		componentList[6],
		circuitgraph.PCBRegion{ID: "power", Role: physicalRegionPower, Bounds: circuitgraph.Bounds{WidthMM: 4, HeightMM: 4}},
		circuitgraph.Board{WidthMM: 4, HeightMM: 4}, catalog,
	)
	if !ok || smallBoardDecision.clearanceMM < physicalThermalMinimumClearanceMM {
		t.Fatalf("small-board thermal clearance lost its safety floor: %#v ok=%t", smallBoardDecision, ok)
	}
	request := designworkflow.Request{ExplicitCircuit: &designworkflow.ExplicitCircuitSpec{Components: []designworkflow.ExplicitComponentSpec{
		{ID: "controller", Placement: designworkflow.ExplicitPlacementSpec{Region: "functional_control"}},
		{ID: "pass", Placement: designworkflow.ExplicitPlacementSpec{Region: "functional_power", Edge: "top"}},
	}}}
	applyPhysicalPlacementEvidence(&request, evidence)
	ordinary := request.ExplicitCircuit.Components[0].Placement
	applied := request.ExplicitCircuit.Components[1].Placement
	if ordinary.ThermalRole != "" || ordinary.Region != "functional_control" ||
		applied.ThermalRole != "power_switch" || applied.ThermalPathID != thermalEvidence.ThermalPathID ||
		applied.ThermalClearanceMM != thermalEvidence.MinimumClearanceMM || applied.ThermalKeepAwayRole != physicalThermalSensitiveRole ||
		!applied.ThermalEdgeRequired || !applied.PreferThermalCopper {
		t.Fatalf("physical request evidence application ordinary=%#v thermal=%#v", ordinary, applied)
	}
	bindings := physicalPlacementBindings(evidence)
	if len(bindings) != len(evidence) || bindings[len(bindings)-1].Kind != "thermal_placement" ||
		bindings[len(bindings)-1].EvidenceSHA != thermalEvidence.EvidenceSHA {
		t.Fatalf("placement bindings = %#v", bindings)
	}
	catalogWithoutPaths, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	catalogWithoutPaths.ThermalPaths = nil
	_, issues = physicalPlacementEvidence(document, catalogWithoutPaths)
	if len(issues) != 1 || issues[0].Path != "physical.placement.pass.thermal_path" {
		t.Fatalf("missing reviewed thermal path did not fail closed: %#v", issues)
	}
}

func TestPhysicalEngineeringValueUsesReadableSIPrefixes(t *testing.T) {
	tests := []struct {
		value float64
		unit  string
		want  string
	}{
		{909000, "Ohm", "909k"},
		{0.22, "ohm", "220m"},
		{15e-9, "F", "15nF"},
		{220e-6, "F", "220uF"},
		{2.2e6, "Hz", "2.2MHz"},
		{1e-18, "F", "1e-18F"},
	}
	for _, test := range tests {
		if got := physicalEngineeringValue(test.value, test.unit); got != test.want {
			t.Errorf("physicalEngineeringValue(%g, %q) = %q, want %q", test.value, test.unit, got, test.want)
		}
	}
}

func TestPhysicalSchematicValueKindsIncludeReferencesAndOscillators(t *testing.T) {
	for _, kind := range []string{"resistance", "capacitance", "inductance", "voltage", "frequency"} {
		if !physicalSchematicValueKind(kind) {
			t.Fatalf("physical schematic value kind %q was omitted", kind)
		}
	}
	if physicalSchematicValueKind("current_rating") {
		t.Fatal("rating-only quantity was rendered as a component value")
	}
}

func TestPhysicalPassiveOrientationsFollowRailAndSignalTopology(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "supply", Scope: "external", Role: "supply"},
			{ID: "signal_a", Scope: "internal", Role: "signal"},
			{ID: "signal_b", Scope: "internal", Role: "signal"},
			{ID: "reference", Scope: "external", Role: "reference"},
		},
		Instances: []GraphInstance{
			{
				ID: "forward_path", Kind: "resistor",
				Terminals: []TerminalConnection{{Terminal: "A", Node: "signal_a"}, {Terminal: "B", Node: "signal_b"}},
			},
			{
				ID: "upper_rail_branch", Kind: "resistor",
				Terminals: []TerminalConnection{{Terminal: "A", Node: "supply"}, {Terminal: "B", Node: "signal_a"}},
			},
			{
				ID: "lower_rail_branch", Kind: "capacitor",
				Terminals: []TerminalConnection{{Terminal: "A", Node: "signal_b"}, {Terminal: "B", Node: "reference"}},
			},
			{
				ID: "controller", Kind: "opamp",
				Terminals: []TerminalConnection{{Terminal: "IN_PLUS", Node: "signal_a"}, {Terminal: "OUT", Node: "signal_b"}},
			},
		},
	}

	orientations := physicalPassiveOrientations(graph)
	if orientations["forward_path"] != "rotated_90" {
		t.Fatalf("forward-path orientation = %q, want horizontal", orientations["forward_path"])
	}
	for _, component := range []string{"upper_rail_branch", "lower_rail_branch"} {
		if orientations[component] != "normal" {
			t.Fatalf("%s orientation = %q, want vertical rail branch", component, orientations[component])
		}
	}
	if _, exists := orientations["controller"]; exists {
		t.Fatalf("active device received passive orientation: %#v", orientations)
	}

	intent := physicalSchematicIntent(graph)
	byComponent := map[string]string{}
	for _, placement := range intent.Placements {
		byComponent[placement.Component] = placement.Orientation
	}
	for component, want := range orientations {
		if byComponent[component] != want {
			t.Fatalf("%s intent orientation = %q, want %q", component, byComponent[component], want)
		}
	}
}

func TestPhysicalTopologyRanksFollowBoundaryDistances(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "input", Scope: "external", Role: "input"},
			{ID: "first", Scope: "internal", Role: "signal"},
			{ID: "second", Scope: "internal", Role: "signal"},
			{ID: "output", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{
			{ID: "early", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "input"}, {Terminal: "B", Node: "first"}}},
			{ID: "middle", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "first"}, {Terminal: "B", Node: "second"}}},
			{ID: "late", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "second"}, {Terminal: "B", Node: "output"}}},
		},
	}
	ranks := physicalTopologyRanks(graph)
	if !(ranks["early"] < ranks["middle"] && ranks["middle"] < ranks["late"]) {
		t.Fatalf("topology ranks = %#v, want monotonic boundary flow", ranks)
	}
}

func TestPhysicalTopologyNetRolesRecognizesPassiveFeedbackReturn(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "drive", Scope: "internal", Role: "signal"},
			{ID: "sense", Scope: "internal", Role: "signal"},
			{ID: "output", Scope: "external", Role: "output"},
			{ID: "setpoint", Scope: "external", Role: "input"},
		},
		Instances: []GraphInstance{
			{
				ID: "controller", Kind: "opamp",
				Terminals: []TerminalConnection{
					{Terminal: "IN_PLUS", Node: "setpoint"},
					{Terminal: "IN_MINUS", Node: "sense"},
					{Terminal: "OUT", Node: "drive"},
				},
			},
			{
				ID: "feedback_a", Kind: "resistor",
				Terminals: []TerminalConnection{
					{Terminal: "A", Node: "drive"},
					{Terminal: "B", Node: "output"},
				},
			},
			{
				ID: "feedback_b", Kind: "resistor",
				Terminals: []TerminalConnection{
					{Terminal: "A", Node: "output"},
					{Terminal: "B", Node: "sense"},
				},
			},
		},
	}

	roles := physicalTopologyNetRoles(graph)
	if roles["sense"] != circuitgraph.NetRoleFeedback {
		t.Fatalf("sense role = %q, want %q", roles["sense"], circuitgraph.NetRoleFeedback)
	}
	if roles["setpoint"] != "" {
		t.Fatalf("setpoint was misclassified as feedback: %q", roles["setpoint"])
	}
}

func TestPhysicalTopologyNetRolesRetainsParallelFeedbackAndPrunesBiasBranch(t *testing.T) {
	graph := CandidateGraph{
		Nodes: []GraphNode{
			{ID: "sense", Scope: "internal", Role: "signal"},
			{ID: "upper", Scope: "internal", Role: "signal"},
			{ID: "lower", Scope: "internal", Role: "signal"},
			{ID: "bias", Scope: "internal", Role: "signal"},
			{ID: "output", Scope: "external", Role: "output"},
		},
		Instances: []GraphInstance{
			{ID: "controller", Kind: "opamp", Terminals: []TerminalConnection{{Terminal: "IN_MINUS", Node: "sense"}, {Terminal: "OUT", Node: "output"}}},
			{ID: "upper_a", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "sense"}, {Terminal: "B", Node: "upper"}}},
			{ID: "upper_b", Kind: "capacitor", Terminals: []TerminalConnection{{Terminal: "A", Node: "upper"}, {Terminal: "B", Node: "output"}}},
			{ID: "lower_a", Kind: "capacitor", Terminals: []TerminalConnection{{Terminal: "A", Node: "sense"}, {Terminal: "B", Node: "lower"}}},
			{ID: "lower_b", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "lower"}, {Terminal: "B", Node: "output"}}},
			{ID: "bias_branch", Kind: "resistor", Terminals: []TerminalConnection{{Terminal: "A", Node: "upper"}, {Terminal: "B", Node: "bias"}}},
		},
	}

	roles := physicalTopologyNetRoles(graph)
	for _, node := range []string{"sense", "upper", "lower"} {
		if roles[node] != circuitgraph.NetRoleFeedback {
			t.Fatalf("%s role = %q, want feedback", node, roles[node])
		}
	}
	for _, node := range []string{"output", "bias"} {
		if roles[node] != "" {
			t.Fatalf("%s was incorrectly classified as feedback: %q", node, roles[node])
		}
	}
}
