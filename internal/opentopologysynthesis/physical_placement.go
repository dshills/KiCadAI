package opentopologysynthesis

import (
	"cmp"
	"crypto/sha256"
	"math"
	"slices"
	"strings"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

const (
	physicalRegionInput            = "input_interface"
	physicalRegionInputProtection  = "input_protection"
	physicalRegionSensing          = "sensing"
	physicalRegionControl          = "control"
	physicalRegionPower            = "power"
	physicalRegionProtection       = "protection"
	physicalRegionOutputProtection = "output_protection"
	physicalRegionOutput           = "output_interface"

	physicalRegionMinimumWeight             = 6.0
	physicalRegionPackagePaddingMM          = 3.0
	physicalRegionAdditionalComponentWeight = 0.35
	physicalThermalMinimumClearanceMM       = 2.0
	physicalThermalSensitiveRole            = "sensor"
	physicalDefaultPackageWidthMM           = 2.0
	physicalDefaultPackageHeightMM          = 2.0
	physicalMinimumPackageDimensionMM       = 1.0
)

type physicalRegionSeed struct {
	role       string
	components []circuitgraph.Component
	rankTotal  int
	rankCount  int
	weight     float64
}

func physicalPCBIntent(
	board circuitgraph.Board,
	componentList []circuitgraph.Component,
	nets []circuitgraph.Net,
	schematic circuitgraph.SchematicIntent,
	catalog *components.Catalog,
) circuitgraph.PCBIntent {
	regions, regionByComponent := physicalFunctionalRegions(board, componentList, schematic, catalog)
	placements := make([]circuitgraph.PCBPlacement, 0, len(componentList))
	orderedComponents := append([]circuitgraph.Component(nil), componentList...)
	slices.SortStableFunc(orderedComponents, func(left, right circuitgraph.Component) int {
		return strings.Compare(left.ID, right.ID)
	})
	regionsByID := make(map[string]circuitgraph.PCBRegion, len(regions))
	for _, region := range regions {
		regionsByID[region.ID] = region
	}
	thermalEdgeLoads := map[circuitgraph.Side]float64{}
	for _, component := range orderedComponents {
		regionID := regionByComponent[component.ID]
		placement := circuitgraph.PCBPlacement{Component: component.ID, Region: regionID, Priority: 80}
		switch component.Role {
		case circuitgraph.RoleInputConnector:
			placement.Edge = circuitgraph.SideLeft
			placement.Priority = 100
		case circuitgraph.RoleOutputConnector:
			placement.Edge = circuitgraph.SideRight
			placement.Priority = 100
		}
		if thermal, ok := physicalThermalPlacement(component, regionsByID[regionID], board, catalog); ok {
			if thermal.boardEdgeRequired {
				width, height := physicalPackageDimensions(component, catalog)
				thermal.edge = physicalCapacityAwareThermalEdge(
					thermal.edge, board, math.Min(width, height)+physicalRegionPackagePaddingMM, thermalEdgeLoads,
				)
			}
			placement.Edge = thermal.edge
			placement.Priority = 110
		}
		placements = append(placements, placement)
	}
	intent := circuitgraph.PCBIntent{
		Regions: regions, Placements: placements,
		Keepouts: []circuitgraph.PCBKeepout{}, Zones: []circuitgraph.PCBZone{},
	}
	if board.Layers != 4 {
		return intent
	}
	if reference := physicalPlaneNet(nets, circuitgraph.NetRoleGround, circuitgraph.NetRoleReturn); reference != "" {
		intent.Zones = append(intent.Zones, circuitgraph.PCBZone{Net: reference, Layers: []string{"In1.Cu"}, ClearanceMM: .2})
	}
	if power := physicalPrimaryPowerPlaneNet(nets); power != "" {
		intent.Zones = append(intent.Zones, circuitgraph.PCBZone{Net: power, Layers: []string{"In2.Cu"}, ClearanceMM: .2})
	}
	return intent
}

func physicalCapacityAwareThermalEdge(
	preferred circuitgraph.Side,
	board circuitgraph.Board,
	spanMM float64,
	loads map[circuitgraph.Side]float64,
) circuitgraph.Side {
	candidates := []circuitgraph.Side{preferred}
	switch preferred {
	case circuitgraph.SideLeft, circuitgraph.SideRight:
		candidates = append(candidates, circuitgraph.SideTop, circuitgraph.SideBottom)
	case circuitgraph.SideTop:
		candidates = append(candidates, circuitgraph.SideBottom)
	case circuitgraph.SideBottom:
		candidates = append(candidates, circuitgraph.SideTop)
	}
	selected := preferred
	selectedLoad := math.Inf(1)
	for _, edge := range candidates {
		capacity := board.WidthMM
		if edge == circuitgraph.SideLeft || edge == circuitgraph.SideRight {
			capacity = board.HeightMM
		}
		if capacity <= 0 {
			continue
		}
		normalizedLoad := loads[edge] / capacity
		if normalizedLoad < selectedLoad-1e-12 {
			selected, selectedLoad = edge, normalizedLoad
		}
	}
	loads[selected] += math.Max(spanMM, physicalMinimumPackageDimensionMM)
	return selected
}

// physicalPrimaryPowerPlaneNet selects the power domain that benefits most
// from a solid distribution plane when a four-layer design contains multiple
// rails. Endpoint fanout is the primary distribution signal, declared current
// is the secondary electrical signal, and the normalized name is only a stable
// final tie-break. This keeps short point-to-point high-current paths as wide
// outer-layer routes while giving shared rails the continuous plane. Other
// power domains remain routable on the outer layers.
func physicalPrimaryPowerPlaneNet(nets []circuitgraph.Net) string {
	candidates := make([]circuitgraph.Net, 0)
	for _, net := range nets {
		switch net.Role {
		case circuitgraph.NetRolePower, circuitgraph.NetRolePowerPos, circuitgraph.NetRolePowerNeg:
			if strings.TrimSpace(net.Name) != "" {
				candidates = append(candidates, net)
			}
		}
	}
	slices.SortStableFunc(candidates, func(left, right circuitgraph.Net) int {
		if order := cmp.Compare(len(right.Endpoints), len(left.Endpoints)); order != 0 {
			return order
		}
		if order := cmp.Compare(right.CurrentMA, left.CurrentMA); order != 0 {
			return order
		}
		return strings.Compare(left.Name, right.Name)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].Name
}

func physicalFunctionalRegions(
	board circuitgraph.Board,
	componentList []circuitgraph.Component,
	schematic circuitgraph.SchematicIntent,
	catalog *components.Catalog,
) ([]circuitgraph.PCBRegion, map[string]string) {
	if len(componentList) == 0 {
		region := circuitgraph.PCBRegion{
			ID: "synthesized_circuit", Role: "signal",
			Bounds: circuitgraph.Bounds{WidthMM: board.WidthMM, HeightMM: board.HeightMM},
		}
		return []circuitgraph.PCBRegion{region}, map[string]string{}
	}
	groupRanks := make(map[string]int, len(schematic.Groups))
	groupRoles := make(map[string]string, len(schematic.Groups))
	groupByComponent := map[string]string{}
	for _, group := range schematic.Groups {
		groupRanks[group.ID] = group.Rank
		groupRoles[group.ID] = group.Role
		for _, component := range group.Members {
			if _, exists := groupByComponent[component]; !exists {
				groupByComponent[component] = group.ID
			}
		}
	}
	for _, placement := range schematic.Placements {
		if placement.Component != "" && placement.Group != "" {
			groupByComponent[placement.Component] = placement.Group
		}
	}
	seeds := map[string]*physicalRegionSeed{}
	regionByComponent := make(map[string]string, len(componentList))
	for _, component := range componentList {
		groupID, grouped := groupByComponent[component.ID]
		groupRank, groupExists := groupRanks[groupID]
		if !grouped || !groupExists {
			// Ungrouped components are valid and use intrinsic role/usage below.
			groupID = ""
			groupRank = 0
		}
		role := physicalFunctionalRole(component, groupID, groupRoles[groupID], groupRank)
		seed := seeds[role]
		if seed == nil {
			seed = &physicalRegionSeed{role: role}
			seeds[role] = seed
		}
		seed.components = append(seed.components, component)
		seed.rankTotal += groupRank
		seed.rankCount++
		regionByComponent[component.ID] = "functional_" + role
	}
	ordered := make([]*physicalRegionSeed, 0, len(seeds))
	for _, seed := range seeds {
		slices.SortStableFunc(seed.components, func(left, right circuitgraph.Component) int {
			return strings.Compare(left.ID, right.ID)
		})
		seed.weight = physicalRegionWeight(seed.components, catalog)
		ordered = append(ordered, seed)
	}
	slices.SortStableFunc(ordered, physicalCompareRegionSeeds)

	totalWeight := 0.0
	for _, seed := range ordered {
		totalWeight += seed.weight
	}
	if totalWeight <= 0 || math.IsNaN(totalWeight) || math.IsInf(totalWeight, 0) {
		// Invalid catalog geometry degrades deterministically to equal regions.
		totalWeight = float64(len(ordered)) * physicalRegionMinimumWeight
		for _, seed := range ordered {
			seed.weight = physicalRegionMinimumWeight
		}
	}
	bounds := physicalFunctionalRegionBounds(board, ordered)
	regions := make([]circuitgraph.PCBRegion, 0, len(ordered))
	for index, seed := range ordered {
		region := circuitgraph.PCBRegion{
			ID: "functional_" + seed.role, Role: seed.role,
			Bounds: bounds[index],
		}
		regions = append(regions, region)
	}
	return regions, regionByComponent
}

// physicalFunctionalRegionBounds preserves strict left-to-right strips while
// their package-derived minimum widths fit. When those minima exceed the board
// width, shrinking every strip makes wide footprints impossible to place. In
// that dense case, retain each minimum width and allow adjacent preferred
// regions to overlap; normal placement collision checks still keep components
// disjoint, while the ordered region centers preserve functional flow.
func physicalFunctionalRegionBounds(
	board circuitgraph.Board,
	ordered []*physicalRegionSeed,
) []circuitgraph.Bounds {
	if len(ordered) == 0 {
		return nil
	}
	gap := math.Min(1, math.Max(0, board.WidthMM*.01))
	if float64(len(ordered)-1)*gap >= board.WidthMM {
		gap = 0
	}
	availableWidth := board.WidthMM - float64(len(ordered)-1)*gap
	totalWeight := 0.0
	for _, seed := range ordered {
		totalWeight += seed.weight
	}
	result := make([]circuitgraph.Bounds, 0, len(ordered))
	if totalWeight <= availableWidth+1e-12 {
		x := 0.0
		for index, seed := range ordered {
			width := availableWidth * seed.weight / totalWeight
			if index == len(ordered)-1 {
				width = math.Max(0, board.WidthMM-x)
			}
			result = append(result, circuitgraph.Bounds{XMM: x, YMM: 0, WidthMM: width, HeightMM: board.HeightMM})
			x += width + gap
		}
		return result
	}
	cumulativeWeight := 0.0
	for _, seed := range ordered {
		width := math.Min(board.WidthMM, math.Max(physicalRegionMinimumWeight, seed.weight))
		center := board.WidthMM * (cumulativeWeight + seed.weight/2) / totalWeight
		x := math.Max(0, math.Min(board.WidthMM-width, center-width/2))
		result = append(result, circuitgraph.Bounds{XMM: x, YMM: 0, WidthMM: width, HeightMM: board.HeightMM})
		cumulativeWeight += seed.weight
	}
	return result
}

func physicalFunctionalRole(component circuitgraph.Component, groupID, groupRole string, groupRank int) string {
	if component.Role == circuitgraph.RoleInputConnector || strings.EqualFold(groupID, "external_inputs") || strings.EqualFold(groupRole, "input_boundary") {
		return physicalRegionInput
	}
	if component.Role == circuitgraph.RoleOutputConnector || strings.EqualFold(groupID, "external_outputs") || strings.EqualFold(groupRole, "output_boundary") {
		return physicalRegionOutput
	}
	if component.Role == circuitgraph.RoleProtection || component.Role == circuitgraph.RoleFuse ||
		component.Role == circuitgraph.RoleTVS || component.Role == circuitgraph.RoleDiode {
		return physicalProtectionRegion(groupRank)
	}
	if component.Role == circuitgraph.RoleSensor {
		return physicalRegionSensing
	}
	tokens := physicalSemanticTokens(component.Usage + " " + groupRole)
	if physicalTokensContainAny(tokens, "sense", "sensing", "sensor", "measure", "measurement", "monitor", "shunt", "feedback") ||
		(tokens["current"] && tokens["clamp"]) {
		return physicalRegionSensing
	}
	if physicalTokensContainAny(tokens, "protect", "protection", "clamp", "fuse", "tvs", "flyback", "snubber", "crowbar") {
		return physicalProtectionRegion(groupRank)
	}
	if physicalTokensContainAny(tokens, "power", "driver", "switch", "switching", "regulator", "regulation", "regulated") {
		return physicalRegionPower
	}
	switch component.Role {
	case circuitgraph.RoleRegulator, circuitgraph.RoleMOSFET, circuitgraph.RoleBJT,
		circuitgraph.RoleTransistor, circuitgraph.RoleInductor, circuitgraph.RoleBulkCapacitor:
		return physicalRegionPower
	default:
		return physicalRegionControl
	}
}

func physicalSemanticTokens(value string) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if token != "" {
			tokens[token] = true
		}
	}
	return tokens
}

