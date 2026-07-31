package schematicir

import (
	"sort"
	"strings"
)

type explicitPlacementIntent struct {
	orientation bool
	mirror      bool
}

func indexExplicitPlacementIntent(placements []Placement) map[string]explicitPlacementIntent {
	indexed := make(map[string]explicitPlacementIntent, len(placements))
	for _, placement := range placements {
		indexed[placement.Target] = explicitPlacementIntent{
			orientation: placement.Orientation != "",
			mirror:      placement.Mirror != "",
		}
	}
	return indexed
}

type topologyLayoutIndex struct {
	components      map[string]Component
	componentIDs    []string
	nets            []Net
	netsByComponent map[string][]int
	netByEndpoint   map[EndpointRef]int
	placementByID   map[string]int
	explicit        map[string]explicitPlacementIntent
	placements      []Placement
	claimed         map[string]struct{}
	dividerNets     map[int]struct{}
}

// inferTopologyPlacements derives conventional schematic relationships from
// terminal-level connectivity. It deliberately uses component/net semantics
// and graph structure only; references are deterministic tie-breakers, never
// fixture identities or circuit-family selectors.
func inferTopologyPlacements(circuit Circuit, placements []Placement, explicit map[string]explicitPlacementIntent) []Placement {
	index := newTopologyLayoutIndex(circuit, placements, explicit)
	// Strong, multi-component motifs claim their members before weaker local
	// patterns so a shunt or rail heuristic cannot steal an axis anchor.
	index.inferDifferentialPairs()
	index.inferCurrentMirrors()
	index.inferVoltageDividers()
	index.inferShuntBranches()
	index.inferRailLoads()
	index.inferTwoTerminalSignalChains()
	index.attachRailAnnotations()
	return index.normalizedPlacements()
}

func newTopologyLayoutIndex(circuit Circuit, placements []Placement, explicit map[string]explicitPlacementIntent) *topologyLayoutIndex {
	index := &topologyLayoutIndex{
		components:      make(map[string]Component, len(circuit.Components)),
		nets:            append([]Net(nil), circuit.Nets...),
		netsByComponent: map[string][]int{},
		netByEndpoint:   map[EndpointRef]int{},
		placementByID:   make(map[string]int, len(placements)),
		explicit:        explicit,
		placements:      append([]Placement(nil), placements...),
		claimed:         map[string]struct{}{},
		dividerNets:     map[int]struct{}{},
	}
	for _, component := range circuit.Components {
		index.components[component.ID] = component
		index.componentIDs = append(index.componentIDs, component.ID)
	}
	sort.Strings(index.componentIDs)
	for netIndex, net := range index.nets {
		seen := map[string]struct{}{}
		for _, endpoint := range net.Connect {
			index.netByEndpoint[endpoint] = netIndex
			componentID, _, ok := endpoint.Split()
			if !ok {
				continue
			}
			if _, duplicate := seen[componentID]; duplicate {
				continue
			}
			seen[componentID] = struct{}{}
			index.netsByComponent[componentID] = append(index.netsByComponent[componentID], netIndex)
		}
	}
	for placementIndex := range index.placements {
		index.placementByID[index.placements[placementIndex].Target] = placementIndex
	}
	return index
}

func (index *topologyLayoutIndex) normalizedPlacements() []Placement {
	for placementIndex := range index.placements {
		placement := &index.placements[placementIndex]
		sort.Strings(placement.Near)
		sort.Strings(placement.Above)
		sort.Strings(placement.RightOf)
		sort.Strings(placement.SameRowAs)
		sort.Strings(placement.SameColumnAs)
		sort.Strings(placement.CenterBetween)
		sort.SliceStable(placement.SameRowAsPin, func(i, j int) bool { return placement.SameRowAsPin[i] < placement.SameRowAsPin[j] })
		sort.SliceStable(placement.SameColumnAsPin, func(i, j int) bool { return placement.SameColumnAsPin[i] < placement.SameColumnAsPin[j] })
	}
	return index.placements
}

