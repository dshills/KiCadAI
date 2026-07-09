package schematicir

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

// ToTransaction converts a validated schematic IR document into the existing
// schematic transaction operation stream.
func ToTransaction(document Document) (transactions.Transaction, []reports.Issue) {
	document = NormalizeLayoutIntent(document)
	if issues := validateDefaulted(document); len(issues) != 0 {
		return transactions.Transaction{}, issues
	}

	state, issues := newAdapterState(document)
	if len(issues) != 0 {
		return transactions.Transaction{}, issues
	}

	tx := transactions.Transaction{
		Name:    document.Metadata.Name,
		Project: document.Metadata.Name,
	}
	state.appendCreateProject(&tx)
	state.appendComponents(&tx)
	state.appendNets(&tx)

	return tx, state.issues
}

type adapterState struct {
	document   Document
	refsByID   map[string]string
	unitsByID  map[string]int
	pointsByID map[string]transactions.Point
	issues     []reports.Issue
}

const (
	defaultLayoutStartXMM       = 25.0
	defaultLayoutSignalYMM      = 55.0
	defaultLayoutPowerYMM       = 25.0
	defaultLayoutGroundYMM      = 95.0
	defaultAdapterGroupSpacing  = DefaultMinGroupSpacingMM
	defaultAdapterSymbolSpacing = DefaultMinComponentSpacingMM
	defaultLayoutFallbackRank   = 2
	defaultLayoutInputRank      = 0
	defaultLayoutPowerRank      = 1
	defaultLayoutProcessingRank = 3
	defaultLayoutOutputRank     = 4
)

type layoutLane string

const (
	layoutLaneSignal layoutLane = "signal"
	layoutLanePower  layoutLane = "power"
	layoutLaneGround layoutLane = "ground"
)

func newAdapterState(document Document) (*adapterState, []reports.Issue) {
	state := &adapterState{
		document:   document,
		refsByID:   map[string]string{},
		unitsByID:  map[string]int{},
		pointsByID: layoutPoints(document),
	}
	refCounters := map[string]int{}
	usedRefs := map[string]struct{}{}
	invalidComponentIDs := map[string]struct{}{}
	trimmedRefs := map[string]string{}
	for index, component := range document.Circuit.Components {
		ref := strings.TrimSpace(component.Ref)
		trimmedRefs[component.ID] = ref
		if ref != "" {
			if _, exists := usedRefs[ref]; exists {
				state.addIssue(fmt.Sprintf("circuit.components[%d].ref", index), "duplicate component reference "+ref)
				invalidComponentIDs[component.ID] = struct{}{}
				continue
			}
			usedRefs[ref] = struct{}{}
			seedRefCounter(refCounters, ref)
		}
	}
	for index, component := range document.Circuit.Components {
		if _, invalid := invalidComponentIDs[component.ID]; invalid {
			continue
		}
		ref := trimmedRefs[component.ID]
		unit, ok := transactionUnit(component.Unit)
		if !ok {
			state.addIssue(fmt.Sprintf("circuit.components[%d].unit", index), "component unit must be a non-negative integer")
		} else {
			state.unitsByID[component.ID] = unit
		}
		if ref == "" {
			if !document.Policy.Repair.AllowRefAssignment {
				state.addIssue(fmt.Sprintf("circuit.components[%d].ref", index), "component reference is required when ref assignment repair is disabled")
				continue
			}
			ref = state.nextRef(component.Role, refCounters, usedRefs)
		}
		usedRefs[ref] = struct{}{}
		state.refsByID[component.ID] = ref
	}
	return state, state.issues
}

func (state *adapterState) appendCreateProject(tx *transactions.Transaction) {
	payload := transactions.CreateProjectOperation{
		Op:    transactions.OpCreateProject,
		Name:  state.document.Metadata.Name,
		Paper: state.document.Metadata.Paper,
	}
	state.appendOperation(tx, transactions.OpCreateProject, payload, "", "")
}

