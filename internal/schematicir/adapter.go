package schematicir

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

const footprintPropertyName = "Footprint"

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

// ToProjectTransaction converts schematic IR into a transaction that can be
// applied to write a KiCad project directory.
func ToProjectTransaction(document Document) (transactions.Transaction, []reports.Issue) {
	tx, issues := ToTransaction(document)
	if reports.HasBlockingIssue(issues) {
		return tx, issues
	}
	payload := transactions.WriteProjectOperation{Op: transactions.OpWriteProject, SchematicOnly: true}
	raw, err := json.Marshal(payload)
	if err != nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "write_project",
			Message:  err.Error(),
		})
		return tx, issues
	}
	tx.Operations = append(tx.Operations, transactions.NewOperation(transactions.OpWriteProject, raw))
	return tx, issues
}

type adapterState struct {
	document     Document
	refsByID     map[string]string
	unitsByID    map[string]int
	pointsByID   map[string]transactions.Point
	rotationByID map[string]float64
	issues       []reports.Issue
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
		document:     document,
		refsByID:     map[string]string{},
		unitsByID:    map[string]int{},
		pointsByID:   layoutPoints(document),
		rotationByID: layoutRotations(document),
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
			Rotation:   state.rotationByID[component.ID],
			Pins:       transactionPins(component),
			Properties: transactionSymbolProperties(component),
		}
		state.appendOperation(tx, transactions.OpAddSymbol, payload, ref, "")
		if component.Footprint != "" {
			// Retain explicit assignment evidence for transaction consumers even
			// though AddSymbol also carries the hidden KiCad Footprint property
			// needed by schematic-only project writes.
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
				Op:        transactions.OpConnect,
				From:      from,
				To:        to,
				NetName:   net.Name,
				UseLabels: schematicNetLabelPreference(state.document, net),
			}
			state.appendOperation(tx, transactions.OpConnect, payload, "", net.Name)
		}
	}
}

func schematicNetLabelPreference(document Document, net Net) *bool {
	if net.UseLabel != nil {
		value := *net.UseLabel
		return &value
	}
	if !document.Policy.Repair.AllowLabelInsertion {
		return nil
	}
	preferLong := document.Layout.Rules.PreferLabelsForLongNets == nil || *document.Layout.Rules.PreferLabelsForLongNets
	if !preferLong {
		return nil
	}
	switch net.Role {
	case NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround, NetRoleReturn, NetRoleShield:
		value := true
		return &value
	}
	if len(net.Connect) > 2 {
		value := true
		return &value
	}
	return nil
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
	result := schematicLayout(document)
	points := make(map[string]transactions.Point, len(result.Components))
	for _, component := range result.Components {
		points[component.Ref] = transactions.Point{
			XMM: float64(component.PlacedAt.X) / 1_000_000,
			YMM: float64(component.PlacedAt.Y) / 1_000_000,
		}
	}
	return points
}

func schematicLayout(document Document) schematiclayout.Result {
	rotationByID := layoutRotations(document)
	groupsByID := map[string]Group{}
	for _, group := range document.Layout.Groups {
		groupsByID[group.ID] = group
	}
	placementsByID := map[string]Placement{}
	for _, placement := range document.Layout.Placements {
		placementsByID[placement.Target] = placement
	}
	rules := schematiclayout.DefaultRules(schematiclayout.ProfileStandard)
	if document.Layout.Rules.MinComponentSpacingMM != nil {
		rules.MinComponentSpacing = kicadfiles.MM(*document.Layout.Rules.MinComponentSpacingMM)
	}
	if document.Layout.Rules.MinGroupSpacingMM != nil {
		rules.MinStageSpacing = kicadfiles.MM(*document.Layout.Rules.MinGroupSpacingMM)
		rules.MinGroupGutter = kicadfiles.MM(*document.Layout.Rules.MinGroupSpacingMM)
	}
	if document.Layout.Rules.PreferLabelsForLongNets != nil {
		rules.LabelFallbackEnabled = *document.Layout.Rules.PreferLabelsForLongNets && document.Policy.Repair.AllowLabelInsertion
	}
	widthMM, heightMM := paperDimensionsMM(document.Metadata.Paper)
	request := schematiclayout.Request{
		Sheet: schematiclayout.Sheet{
			Name:   document.Metadata.Name,
			Width:  kicadfiles.MM(widthMM),
			Height: kicadfiles.MM(heightMM),
			Margin: kicadfiles.MM(10.16),
		},
		Rules: rules,
	}
	for _, component := range document.Circuit.Components {
		placement := placementsByID[component.ID]
		group := groupsByID[placement.Group]
		if group.ID == "" {
			for _, candidate := range document.Layout.Groups {
				if stringSliceContains(candidate.Members, component.ID) {
					group = candidate
					break
				}
			}
		}
		request.Components = append(request.Components, schematiclayout.Component{
			Ref:       component.ID,
			Value:     component.Value,
			LibraryID: component.Symbol,
			Role:      string(component.Role),
			GroupID:   group.ID,
			Stage:     schematicStageForGroup(group.Role),
			FlowRank:  group.Rank,
			RankFixed: group.ID != "" && !group.Inferred,
			Near:      append([]string(nil), placement.Near...),
			Rotation:  kicadfiles.Angle(rotationByID[component.ID]),
			Pins:      schematicLayoutPins(component),
		})
	}
	for index, net := range document.Circuit.Nets {
		layoutNet := schematiclayout.Net{Name: net.Name, Role: string(net.Role), OriginalOrdinal: index}
		if net.UseLabel != nil {
			layoutNet.PreferredLabels = *net.UseLabel
		}
		for _, endpoint := range net.Connect {
			componentID, pin, ok := endpoint.Split()
			if ok {
				layoutNet.Endpoints = append(layoutNet.Endpoints, schematiclayout.Endpoint{Ref: componentID, Pin: pin})
			}
		}
		request.Nets = append(request.Nets, layoutNet)
	}
	for index, group := range document.Layout.Groups {
		request.Groups = append(request.Groups, schematiclayout.Group{
			ID:              group.ID,
			Role:            string(group.Role),
			Stage:           schematicStageForGroup(group.Role),
			OriginalOrdinal: index,
		})
	}
	return schematiclayout.Place(request)
}