func (index *topologyLayoutIndex) inferRailLoads() {
	for _, componentID := range index.componentIDs {
		component := index.components[componentID]
		if !isTopologyPassive(component) || index.isClaimed(componentID) {
			continue
		}
		nets := index.netsByComponent[componentID]
		if len(nets) != 2 {
			continue
		}
		if index.isPowerNet(nets[0]) && index.isGroundNet(nets[1]) || index.isPowerNet(nets[1]) && index.isGroundNet(nets[0]) {
			index.setOrientation(componentID, OrientationNormal)
			index.claim(componentID)
		}
	}
}

func (index *topologyLayoutIndex) inferVoltageDividers() {
	for midpointIndex := range index.nets {
		if index.isPowerNet(midpointIndex) || index.isGroundNet(midpointIndex) {
			continue
		}
		resistors := index.componentsOnNet(midpointIndex, func(component Component) bool {
			return isTopologyResistor(component)
		})
		if len(resistors) != 2 {
			continue
		}
		if index.isClaimed(resistors[0]) || index.isClaimed(resistors[1]) {
			continue
		}
		firstOuter, firstOK := index.otherNet(resistors[0], midpointIndex)
		secondOuter, secondOK := index.otherNet(resistors[1], midpointIndex)
		if !firstOK || !secondOK {
			continue
		}
		upper, lower := "", ""
		switch {
		case index.isPowerNet(firstOuter) && index.isGroundNet(secondOuter):
			upper, lower = resistors[0], resistors[1]
		case index.isPowerNet(secondOuter) && index.isGroundNet(firstOuter):
			upper, lower = resistors[1], resistors[0]
		default:
			continue
		}
		index.setOrientation(upper, OrientationNormal)
		index.setOrientation(lower, OrientationNormal)
		index.addAbove(upper, lower)
		index.addSameColumn(upper, lower)
		index.placeRailTerminals(firstOuter, secondOuter, upper, lower)
		if output := index.boundaryOnNet(midpointIndex, map[string]struct{}{upper: {}, lower: {}}, true); output != "" {
			index.addRightOf(output, lower)
			if endpoint, ok := index.endpointOnNet(midpointIndex, upper); ok {
				index.addSameRowAsPin(output, endpoint)
			}
		}
		index.claim(upper, lower)
		index.dividerNets[midpointIndex] = struct{}{}
	}
}

func (index *topologyLayoutIndex) inferShuntBranches() {
	for nodeIndex := range index.nets {
		if _, divider := index.dividerNets[nodeIndex]; divider || index.isPowerNet(nodeIndex) || index.isGroundNet(nodeIndex) {
			continue
		}
		if index.netHasActiveFunctionalEndpoint(nodeIndex) {
			continue
		}
		passives := index.componentsOnNet(nodeIndex, isTopologyPassive)
		for _, shunt := range passives {
			if index.isClaimed(shunt) {
				continue
			}
			groundNet, ok := index.otherNet(shunt, nodeIndex)
			if !ok || !index.isGroundNet(groundNet) {
				continue
			}
			for _, series := range passives {
				if series == shunt || index.isClaimed(series) {
					continue
				}
				upstreamNet, seriesOK := index.otherNet(series, nodeIndex)
				if !seriesOK || index.isGroundNet(upstreamNet) || index.isPowerNet(upstreamNet) {
					continue
				}
				index.setOrientation(series, OrientationRotated)
				index.setOrientation(shunt, OrientationNormal)
				index.addAbove(series, shunt)
				if endpoint, endpointOK := index.endpointOnNet(nodeIndex, series); endpointOK {
					index.addSameColumnAsPin(shunt, endpoint)
				}
				if input := index.boundaryOnNet(upstreamNet, map[string]struct{}{series: {}}, false); input != "" {
					index.addRightOf(series, input)
					if endpoint, endpointOK := index.endpointOnNet(upstreamNet, series); endpointOK {
						index.addSameRowAsPin(input, endpoint)
					}
				}
				if output := index.boundaryOnNet(nodeIndex, map[string]struct{}{series: {}, shunt: {}}, true); output != "" {
					index.addRightOf(output, series)
					if endpoint, endpointOK := index.endpointOnNet(nodeIndex, series); endpointOK {
						index.addSameRowAsPin(output, endpoint)
					}
				}
				index.placeGroundBelow(groundNet, shunt)
				index.claim(series, shunt)
				break
			}
		}
	}
}