func (state *adapterState) appendComponents(tx *transactions.Transaction) {
	for _, component := range state.document.Circuit.Components {
		ref := state.refsByID[component.ID]
		if ref == "" {
			continue
		}
		payload := transactions.AddSymbolOperation{
			Op:         transactions.OpAddSymbol,
			Ref:        ref,
			Unit:       state.unitsByID[component.ID],
			Role:       string(component.Role),
			Value:      component.Value,
			LibraryID:  component.Symbol,
			At:         state.pointsByID[component.ID],
			Pins:       transactionPins(component.Pins),
			Properties: transactionProperties(component.Properties),
		}
		state.appendOperation(tx, transactions.OpAddSymbol, payload, ref, "")
		if component.Footprint != "" {
			assign := transactions.AssignFootprintOperation{
				Op:          transactions.OpAssignFootprint,
				Ref:         ref,
				Role:        string(component.Role),
				FootprintID: component.Footprint,
			}
			state.appendOperation(tx, transactions.OpAssignFootprint, assign, ref, "")
		}
	}
}

func (state *adapterState) appendNets(tx *transactions.Transaction) {
	for netIndex, net := range state.document.Circuit.Nets {
		if net.Role == NetRoleNoConnect {
			for endpointIndex, endpoint := range net.Connect {
				mapped, ok := state.transactionEndpoint(endpoint, fmt.Sprintf("circuit.nets[%d].connect[%d]", netIndex, endpointIndex))
				if ok {
					payload := transactions.AddNoConnectOperation{Op: transactions.OpAddNoConnect, Endpoint: mapped}
					state.appendOperation(tx, transactions.OpAddNoConnect, payload, mapped.Ref, "")
				}
			}
			continue
		}
		mappedEndpoints, ok := state.transactionEndpoints(net.Connect, fmt.Sprintf("circuit.nets[%d].connect", netIndex))
		if !ok || len(mappedEndpoints) < 2 {
			continue
		}
		for endpointIndex := 1; endpointIndex < len(mappedEndpoints); endpointIndex++ {
			from := mappedEndpoints[endpointIndex-1]
			to := mappedEndpoints[endpointIndex]
			payload := transactions.ConnectOperation{
				Op:      transactions.OpConnect,
				From:    from,
				To:      to,
				NetName: net.Name,
			}
			state.appendOperation(tx, transactions.OpConnect, payload, "", net.Name)
		}
	}
}

func (state *adapterState) transactionEndpoint(endpoint EndpointRef, path string) (transactions.Endpoint, bool) {
	componentID, pin, ok := endpoint.Split()
	if !ok {
		state.addIssue(path, "endpoint must be component.pin")
		return transactions.Endpoint{}, false
	}
	ref := state.refsByID[componentID]
	if ref == "" {
		state.addIssue(path, "endpoint references component without transaction reference "+componentID)
		return transactions.Endpoint{}, false
	}
	return transactions.Endpoint{Ref: ref, Pin: pin}, true
}

func (state *adapterState) transactionEndpoints(endpoints []EndpointRef, path string) ([]transactions.Endpoint, bool) {
	mappedEndpoints := make([]transactions.Endpoint, 0, len(endpoints))
	ok := true
	for index, endpoint := range endpoints {
		mapped, mappedOK := state.transactionEndpoint(endpoint, fmt.Sprintf("%s[%d]", path, index))
		if !mappedOK {
			ok = false
			continue
		}
		mappedEndpoints = append(mappedEndpoints, mapped)
	}
	return mappedEndpoints, ok
}

func (state *adapterState) appendOperation(tx *transactions.Transaction, kind transactions.OperationKind, payload any, ref string, netName string) {
	raw, err := json.Marshal(payload)
	if err != nil {
		state.addIssue(string(kind), err.Error())
		return
	}
	tx.Operations = append(tx.Operations, transactions.NewOperationWithMetadata(kind, raw, ref, netName))
}

