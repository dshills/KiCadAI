package schematicir

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

const footprintPropertyName = "Footprint"

// ToTransaction converts a validated schematic IR document into the existing
// schematic transaction operation stream.
func ToTransaction(document Document) (transactions.Transaction, []reports.Issue) {
	return toTransaction(document, nil)
}

// ToTransactionWithLibraryIndex uses resolver geometry for symbols that are
// not covered by KiCadAI's verified template set.
func ToTransactionWithLibraryIndex(document Document, index *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
	return toTransaction(document, index)
}

func toTransaction(document Document, index *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
	document = NormalizeLayoutIntent(document)
	if issues := validateDefaulted(document); len(issues) != 0 {
		return transactions.Transaction{}, issues
	}

	state, issues := newAdapterState(document, index)
	if len(issues) != 0 {
		return transactions.Transaction{}, issues
	}

	tx := transactions.Transaction{
		Name:    document.Metadata.Name,
		Project: document.Metadata.Name,
	}
	state.appendCreateProject(&tx)
	state.appendComponents(&tx)
	state.appendBuses(&tx)
	state.appendNets(&tx)
	state.appendPorts(&tx)

	return tx, state.issues
}

// ToProjectTransaction converts schematic IR into a transaction that can be
// applied to write a KiCad project directory.
func ToProjectTransaction(document Document) (transactions.Transaction, []reports.Issue) {
	return toProjectTransaction(document, nil)
}

// ToProjectTransactionWithLibraryIndex carries resolver geometry through the
// complete IR-to-project path while preserving the template-only API above.
func ToProjectTransactionWithLibraryIndex(document Document, index *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
	return toProjectTransaction(document, index)
}