func (index *topologyLayoutIndex) netHasActiveFunctionalEndpoint(netIndex int) bool {
	for _, componentID := range index.componentsOnNet(netIndex, func(Component) bool { return true }) {
		component := index.components[componentID]
		if isTopologyPassive(component) || isBoundaryComponent(component) || component.Role == ComponentRolePowerSymbol || component.Role == ComponentRoleGroundSymbol {
			continue
		}
		return true
	}
	return false
}

func (index *topologyLayoutIndex) inferDifferentialPairs() {
	bjts := index.componentIDsMatching(isTopologyBJT)
	for leftIndex := 0; leftIndex < len(bjts); leftIndex++ {
		for rightIndex := leftIndex + 1; rightIndex < len(bjts); rightIndex++ {
			first, second := bjts[leftIndex], bjts[rightIndex]
			if index.isClaimed(first) || index.isClaimed(second) {
				continue
			}
			firstEmitter, firstEmitterOK := index.terminalNet(first, "emitter")
			secondEmitter, secondEmitterOK := index.terminalNet(second, "emitter")
			if !firstEmitterOK || !secondEmitterOK || firstEmitter != secondEmitter {
				continue
			}
			firstBase, firstBaseOK := index.terminalNet(first, "base")
			secondBase, secondBaseOK := index.terminalNet(second, "base")
			firstCollector, firstCollectorOK := index.terminalNet(first, "collector")
			secondCollector, secondCollectorOK := index.terminalNet(second, "collector")
			if !firstBaseOK || !secondBaseOK || !firstCollectorOK || !secondCollectorOK || firstBase == secondBase || firstCollector == secondCollector {
				continue
			}
			tail := index.resistorOnNet(firstEmitter, map[string]struct{}{first: {}, second: {}})
			if tail == "" {
				continue
			}
			tailReturn, tailReturnOK := index.otherNet(tail, firstEmitter)
			if !tailReturnOK || !(index.isGroundNet(tailReturn) || index.isNegativeNet(tailReturn)) {
				continue
			}
			firstLoad, firstSupply, firstLoadOK := index.railLoadOnNet(firstCollector, first)
			secondLoad, secondSupply, secondLoadOK := index.railLoadOnNet(secondCollector, second)
			if !firstLoadOK || !secondLoadOK || firstSupply != secondSupply {
				continue
			}
			index.placeSymmetricPair(first, second, firstLoad, secondLoad, tail)
			index.placeDifferentialBoundary(first, firstBase, false)
			index.placeDifferentialBoundary(second, secondBase, true)
			index.placeCollectorOutput(first, firstCollector, firstLoad)
			index.placeCollectorOutput(second, secondCollector, secondLoad)
			index.placeGroundBelow(tailReturn, tail)
			index.placePowerAbove(firstSupply, firstLoad, secondLoad)
			index.claim(first, second, firstLoad, secondLoad, tail)
		}
	}
}