func (state *adapterState) nextRef(role ComponentRole, counters map[string]int, usedRefs map[string]struct{}) string {
	prefix := refPrefix(role)
	for {
		counters[prefix]++
		ref := prefix + strconv.Itoa(counters[prefix])
		if _, exists := usedRefs[ref]; !exists {
			return ref
		}
	}
}

func seedRefCounter(counters map[string]int, ref string) {
	prefix, suffix, ok := splitReferenceSuffix(ref)
	if !ok {
		return
	}
	if suffix > counters[prefix] {
		counters[prefix] = suffix
	}
}

func splitReferenceSuffix(ref string) (string, int, bool) {
	index := len(ref)
	for index > 0 && ref[index-1] >= '0' && ref[index-1] <= '9' {
		index--
	}
	if index == len(ref) {
		return "", 0, false
	}
	suffix, err := strconv.Atoi(ref[index:])
	if err != nil {
		return "", 0, false
	}
	return ref[:index], suffix, true
}

func (state *adapterState) addIssue(path string, message string) {
	state.issues = append(state.issues, reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  message,
	})
}

func refPrefix(role ComponentRole) string {
	switch role {
	case ComponentRoleResistor, ComponentRoleCurrentLimiter, ComponentRolePullup:
		return "R"
	case ComponentRoleCapacitor, ComponentRoleDecouplingCapacitor, ComponentRoleBulkCapacitor:
		return "C"
	case ComponentRoleInductor:
		return "L"
	case ComponentRoleDiode, ComponentRoleIndicatorLED, ComponentRoleTVS:
		return "D"
	case ComponentRoleIC, ComponentRoleSensor, ComponentRoleRegulator:
		return "U"
	case ComponentRoleInputConnector, ComponentRoleOutputConnector, ComponentRoleConnector:
		return "J"
	case ComponentRoleTransistor, ComponentRoleBJT, ComponentRoleMOSFET:
		return "Q"
	case ComponentRoleFuse:
		return "F"
	case ComponentRoleSwitch:
		return "SW"
	case ComponentRoleTestpoint:
		return "TP"
	case ComponentRolePowerSymbol, ComponentRoleGroundSymbol:
		return "#PWR"
	default:
		return "X"
	}
}