func toProjectTransaction(document Document, index *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
	document = NormalizeLayoutIntent(document)
	tx, issues := toTransaction(document, index)
	if reports.HasBlockingIssue(issues) {
		return tx, issues
	}
	hierarchy, hierarchyIssues := schematicHierarchy(document, index)
	issues = append(issues, hierarchyIssues...)
	if reports.HasBlockingIssue(hierarchyIssues) {
		return tx, issues
	}
	payload := transactions.WriteProjectOperation{
		Op:                          transactions.OpWriteProject,
		SchematicOnly:               true,
		RequireSchematicReadability: document.Policy.Acceptance == AcceptanceReadable,
		Hierarchy:                   hierarchy,
	}
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

func schematicHierarchy(document Document, index *libraryresolver.LibraryIndex) (*transactions.SchematicHierarchy, []reports.Issue) {
	layout := schematicLayoutWithLibraryIndex(document, index)
	if layout.Partition == nil || len(layout.Partition.Sheets) < 2 {
		return nil, nil
	}
	state, stateIssues := newAdapterState(document, index)
	if len(stateIssues) != 0 {
		return nil, stateIssues
	}
	componentSheets := map[string]string{}
	hierarchy := &transactions.SchematicHierarchy{}
	for _, sheet := range layout.Partition.Sheets {
		refs := make([]string, 0, len(sheet.Components))
		symbols := make([]transactions.SchematicSymbolRef, 0, len(sheet.Components))
		for _, componentID := range sheet.Components {
			componentSheets[componentID] = sheet.ID
			if ref := state.refsByID[componentID]; ref != "" {
				refs = append(refs, ref)
				symbols = append(symbols, transactions.SchematicSymbolRef{Ref: ref, Unit: state.unitsByID[componentID]})
			}
		}
		sort.Strings(refs)
		sort.SliceStable(symbols, func(i, j int) bool {
			if symbols[i].Ref != symbols[j].Ref {
				return symbols[i].Ref < symbols[j].Ref
			}
			return symbols[i].Unit < symbols[j].Unit
		})
		hierarchy.Sheets = append(hierarchy.Sheets, transactions.SchematicHierarchySheet{
			ID:         sheet.ID,
			Name:       sheet.Name,
			Filename:   "sch/" + sheet.ID + ".kicad_sch",
			References: refs,
			Symbols:    symbols,
		})
	}
	netsByName := make(map[string]Net, len(document.Circuit.Nets))
	for _, net := range document.Circuit.Nets {
		netsByName[net.Name] = net
	}
	for _, cross := range layout.Partition.CrossSheetNets {
		if net, ok := netsByName[cross.Name]; ok {
			entry := transactions.SchematicCrossSheetNet{Name: net.Name}
			for _, endpoint := range net.Connect {
				componentID, pin, ok := endpoint.Split()
				if !ok || componentSheets[componentID] == "" || state.refsByID[componentID] == "" {
					continue
				}
				entry.Endpoints = append(entry.Endpoints, transactions.Endpoint{Ref: state.refsByID[componentID], Pin: pin, Unit: state.unitsByID[componentID]})
			}
			if len(entry.Endpoints) > 0 {
				hierarchy.CrossSheetNets = append(hierarchy.CrossSheetNets, entry)
			}
		}
	}
	if len(hierarchy.Sheets) < 2 {
		return nil, nil
	}
	return hierarchy, nil
}

type adapterState struct {
	document     Document
	libraryIndex *libraryresolver.LibraryIndex
	paper        string
	refsByID     map[string]string
	unitsByID    map[string]int
	pointsByID   map[string]transactions.Point
	rotationByID map[string]float64
	routesByKey  map[string]schematiclayout.RoutedConnection
	labelsByKey  map[string]kicadfiles.Point
	textByID     map[string]layoutTextPlacement
	layoutResult schematiclayout.Result
	issues       []reports.Issue
}

const (
	defaultLayoutStartXMM       = 25.0
	defaultLayoutSignalYMM      = 55.0
	defaultLayoutPowerYMM       = 25.0
	defaultLayoutGroundYMM      = 95.0
	defaultComponentPadding     = kicadfiles.IU(1270000)
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

func newAdapterState(document Document, index *libraryresolver.LibraryIndex) (*adapterState, []reports.Issue) {
	var preflightIssues []reports.Issue
	validatedUnits := make(map[string]int, len(document.Circuit.Components))
	for componentIndex, component := range document.Circuit.Components {
		if unit, ok := transactionUnit(component.Unit); ok {
			validatedUnits[component.ID] = unit
		} else {
			preflightIssues = append(preflightIssues, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     fmt.Sprintf("circuit.components[%d].unit", componentIndex),
				Message:  "component unit must be a non-negative integer",
			})
		}
	}
	preflightIssues = append(preflightIssues, resolverRecordIssues(document, index)...)
	layoutResult := schematicLayoutWithLibraryIndex(document, index)
	paper := layoutResult.Sheet.Name
	if paper == "" {
		paper = document.Metadata.Paper
	}
	state := &adapterState{
		document:     document,
		libraryIndex: index,
		paper:        paper,
		refsByID:     map[string]string{},
		unitsByID:    map[string]int{},
		pointsByID:   layoutResultPoints(layoutResult),
		layoutResult: layoutResult,
		rotationByID: layoutRotations(document),
		routesByKey:  layoutRouteHints(layoutResult),
		labelsByKey:  layoutEndpointLabelHints(layoutResult),
		textByID:     layoutTextPlacements(layoutResult),
		issues:       preflightIssues,
	}
	state.issues = append(state.issues, schematicLayoutAcceptanceIssues(document, layoutResult)...)
	connectedPins := connectedPinSet(document)
	for componentIndex, component := range document.Circuit.Components {
		knownPins := knownSchematicPinNumbers(component, index)
		for pinIndex, pin := range component.Pins {
			number := strings.TrimSpace(pin.Number)
			key := strings.TrimSpace(component.ID) + "." + strings.TrimSpace(pin.Number)
			if _, required := connectedPins[key]; !required {
				continue
			}
			if _, ok := explicitPinOffset(pin, kicadfiles.Point{}); ok {
				continue
			}
			if _, known := knownPins[number]; known {
				continue
			}
			state.addIssue(fmt.Sprintf("circuit.components[%d].pins[%d]", componentIndex, pinIndex), "pin geometry is unresolved; provide explicit offsets or a resolver index")
		}
	}
	refCounters := map[string]int{}
	usedRefs := map[string]struct{}{}
	usedRefUnits := map[string]struct{}{}
	invalidComponentIDs := map[string]struct{}{}
	trimmedRefs := map[string]string{}
	for index, component := range document.Circuit.Components {
		unit, ok := validatedUnits[component.ID]
		if !ok {
			invalidComponentIDs[component.ID] = struct{}{}
			continue
		}
		state.unitsByID[component.ID] = unit
		ref := strings.TrimSpace(component.Ref)
		trimmedRefs[component.ID] = ref
		if ref != "" {
			unitKey := strconv.Itoa(maxUnit(unit))
			refKey := adapterReferenceKey(ref)
			refUnitKey := refKey + "#" + unitKey
			if _, exists := usedRefUnits[refUnitKey]; exists {
				state.addIssue(fmt.Sprintf("circuit.components[%d].ref", index), "duplicate component reference and unit "+ref+"/"+unitKey)
				invalidComponentIDs[component.ID] = struct{}{}
				continue
			}
			usedRefUnits[refUnitKey] = struct{}{}
			if _, exists := usedRefs[refKey]; !exists {
				usedRefs[refKey] = struct{}{}
			}
			seedRefCounter(refCounters, ref)
		}
	}
	for index, component := range document.Circuit.Components {
		if _, invalid := invalidComponentIDs[component.ID]; invalid {
			continue
		}
		ref := trimmedRefs[component.ID]
		if ref == "" {
			if !document.Policy.Repair.AllowRefAssignment {
				state.addIssue(fmt.Sprintf("circuit.components[%d].ref", index), "component reference is required when ref assignment repair is disabled")
				continue
			}
			ref = state.nextRef(component.Role, refCounters, usedRefs)
		}
		usedRefs[adapterReferenceKey(ref)] = struct{}{}
		state.refsByID[component.ID] = ref
	}
	return state, state.issues
}

// resolverRecordIssues keeps the AI-facing adapter fail-closed when a caller
// supplied a library index but one of the requested records was not indexed.
// The template-only path remains available when no index is supplied.
func resolverRecordIssues(document Document, index *libraryresolver.LibraryIndex) []reports.Issue {
	if index == nil {
		return nil
	}
	var issues []reports.Issue
	for componentIndex, component := range document.Circuit.Components {
		symbolID := strings.TrimSpace(component.Symbol)
		if _, embedded := schematic.EmbeddedSymbolTemplate(symbolID); !embedded {
			if _, found := libraryresolver.ResolveSymbol(*index, symbolID); !found {
				issues = append(issues, reports.Issue{
					Code:       reports.CodeUnknownSymbolLibrary,
					Severity:   reports.SeverityError,
					Path:       fmt.Sprintf("circuit.components[%d].symbol", componentIndex),
					Message:    "symbol library record not found: " + symbolID,
					Suggestion: "configure the correct KiCad symbol root or provide a supported embedded symbol",
				})
			}
		}
		footprintID := strings.TrimSpace(component.Footprint)
		if footprintID == "" {
			continue
		}
		if _, found := libraryresolver.ResolveFootprint(*index, footprintID); !found {
			issues = append(issues, reports.Issue{
				Code:       reports.CodeUnknownFootprintLibrary,
				Severity:   reports.SeverityError,
				Path:       fmt.Sprintf("circuit.components[%d].footprint", componentIndex),
				Message:    "footprint library record not found: " + footprintID,
				Suggestion: "configure the correct KiCad footprint root or remove the unresolved footprint assignment",
			})
		}
	}
	return issues
}

func schematicLayoutAcceptanceIssues(document Document, result schematiclayout.Result) []reports.Issue {
	if document.Policy.Acceptance != AcceptanceReadable {
		return nil
	}
	issues := make([]reports.Issue, 0)
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == schematiclayout.SeverityInfo || readableLayoutDiagnosticAllowed(result, diagnostic.Code) {
			continue
		}
		severity := reports.SeverityError
		if diagnostic.Severity == schematiclayout.SeverityWarning {
			severity = reports.SeverityBlocked
		}
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   severity,
			Path:       "layout." + diagnostic.Code,
			Message:    diagnostic.Message,
			Suggestion: diagnostic.Repair,
		})
	}
	return issues
}