func (index *topologyLayoutIndex) inferCurrentMirrors() {
	bjts := index.componentIDsMatching(isTopologyBJT)
	for firstIndex := 0; firstIndex < len(bjts); firstIndex++ {
		for secondIndex := firstIndex + 1; secondIndex < len(bjts); secondIndex++ {
			first, second := bjts[firstIndex], bjts[secondIndex]
			if index.isClaimed(first) || index.isClaimed(second) {
				continue
			}
			firstBase, firstBaseOK := index.terminalNet(first, "base")
			secondBase, secondBaseOK := index.terminalNet(second, "base")
			firstEmitter, firstEmitterOK := index.terminalNet(first, "emitter")
			secondEmitter, secondEmitterOK := index.terminalNet(second, "emitter")
			if !firstBaseOK || !secondBaseOK || !firstEmitterOK || !secondEmitterOK || firstBase != secondBase || firstEmitter != secondEmitter {
				continue
			}
			firstCollector, firstCollectorOK := index.terminalNet(first, "collector")
			secondCollector, secondCollectorOK := index.terminalNet(second, "collector")
			if !firstCollectorOK || !secondCollectorOK {
				continue
			}
			reference, output := first, second
			referenceCollector, outputCollector := firstCollector, secondCollector
			if secondCollector == firstBase && firstCollector != firstBase {
				reference, output = second, first
				referenceCollector, outputCollector = secondCollector, firstCollector
			}
			if referenceCollector != firstBase || outputCollector == firstBase {
				continue
			}
			referenceLoad, referenceSupply, referenceOK := index.railLoadOnNet(referenceCollector, reference)
			outputLoad, outputSupply, outputOK := index.railLoadOnNet(outputCollector, output)
			if !referenceOK || !outputOK || referenceSupply != outputSupply {
				continue
			}
			index.setOrientation(referenceLoad, OrientationNormal)
			index.setOrientation(outputLoad, OrientationNormal)
			index.addRightOf(output, reference)
			index.addSameRow(output, reference)
			index.addAbove(referenceLoad, reference)
			index.addSameColumn(referenceLoad, reference)
			index.addAbove(outputLoad, output)
			index.addSameColumn(outputLoad, output)
			index.addSameRow(outputLoad, referenceLoad)
			index.placeGroundBelow(firstEmitter, reference)
			index.placeCollectorOutput(output, outputCollector, outputLoad)
			index.claim(reference, output, referenceLoad, outputLoad)
		}
	}
}

func (index *topologyLayoutIndex) inferTwoTerminalSignalChains() {
	for _, componentID := range index.componentIDs {
		component := index.components[componentID]
		if !isTopologyPassive(component) || index.isClaimed(componentID) {
			continue
		}
		nets := index.netsByComponent[componentID]
		if len(nets) != 2 || index.isPowerNet(nets[0]) || index.isPowerNet(nets[1]) || index.isGroundNet(nets[0]) || index.isGroundNet(nets[1]) {
			continue
		}
		left := index.boundaryOnNet(nets[0], map[string]struct{}{componentID: {}}, false)
		right := index.boundaryOnNet(nets[1], map[string]struct{}{componentID: {}}, true)
		leftNet, rightNet := nets[0], nets[1]
		if left == "" || right == "" {
			left = index.boundaryOnNet(nets[1], map[string]struct{}{componentID: {}}, false)
			right = index.boundaryOnNet(nets[0], map[string]struct{}{componentID: {}}, true)
			leftNet, rightNet = nets[1], nets[0]
		}
		if left == "" || right == "" {
			continue
		}
		index.setOrientation(componentID, OrientationRotated)
		index.addRightOf(componentID, left)
		index.addRightOf(right, componentID)
		if endpoint, ok := index.endpointOnNet(leftNet, componentID); ok {
			index.addSameRowAsPin(left, endpoint)
		}
		if endpoint, ok := index.endpointOnNet(rightNet, componentID); ok {
			index.addSameRowAsPin(right, endpoint)
		}
	}
}

func (index *topologyLayoutIndex) placeSymmetricPair(left, right, leftLoad, rightLoad, tail string) {
	index.addRightOf(right, left)
	index.addSameRow(right, left)
	index.addAbove(left, tail)
	index.addAbove(right, tail)
	index.addCenterBetween(tail, left, right)
	index.addAbove(leftLoad, left)
	index.addSameColumn(leftLoad, left)
	index.addAbove(rightLoad, right)
	index.addSameColumn(rightLoad, right)
	index.addSameRow(rightLoad, leftLoad)
	index.setOrientation(leftLoad, OrientationNormal)
	index.setOrientation(rightLoad, OrientationNormal)
	index.setOrientation(tail, OrientationNormal)
	if !index.explicit[right].mirror {
		index.placement(right).Mirror = MirrorY
	}
}