func physicalTokensContainAny(tokens map[string]bool, candidates ...string) bool {
	for _, candidate := range candidates {
		if tokens[candidate] {
			return true
		}
	}
	return false
}

func physicalProtectionRegion(groupRank int) string {
	if groupRank <= 1 {
		return physicalRegionInputProtection
	}
	if groupRank >= 3 {
		return physicalRegionOutputProtection
	}
	return physicalRegionProtection
}

func physicalCompareRegionSeeds(left, right *physicalRegionSeed) int {
	leftBoundary, rightBoundary := physicalFunctionalRoleOrder(left.role), physicalFunctionalRoleOrder(right.role)
	if leftBoundary < 0 || rightBoundary < 0 || leftBoundary >= 100 || rightBoundary >= 100 {
		return cmp.Or(cmp.Compare(leftBoundary, rightBoundary), strings.Compare(left.role, right.role))
	}
	leftRank, rightRank := float64(left.rankTotal)/float64(left.rankCount), float64(right.rankTotal)/float64(right.rankCount)
	return cmp.Or(cmp.Compare(leftRank, rightRank), cmp.Compare(leftBoundary, rightBoundary), strings.Compare(left.role, right.role))
}

func physicalFunctionalRoleOrder(role string) int {
	switch role {
	case physicalRegionInput:
		return -100
	case physicalRegionInputProtection:
		return -50
	case physicalRegionSensing:
		return 10
	case physicalRegionControl:
		return 20
	case physicalRegionPower:
		return 30
	case physicalRegionProtection:
		return 40
	case physicalRegionOutputProtection:
		return 50
	case physicalRegionOutput:
		return 100
	default:
		return 25
	}
}