func readableLayoutDiagnosticAllowed(result schematiclayout.Result, code string) bool {
	switch code {
	case "page_escalated", "page_fit_exhausted", "hierarchy_partition_required":
		return true
	case "wire_symbol_overlap":
		// The planner may use a conservative/fallback body envelope before the
		// writer hydrates the exact embedded symbol geometry. Readback tests
		// validate the final KiCad-native body for this transitional diagnostic.
		return true
	case "outside_sheet":
		return result.Partition != nil
	default:
		return false
	}
}

func (state *adapterState) appendCreateProject(tx *transactions.Transaction) {
	payload := transactions.CreateProjectOperation{
		Op:    transactions.OpCreateProject,
		Name:  state.document.Metadata.Name,
		Paper: state.paper,
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
			Pins:       transactionPinsWithLibraryIndex(component, state.libraryIndex),
			Properties: transactionSymbolPropertiesWithLayout(component, ref, state.textByID[component.ID]),
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

func (state *adapterState) appendBuses(tx *transactions.Transaction) {
	if len(state.document.Circuit.Buses) == 0 {
		return
	}
	if state.layoutResult.Partition != nil && len(state.layoutResult.Partition.Sheets) > 1 {
		state.addIssue("circuit.buses", "vector bus generation is not supported across generated hierarchy sheets")
		return
	}
	busesByID := make(map[string]Bus, len(state.document.Circuit.Buses))
	for _, bus := range state.document.Circuit.Buses {
		busesByID[bus.ID] = bus
	}
	for layoutIndex, layout := range state.document.Layout.Buses {
		bus, ok := busesByID[layout.Bus]
		if !ok {
			continue
		}
		points := make([]transactions.Point, 0, len(layout.Points))
		for _, item := range layout.Points {
			points = append(points, transactions.Point{XMM: item.XMM, YMM: item.YMM})
		}
		state.appendOperation(tx, transactions.OpAddBus, transactions.AddBusOperation{
			Op:     transactions.OpAddBus,
			Points: points,
		}, "", "")
		if strings.TrimSpace(bus.Name) != "" && len(points) > 0 {
			state.appendOperation(tx, transactions.OpAddLabel, transactions.AddLabelOperation{
				Op:   transactions.OpAddLabel,
				Text: bus.Name,
				At:   points[0],
				Kind: "local",
			}, "", "")
		}
		members := make(map[string]BusMember, len(bus.Members))
		for _, member := range bus.Members {
			members[member.Net] = member
		}
		for entryIndex, entry := range layout.Entries {
			member, exists := members[entry.Member]
			if !exists {
				continue
			}
			state.appendOperation(tx, transactions.OpAddBusEntry, transactions.AddBusEntryOperation{
				Op:   transactions.OpAddBusEntry,
				At:   transactions.Point{XMM: entry.At.XMM, YMM: entry.At.YMM},
				Size: transactions.Point{XMM: entry.Size.XMM, YMM: entry.Size.YMM},
			}, "", "")
			anchor, anchorOK := state.portEndpointAnchor(entry.Endpoint)
			if !anchorOK {
				state.addIssue(fmt.Sprintf("layout.buses[%d].entries[%d].endpoint", layoutIndex, entryIndex), "bus entry endpoint has no resolved schematic anchor")
				continue
			}
			entryPoint := transactions.Point{XMM: entry.At.XMM + entry.Size.XMM, YMM: entry.At.YMM + entry.Size.YMM}
			pinStub := state.busPinStub(anchor, entry.Endpoint)
			entryStub := transactions.Point{XMM: entryPoint.XMM + 5.08, YMM: entryPoint.YMM}
			state.appendOperation(tx, transactions.OpAddSchematicWire, transactions.AddSchematicWireOperation{
				Op:          transactions.OpAddSchematicWire,
				NetName:     member.Net,
				Points:      []transactions.Point{anchor, pinStub},
				Label:       member.Label,
				LabelAt:     &pinStub,
				LabelRotate: 0,
			}, "", member.Net)
			state.appendOperation(tx, transactions.OpAddSchematicWire, transactions.AddSchematicWireOperation{
				Op:          transactions.OpAddSchematicWire,
				NetName:     member.Net,
				Points:      []transactions.Point{entryPoint, entryStub},
				Label:       member.Label,
				LabelAt:     &entryStub,
				LabelRotate: 0,
			}, "", member.Net)
		}
	}
}

func (state *adapterState) busPinStub(anchor transactions.Point, endpoint EndpointRef) transactions.Point {
	componentID, _, _ := endpoint.Split()
	origin, ok := state.pointsByID[componentID]
	if ok {
		dx := anchor.XMM - origin.XMM
		dy := anchor.YMM - origin.YMM
		if math.Abs(dx) >= math.Abs(dy) && dx != 0 {
			stubDistance := 2.54
			if dx < 0 {
				stubDistance = 5.08
			}
			return transactions.Point{XMM: anchor.XMM + math.Copysign(stubDistance, dx), YMM: anchor.YMM}
		}
		if dy != 0 {
			return transactions.Point{XMM: anchor.XMM, YMM: anchor.YMM + math.Copysign(2.54, dy)}
		}
	}
	return transactions.Point{XMM: anchor.XMM + 2.54, YMM: anchor.YMM}
}

type layoutTextPlacement struct {
	reference *transactions.Point
	value     *transactions.Point
}

func layoutTextPlacements(result schematiclayout.Result) map[string]layoutTextPlacement {
	placements := make(map[string]layoutTextPlacement, len(result.Components))
	for _, component := range result.Components {
		placement := layoutTextPlacement{}
		if !component.ReferenceText.Box.Empty() {
			point := transactions.Point{
				XMM: float64(component.PlacedAt.X+component.ReferenceText.At.X) / 1_000_000,
				YMM: float64(component.PlacedAt.Y+component.ReferenceText.At.Y) / 1_000_000,
			}
			placement.reference = &point
		}
		if !component.ValueText.Box.Empty() {
			point := transactions.Point{
				XMM: float64(component.PlacedAt.X+component.ValueText.At.X) / 1_000_000,
				YMM: float64(component.PlacedAt.Y+component.ValueText.At.Y) / 1_000_000,
			}
			placement.value = &point
		}
		placements[component.Ref] = placement
	}
	return placements
}

func (state *adapterState) appendNets(tx *transactions.Transaction) {
	for netIndex, net := range state.orderedNetsForEmission() {
		if state.isBusMember(net.Name) {
			continue
		}
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
			useLabels := schematicNetLabelPreference(state.document, net)
			var waypoints []transactions.Point
			var fromLabelAt, toLabelAt *transactions.Point
			fromIR := net.Connect[endpointIndex-1]
			toIR := net.Connect[endpointIndex]
			if hint, exists := state.routesByKey[schematicRouteKey(net.Name, fromIR, toIR)]; exists {
				if hint.UseLabels && net.UseLabel == nil {
					value := true
					useLabels = &value
					fromLayout, toLayout := hint.FromLabelAt, hint.ToLabelAt
					if !schematicRouteMatches(hint, fromIR, toIR) {
						fromLayout, toLayout = toLayout, fromLayout
					}
					fromLabelAt = transactionPoint(fromLayout)
					toLabelAt = transactionPoint(toLayout)
				} else if len(hint.Points) != 0 {
					value := false
					useLabels = &value
					points := hint.Points
					if !schematicRouteMatches(hint, fromIR, toIR) {
						points = reversedLayoutPoints(points)
					}
					waypoints = transactionPoints(points)
				}
			}
			portNet := state.hasPortNet(net.Name)
			skipFromLabel := false
			skipToLabel := false
			if portNet {
				value := true
				useLabels = &value
				waypoints = nil
				portEndpoint := state.portEndpointForNet(net.Name)
				skipFromLabel = portEndpoint == fromIR
				skipToLabel = portEndpoint == toIR
				if !skipFromLabel {
					if point, pointOK := state.labelPointForEndpoint(net.Name, fromIR); pointOK {
						fromLabelAt = &point
					}
				}
				if !skipToLabel {
					if point, pointOK := state.labelPointForEndpoint(net.Name, toIR); pointOK {
						toLabelAt = &point
					}
				}
			}
			if useLabels != nil && *useLabels {
				if fromLabelAt == nil {
					fromLabel, fromOK := state.labelsByKey[schematicEndpointLabelKey(net.Name, fromIR)]
					fromLabelAt = transactionPointValue(fromLabel, fromOK)
				}
				if toLabelAt == nil {
					toLabel, toOK := state.labelsByKey[schematicEndpointLabelKey(net.Name, toIR)]
					toLabelAt = transactionPointValue(toLabel, toOK)
				}
			}
			payload := transactions.ConnectOperation{
				Op:                 transactions.OpConnect,
				From:               from,
				To:                 to,
				NetName:            net.Name,
				UseLabels:          useLabels,
				SuppressBendLabels: net.UseLabel != nil && !*net.UseLabel,
				SkipFromLabel:      skipFromLabel,
				SkipToLabel:        skipToLabel,
				Waypoints:          waypoints,
				FromLabelAt:        fromLabelAt,
				ToLabelAt:          toLabelAt,
			}
			state.appendOperation(tx, transactions.OpConnect, payload, "", net.Name)
		}
	}
}

func (state *adapterState) isBusMember(netName string) bool {
	for _, bus := range state.document.Circuit.Buses {
		for _, member := range bus.Members {
			if member.Net == netName {
				return true
			}
		}
	}
	return false
}

func (state *adapterState) appendPorts(tx *transactions.Transaction) {
	netsByName := make(map[string]Net, len(state.document.Circuit.Nets))
	for _, net := range state.document.Circuit.Nets {
		netsByName[net.Name] = net
	}
	for _, port := range state.document.Circuit.Ports {
		net, ok := netsByName[port.Net]
		if !ok || len(net.Connect) == 0 {
			continue
		}
		_, at, ok := state.portEndpointInfoForSide(net.Name, net.Connect, port.Side)
		if !ok {
			state.addIssue("circuit.ports", "could not resolve schematic anchor for port "+port.Name)
			continue
		}
		payload := transactions.AddLabelOperation{
			Op:          transactions.OpAddLabel,
			Text:        port.Name,
			At:          at,
			Kind:        "global",
			RotationDeg: portLabelRotation(port.Side),
		}
		state.appendOperation(tx, transactions.OpAddLabel, payload, "", port.Net)
	}
}

func (state *adapterState) portEndpointForSide(netName string, endpoints []EndpointRef, side Side) EndpointRef {
	endpoint, _, _ := state.portEndpointInfoForSide(netName, endpoints, side)
	return endpoint
}

func (state *adapterState) portEndpointInfoForSide(netName string, endpoints []EndpointRef, side Side) (EndpointRef, transactions.Point, bool) {
	if len(endpoints) == 0 {
		return "", transactions.Point{}, false
	}
	type candidate struct {
		endpoint EndpointRef
		point    transactions.Point
	}
	var candidates []candidate
	for _, endpoint := range endpoints {
		if point, ok := state.portEndpointPoint(netName, endpoint); ok {
			candidates = append(candidates, candidate{endpoint: endpoint, point: point})
		}
	}
	if len(candidates) == 0 {
		point, ok := state.portEndpointPoint(netName, endpoints[0])
		return endpoints[0], point, ok
	}
	best := candidates[0]
	for _, current := range candidates[1:] {
		better := false
		switch side {
		case SideRight:
			better = current.point.XMM > best.point.XMM
		case SideTop:
			better = current.point.YMM < best.point.YMM
		case SideBottom:
			better = current.point.YMM > best.point.YMM
		default:
			better = current.point.XMM < best.point.XMM
		}
		if better || (current.point == best.point && current.endpoint < best.endpoint) {
			best = current
		}
	}
	return best.endpoint, best.point, true
}

func (state *adapterState) hasPortNet(netName string) bool {
	for _, port := range state.document.Circuit.Ports {
		if port.Net == netName {
			return true
		}
	}
	return false
}

func (state *adapterState) portEndpointPoint(netName string, endpoint EndpointRef) (transactions.Point, bool) {
	_, _, ok := endpoint.Split()
	if !ok {
		return transactions.Point{}, false
	}
	anchor, ok := state.portEndpointAnchor(endpoint)
	if !ok {
		return transactions.Point{}, false
	}
	return anchor, true
}

func (state *adapterState) portEndpointAnchor(endpoint EndpointRef) (transactions.Point, bool) {
	componentID, pinNumber, ok := endpoint.Split()
	if !ok {
		return transactions.Point{}, false
	}
	origin, ok := state.pointsByID[componentID]
	if !ok {
		return transactions.Point{}, false
	}
	var component Component
	for _, candidate := range state.document.Circuit.Components {
		if candidate.ID == componentID {
			component = candidate
			break
		}
	}
	if component.ID == "" {
		return transactions.Point{}, false
	}
	for _, pin := range transactionPinsWithLibraryIndex(component, state.libraryIndex) {
		if pin.Number != pinNumber {
			continue
		}
		x, y := rotateSchematicPoint(pin.XMM, pin.YMM, state.rotationByID[componentID])
		return transactions.Point{XMM: origin.XMM + x, YMM: origin.YMM + y}, true
	}
	return transactions.Point{}, false
}

func (state *adapterState) labelPointForEndpoint(netName string, endpoint EndpointRef) (transactions.Point, bool) {
	if point, ok := state.labelsByKey[schematicEndpointLabelKey(netName, endpoint)]; ok {
		return transactions.Point{XMM: float64(point.X) / float64(kicadfiles.MM(1)), YMM: float64(point.Y) / float64(kicadfiles.MM(1))}, true
	}
	anchor, ok := state.portEndpointAnchor(endpoint)
	if !ok {
		return transactions.Point{}, false
	}
	componentID, _, _ := endpoint.Split()
	origin, ok := state.pointsByID[componentID]
	if !ok {
		return transactions.Point{}, false
	}
	dx, dy := anchor.XMM-origin.XMM, anchor.YMM-origin.YMM
	if math.Abs(dx) >= math.Abs(dy) {
		anchor.XMM += math.Copysign(2.54, dx)
	} else if dy != 0 {
		anchor.YMM += math.Copysign(2.54, dy)
	} else {
		anchor.YMM -= 2.54
	}
	return anchor, true
}

func (state *adapterState) portEndpointInfo(netName string, endpoints []EndpointRef) (EndpointRef, transactions.Point, bool) {
	for _, port := range state.document.Circuit.Ports {
		if port.Net == netName {
			return state.portEndpointInfoForSide(netName, endpoints, port.Side)
		}
	}
	return state.portEndpointInfoForSide(netName, endpoints, SideLeft)
}

func (state *adapterState) portEndpointForNet(netName string) EndpointRef {
	endpoint, _, _ := state.portEndpointInfo(netName, state.netEndpoints(netName))
	return endpoint
}

func (state *adapterState) netEndpoints(netName string) []EndpointRef {
	for _, net := range state.document.Circuit.Nets {
		if net.Name == netName {
			return net.Connect
		}
	}
	return nil
}

func portLabelRotation(side Side) float64 {
	switch side {
	case SideRight:
		return 0
	case SideTop:
		return 270
	case SideBottom:
		return 90
	default:
		return 180
	}
}

func rotateSchematicPoint(x, y, angle float64) (float64, float64) {
	theta := angle * math.Pi / 180
	sin, cos := math.Sincos(theta)
	return x*cos - y*sin, x*sin + y*cos
}

func (state *adapterState) orderedNetsForEmission() []Net {
	nets := append([]Net(nil), state.document.Circuit.Nets...)
	sort.SliceStable(nets, func(i, j int) bool {
		left := state.netEmissionPriority(nets[i])
		right := state.netEmissionPriority(nets[j])
		return left < right
	})
	return nets
}

func (state *adapterState) netEmissionPriority(net Net) int {
	if net.Role == NetRoleNoConnect {
		return 3
	}
	for endpointIndex := 1; endpointIndex < len(net.Connect); endpointIndex++ {
		if hint, exists := state.routesByKey[schematicRouteKey(net.Name, net.Connect[endpointIndex-1], net.Connect[endpointIndex])]; exists && !hint.UseLabels && len(hint.Points) != 0 {
			return 0
		}
	}
	if preference := schematicNetLabelPreference(state.document, net); preference != nil && *preference {
		return 2
	}
	return 1
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
	case NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround, NetRoleReturn, NetRoleShield, NetRoleBus:
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
	return transactions.Endpoint{Ref: ref, Pin: pin, Unit: state.unitsByID[componentID]}, true
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
		if _, exists := usedRefs[adapterReferenceKey(ref)]; !exists {
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

func maxUnit(unit int) int {
	// KiCad symbol units are one-based; IR's empty/zero unit means the first
	// unit, while unit zero in resolver records denotes common geometry.
	if unit <= 0 {
		return 1
	}
	return unit
}

func adapterReferenceKey(reference string) string {
	return strings.ToUpper(strings.TrimSpace(reference))
}

func layoutPoints(document Document) map[string]transactions.Point {
	return layoutResultPoints(schematicLayout(document))
}

func layoutResultPoints(result schematiclayout.Result) map[string]transactions.Point {
	points := make(map[string]transactions.Point, len(result.Components))
	for _, component := range result.Components {
		points[component.Ref] = transactions.Point{
			XMM: float64(component.PlacedAt.X) / 1_000_000,
			YMM: float64(component.PlacedAt.Y) / 1_000_000,
		}
	}
	return points
}

func layoutRouteHints(result schematiclayout.Result) map[string]schematiclayout.RoutedConnection {
	hints := make(map[string]schematiclayout.RoutedConnection, len(result.Connections))
	for _, connection := range result.Connections {
		from := EndpointRef(connection.From.Ref + "." + connection.From.Pin)
		to := EndpointRef(connection.To.Ref + "." + connection.To.Pin)
		hints[schematicRouteKey(connection.NetName, from, to)] = connection
	}
	return hints
}

func layoutEndpointLabelHints(result schematiclayout.Result) map[string]kicadfiles.Point {
	hints := map[string]kicadfiles.Point{}
	for _, connection := range result.Connections {
		if !connection.UseLabels {
			continue
		}
		if connection.FromLabelAt != nil {
			endpoint := EndpointRef(connection.From.Ref + "." + connection.From.Pin)
			hints[schematicEndpointLabelKey(connection.NetName, endpoint)] = *connection.FromLabelAt
		}
		if connection.ToLabelAt != nil {
			endpoint := EndpointRef(connection.To.Ref + "." + connection.To.Pin)
			hints[schematicEndpointLabelKey(connection.NetName, endpoint)] = *connection.ToLabelAt
		}
	}
	return hints
}

func schematicEndpointLabelKey(netName string, endpoint EndpointRef) string {
	return netName + "\x00" + string(endpoint)
}

func schematicRouteKey(netName string, first, second EndpointRef) string {
	left, right := string(first), string(second)
	if right < left {
		left, right = right, left
	}
	return netName + "\x00" + left + "\x00" + right
}

func schematicRouteMatches(route schematiclayout.RoutedConnection, from, to EndpointRef) bool {
	return string(from) == route.From.Ref+"."+route.From.Pin && string(to) == route.To.Ref+"."+route.To.Pin
}

func reversedLayoutPoints(points []kicadfiles.Point) []kicadfiles.Point {
	out := append([]kicadfiles.Point(nil), points...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

func transactionPoints(points []kicadfiles.Point) []transactions.Point {
	out := make([]transactions.Point, 0, len(points))
	for _, point := range points {
		out = append(out, transactions.Point{XMM: float64(point.X) / 1_000_000, YMM: float64(point.Y) / 1_000_000})
	}
	return out
}

func transactionPoint(point *kicadfiles.Point) *transactions.Point {
	if point == nil {
		return nil
	}
	converted := transactions.Point{XMM: float64(point.X) / 1_000_000, YMM: float64(point.Y) / 1_000_000}
	return &converted
}

func transactionPointValue(point kicadfiles.Point, exists bool) *transactions.Point {
	if !exists {
		return nil
	}
	return transactionPoint(&point)
}

func schematicLayout(document Document) schematiclayout.Result {
	return schematicLayoutWithLibraryIndex(document, nil)
}

func schematicLayoutWithLibraryIndex(document Document, index *libraryresolver.LibraryIndex) schematiclayout.Result {
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
	request := schematiclayout.Request{
		Sheet: schematiclayout.SheetForPaper(document.Metadata.Paper),
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
			Body:      schematicLayoutBody(component, index),
			Pins:      schematicLayoutPins(component, index),
		})
	}
	for index, net := range document.Circuit.Nets {
		if documentNetIsBusMember(document, net.Name) {
			continue
		}
		layoutNet := schematiclayout.Net{Name: net.Name, Role: string(net.Role), OriginalOrdinal: index, PreferDirect: stateDocumentHasPortNet(document, net.Name)}
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
			Inferred:        group.Inferred,
			OriginalOrdinal: index,
		})
	}
	return schematiclayout.Layout(request)
}

func documentNetIsBusMember(document Document, netName string) bool {
	for _, bus := range document.Circuit.Buses {
		for _, member := range bus.Members {
			if member.Net == netName {
				return true
			}
		}
	}
	return false
}

func stateDocumentHasPortNet(document Document, netName string) bool {
	for _, port := range document.Circuit.Ports {
		if port.Net == netName {
			return true
		}
	}
	return false
}

func schematicLayoutBody(component Component, index *libraryresolver.LibraryIndex) schematiclayout.Rect {
	if component.Body != nil {
		return schematiclayout.Rect{
			MinX: kicadfiles.MM(component.Body.MinXMM),
			MinY: kicadfiles.MM(component.Body.MinYMM),
			MaxX: kicadfiles.MM(component.Body.MaxXMM),
			MaxY: kicadfiles.MM(component.Body.MaxYMM),
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(component.Symbol)), "kicadai:") {
		if bounds, ok := schematic.EmbeddedSymbolBodyBounds(component.Symbol); ok {
			return schematiclayout.Rect{
				MinX: bounds.Min.X,
				MinY: bounds.Min.Y,
				MaxX: bounds.Max.X,
				MaxY: bounds.Max.Y,
			}
		}
	}
	if index == nil {
		if _, known := schematic.EmbeddedSymbolTemplate(component.Symbol); known {
			return schematiclayout.Rect{}
		}
		return fallbackComponentBody(component)
	}
	record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol)
	if !ok {
		return fallbackComponentBody(component)
	}
	unit := componentUnitOrZero(component)
	var bounds schematiclayout.Rect
	hasGraphics := false
	for _, graphic := range record.Graphics {
		if graphic.Unit != 0 && graphic.Unit != maxUnit(unit) {
			continue
		}
		graphicBounds := schematiclayout.Rect{
			MinX: graphic.Bounds.Min.X,
			MinY: graphic.Bounds.Min.Y,
			MaxX: graphic.Bounds.Max.X,
			MaxY: graphic.Bounds.Max.Y,
		}
		if !hasGraphics {
			bounds = graphicBounds
		} else {
			bounds = unionLayoutRect(bounds, graphicBounds)
		}
		hasGraphics = true
	}
	if hasGraphics {
		return bounds
	}
	// Some KiCad symbols contain only pins or inherit graphics from a library
	// base. Keep a conservative pin envelope as a placement obstacle.
	var pinBounds schematiclayout.Rect
	hasPins := false
	for _, pin := range record.Pins {
		if pin.Unit != 0 && pin.Unit != maxUnit(unit) {
			continue
		}
		point := schematiclayout.Rect{MinX: pin.Position.X, MinY: pin.Position.Y, MaxX: pin.Position.X, MaxY: pin.Position.Y}
		if !hasPins {
			pinBounds = point
		} else {
			pinBounds = unionLayoutRect(pinBounds, point)
		}
		hasPins = true
	}
	if !hasPins {
		return fallbackComponentBody(component)
	}
	padding := defaultComponentPadding
	pinBounds.MinX -= padding
	pinBounds.MinY -= padding
	pinBounds.MaxX += padding
	pinBounds.MaxY += padding
	return pinBounds
}