func (index *topologyLayoutIndex) placeDifferentialBoundary(transistor string, netIndex int, onRight bool) {
	boundary := index.boundaryOnNet(netIndex, map[string]struct{}{transistor: {}}, false)
	if boundary == "" {
		return
	}
	if onRight {
		index.addRightOf(boundary, transistor)
	} else {
		index.addRightOf(transistor, boundary)
	}
	index.addSameRow(boundary, transistor)
}

func (index *topologyLayoutIndex) placeCollectorOutput(transistor string, collectorNet int, load string) {
	output := index.boundaryOnNet(collectorNet, map[string]struct{}{transistor: {}, load: {}}, true)
	if output == "" {
		return
	}
	index.addRightOf(output, transistor)
	if endpoint, ok := index.endpointOnNet(collectorNet, load); ok {
		index.addSameRowAsPin(output, endpoint)
	}
}

func (index *topologyLayoutIndex) placeRailTerminals(firstOuter, secondOuter int, upper, lower string) {
	powerNet, groundNet := firstOuter, secondOuter
	if index.isGroundNet(firstOuter) {
		powerNet, groundNet = secondOuter, firstOuter
	}
	index.placePowerAbove(powerNet, upper, lower)
	index.placeGroundBelow(groundNet, lower)
}

func (index *topologyLayoutIndex) placePowerAbove(netIndex int, left, right string) {
	power := index.railTerminalOnNet(netIndex, false)
	if power == "" {
		return
	}
	index.addAbove(power, left)
	if right != "" && right != left {
		index.addAbove(power, right)
		index.addCenterBetween(power, left, right)
	} else {
		index.addSameColumn(power, left)
	}
}

func (index *topologyLayoutIndex) placeGroundBelow(netIndex int, anchor string) {
	ground := index.railTerminalOnNet(netIndex, true)
	if ground == "" {
		return
	}
	index.addAbove(anchor, ground)
	index.addSameColumn(ground, anchor)
}

func (index *topologyLayoutIndex) attachRailAnnotations() {
	for netIndex := range index.nets {
		if !index.isPowerNet(netIndex) && !index.isGroundNet(netIndex) && !index.isNegativeNet(netIndex) {
			continue
		}
		primary := index.railTerminalOnNet(netIndex, index.isGroundNet(netIndex))
		if primary == "" {
			continue
		}
		for _, componentID := range index.componentsOnNet(netIndex, isPowerFlagComponent) {
			if componentID == primary {
				continue
			}
			index.addRightOf(componentID, primary)
			index.addSameRow(componentID, primary)
		}
	}
}

func (index *topologyLayoutIndex) railLoadOnNet(netIndex int, active string) (string, int, bool) {
	for _, componentID := range index.componentsOnNet(netIndex, isTopologyPassive) {
		if componentID == active {
			continue
		}
		outer, ok := index.otherNet(componentID, netIndex)
		if ok && index.isPowerNet(outer) {
			return componentID, outer, true
		}
	}
	return "", 0, false
}

func (index *topologyLayoutIndex) resistorOnNet(netIndex int, excluded map[string]struct{}) string {
	for _, componentID := range index.componentsOnNet(netIndex, isTopologyResistor) {
		if _, skip := excluded[componentID]; !skip {
			return componentID
		}
	}
	return ""
}

func (index *topologyLayoutIndex) terminalNet(componentID, terminal string) (int, bool) {
	component := index.components[componentID]
	for _, pin := range component.Pins {
		if topologyTerminalKind(component, pin) != terminal {
			continue
		}
		endpoint := EndpointRef(componentID + "." + pin.Number)
		if netIndex, ok := index.netByEndpoint[endpoint]; ok {
			return netIndex, true
		}
	}
	return 0, false
}