func physicalRegionWeight(componentList []circuitgraph.Component, catalog *components.Catalog) float64 {
	area, widest := 0.0, 0.0
	for _, component := range componentList {
		width, height := physicalPackageDimensions(component, catalog)
		area += math.Max(1, width*height)
		widest = math.Max(widest, width)
	}
	// Package dimensions describe the component body, while placement uses the
	// resolved footprint courtyard (including leads, pads, and assembly space).
	// Reserve the package allowance on both sides of the widest body so a
	// synthesized functional strip cannot be narrower than its only part's
	// legal placement envelope.
	packageAllowance := 2 * physicalRegionPackagePaddingMM
	return math.Max(
		physicalRegionMinimumWeight,
		math.Max(
			widest+packageAllowance,
			math.Sqrt(math.Max(0, area))+packageAllowance+float64(len(componentList)-1)*physicalRegionAdditionalComponentWeight,
		),
	)
}

func physicalPackageDimensions(component circuitgraph.Component, catalog *components.Catalog) (float64, float64) {
	_, variant, ok := physicalCatalogVariant(component, catalog)
	if !ok || variant.DimensionsMM == nil {
		return physicalDefaultPackageWidthMM, physicalDefaultPackageHeightMM
	}
	width, height := variant.DimensionsMM.Width, variant.DimensionsMM.Height
	if width <= 0 || math.IsNaN(width) || math.IsInf(width, 0) {
		width = physicalDefaultPackageWidthMM
	}
	if height <= 0 || math.IsNaN(height) || math.IsInf(height, 0) {
		height = physicalDefaultPackageHeightMM
	}
	return math.Max(physicalMinimumPackageDimensionMM, width), math.Max(physicalMinimumPackageDimensionMM, height)
}