func fallbackComponentBody(component Component) schematiclayout.Rect {
	var bounds schematiclayout.Rect
	hasPins := false
	for _, pin := range component.Pins {
		offset, ok := explicitPinOffset(pin, kicadfiles.Point{})
		if !ok {
			continue
		}
		point := schematiclayout.Rect{MinX: offset.X, MinY: offset.Y, MaxX: offset.X, MaxY: offset.Y}
		if !hasPins {
			bounds = point
		} else {
			bounds = unionLayoutRect(bounds, point)
		}
		hasPins = true
	}
	if !hasPins {
		return schematiclayout.Rect{MinX: -defaultComponentPadding, MinY: -defaultComponentPadding, MaxX: defaultComponentPadding, MaxY: defaultComponentPadding}
	}
	padding := defaultComponentPadding
	bounds.MinX -= padding
	bounds.MinY -= padding
	bounds.MaxX += padding
	bounds.MaxY += padding
	return bounds
}

func knownSchematicPinNumbers(component Component, index *libraryresolver.LibraryIndex) map[string]struct{} {
	known := map[string]struct{}{}
	if templatePins, ok := schematic.EmbeddedSymbolPinOffsets(component.Symbol); ok {
		for _, pin := range templatePins {
			known[strings.TrimSpace(pin.Number)] = struct{}{}
		}
	}
	if index == nil {
		return known
	}
	record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol)
	if !ok {
		return known
	}
	unit := componentUnitOrZero(component)
	for _, pin := range record.Pins {
		if pin.Unit != 0 && pin.Unit != maxUnit(unit) {
			continue
		}
		known[strings.TrimSpace(pin.Number)] = struct{}{}
	}
	return known
}