func topologyTerminalKind(component Component, pin Pin) string {
	name := strings.ToLower(strings.TrimSpace(pin.Name))
	switch name {
	case "b", "base":
		return "base"
	case "c", "collector":
		return "collector"
	case "e", "emitter":
		return "emitter"
	}
	if isTopologyBJT(component) {
		symbol := strings.ToLower(component.Symbol)
		for _, order := range []string{"cbe", "ceb", "bce", "bec", "ecb", "ebc"} {
			if !strings.HasSuffix(symbol, "_"+order) {
				continue
			}
			for index, number := range []string{"1", "2", "3"} {
				if pin.Number == number {
					return map[byte]string{'b': "base", 'c': "collector", 'e': "emitter"}[order[index]]
				}
			}
		}
	}
	return ""
}

func (index *topologyLayoutIndex) otherNet(componentID string, excluded int) (int, bool) {
	other := -1
	for _, netIndex := range index.netsByComponent[componentID] {
		if netIndex == excluded {
			continue
		}
		if other != -1 {
			return 0, false
		}
		other = netIndex
	}
	return other, other >= 0
}

func (index *topologyLayoutIndex) endpointOnNet(netIndex int, componentID string) (EndpointRef, bool) {
	for _, endpoint := range index.nets[netIndex].Connect {
		candidate, _, ok := endpoint.Split()
		if ok && candidate == componentID {
			return endpoint, true
		}
	}
	return "", false
}

func (index *topologyLayoutIndex) componentsOnNet(netIndex int, predicate func(Component) bool) []string {
	seen := map[string]struct{}{}
	var componentIDs []string
	for _, endpoint := range index.nets[netIndex].Connect {
		componentID, _, ok := endpoint.Split()
		if !ok {
			continue
		}
		component, exists := index.components[componentID]
		if !exists || !predicate(component) {
			continue
		}
		if _, duplicate := seen[componentID]; duplicate {
			continue
		}
		seen[componentID] = struct{}{}
		componentIDs = append(componentIDs, componentID)
	}
	sort.Strings(componentIDs)
	return componentIDs
}

func (index *topologyLayoutIndex) componentIDsMatching(predicate func(Component) bool) []string {
	var result []string
	for _, componentID := range index.componentIDs {
		if predicate(index.components[componentID]) {
			result = append(result, componentID)
		}
	}
	return result
}

func (index *topologyLayoutIndex) boundaryOnNet(netIndex int, excluded map[string]struct{}, preferOutput bool) string {
	type candidate struct {
		id   string
		rank int
	}
	var candidates []candidate
	for _, componentID := range index.componentsOnNet(netIndex, isBoundaryComponent) {
		if _, skip := excluded[componentID]; skip {
			continue
		}
		role := index.components[componentID].Role
		rank := 2
		if preferOutput && role == ComponentRoleOutputConnector || !preferOutput && role == ComponentRoleInputConnector {
			rank = 0
		} else if role == ComponentRoleTestpoint {
			rank = 1
		}
		candidates = append(candidates, candidate{id: componentID, rank: rank})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		return candidates[i].id < candidates[j].id
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].id
}

func (index *topologyLayoutIndex) railTerminalOnNet(netIndex int, ground bool) string {
	predicate := func(component Component) bool {
		if isPowerFlagComponent(component) {
			return false
		}
		if ground {
			return component.Role == ComponentRoleGroundSymbol
		}
		return component.Role == ComponentRolePowerSymbol
	}
	components := index.componentsOnNet(netIndex, predicate)
	if len(components) == 0 {
		return ""
	}
	return components[0]
}

func (index *topologyLayoutIndex) isPowerNet(netIndex int) bool {
	role := index.nets[netIndex].Role
	if role == NetRolePower || role == NetRolePowerPos {
		return true
	}
	name := strings.ToLower(index.nets[netIndex].Name)
	return strings.Contains(name, "vcc") || strings.Contains(name, "vdd") || strings.HasPrefix(name, "+")
}

func (index *topologyLayoutIndex) isNegativeNet(netIndex int) bool {
	role := index.nets[netIndex].Role
	return role == NetRolePowerNeg
}

func (index *topologyLayoutIndex) isGroundNet(netIndex int) bool {
	if index.nets[netIndex].Role == NetRoleGround {
		return true
	}
	name := strings.ToLower(index.nets[netIndex].Name)
	return name == "gnd" || strings.Contains(name, "ground") || strings.Contains(name, "return")
}