func schematicLayoutPins(component Component) []schematiclayout.Pin {
	roles := map[string]string{}
	for _, pin := range component.Pins {
		roles[pin.Number] = string(pin.Role)
	}
	templatePins, _ := schematic.EmbeddedSymbolPinOffsets(component.Symbol)
	offsets := map[string]kicadfiles.Point{}
	for _, pin := range templatePins {
		offset := pin.Offset
		if connectionOffset, ok := schematic.EmbeddedSymbolConnectionPinOffset(component.Symbol, pin.Number); ok {
			offset = connectionOffset
		}
		offsets[pin.Number] = offset
	}
	if len(component.Pins) == 0 {
		pins := make([]schematiclayout.Pin, 0, len(templatePins))
		for _, pin := range templatePins {
			pins = append(pins, schematiclayout.Pin{Number: pin.Number, At: offsets[pin.Number]})
		}
		return pins
	}
	pins := make([]schematiclayout.Pin, 0, len(component.Pins))
	for _, pin := range component.Pins {
		pins = append(pins, schematiclayout.Pin{Number: pin.Number, Role: roles[pin.Number], At: offsets[pin.Number]})
	}
	return pins
}

func schematicStageForGroup(role GroupRole) schematiclayout.Stage {
	switch role {
	case GroupRoleInputStage, GroupRoleConnectorStage:
		return schematiclayout.StageBoundaryInput
	case GroupRoleProtectionStage:
		return schematiclayout.StageConditioning
	case GroupRoleOutputStage:
		return schematiclayout.StageBoundaryOutput
	case GroupRoleRegulatorStage, GroupRolePowerStage, GroupRoleProcessingStage, GroupRoleDecouplingStage:
		return schematiclayout.StageProcessing
	default:
		return schematiclayout.StageUnknown
	}
}

func paperDimensionsMM(paper string) (float64, float64) {
	switch strings.ToUpper(strings.TrimSpace(paper)) {
	case "A0":
		return 1189, 841
	case "A1":
		return 841, 594
	case "A2":
		return 594, 420
	case "A3":
		return 420, 297
	case "A5":
		return 210, 148
	default:
		return 297, 210
	}
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func layoutRotations(document Document) map[string]float64 {
	rotations := map[string]float64{}
	for _, placement := range document.Layout.Placements {
		if placement.Target == "" {
			continue
		}
		switch placement.Orientation {
		case OrientationRotated:
			rotations[placement.Target] = 90
		case OrientationRotated90:
			rotations[placement.Target] = 90
		case OrientationRotated180:
			rotations[placement.Target] = 180
		case OrientationRotated270:
			rotations[placement.Target] = 270
		default:
			rotations[placement.Target] = 0
		}
	}
	return rotations
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

func transactionPins(component Component) []transactions.PinSpec {
	if len(component.Pins) == 0 {
		return nil
	}
	offsets := map[string]kicadfiles.Point{}
	if templatePins, ok := schematic.EmbeddedSymbolPinOffsets(component.Symbol); ok {
		for _, pin := range templatePins {
			offset := pin.Offset
			if connectionOffset, exists := schematic.EmbeddedSymbolConnectionPinOffset(component.Symbol, pin.Number); exists {
				offset = connectionOffset
			}
			offsets[pin.Number] = offset
		}
	}
	out := make([]transactions.PinSpec, 0, len(component.Pins))
	for _, pin := range component.Pins {
		offset := offsets[pin.Number]
		out = append(out, transactions.PinSpec{Number: pin.Number, XMM: float64(offset.X) / 1_000_000, YMM: float64(offset.Y) / 1_000_000})
	}
	return out
}

func transactionSymbolProperties(component Component) []transactions.SymbolProperty {
	footprint := strings.TrimSpace(component.Footprint)
	properties := make([]transactions.SymbolProperty, 0, len(component.Properties)+1)
	keys := make([]string, 0, len(component.Properties))
	for key := range component.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seen := map[string]struct{}{}
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if footprint != "" && strings.EqualFold(trimmedKey, footprintPropertyName) {
			continue
		}
		normalizedKey := strings.ToLower(trimmedKey)
		if _, ok := seen[normalizedKey]; ok {
			// Keep the first sorted spelling/value for deterministic output when
			// IR producers send case-only duplicate property names.
			continue
		}
		seen[normalizedKey] = struct{}{}
		properties = append(properties, transactions.SymbolProperty{Name: trimmedKey, Value: component.Properties[key]})
	}
	if footprint != "" {
		properties = append(properties, transactions.SymbolProperty{
			Name:   footprintPropertyName,
			Value:  footprint,
			Hidden: true,
		})
	}
	sort.Slice(properties, func(i, j int) bool {
		return properties[i].Name < properties[j].Name
	})
	if len(properties) == 0 {
		return nil
	}
	return properties
}