func physicalCatalogVariant(component circuitgraph.Component, catalog *components.Catalog) (components.ComponentRecord, components.PackageVariant, bool) {
	record, ok := components.LookupRecord(catalog, component.ComponentID)
	if !ok {
		return components.ComponentRecord{}, components.PackageVariant{}, false
	}
	for _, variant := range record.Packages {
		if variant.ID == component.VariantID {
			return record, variant, true
		}
	}
	return components.ComponentRecord{}, components.PackageVariant{}, false
}

type physicalThermalPlacementDecision struct {
	role              string
	packageType       string
	pathID            string
	pathCPerW         float64
	edge              circuitgraph.Side
	clearanceMM       float64
	boardEdgeRequired bool
	rationale         string
}

func physicalThermalPlacement(
	component circuitgraph.Component,
	region circuitgraph.PCBRegion,
	board circuitgraph.Board,
	catalog *components.Catalog,
) (physicalThermalPlacementDecision, bool) {
	record, variant, ok := physicalCatalogVariant(component, catalog)
	if !ok || strings.TrimSpace(variant.PackageType) == "" {
		return physicalThermalPlacementDecision{}, false
	}
	width, height := physicalPackageDimensions(component, catalog)
	clearance := math.Max(physicalThermalMinimumClearanceMM, math.Hypot(width, height)/2+physicalThermalMinimumClearanceMM)
	edge := physicalRegionBoardEdge(region, board, component.ID)
	if !physicalRequiresExternalThermalPath(record) {
		pathID, pathCPerW, found := physicalIntrinsicAmbientThermalPath(record)
		if !found {
			return physicalThermalPlacementDecision{}, false
		}
		return physicalThermalPlacementDecision{
			role: physicalThermalRole(component), packageType: variant.PackageType,
			pathID: pathID, pathCPerW: pathCPerW, edge: edge, clearanceMM: clearance,
			rationale: "reviewed package junction-to-ambient evidence selects heat-source separation with thermal-copper preference",
		}, true
	}
	selectedPath := components.ThermalPathRecord{}
	pathFound := false
	for _, path := range catalog.ThermalPaths {
		if path.ReviewStatus != "reviewed" || !strings.EqualFold(path.Lifecycle, "active") ||
			!acceptedConfidence(path.Verification.Confidence) || path.MaximumSharedDevices < 1 ||
			path.CaseToSinkCPerW < 0 || path.NaturalSinkToAmbientCPerW <= 0 ||
			!physicalPackageCompatible(path.CompatiblePackages, variant.PackageType) {
			continue
		}
		pathResistance := path.CaseToSinkCPerW + path.NaturalSinkToAmbientCPerW
		selectedResistance := selectedPath.CaseToSinkCPerW + selectedPath.NaturalSinkToAmbientCPerW
		if !pathFound || cmp.Or(cmp.Compare(pathResistance, selectedResistance), strings.Compare(path.ID, selectedPath.ID)) < 0 {
			selectedPath = path
			pathFound = true
		}
	}
	if !pathFound {
		return physicalThermalPlacementDecision{}, false
	}
	return physicalThermalPlacementDecision{
		role: physicalThermalRole(component), packageType: variant.PackageType,
		pathID: selectedPath.ID, pathCPerW: selectedPath.CaseToSinkCPerW + selectedPath.NaturalSinkToAmbientCPerW,
		edge: edge, clearanceMM: clearance, boardEdgeRequired: true,
		rationale: "reviewed junction-to-case package compatibility selects board-edge heatsink access with sensor keep-away and thermal-copper preference",
	}, true
}