func (index *topologyLayoutIndex) placement(componentID string) *Placement {
	return &index.placements[index.placementByID[componentID]]
}

func (index *topologyLayoutIndex) setOrientation(componentID string, orientation Orientation) {
	if index.explicit[componentID].orientation {
		return
	}
	index.placement(componentID).Orientation = orientation
}

func (index *topologyLayoutIndex) addAbove(componentID, target string) {
	placement := index.placement(componentID)
	placement.Above = appendUniqueString(placement.Above, target)
}

func (index *topologyLayoutIndex) addRightOf(componentID, target string) {
	placement := index.placement(componentID)
	placement.RightOf = appendUniqueString(placement.RightOf, target)
}

func (index *topologyLayoutIndex) addSameRow(componentID, target string) {
	placement := index.placement(componentID)
	// The IR permits one alignment anchor per axis. Preserve the first
	// deterministic inference instead of manufacturing an invalid constraint.
	if len(placement.SameRowAs)+len(placement.SameRowAsPin) == 0 {
		placement.SameRowAs = []string{target}
	}
}

func (index *topologyLayoutIndex) addSameColumn(componentID, target string) {
	placement := index.placement(componentID)
	// As on rows, keep the first deterministic anchor for this axis.
	if len(placement.SameColumnAs)+len(placement.SameColumnAsPin)+len(placement.CenterBetween) == 0 {
		placement.SameColumnAs = []string{target}
	}
}

func (index *topologyLayoutIndex) addSameRowAsPin(componentID string, endpoint EndpointRef) {
	placement := index.placement(componentID)
	if len(placement.SameRowAs)+len(placement.SameRowAsPin) == 0 {
		placement.SameRowAsPin = []EndpointRef{endpoint}
	}
}

func (index *topologyLayoutIndex) addSameColumnAsPin(componentID string, endpoint EndpointRef) {
	placement := index.placement(componentID)
	if len(placement.SameColumnAs)+len(placement.SameColumnAsPin)+len(placement.CenterBetween) == 0 {
		placement.SameColumnAsPin = []EndpointRef{endpoint}
	}
}

func (index *topologyLayoutIndex) addCenterBetween(componentID, first, second string) {
	placement := index.placement(componentID)
	if len(placement.SameColumnAs)+len(placement.SameColumnAsPin)+len(placement.CenterBetween) != 0 {
		return
	}
	placement.CenterBetween = []string{first, second}
}

func (index *topologyLayoutIndex) claim(componentIDs ...string) {
	for _, componentID := range componentIDs {
		index.claimed[componentID] = struct{}{}
	}
}

func (index *topologyLayoutIndex) isClaimed(componentID string) bool {
	_, claimed := index.claimed[componentID]
	return claimed
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func isTopologyResistor(component Component) bool {
	return component.Role == ComponentRoleResistor || component.Role == ComponentRoleCurrentLimiter || component.Role == ComponentRolePullup
}

func isTopologyPassive(component Component) bool {
	return isTopologyResistor(component) || component.Role == ComponentRoleCapacitor || component.Role == ComponentRoleDecouplingCapacitor || component.Role == ComponentRoleBulkCapacitor || component.Role == ComponentRoleInductor
}

func isTopologyBJT(component Component) bool {
	return component.Role == ComponentRoleBJT || component.Role == ComponentRoleTransistor && strings.Contains(strings.ToLower(component.Symbol), "bjt")
}

func isBoundaryComponent(component Component) bool {
	switch component.Role {
	case ComponentRoleConnector, ComponentRoleInputConnector, ComponentRoleOutputConnector, ComponentRoleTestpoint:
		return true
	default:
		return false
	}
}

func isPowerFlagComponent(component Component) bool {
	return strings.EqualFold(strings.TrimSpace(component.Value), "PWR_FLAG") || strings.EqualFold(strings.TrimSpace(component.Symbol), "power:PWR_FLAG")
}