func connectedPinSet(document Document) map[string]struct{} {
	connected := map[string]struct{}{}
	for _, net := range document.Circuit.Nets {
		if net.Role == NetRoleNoConnect {
			continue
		}
		for _, endpoint := range net.Connect {
			connected[strings.TrimSpace(string(endpoint))] = struct{}{}
		}
	}
	return connected
}

func componentUnitOrZero(component Component) int {
	unit, ok := transactionUnit(component.Unit)
	if !ok {
		return 0
	}
	return unit
}

func unionLayoutRect(left, right schematiclayout.Rect) schematiclayout.Rect {
	if right.MinX < left.MinX {
		left.MinX = right.MinX
	}
	if right.MinY < left.MinY {
		left.MinY = right.MinY
	}
	if right.MaxX > left.MaxX {
		left.MaxX = right.MaxX
	}
	if right.MaxY > left.MaxY {
		left.MaxY = right.MaxY
	}
	return left
}

func schematicLayoutPins(component Component, index *libraryresolver.LibraryIndex) []schematiclayout.Pin {
	roles := map[string]string{}
	for _, pin := range component.Pins {
		roles[strings.TrimSpace(pin.Number)] = string(pin.Role)
	}
	templatePins, _ := schematic.EmbeddedSymbolPinOffsets(component.Symbol)
	offsets := map[string]kicadfiles.Point{}
	for _, pin := range templatePins {
		offset := pin.Offset
		if connectionOffset, ok := schematic.EmbeddedSymbolConnectionPinOffset(component.Symbol, pin.Number); ok {
			offset = connectionOffset
		}
		offsets[strings.TrimSpace(pin.Number)] = offset
	}
	if index != nil {
		if record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol); ok {
			unit := componentUnitOrZero(component)
			for _, pin := range record.Pins {
				if pin.Unit != 0 && pin.Unit != maxUnit(unit) {
					continue
				}
				pinNumber := strings.TrimSpace(pin.Number)
				if _, known := offsets[pinNumber]; !known {
					offsets[pinNumber] = pin.Position
				}
			}
		}
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
		number := strings.TrimSpace(pin.Number)
		offset := offsets[number]
		if explicit, ok := explicitPinOffset(pin, offset); ok {
			offset = explicit
		}
		pins = append(pins, schematiclayout.Pin{Number: number, Role: roles[number], At: offset})
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
	return transactionPinsWithLibraryIndex(component, nil)
}