func physicalIntrinsicAmbientThermalPath(record components.ComponentRecord) (string, float64, bool) {
	type candidate struct {
		id        string
		pathCPerW *float64
	}
	candidates := []candidate{
		{id: "power_semiconductor_junction_to_ambient", pathCPerW: nil},
		{id: "component_junction_to_ambient", pathCPerW: nil},
		{id: "op_amp_junction_to_ambient", pathCPerW: nil},
	}
	if record.PowerSemiconductor != nil {
		candidates[0].pathCPerW = record.PowerSemiconductor.JunctionToAmbientCPerW
	}
	if record.Thermal != nil {
		candidates[1].pathCPerW = record.Thermal.JunctionToAmbientCPerW
	}
	if record.OpAmp != nil {
		candidates[2].pathCPerW = record.OpAmp.JunctionToAmbientCPerW
	}
	selected := candidate{}
	for _, item := range candidates {
		if !physicalPositiveFloat(item.pathCPerW) ||
			(selected.pathCPerW != nil && *item.pathCPerW <= *selected.pathCPerW) {
			continue
		}
		selected = item
	}
	if selected.pathCPerW == nil {
		return "", 0, false
	}
	return record.ID + "." + selected.id, *selected.pathCPerW, true
}

func physicalRequiresExternalThermalPath(record components.ComponentRecord) bool {
	for _, model := range record.SimulationModels {
		// A junction-to-case RC model declares an applied case boundary; ambient
		// headline data cannot close that model without a case-to-ambient path.
		if model.ThermalModel != nil && strings.EqualFold(strings.TrimSpace(model.ThermalModel.Reference), "junction_to_case") {
			return true
		}
	}
	if physicalHasThermalEdgeHint(record.PlacementHints) {
		return (record.PowerSemiconductor != nil && physicalPositiveFloat(record.PowerSemiconductor.JunctionToCaseCPerW)) ||
			(record.Thermal != nil && physicalPositiveFloat(record.Thermal.JunctionToCaseCPerW))
	}
	// A junction-to-case value establishes heatsink capability, not an applied
	// requirement. Without an explicit edge hint, require external closure only
	// when the catalog provides no complete junction-to-ambient path.
	if record.PowerSemiconductor != nil {
		return physicalPositiveFloat(record.PowerSemiconductor.JunctionToCaseCPerW) &&
			!physicalPositiveFloat(record.PowerSemiconductor.JunctionToAmbientCPerW)
	}
	return record.Thermal != nil && physicalPositiveFloat(record.Thermal.JunctionToCaseCPerW) &&
		!physicalPositiveFloat(record.Thermal.JunctionToAmbientCPerW)
}