func transactionUnit(unit string) (int, bool) {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func layoutPoints(document Document) map[string]transactions.Point {
	points := map[string]transactions.Point{}
	componentOrder := make([]string, 0, len(document.Circuit.Components))
	componentRoles := map[string]ComponentRole{}
	for _, component := range document.Circuit.Components {
		componentOrder = append(componentOrder, component.ID)
		componentRoles[component.ID] = component.Role
	}

	groupByID := map[string]Group{}
	for _, group := range document.Layout.Groups {
		groupByID[group.ID] = group
	}
	placementGroupByTarget := map[string]string{}
	for _, placement := range document.Layout.Placements {
		if placement.Target != "" && placement.Group != "" {
			placementGroupByTarget[placement.Target] = placement.Group
		}
	}

	componentRanks := map[string]int{}
	for _, group := range document.Layout.Groups {
		for _, member := range group.Members {
			if _, known := componentRoles[member]; !known {
				continue
			}
			if _, exists := componentRanks[member]; exists {
				continue
			}
			componentRanks[member] = group.Rank
		}
	}
	for _, componentID := range componentOrder {
		if _, exists := componentRanks[componentID]; exists {
			continue
		}
		rank := inferredRank(componentRoles[componentID])
		if groupID := placementGroupByTarget[componentID]; groupID != "" {
			if group, exists := groupByID[groupID]; exists {
				rank = group.Rank
			}
		}
		componentRanks[componentID] = rank
	}
	laneTotals := map[int]map[layoutLane]int{}
	for componentID, rank := range componentRanks {
		incrementLaneCount(laneTotals, rank, laneForRole(componentRoles[componentID]))
	}

	rankLaneCounts := map[int]map[layoutLane]int{}
	for _, group := range document.Layout.Groups {
		for _, member := range group.Members {
			if _, known := componentRoles[member]; !known {
				continue
			}
			if _, exists := points[member]; exists {
				continue
			}
			points[member] = pointForRankLane(document, componentRanks[member], componentRoles[member], rankLaneCounts, laneTotals)
		}
	}
	for _, componentID := range componentOrder {
		if _, exists := points[componentID]; exists {
			continue
		}
		points[componentID] = pointForRankLane(document, componentRanks[componentID], componentRoles[componentID], rankLaneCounts, laneTotals)
	}
	return points
}

func pointForRankLane(document Document, rank int, role ComponentRole, counts map[int]map[layoutLane]int, totals map[int]map[layoutLane]int) transactions.Point {
	lane := laneForRole(role)
	if counts[rank] == nil {
		counts[rank] = map[layoutLane]int{}
	}
	slot := counts[rank][lane]
	counts[rank][lane]++
	return pointForRankLaneSlot(document, rank, lane, slot, totals[rank])
}

func pointForRankLaneSlot(document Document, rank int, lane layoutLane, slot int, totals map[layoutLane]int) transactions.Point {
	groupSpacing := defaultAdapterGroupSpacing
	if document.Layout.Rules.MinGroupSpacingMM != nil {
		groupSpacing = *document.Layout.Rules.MinGroupSpacingMM
	}
	componentSpacing := defaultAdapterSymbolSpacing
	if document.Layout.Rules.MinComponentSpacingMM != nil {
		componentSpacing = *document.Layout.Rules.MinComponentSpacingMM
	}
	laneGap := componentSpacing * 2
	signalY := maxFloat(defaultLayoutSignalYMM, defaultLayoutPowerYMM+float64(totals[layoutLanePower])*componentSpacing+laneGap)
	groundY := maxFloat(defaultLayoutGroundYMM, signalY+float64(totals[layoutLaneSignal])*componentSpacing+laneGap)
	y := signalY + float64(slot)*componentSpacing
	switch lane {
	case layoutLanePower:
		y = defaultLayoutPowerYMM + float64(slot)*componentSpacing
	case layoutLaneGround:
		y = groundY + float64(slot)*componentSpacing
	}
	return transactions.Point{
		XMM: defaultLayoutStartXMM + float64(rank)*groupSpacing,
		YMM: y,
	}
}

func laneForRole(role ComponentRole) layoutLane {
	switch role {
	case ComponentRolePowerSymbol:
		return layoutLanePower
	case ComponentRoleGroundSymbol:
		return layoutLaneGround
	default:
		return layoutLaneSignal
	}
}

func incrementLaneCount(counts map[int]map[layoutLane]int, rank int, lane layoutLane) {
	if counts[rank] == nil {
		counts[rank] = map[layoutLane]int{}
	}
	counts[rank][lane]++
}

func maxFloat(left float64, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func inferredRank(role ComponentRole) int {
	switch role {
	case ComponentRoleInputConnector:
		return defaultLayoutInputRank
	case ComponentRoleOutputConnector:
		return defaultLayoutOutputRank
	case ComponentRoleRegulator, ComponentRoleProtection, ComponentRoleFuse, ComponentRoleTVS:
		return defaultLayoutPowerRank
	case ComponentRoleSensor, ComponentRoleIC:
		return defaultLayoutProcessingRank
	default:
		return defaultLayoutFallbackRank
	}
}

func transactionPins(pins []Pin) []transactions.PinSpec {
	if len(pins) == 0 {
		return nil
	}
	out := make([]transactions.PinSpec, 0, len(pins))
	for _, pin := range pins {
		out = append(out, transactions.PinSpec{Number: pin.Number})
	}
	return out
}

func transactionProperties(properties map[string]string) []transactions.SymbolProperty {
	if len(properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]transactions.SymbolProperty, 0, len(keys))
	for _, key := range keys {
		out = append(out, transactions.SymbolProperty{Name: key, Value: properties[key]})
	}
	return out
}