func transactionPinsWithLibraryIndex(component Component, index *libraryresolver.LibraryIndex) []transactions.PinSpec {
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
			offsets[strings.TrimSpace(pin.Number)] = offset
		}
	}
	if index != nil {
		if record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol); ok {
			unit := componentUnitOrZero(component)
			for _, pin := range record.Pins {
				if pin.Unit != 0 && pin.Unit != maxUnit(unit) {
					continue
				}
				pinNumber := strings.TrimSpace(pin.Number)
				if _, known := offsets[pinNumber]; !known {
					offsets[pinNumber] = pin.Position
				}
			}
		}
	}
	out := make([]transactions.PinSpec, 0, len(component.Pins))
	for _, pin := range component.Pins {
		number := strings.TrimSpace(pin.Number)
		offset := offsets[number]
		explicit, ok := explicitPinOffset(pin, offset)
		if ok {
			offset = explicit
		}
		out = append(out, transactions.PinSpec{Number: number, XMM: float64(offset.X) / float64(kicadfiles.MM(1)), YMM: float64(offset.Y) / float64(kicadfiles.MM(1)), ExplicitOffset: ok})
	}
	return out
}

func explicitPinOffset(pin Pin, defaultOffset kicadfiles.Point) (kicadfiles.Point, bool) {
	if pin.OffsetXMM == nil && pin.OffsetYMM == nil {
		return kicadfiles.Point{}, false
	}
	offset := defaultOffset
	if pin.OffsetXMM != nil {
		offset.X = kicadfiles.MM(*pin.OffsetXMM)
	}
	if pin.OffsetYMM != nil {
		offset.Y = kicadfiles.MM(*pin.OffsetYMM)
	}
	return offset, true
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

func transactionSymbolPropertiesWithLayout(component Component, reference string, layout layoutTextPlacement) []transactions.SymbolProperty {
	properties := transactionSymbolProperties(component)
	rotation := 0.0
	doNotAutoplace := true
	visible := []transactions.SymbolProperty{
		{Name: "Reference", Value: reference, At: layout.reference, Rotation: &rotation, DoNotAutoplace: &doNotAutoplace},
	}
	value := component.Value
	hiddenValue := false
	if value == "" {
		value = reference
		hiddenValue = true
	}
	visible = append(visible, transactions.SymbolProperty{Name: "Value", Value: value, Hidden: hiddenValue, At: layout.value, Rotation: &rotation, DoNotAutoplace: &doNotAutoplace})
	for _, property := range visible {
		replaced := false
		for index := range properties {
			if strings.EqualFold(strings.TrimSpace(properties[index].Name), property.Name) {
				properties[index] = property
				replaced = true
				break
			}
		}
		if !replaced {
			properties = append(properties, property)
		}
	}
	sort.SliceStable(properties, func(i, j int) bool {
		return strings.ToLower(properties[i].Name) < strings.ToLower(properties[j].Name)
	})
	return properties
}