func physicalHasThermalEdgeHint(hints []components.PlacementHint) bool {
	for _, hint := range hints {
		if strings.EqualFold(strings.TrimSpace(hint.Kind), "thermal_edge") {
			return true
		}
	}
	return false
}

func physicalPositiveFloat(value *float64) bool {
	return value != nil && *value > 0
}

func physicalPackageCompatible(packages []string, packageType string) bool {
	for _, candidate := range packages {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(packageType)) {
			return true
		}
	}
	return false
}

func physicalThermalRole(component circuitgraph.Component) string {
	switch component.Role {
	case circuitgraph.RoleRegulator:
		return "regulator"
	case circuitgraph.RoleMOSFET, circuitgraph.RoleBJT, circuitgraph.RoleTransistor:
		return "power_switch"
	default:
		return "heat_source"
	}
}

func physicalRegionBoardEdge(region circuitgraph.PCBRegion, board circuitgraph.Board, componentID string) circuitgraph.Side {
	const epsilon = 1e-9
	if region.Role == physicalRegionInput || math.Abs(region.Bounds.XMM) <= epsilon {
		return circuitgraph.SideLeft
	}
	if region.Role == physicalRegionOutput || math.Abs(region.Bounds.XMM+region.Bounds.WidthMM-board.WidthMM) <= epsilon {
		return circuitgraph.SideRight
	}
	// Functional regions span the board height, so top and bottom are tied before
	// placement. Spread central heat sources deterministically across both edges.
	if identity := sha256.Sum256([]byte(componentID)); identity[0]&1 == 0 {
		return circuitgraph.SideBottom
	}
	return circuitgraph.SideTop
}

func physicalPlacementEvidence(document circuitgraph.Document, catalog *components.Catalog) ([]PhysicalPlacementEvidence, []reports.Issue) {
	var issues []reports.Issue
	membersByRegion := map[string][]string{}
	placementByComponent := make(map[string]circuitgraph.PCBPlacement, len(document.PCB.Placements))
	for _, placement := range document.PCB.Placements {
		membersByRegion[placement.Region] = append(membersByRegion[placement.Region], placement.Component)
		placementByComponent[placement.Component] = placement
	}
	evidence := make([]PhysicalPlacementEvidence, 0, len(document.PCB.Regions)+len(document.PCB.Placements))
	for _, region := range document.PCB.Regions {
		bounds := region.Bounds
		members := append([]string(nil), membersByRegion[region.ID]...)
		slices.Sort(members)
		evidence = append(evidence, finalizePhysicalPlacementEvidence(PhysicalPlacementEvidence{
			Kind: "functional_region", Region: region.ID, Role: region.Role, Bounds: &bounds, Members: members,
			Rationale: "normalized component roles and schematic-group ranks define this left-to-right functional region",
		}))
	}
	orderedComponents := append([]circuitgraph.Component(nil), document.Components...)
	slices.SortStableFunc(orderedComponents, func(left, right circuitgraph.Component) int {
		return strings.Compare(left.ID, right.ID)
	})
	regionsByID := make(map[string]circuitgraph.PCBRegion, len(document.PCB.Regions))
	for _, region := range document.PCB.Regions {
		regionsByID[region.ID] = region
	}
	for _, component := range orderedComponents {
		placement, ok := placementByComponent[component.ID]
		if !ok {
			continue
		}
		thermal, ok := physicalThermalPlacement(component, regionsByID[placement.Region], document.Project.Board, catalog)
		if !ok {
			if record, _, found := physicalCatalogVariant(component, catalog); found && physicalRequiresExternalThermalPath(record) {
				issues = append(issues, physicalLoweringError(
					"physical.placement."+component.ID+".thermal_path",
					"junction-to-case package has no reviewed compatible thermal path for deterministic board-edge placement",
				))
			}
			continue
		}
		if placement.Edge != "" {
			thermal.edge = placement.Edge
		}
		evidence = append(evidence, finalizePhysicalPlacementEvidence(PhysicalPlacementEvidence{
			Kind: "thermal_placement", Component: component.ID, Region: placement.Region, Role: thermal.role,
			Edge: thermal.edge, CatalogID: component.ComponentID, VariantID: component.VariantID,
			PackageType: thermal.packageType, ThermalPathID: thermal.pathID,
			ThermalPathCPerW:   thermal.pathCPerW,
			KeepAwayRole:       physicalThermalSensitiveRole,
			MinimumClearanceMM: thermal.clearanceMM, BoardEdgeRequired: thermal.boardEdgeRequired, PreferThermalCopper: true,
			Rationale: thermal.rationale,
		}))
	}
	return evidence, reports.SortedIssues(issues)
}

func finalizePhysicalPlacementEvidence(entry PhysicalPlacementEvidence) PhysicalPlacementEvidence {
	copy := entry
	copy.EvidenceSHA = ""
	entry.EvidenceSHA = hashJSON(copy)
	return entry
}

func physicalPlacementBindings(evidence []PhysicalPlacementEvidence) []PhysicalSemanticBinding {
	bindings := make([]PhysicalSemanticBinding, 0, len(evidence))
	for _, entry := range evidence {
		binding := PhysicalSemanticBinding{
			Kind: entry.Kind, SemanticID: entry.Role, GraphNode: entry.Region,
			Component: entry.Component, CatalogID: entry.CatalogID, VariantID: entry.VariantID,
			EvidenceSHA: entry.EvidenceSHA,
		}
		if entry.Kind == "thermal_placement" {
			binding.SemanticID = entry.ThermalPathID
		}
		bindings = append(bindings, binding)
	}
	return bindings
}

func applyPhysicalPlacementEvidence(request *designworkflow.Request, evidence []PhysicalPlacementEvidence) {
	if request == nil || request.ExplicitCircuit == nil {
		return
	}
	thermalByComponent := map[string]PhysicalPlacementEvidence{}
	for _, entry := range evidence {
		if entry.Kind == "thermal_placement" {
			thermalByComponent[entry.Component] = entry
		}
	}
	for index := range request.ExplicitCircuit.Components {
		component := &request.ExplicitCircuit.Components[index]
		thermal, ok := thermalByComponent[component.ID]
		if !ok {
			continue
		}
		component.Placement.ThermalRole = thermal.Role
		component.Placement.ThermalPathID = thermal.ThermalPathID
		component.Placement.ThermalPackage = thermal.PackageType
		component.Placement.ThermalPathCPerW = thermal.ThermalPathCPerW
		component.Placement.ThermalClearanceMM = thermal.MinimumClearanceMM
		component.Placement.ThermalKeepAwayRole = thermal.KeepAwayRole
		component.Placement.ThermalEdgeRequired = thermal.BoardEdgeRequired
		component.Placement.PreferThermalCopper = thermal.PreferThermalCopper
	}
}
