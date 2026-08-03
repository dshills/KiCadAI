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

// LayoutDocument normalizes a schematic IR document and returns the same
// deterministic layout result used by transaction generation.
func LayoutDocument(document Document) schematiclayout.Result {
	return LayoutDocumentWithLibraryIndex(document, nil)
}

// LayoutDocumentWithLibraryIndex includes resolver-backed symbol geometry.
func LayoutDocumentWithLibraryIndex(document Document, index *libraryresolver.LibraryIndex) schematiclayout.Result {
	document = NormalizeLayoutIntent(document)
	return schematicLayoutWithLibraryIndex(document, index)
}

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
	defaulted := applyDefaults(document, true)
	if issues := validateDefaulted(defaulted); len(issues) != 0 {
		return transactions.Transaction{}, issues
	}
	document = NormalizeLayoutIntent(defaulted)

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

// HierarchyForProject returns the deterministic child-sheet plan that would be
// used when writing an oversized schematic. Callers that own their project
// write operation can use it without adding a second write_project operation.
func HierarchyForProject(document Document, index *libraryresolver.LibraryIndex) (*transactions.SchematicHierarchy, []reports.Issue) {
	defaulted := applyDefaults(document, true)
	if issues := validateDefaulted(defaulted); len(issues) != 0 {
		return nil, issues
	}
	document = NormalizeLayoutIntent(defaulted)
	return schematicHierarchy(document, index)
}

func toProjectTransaction(document Document, index *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
	tx, issues := toTransaction(document, index)
	if reports.HasBlockingIssue(issues) {
		return tx, issues
	}
	document = NormalizeLayoutIntent(document)
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
	busesByID := make(map[string]Bus, len(document.Circuit.Buses))
	for _, candidate := range document.Circuit.Buses {
		busesByID[candidate.ID] = candidate
	}
	for _, busLayout := range document.Layout.Buses {
		bus := busesByID[busLayout.Bus]
		if bus.ID == "" || len(busLayout.Points) < 2 {
			continue
		}
		entry := transactions.SchematicHierarchyBus{ID: bus.ID, Name: bus.Name}
		for _, point := range busLayout.Points {
			entry.Points = append(entry.Points, transactions.Point{XMM: point.XMM, YMM: point.YMM})
		}
		members := make(map[string]BusMember, len(bus.Members))
		for _, member := range bus.Members {
			members[member.Net] = member
		}
		for _, busEntry := range busLayout.Entries {
			member, ok := members[busEntry.Member]
			if !ok {
				continue
			}
			componentID, pin, ok := busEntry.Endpoint.Split()
			if !ok || componentSheets[componentID] == "" || state.refsByID[componentID] == "" {
				continue
			}
			entry.Entries = append(entry.Entries, transactions.SchematicHierarchyEntry{
				Member:   busEntry.Member,
				Label:    member.Label,
				Endpoint: transactions.Endpoint{Ref: state.refsByID[componentID], Pin: pin, Unit: state.unitsByID[componentID]},
				At:       busEntryPoint(busEntry.At),
				Size:     busEntryPoint(busEntry.Size),
			})
		}
		if len(entry.Entries) > 0 {
			hierarchy.Buses = append(hierarchy.Buses, entry)
		}
	}
	if len(hierarchy.Sheets) < 2 {
		return nil, nil
	}
	return hierarchy, nil
}

func busEntryPoint(point LayoutPoint) transactions.Point {
	return transactions.Point{XMM: point.XMM, YMM: point.YMM}
}

type adapterState struct {
	document            Document
	libraryIndex        *libraryresolver.LibraryIndex
	paper               string
	paperPortrait       bool
	refsByID            map[string]string
	componentsByID      map[string]Component
	unitsByID           map[string]int
	pointsByID          map[string]transactions.Point
	rotationByID        map[string]float64
	mirrorByID          map[string]Mirror
	routesByKey         map[string]schematiclayout.RoutedConnection
	labelsByKey         map[string]kicadfiles.Point
	pinAnchorsByX       map[kicadfiles.IU][]indexedSchematicAnchor
	pinAnchorsByY       map[kicadfiles.IU][]indexedSchematicAnchor
	labelAnchorsByX     map[kicadfiles.IU][]indexedSchematicAnchor
	labelAnchorsByY     map[kicadfiles.IU][]indexedSchematicAnchor
	netLabelPreferences map[string]bool
	textByID            map[string]layoutTextPlacement
	layoutResult        schematiclayout.Result
	issues              []reports.Issue
}

type indexedSchematicAnchor struct {
	point       kicadfiles.Point
	componentID string
	pinNumber   string
	netName     string
}

// connectionOverrideCacheEntry caches the symbol-level KiCad connection
// anchor for one pin number. The cache is intentionally keyed by symbol ID;
// embedded connection overrides are shared by every instance of that symbol.
type connectionOverrideCacheEntry struct {
	offset kicadfiles.Point
	known  bool
	found  bool
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
	netLabelPreferences := schematicNetLabelPreferencesWithLibraryIndex(document, index)
	layoutResult := schematicLayoutWithLibraryIndexAndPreferences(document, index, netLabelPreferences)
	paper := layoutResult.Sheet.Name
	if paper == "" {
		paper = document.Metadata.Paper
	}
	labelConflicts := layoutCrossNetLabelPoints(layoutResult)
	state := &adapterState{
		document:            document,
		libraryIndex:        index,
		paper:               paper,
		paperPortrait:       layoutResult.Sheet.Width < layoutResult.Sheet.Height,
		refsByID:            map[string]string{},
		componentsByID:      indexComponentsByID(document.Circuit.Components),
		unitsByID:           map[string]int{},
		pointsByID:          layoutResultPoints(layoutResult),
		layoutResult:        layoutResult,
		rotationByID:        layoutRotations(document),
		mirrorByID:          layoutMirrors(document),
		routesByKey:         layoutRouteHints(layoutResult, labelConflicts),
		labelsByKey:         layoutEndpointLabelHints(layoutResult, labelConflicts),
		netLabelPreferences: netLabelPreferences,
		textByID:            layoutTextPlacements(layoutResult),
		issues:              preflightIssues,
	}
	state.indexSchematicCollisionAnchors()
	state.issues = append(state.issues, schematicLayoutAcceptanceIssues(document, layoutResult)...)
	connectedPins := connectedPinSet(document)
	connectionOverrides := map[string]map[string]connectionOverrideCacheEntry{}
	for componentIndex, component := range document.Circuit.Components {
		componentID := strings.TrimSpace(component.ID)
		componentSymbol := strings.TrimSpace(component.Symbol)
		if _, exists := connectionOverrides[componentSymbol]; !exists {
			connectionOverrides[componentSymbol] = map[string]connectionOverrideCacheEntry{}
		}
		overrides := connectionOverrides[componentSymbol]
		knownPins := knownSchematicPinNumbers(component, index)
		var resolverRecord *libraryresolver.SymbolRecord
		if index != nil {
			if record, found := libraryresolver.ResolveSymbol(*index, component.Symbol); found && resolverGeometryAuthoritative(record, component.Symbol) {
				resolverRecord = &record
			}
		}
		resolverPins := map[string]kicadfiles.Point{}
		if resolverRecord != nil {
			resolverPins = resolverPinOffsets(*resolverRecord, component)
		}
		for pinIndex, pin := range component.Pins {
			number := strings.TrimSpace(pin.Number)
			key := componentID + "." + number
			if _, required := connectedPins[key]; !required {
				continue
			}
			cached := overrides[number]
			if !cached.known {
				if connectionOffset, hasConnectionOverride := schematic.EmbeddedSymbolConnectionPinOffset(componentSymbol, number); hasConnectionOverride {
					cached.offset = connectionOffset
					cached.found = true
				}
				cached.known = true
				overrides[number] = cached
			}
			connectionOffset := cached.offset
			connectionOffsetFound := cached.found
			connectionOffsetSource := "KiCad-validated connection anchor"
			if resolverOffset, resolverOK := resolverPins[number]; resolverOK {
				connectionOffset = resolverOffset
				connectionOffsetFound = true
				connectionOffsetSource = "resolver pin anchor"
			}
			explicit, hasExplicitOffset := explicitPinOffset(pin, connectionOffset)
			if connectionOffsetFound && hasExplicitOffset && !schematicAnchorsMatch(explicit, connectionOffset) {
				state.addIssue(fmt.Sprintf("circuit.components[%d].pins[%d]", componentIndex, pinIndex), fmt.Sprintf("explicit pin offset (%.4f,%.4f) conflicts with the %s (%.4f,%.4f)", float64(explicit.X)/float64(kicadfiles.MM(1)), float64(explicit.Y)/float64(kicadfiles.MM(1)), connectionOffsetSource, float64(connectionOffset.X)/float64(kicadfiles.MM(1)), float64(connectionOffset.Y)/float64(kicadfiles.MM(1))))
			}
			if hasExplicitOffset {
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
		Op:            transactions.OpCreateProject,
		Name:          state.document.Metadata.Name,
		Paper:         state.paper,
		PaperPortrait: state.paperPortrait,
	}
	state.appendOperation(tx, transactions.OpCreateProject, payload, "", "")
}

func (state *adapterState) appendComponents(tx *transactions.Transaction) {
	assignedFootprints := map[string]string{}
	primaryUnitByRef := map[string]int{}
	for _, component := range state.document.Circuit.Components {
		ref := state.refsByID[component.ID]
		if ref == "" {
			continue
		}
		key := adapterReferenceKey(ref)
		unit := maxUnit(state.unitsByID[component.ID])
		if primary, found := primaryUnitByRef[key]; !found || unit < primary {
			primaryUnitByRef[key] = unit
		}
	}
	for _, component := range state.document.Circuit.Components {
		ref := state.refsByID[component.ID]
		if ref == "" {
			continue
		}
		rotation, mirror := schematic.CanonicalSymbolTransform(kicadfiles.Angle(state.rotationByID[component.ID]), schematic.SymbolMirror(state.mirrorByID[component.ID]))
		payload := transactions.AddSymbolOperation{
			Op:        transactions.OpAddSymbol,
			Ref:       ref,
			Unit:      state.unitsByID[component.ID],
			Role:      string(component.Role),
			Value:     component.Value,
			LibraryID: component.Symbol,
			At:        state.pointsByID[component.ID],
			Rotation:  float64(rotation),
			Mirror:    string(mirror),
			Pins:      transactionPinsWithLibraryIndex(component, state.libraryIndex),
			Properties: transactionSymbolPropertiesWithLayout(
				component,
				ref,
				state.textByID[component.ID],
				float64(rotation),
				maxUnit(state.unitsByID[component.ID]) != primaryUnitByRef[adapterReferenceKey(ref)],
			),
		}
		state.appendOperation(tx, transactions.OpAddSymbol, payload, ref, "")
		for _, pin := range component.Pins {
			if !pin.NoConnect {
				continue
			}
			state.appendOperation(tx, transactions.OpAddNoConnect, transactions.AddNoConnectOperation{
				Op: transactions.OpAddNoConnect,
				Endpoint: transactions.Endpoint{
					Ref:  ref,
					Unit: state.unitsByID[component.ID],
					Pin:  strings.TrimSpace(pin.Number),
				},
			}, ref, "")
		}
		if component.Footprint != "" {
			if assigned, exists := assignedFootprints[ref]; exists {
				if assigned != component.Footprint {
					state.addIssue("circuit.components."+component.ID+".footprint", "shared reference "+ref+" resolves to conflicting footprints")
				}
				continue
			}
			assignedFootprints[ref] = component.Footprint
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
		if label := schematicBusLabel(bus); label != "" && len(points) > 0 {
			state.appendOperation(tx, transactions.OpAddLabel, transactions.AddLabelOperation{
				Op:   transactions.OpAddLabel,
				Text: label,
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
			pinStub, stubOK := state.busPinStub(anchor, entry.Endpoint, member.Net)
			if !stubOK {
				state.addIssue(fmt.Sprintf("layout.buses[%d].entries[%d].endpoint", layoutIndex, entryIndex), "bus member pin has no collision-free label stub")
				continue
			}
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

func schematicBusLabel(bus Bus) string {
	name := strings.TrimSpace(bus.Name)
	if strings.ContainsAny(name, "[{") {
		// Explicit KiCad vector/group syntax is authoritative for the label;
		// Bus.Members still controls entry and scalar-wire emission.
		return name
	}
	members := make([]string, 0, len(bus.Members))
	prefix := name + "."
	for _, member := range bus.Members {
		label := strings.TrimSpace(member.Label)
		if label == "" {
			continue
		}
		members = append(members, label)
	}
	if len(members) == 0 {
		return name
	}
	namedMembers := name != ""
	if namedMembers {
		for _, member := range members {
			if !strings.HasPrefix(member, prefix) || len(member) == len(prefix) {
				namedMembers = false
				break
			}
		}
	}
	if namedMembers {
		for index := range members {
			members[index] = strings.TrimPrefix(members[index], prefix)
		}
	}
	group := "{" + strings.Join(members, " ") + "}"
	if namedMembers {
		return name + group
	}
	// KiCad prefixes every member of a named group. Keep ordinary scalar net
	// labels unprefixed by emitting an unnamed group; prepending name here
	// would silently change SCL into name.SCL in the electrical netlist.
	return group
}

func (state *adapterState) busPinStub(anchor transactions.Point, endpoint EndpointRef, netName string) (transactions.Point, bool) {
	componentID, _, _ := endpoint.Split()
	origin, ok := state.pointsByID[componentID]
	if ok {
		dx := anchor.XMM - origin.XMM
		dy := anchor.YMM - origin.YMM
		if math.Abs(dx) >= math.Abs(dy) && dx != 0 {
			distances := []float64{2.54, 1.27}
			if dx < 0 {
				distances = []float64{5.08, 2.54, 1.27}
			}
			for _, distance := range distances {
				candidate := transactions.Point{XMM: anchor.XMM + math.Copysign(distance, dx), YMM: anchor.YMM}
				if !state.labelSegmentConflicts(netName, endpoint, anchor, candidate) {
					return candidate, true
				}
			}
			return transactions.Point{}, false
		}
		if dy != 0 {
			for _, distance := range []float64{2.54, 1.27} {
				candidate := transactions.Point{XMM: anchor.XMM, YMM: anchor.YMM + math.Copysign(distance, dy)}
				if !state.labelSegmentConflicts(netName, endpoint, anchor, candidate) {
					return candidate, true
				}
			}
		}
		for _, distance := range []float64{2.54, 1.27} {
			candidate := transactions.Point{XMM: anchor.XMM, YMM: anchor.YMM - distance}
			if !state.labelSegmentConflicts(netName, endpoint, anchor, candidate) {
				return candidate, true
			}
		}
	}
	return transactions.Point{}, false
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
	type endpointPair struct {
		from EndpointRef
		to   EndpointRef
	}
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
		pairs := make([]endpointPair, 0, len(mappedEndpoints)-1)
		if len(mappedEndpoints) > 2 {
			for _, connection := range state.layoutResult.Connections {
				if connection.NetName != net.Name {
					continue
				}
				pairs = append(pairs, endpointPair{
					from: EndpointRef(connection.From.Ref + "." + connection.From.Pin),
					to:   EndpointRef(connection.To.Ref + "." + connection.To.Pin),
				})
			}
		}
		if len(pairs) != len(mappedEndpoints)-1 {
			pairs = pairs[:0]
			for endpointIndex := 1; endpointIndex < len(net.Connect); endpointIndex++ {
				pairs = append(pairs, endpointPair{from: net.Connect[endpointIndex-1], to: net.Connect[endpointIndex]})
			}
		}
		useLabelsValue := state.netLabelPreferences[net.Name]
		useLabels := &useLabelsValue
		hasLayoutRouteAnnotation := layoutHasRouteAnnotation(state.layoutResult, net.Name)
		routeAnnotationAssigned := false
		for pairIndex, pair := range pairs {
			from, fromOK := state.transactionEndpoint(pair.from, fmt.Sprintf("circuit.nets[%d].route[%d].from", netIndex, pairIndex))
			to, toOK := state.transactionEndpoint(pair.to, fmt.Sprintf("circuit.nets[%d].route[%d].to", netIndex, pairIndex))
			if !fromOK || !toOK {
				continue
			}
			var waypoints []transactions.Point
			var fromLabelAt, toLabelAt, bendLabelAt *transactions.Point
			layoutLabelsRequested := false
			fromIR := pair.from
			toIR := pair.to
			if hint, exists := state.routesByKey[schematicRouteKey(net.Name, fromIR, toIR)]; exists {
				if hint.UseLabels {
					layoutLabelsRequested = true
					value := true
					useLabels = &value
					fromLayout, toLayout := hint.FromLabelAt, hint.ToLabelAt
					if !schematicRouteMatches(hint, fromIR, toIR) {
						fromLayout, toLayout = toLayout, fromLayout
					}
					fromLabelAt = transactionPoint(fromLayout)
					toLabelAt = transactionPoint(toLayout)
				}
				if !hint.UseLabels && len(hint.Points) != 0 {
					value := false
					useLabels = &value
					points := hint.Points
					if !schematicRouteMatches(hint, fromIR, toIR) {
						points = reversedLayoutPoints(points)
					}
					waypoints = transactionPoints(points)
					bendLabelAt = layoutBendLabelPoint(state.layoutResult, net.Name, hint.Points)
					if bendLabelAt != nil {
						routeAnnotationAssigned = true
					}
					if bendLabelAt == nil && !hasLayoutRouteAnnotation && !routeAnnotationAssigned && len(hint.Points) > 2 {
						bendLabelAt = schematicRouteLabelFallbackPoint(state.layoutResult, net.Name, hint.Points)
						if bendLabelAt != nil {
							routeAnnotationAssigned = true
						}
					}
				}
			}
			fromLabelAt = state.validLabelPointForEndpoint(net.Name, fromIR, fromLabelAt)
			toLabelAt = state.validLabelPointForEndpoint(net.Name, toIR, toLabelAt)
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
					if point, pointOK := state.labelPointForEndpoint(net.Name, fromIR); pointOK {
						fromLabelAt = &point
					}
				}
				if toLabelAt == nil {
					if point, pointOK := state.labelPointForEndpoint(net.Name, toIR); pointOK {
						toLabelAt = &point
					}
				}
				if !layoutLabelsRequested && ((fromLabelAt == nil && !skipFromLabel) || (toLabelAt == nil && !skipToLabel)) {
					// Without layout route evidence, retain the deterministic
					// direct-wire fallback. A layout-selected label route may keep
					// nil coordinates: the builder derives safe stubs from its final
					// post-collision pin anchors.
					value := false
					useLabels = &value
					fromLabelAt = nil
					toLabelAt = nil
				}
			}
			fromAnchor, fromAnchorOK := state.portEndpointAnchor(fromIR)
			toAnchor, toAnchorOK := state.portEndpointAnchor(toIR)
			if fromAnchorOK && toAnchorOK && fromAnchor == toAnchor {
				// Some reviewed symbols expose several physical pins at one
				// electrical anchor (for example, parallel switch pins). Keep
				// the connect operations so every pin receives the net, but do
				// not emit duplicate label stubs or zero-length wires at the
				// shared anchor.
				value := false
				useLabels = &value
				waypoints = nil
				fromLabelAt = nil
				toLabelAt = nil
				skipFromLabel = false
				skipToLabel = false
			}
			payload := transactions.ConnectOperation{
				Op:        transactions.OpConnect,
				From:      from,
				To:        to,
				NetName:   net.Name,
				UseLabels: useLabels,
				// A direct layout route is already a complete visible
				// conductor. Do not let the writer re-introduce a label at
				// every orthogonal bend merely to preserve the internal name.
				SuppressBendLabels:  useLabels != nil && !*useLabels,
				BendLabelAt:         bendLabelAt,
				SkipFromLabel:       skipFromLabel,
				SkipToLabel:         skipToLabel,
				Waypoints:           waypoints,
				FromLabelAt:         fromLabelAt,
				ToLabelAt:           toLabelAt,
				OrientLabelsOutward: state.document.Layout.Rules.OrientEndpointLabels,
			}
			state.appendOperation(tx, transactions.OpConnect, payload, "", net.Name)
		}
	}
}

func layoutHasRouteAnnotation(result schematiclayout.Result, netName string) bool {
	for _, label := range result.Labels {
		if label.NetName == netName && label.RouteAnnotation {
			return true
		}
	}
	return false
}

func layoutBendLabelPoint(result schematiclayout.Result, netName string, points []kicadfiles.Point) *transactions.Point {
	if len(points) < 2 {
		return nil
	}
	for _, label := range result.Labels {
		if label.NetName != netName || !label.RouteAnnotation {
			continue
		}
		for index := 1; index < len(points); index++ {
			if !schematicPointOnSegment(label.Position, points[index-1], points[index]) {
				continue
			}
			point := transactions.Point{
				XMM: float64(label.Position.X) / 1_000_000,
				YMM: float64(label.Position.Y) / 1_000_000,
			}
			return &point
		}
	}
	return nil
}

func schematicPointOnSegment(point, start, end kicadfiles.Point) bool {
	switch {
	case start.X == end.X:
		return point.X == start.X && betweenSchematicCoordinates(point.Y, start.Y, end.Y)
	case start.Y == end.Y:
		return point.Y == start.Y && betweenSchematicCoordinates(point.X, start.X, end.X)
	default:
		return false
	}
}

func schematicRouteLabelFallbackPoint(result schematiclayout.Result, netName string, points []kicadfiles.Point) *transactions.Point {
	if len(points) < 2 {
		return nil
	}
	netWireIndexes := make([]int, 0, len(result.Wires))
	for index := range result.Wires {
		if result.Wires[index].NetName == netName {
			netWireIndexes = append(netWireIndexes, index)
		}
	}
	type candidate struct {
		point  kicadfiles.Point
		length kicadfiles.IU
		index  int
	}
	var candidates []candidate
	for index := 1; index < len(points); index++ {
		dx := points[index].X - points[index-1].X
		if dx < 0 {
			dx = -dx
		}
		dy := points[index].Y - points[index-1].Y
		if dy < 0 {
			dy = -dy
		}
		length := dx + dy
		if length == 0 {
			continue
		}
		start, end := points[index-1], points[index]
		candidates = append(candidates, candidate{
			point: kicadfiles.Point{
				X: start.X + (end.X-start.X)/2,
				Y: start.Y + (end.Y-start.Y)/2,
			},
			length: length,
			index:  index,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].length != candidates[j].length {
			return candidates[i].length > candidates[j].length
		}
		return candidates[i].index < candidates[j].index
	})
	for candidateIndex := range candidates {
		candidate := &candidates[candidateIndex]
		contacts := 0
		for _, wireIndex := range netWireIndexes {
			wire := result.Wires[wireIndex]
			if schematicPointOnSegment(candidate.point, wire.From, wire.To) {
				contacts++
			}
		}
		if contacts == 1 && schematicRouteLabelPointClear(result, netName, candidate.point) {
			return transactionPoint(&candidates[candidateIndex].point)
		}
	}
	// A dense route can have every segment midpoint occupied by component or
	// text geometry while still exposing a clear orthogonal bend. Preserve the
	// midpoint preference for readability, then use a deterministic interior
	// vertex so a direct-only net retains one explicit KiCad net anchor.
	type vertexCandidate struct {
		point kicadfiles.Point
		span  kicadfiles.IU
		index int
	}
	vertices := make([]vertexCandidate, 0, len(points)-2)
	for index := 1; index < len(points)-1; index++ {
		previous := points[index-1]
		point := points[index]
		next := points[index+1]
		span := schematicManhattanDistance(previous, point) + schematicManhattanDistance(point, next)
		vertices = append(vertices, vertexCandidate{point: point, span: span, index: index})
	}
	sort.SliceStable(vertices, func(i, j int) bool {
		if vertices[i].span != vertices[j].span {
			return vertices[i].span > vertices[j].span
		}
		return vertices[i].index < vertices[j].index
	})
	for candidateIndex := range vertices {
		candidate := &vertices[candidateIndex]
		contact := false
		for _, wireIndex := range netWireIndexes {
			wire := result.Wires[wireIndex]
			if schematicPointOnSegment(candidate.point, wire.From, wire.To) {
				contact = true
				break
			}
		}
		if contact && schematicRouteLabelPointClear(result, netName, candidate.point) {
			return transactionPoint(&vertices[candidateIndex].point)
		}
	}
	if len(vertices) != 0 {
		// KiCad 10 treats a completely label-free routed island as dangling in
		// ERC even when every wire endpoint is geometrically coincident with a
		// pin. A congested route may offer no text-clear point, so retain the
		// most-connected deterministic bend as the final electrical anchor.
		// This is preferable to silently replacing the visible conductor with
		// disconnected endpoint labels.
		return transactionPoint(&vertices[0].point)
	}
	return nil
}

func schematicManhattanDistance(first, second kicadfiles.Point) kicadfiles.IU {
	dx := first.X - second.X
	if dx < 0 {
		dx = -dx
	}
	dy := first.Y - second.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

func schematicRouteLabelPointClear(result schematiclayout.Result, netName string, point kicadfiles.Point) bool {
	box := schematiclayout.TextEstimate(netName, point, 0, 0)
	for _, component := range result.Components {
		body := schematiclayout.TransformRect(schematiclayout.DefaultBodyFor(component), component.Rotation, component.Mirror).Translate(component.PlacedAt)
		if box.Intersects(body) {
			return false
		}
		for _, text := range []schematiclayout.TextBox{component.ReferenceText, component.ValueText} {
			if !text.Box.Empty() && box.Intersects(text.Box.Translate(component.PlacedAt)) {
				return false
			}
		}
	}
	for _, label := range result.Labels {
		if box.Intersects(schematiclayout.TextEstimateOriented(label.Text, label.Position, label.Rotation, label.JustifyRight)) {
			return false
		}
	}
	for _, wire := range result.Wires {
		if wire.NetName != netName && schematiclayout.SegmentIntersectsRect(wire, box) {
			return false
		}
	}
	return true
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
	component, ok := state.componentsByID[componentID]
	if !ok {
		return transactions.Point{}, false
	}
	for _, pin := range transactionPinsWithLibraryIndex(component, state.libraryIndex) {
		if pin.Number != pinNumber {
			continue
		}
		anchor := schematic.CanonicalConnectionAnchor(
			kicadfiles.Point{X: kicadfiles.MM(origin.XMM), Y: kicadfiles.MM(origin.YMM)},
			kicadfiles.Point{X: kicadfiles.MM(pin.XMM), Y: kicadfiles.MM(pin.YMM)},
			kicadfiles.Angle(state.rotationByID[componentID]), schematic.SymbolMirror(state.mirrorByID[componentID]),
		)
		return transactions.Point{
			XMM: float64(anchor.X) / float64(kicadfiles.MM(1)),
			YMM: float64(anchor.Y) / float64(kicadfiles.MM(1)),
		}, true
	}
	return transactions.Point{}, false
}

func (state *adapterState) labelPointForEndpoint(netName string, endpoint EndpointRef) (transactions.Point, bool) {
	if point, ok := state.labelsByKey[schematicEndpointLabelKey(netName, endpoint)]; ok {
		candidate := transactions.Point{XMM: float64(point.X) / float64(kicadfiles.MM(1)), YMM: float64(point.Y) / float64(kicadfiles.MM(1))}
		if state.validLabelPointForEndpoint(netName, endpoint, &candidate) != nil {
			return candidate, true
		}
	}
	// A rejected or missing layout hint falls back to calibrated pin geometry.
	return state.fallbackLabelPointForEndpoint(netName, endpoint)
}

func (state *adapterState) validLabelPointForEndpoint(netName string, endpoint EndpointRef, point *transactions.Point) *transactions.Point {
	if point == nil {
		return nil
	}
	anchor, ok := state.portEndpointAnchor(endpoint)
	if !ok {
		return nil
	}
	if point.XMM != anchor.XMM && point.YMM != anchor.YMM && state.endpointUsesResolverGeometry(endpoint) {
		return nil
	}
	if state.labelSegmentConflicts(netName, endpoint, anchor, *point) {
		return nil
	}
	return point
}

func (state *adapterState) labelSegmentConflicts(netName string, endpoint EndpointRef, anchor, label transactions.Point) bool {
	return state.labelSegmentTouchesForeignPin(netName, endpoint, anchor, label) || state.labelSegmentTouchesOtherNetLabel(netName, anchor, label)
}

func (state *adapterState) labelSegmentTouchesOtherNetLabel(netName string, anchor, label transactions.Point) bool {
	start := transactionPointToSchematicPoint(anchor)
	end := transactionPointToSchematicPoint(label)
	for _, candidate := range indexedSchematicAnchorsForSegment(state.labelAnchorsByX, state.labelAnchorsByY, start, end) {
		if candidate.netName != netName && indexedPointWithinSegment(candidate.point, start, end) {
			return true
		}
	}
	return false
}

func (state *adapterState) labelSegmentTouchesForeignPin(netName string, endpoint EndpointRef, anchor, label transactions.Point) bool {
	componentID, pinNumber, ok := endpoint.Split()
	if !ok {
		return false
	}
	start := transactionPointToSchematicPoint(anchor)
	end := transactionPointToSchematicPoint(label)
	for _, candidate := range indexedSchematicAnchorsForSegment(state.pinAnchorsByX, state.pinAnchorsByY, start, end) {
		if candidate.componentID == componentID && candidate.pinNumber == pinNumber {
			continue
		}
		if candidate.componentID == componentID && candidate.netName != "" && candidate.netName == netName {
			continue
		}
		if indexedPointWithinSegment(candidate.point, start, end) {
			return true
		}
	}
	return false
}

func (state *adapterState) indexSchematicCollisionAnchors() {
	state.pinAnchorsByX = map[kicadfiles.IU][]indexedSchematicAnchor{}
	state.pinAnchorsByY = map[kicadfiles.IU][]indexedSchematicAnchor{}
	state.labelAnchorsByX = map[kicadfiles.IU][]indexedSchematicAnchor{}
	state.labelAnchorsByY = map[kicadfiles.IU][]indexedSchematicAnchor{}
	pinNets := schematicPinNetsByComponent(state.document.Circuit.Nets)
	for componentID, component := range state.componentsByID {
		origin, ok := state.pointsByID[componentID]
		if !ok {
			continue
		}
		for _, pin := range transactionPinsWithLibraryIndex(component, state.libraryIndex) {
			point := schematic.CanonicalConnectionAnchor(
				kicadfiles.Point{X: kicadfiles.MM(origin.XMM), Y: kicadfiles.MM(origin.YMM)},
				kicadfiles.Point{X: kicadfiles.MM(pin.XMM), Y: kicadfiles.MM(pin.YMM)},
				kicadfiles.Angle(state.rotationByID[componentID]), schematic.SymbolMirror(state.mirrorByID[componentID]),
			)
			indexed := indexedSchematicAnchor{point: point, componentID: componentID, pinNumber: pin.Number, netName: pinNets[componentID][pin.Number]}
			state.pinAnchorsByX[point.X] = append(state.pinAnchorsByX[point.X], indexed)
			state.pinAnchorsByY[point.Y] = append(state.pinAnchorsByY[point.Y], indexed)
		}
	}
	for key, point := range state.labelsByKey {
		netName, _, _ := strings.Cut(key, "\x00")
		indexed := indexedSchematicAnchor{point: point, netName: netName}
		state.labelAnchorsByX[point.X] = append(state.labelAnchorsByX[point.X], indexed)
		state.labelAnchorsByY[point.Y] = append(state.labelAnchorsByY[point.Y], indexed)
	}
}

func schematicPinNetsByComponent(nets []Net) map[string]map[string]string {
	indexed := map[string]map[string]string{}
	for _, net := range nets {
		for _, endpoint := range net.Connect {
			componentID, pinNumber, ok := endpoint.Split()
			if !ok {
				continue
			}
			if indexed[componentID] == nil {
				indexed[componentID] = map[string]string{}
			}
			indexed[componentID][pinNumber] = net.Name
		}
	}
	return indexed
}

func indexedSchematicAnchorsForSegment(byX, byY map[kicadfiles.IU][]indexedSchematicAnchor, start, end kicadfiles.Point) []indexedSchematicAnchor {
	switch {
	case start.X == end.X:
		return byX[start.X]
	case start.Y == end.Y:
		return byY[start.Y]
	default:
		return nil
	}
}

func transactionPointToSchematicPoint(point transactions.Point) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(point.XMM), Y: kicadfiles.MM(point.YMM)}
}

func indexedPointWithinSegment(point, start, end kicadfiles.Point) bool {
	switch {
	case start.X == end.X:
		return betweenSchematicCoordinates(point.Y, start.Y, end.Y)
	case start.Y == end.Y:
		return betweenSchematicCoordinates(point.X, start.X, end.X)
	default:
		return false
	}
}

func betweenSchematicCoordinates(value, start, end kicadfiles.IU) bool {
	if start > end {
		start, end = end, start
	}
	return value >= start && value <= end
}

func (state *adapterState) endpointUsesResolverGeometry(endpoint EndpointRef) bool {
	if state.libraryIndex == nil {
		return false
	}
	componentID, _, ok := endpoint.Split()
	if !ok {
		return false
	}
	component, ok := state.componentsByID[componentID]
	if !ok {
		return false
	}
	record, resolved := libraryresolver.ResolveSymbol(*state.libraryIndex, component.Symbol)
	return resolved && resolverGeometryAuthoritative(record, component.Symbol)
}

func indexComponentsByID(components []Component) map[string]Component {
	indexed := make(map[string]Component, len(components))
	for _, component := range components {
		indexed[component.ID] = component
	}
	return indexed
}

func (state *adapterState) fallbackLabelPointForEndpoint(netName string, endpoint EndpointRef) (transactions.Point, bool) {
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
	for _, distance := range []float64{2.54, 1.27, 0} {
		candidate := anchor
		if math.Abs(dx) >= math.Abs(dy) && dx != 0 {
			candidate.XMM += math.Copysign(distance, dx)
		} else if dy != 0 {
			candidate.YMM += math.Copysign(distance, dy)
		} else {
			candidate.YMM -= distance
		}
		if !state.labelSegmentConflicts(netName, endpoint, anchor, candidate) {
			return candidate, true
		}
	}
	return transactions.Point{}, false
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
	if preference, ok := state.netLabelPreferences[net.Name]; ok && preference {
		return 2
	}
	return 1
}

func schematicNetLabelPreferences(document Document) map[string]bool {
	return schematicNetLabelPreferencesWithLibraryIndex(document, nil)
}

func schematicNetLabelPreferencesWithLibraryIndex(document Document, index *libraryresolver.LibraryIndex) map[string]bool {
	pinsByComponent := make(map[string]map[string]*Pin, len(document.Circuit.Components))
	for componentIndex := range document.Circuit.Components {
		component := &document.Circuit.Components[componentIndex]
		pins := make(map[string]*Pin, len(component.Pins))
		for pinIndex := range component.Pins {
			pins[component.Pins[pinIndex].Number] = &component.Pins[pinIndex]
		}
		pinsByComponent[component.ID] = pins
	}
	drivenPorts := make(map[string]bool, len(document.Circuit.Ports))
	for _, port := range document.Circuit.Ports {
		switch port.Direction {
		case PortDirectionOutput, PortDirectionBidirectional:
			drivenPorts[port.Net] = true
		}
	}
	preferences := make(map[string]bool, len(document.Circuit.Nets))
	for _, net := range document.Circuit.Nets {
		if value, ok := schematicNetLabelPreferenceFor(document, net, pinsByComponent, drivenPorts, index); ok {
			preferences[net.Name] = value
		}
	}
	return preferences
}

func schematicNetLabelPreferenceFor(document Document, net Net, pinsByComponent map[string]map[string]*Pin, drivenPorts map[string]bool, index *libraryresolver.LibraryIndex) (bool, bool) {
	if net.UseLabel != nil {
		return *net.UseLabel, true
	}
	if !document.Policy.Repair.AllowLabelInsertion {
		return false, false
	}
	// KiCad's generated embedded-symbol endpoint convention is not uniformly
	// derivable from a template offset after an uncalibrated instance transform.
	// Prefer explicit local labels for automatic routes touching those symbols
	// until a family has KiCad-backed direct-wire calibration. Callers that
	// explicitly request use_label:false retain that direct-only intent.
	if schematicNetHasUnsafeTransform(document, net, index) {
		return true, true
	}
	// Undriven passive-only nets use local labels instead of relying on a
	// direct wire to establish a KiCad net. This applies to built-in and
	// resolver-backed symbols alike. Explicit use_label:false remains
	// authoritative for callers that intentionally accept that tradeoff.
	needsLabel, complete := schematicNetNeedsLabelForFloatingNet(net, pinsByComponent)
	if complete && needsLabel && !drivenPorts[net.Name] {
		return true, true
	}
	preferLong := document.Layout.Rules.PreferLabelsForLongNets == nil || *document.Layout.Rules.PreferLabelsForLongNets
	if !preferLong {
		return false, false
	}
	switch net.Role {
	case NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround, NetRoleReturn, NetRoleShield, NetRoleBus:
		return true, true
	}
	if len(net.Connect) > 2 {
		return true, true
	}
	return false, false
}

func schematicNetHasUncalibratedTransform(document Document, net Net) bool {
	rotations := layoutRotations(document)
	mirrors := layoutMirrors(document)
	for _, endpoint := range net.Connect {
		componentID, _, ok := endpoint.Split()
		if ok && (rotations[componentID] != 0 || mirrors[componentID] != MirrorNone) {
			return true
		}
	}
	return false
}

// schematicNetHasUnsafeTransform distinguishes unknown endpoint transforms
// from resolver-backed quarter turns and mirrors. Resolver geometry supplies
// the exact KiCad pin offsets that both layout and the writer transform, so it
// can retain a continuous local route. Template-only transformed geometry
// remains conservative and uses endpoint labels.
func schematicNetHasUnsafeTransform(document Document, net Net, index *libraryresolver.LibraryIndex) bool {
	if !schematicNetHasUncalibratedTransform(document, net) {
		return false
	}
	if index == nil {
		return true
	}
	components := indexComponentsByID(document.Circuit.Components)
	rotations := layoutRotations(document)
	mirrors := layoutMirrors(document)
	for _, endpoint := range net.Connect {
		componentID, _, ok := endpoint.Split()
		if !ok || (rotations[componentID] == 0 && mirrors[componentID] == MirrorNone) {
			continue
		}
		component, exists := components[componentID]
		if !exists {
			return true
		}
		record, resolved := libraryresolver.ResolveSymbol(*index, component.Symbol)
		if !resolved || !resolverGeometryAuthoritative(record, component.Symbol) {
			return true
		}
	}
	return false
}

// KiCad reports a wire-only subgraph with no declared driver as a floating
// wire. An undriven net is electrically meaningful, but local labels give
// it a named connection context without inventing an active pin. This applies
// to built-in and resolver-backed symbols alike. Explicit use_label:false
// remains authoritative for callers that intentionally accept that KiCad ERC
// tradeoff.
func schematicNetNeedsLabelForFloatingNet(net Net, pinsByComponent map[string]map[string]*Pin) (bool, bool) {
	if len(net.Connect) < 2 {
		return false, false
	}
	seenPin := false
	cachedComponentID := ""
	var cachedPins map[string]*Pin
	for _, endpoint := range net.Connect {
		componentID, pinNumber, ok := endpoint.Split()
		if !ok {
			return false, false
		}
		if componentID != cachedComponentID {
			cachedComponentID = componentID
			cachedPins = pinsByComponent[componentID]
		}
		pins := cachedPins
		if pins == nil {
			return false, false
		}
		found := pins[pinNumber]
		if found == nil {
			return false, false
		}
		seenPin = true
		switch found.Role {
		case PinRolePower, PinRoleGround:
			// Power and ground pins follow the net-role policy below; they are
			// never classified as passive floating endpoints.
			return false, true
		case PinRoleOutput, PinRoleBidirectional:
			return false, true
		}
	}
	// Ports are checked by the caller's document-level preference function.
	return seenPin, true
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

type schematicLabelPointKey struct {
	x kicadfiles.IU
	y kicadfiles.IU
}

func newSchematicLabelPointKey(point kicadfiles.Point) schematicLabelPointKey {
	return schematicLabelPointKey{x: point.X, y: point.Y}
}

func layoutRouteHints(result schematiclayout.Result, conflicts map[schematicLabelPointKey]struct{}) map[string]schematiclayout.RoutedConnection {
	hints := make(map[string]schematiclayout.RoutedConnection, len(result.Connections))
	for _, connection := range result.Connections {
		hint := connection
		if hint.FromLabelAt != nil {
			if _, conflict := conflicts[newSchematicLabelPointKey(*hint.FromLabelAt)]; conflict {
				hint.FromLabelAt = nil
			}
		}
		if hint.ToLabelAt != nil {
			if _, conflict := conflicts[newSchematicLabelPointKey(*hint.ToLabelAt)]; conflict {
				hint.ToLabelAt = nil
			}
		}
		from := EndpointRef(hint.From.Ref + "." + hint.From.Pin)
		to := EndpointRef(hint.To.Ref + "." + hint.To.Pin)
		hints[schematicRouteKey(hint.NetName, from, to)] = hint
	}
	return hints
}

func layoutEndpointLabelHints(result schematiclayout.Result, conflicts map[schematicLabelPointKey]struct{}) map[string]kicadfiles.Point {
	hints := map[string]kicadfiles.Point{}
	for _, connection := range result.Connections {
		if !connection.UseLabels {
			continue
		}
		if connection.FromLabelAt != nil {
			if _, conflict := conflicts[newSchematicLabelPointKey(*connection.FromLabelAt)]; !conflict {
				endpoint := EndpointRef(connection.From.Ref + "." + connection.From.Pin)
				hints[schematicEndpointLabelKey(connection.NetName, endpoint)] = *connection.FromLabelAt
			}
		}
		if connection.ToLabelAt != nil {
			if _, conflict := conflicts[newSchematicLabelPointKey(*connection.ToLabelAt)]; !conflict {
				endpoint := EndpointRef(connection.To.Ref + "." + connection.To.Pin)
				hints[schematicEndpointLabelKey(connection.NetName, endpoint)] = *connection.ToLabelAt
			}
		}
	}
	return hints
}

func layoutCrossNetLabelPoints(result schematiclayout.Result) map[schematicLabelPointKey]struct{} {
	netsByPoint := make(map[schematicLabelPointKey]string, len(result.Connections)*2)
	conflicts := make(map[schematicLabelPointKey]struct{}, len(result.Connections))
	add := func(point *kicadfiles.Point, netName string) {
		if point == nil {
			return
		}
		key := newSchematicLabelPointKey(*point)
		firstNet, ok := netsByPoint[key]
		if !ok {
			netsByPoint[key] = netName
			return
		}
		if firstNet != netName {
			conflicts[key] = struct{}{}
		}
	}
	for _, connection := range result.Connections {
		if !connection.UseLabels {
			continue
		}
		add(connection.FromLabelAt, connection.NetName)
		add(connection.ToLabelAt, connection.NetName)
	}
	return conflicts
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
	return schematicLayoutWithLibraryIndexAndPreferences(document, index, schematicNetLabelPreferencesWithLibraryIndex(document, index))
}

func schematicLayoutWithLibraryIndexAndPreferences(document Document, index *libraryresolver.LibraryIndex, netLabelPreferences map[string]bool) schematiclayout.Result {
	rotationByID := layoutRotations(document)
	mirrorByID := layoutMirrors(document)
	groupsByID := map[string]Group{}
	for _, group := range document.Layout.Groups {
		groupsByID[group.ID] = group
	}
	placementsByID := map[string]Placement{}
	for _, placement := range document.Layout.Placements {
		placementsByID[placement.Target] = placement
	}
	componentsByID := indexComponentsByID(document.Circuit.Components)
	powerFlagNeighbors := schematicPowerFlagNeighbors(document, componentsByID)
	ordinalByID := schematicLayoutOrdinals(document)
	netOrdinalByName := schematicLayoutNetOrdinals(document.Circuit.Nets)
	rules := schematiclayout.DefaultRules(schematiclayout.ProfileStandard)
	if document.Layout.Rules.MinComponentSpacingMM != nil {
		rules.MinComponentSpacing = kicadfiles.MM(effectiveMinComponentSpacingMM(document))
	}
	if document.Layout.Rules.MinGroupSpacingMM != nil {
		rules.MinStageSpacing = kicadfiles.MM(*document.Layout.Rules.MinGroupSpacingMM)
		rules.MinGroupGutter = kicadfiles.MM(*document.Layout.Rules.MinGroupSpacingMM)
	}
	rules.MaxAuxiliaryPerRank = document.Layout.Rules.MaxAuxiliaryPerRank
	rules.ReserveTitleBlock = document.Layout.Rules.ReserveTitleBlock
	rules.OrientEndpointLabels = document.Layout.Rules.OrientEndpointLabels
	if document.Layout.Rules.PreferLabelsForLongNets != nil {
		rules.LabelFallbackEnabled = *document.Layout.Rules.PreferLabelsForLongNets && document.Policy.Repair.AllowLabelInsertion
		rules.LabelFallbackConfigured = true
	}
	request := schematiclayout.Request{
		Sheet:                 schematiclayout.SheetForPaper(document.Metadata.Paper),
		Rules:                 rules,
		MaxComponentsPerSheet: document.Layout.MaxComponentsPerSheet,
	}
	if rules.ReserveTitleBlock {
		request.Sheet = schematiclayout.SheetWithStandardTitleBlock(request.Sheet)
	}
	var invalidPlacementEndpointDiagnostics []schematiclayout.Diagnostic
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
		geometry := schematicLayoutGeometry(component, index)
		flowRank := group.Rank
		rankFixed := group.ID != "" && !group.Inferred
		if schematicComponentIsPowerFlag(component) {
			flowRank = 0
			rankFixed = true
		}
		near := append([]string(nil), placement.Near...)
		if neighbor := powerFlagNeighbors[component.ID]; neighbor != "" && !stringSliceContains(near, neighbor) {
			near = append(near, neighbor)
			sort.Strings(near)
		}
		sameRowAsPin, invalidRowEndpoints := schematicLayoutEndpoints(placement.SameRowAsPin)
		sameColumnAsPin, invalidColumnEndpoints := schematicLayoutEndpoints(placement.SameColumnAsPin)
		for _, invalid := range invalidRowEndpoints {
			invalidPlacementEndpointDiagnostics = append(invalidPlacementEndpointDiagnostics, schematiclayout.Diagnostic{Severity: schematiclayout.SeverityError, Code: "invalid_relative_endpoint", Ref: component.ID, Message: fmt.Sprintf("same_row_as_pin contains malformed endpoint %q", invalid), Repair: "use component.pin endpoint syntax"})
		}
		for _, invalid := range invalidColumnEndpoints {
			invalidPlacementEndpointDiagnostics = append(invalidPlacementEndpointDiagnostics, schematiclayout.Diagnostic{Severity: schematiclayout.SeverityError, Code: "invalid_relative_endpoint", Ref: component.ID, Message: fmt.Sprintf("same_column_as_pin contains malformed endpoint %q", invalid), Repair: "use component.pin endpoint syntax"})
		}
		request.Components = append(request.Components, schematiclayout.Component{
			Ref:             component.ID,
			DisplayRef:      component.Ref,
			Value:           component.Value,
			LibraryID:       component.Symbol,
			Role:            schematicLayoutComponentRole(component),
			GroupID:         group.ID,
			Stage:           schematicStageForGroup(group.Role),
			FlowRank:        flowRank,
			RankFixed:       rankFixed,
			Near:            near,
			Above:           append([]string(nil), placement.Above...),
			RightOf:         append([]string(nil), placement.RightOf...),
			SameRowAs:       append([]string(nil), placement.SameRowAs...),
			SameColumnAs:    append([]string(nil), placement.SameColumnAs...),
			SameRowAsPin:    sameRowAsPin,
			SameColumnAsPin: sameColumnAsPin,
			CenterBetween:   append([]string(nil), placement.CenterBetween...),
			Rotation:        kicadfiles.Angle(rotationByID[component.ID]),
			Mirror:          schematiclayout.Mirror(mirrorByID[component.ID]),
			Body:            geometry.Body,
			BodyKnown:       geometry.known(),
			GeometrySource:  geometry.Source,
			Pins:            schematicLayoutPins(component, index),
			OriginalOrdinal: ordinalByID[component.ID],
		})
	}
	for _, net := range document.Circuit.Nets {
		if documentNetIsBusMember(document, net.Name) {
			continue
		}
		layoutNet := schematiclayout.Net{Name: net.Name, Role: string(net.Role), OriginalOrdinal: netOrdinalByName[net.Name], PreferDirect: stateDocumentHasPortNet(document, net.Name)}
		if net.UseLabel != nil {
			layoutNet.PreferredLabels = *net.UseLabel
			layoutNet.EndpointLabels = *net.UseLabel
			layoutNet.PreferDirect = !*net.UseLabel
		} else if preference, ok := netLabelPreferences[net.Name]; ok {
			layoutNet.PreferredLabels = preference
			layoutNet.EndpointLabels = preference && schematicNetRequiresEndpointLabels(document, net, index, componentsByID)
			layoutNet.PreferDirect = !preference
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
	result := schematiclayout.Layout(request)
	if document.Policy.Acceptance == AcceptanceReadable && document.Policy.Repair.AllowLabelInsertion && !request.Rules.LabelFallbackEnabled && layoutNeedsLabelRepair(result) {
		// schematiclayout.Layout begins with NormalizeRequest, which deep-copies
		// mutable request slices before sorting or mutating them.
		repairRequest := request
		repairRules := request.Rules
		repairRules.LabelFallbackEnabled = true
		repairRules.LabelFallbackConfigured = true
		repairRequest.Rules = repairRules
		repaired := schematiclayout.Layout(repairRequest)
		if layoutDiagnosticScore(repaired) < layoutDiagnosticScore(result) {
			repaired.Diagnostics = append(repaired.Diagnostics, schematiclayout.Diagnostic{
				Severity: schematiclayout.SeverityInfo,
				Code:     "readability_repair_label_fallback",
				Message:  "readable layout retry enabled deterministic label fallback after direct routing conflicts",
				Repair:   "retain label fallback or provide explicit route hints",
			})
			result = repaired
			request = repairRequest
		}
	}
	if document.Policy.Acceptance == AcceptanceReadable && document.Policy.Repair.AllowGroupSpacingAdjustment && layoutNeedsSpacingRepair(result) {
		// Dense generated stages can leave no collision-free outward stub for a
		// boundary label even though the topology and symbol geometry are
		// otherwise valid. Search a small, deterministic sequence of spacing
		// increments and retain the smallest candidate with the best diagnostic
		// score. Page selection remains delegated to schematiclayout.Layout, so
		// a larger sheet is used only when the repaired bounds require it.
		best := result
		bestStep := 0
		minorGrid := request.Rules.MinorGrid
		if minorGrid <= 0 {
			minorGrid = kicadfiles.MM(1.27)
		}
		maxRepairPasses := request.Rules.MaxSpacingRepairs
		if len(request.Components) > request.Rules.FullRepairComponents && maxRepairPasses > 1 {
			maxRepairPasses = 1
		}
		for step := 1; step <= maxRepairPasses; step++ {
			repairRequest := request
			repairRules := request.Rules
			repairRules.MinComponentSpacing += minorGrid * kicadfiles.IU(2*step)
			repairRules.MinStageSpacing += minorGrid * kicadfiles.IU(4*step)
			repairRules.MinGroupGutter += minorGrid * kicadfiles.IU(2*step)
			repairRequest.Rules = repairRules
			candidate := schematiclayout.Layout(repairRequest)
			if layoutDiagnosticScore(candidate) < layoutDiagnosticScore(best) {
				best = candidate
				bestStep = step
			}
			if !layoutNeedsSpacingRepair(candidate) {
				if layoutDiagnosticScore(candidate) <= layoutDiagnosticScore(best) {
					best = candidate
					bestStep = step
				}
				break
			}
		}
		if bestStep > 0 {
			best.Diagnostics = append(best.Diagnostics, schematiclayout.Diagnostic{
				Severity: schematiclayout.SeverityInfo,
				Code:     "readability_repair_spacing",
				Message:  fmt.Sprintf("readable layout retry increased deterministic component spacing by %.2f mm", float64(minorGrid*kicadfiles.IU(2*bestStep))/1_000_000),
				Repair:   "retain topology groups and collision-free boundary label clearance",
			})
			result = best
		}
	}
	if len(invalidPlacementEndpointDiagnostics) != 0 {
		result.Diagnostics = append(result.Diagnostics, invalidPlacementEndpointDiagnostics...)
		result = schematiclayout.NormalizeResult(result, rules)
	}
	return result
}

func schematicNetRequiresEndpointLabels(document Document, net Net, index *libraryresolver.LibraryIndex, components map[string]Component) bool {
	if stateDocumentHasPortNet(document, net.Name) {
		return true
	}
	for _, endpoint := range net.Connect {
		componentID, _, ok := endpoint.Split()
		if !ok {
			return true
		}
		switch components[componentID].Role {
		case ComponentRoleConnector, ComponentRoleInputConnector, ComponentRoleOutputConnector:
			return true
		}
	}
	if schematicNetHasUnsafeTransform(document, net, index) {
		return true
	}
	// Direct routes are only trustworthy when their final KiCad symbol
	// geometry and anchors came from the resolver. Conservative/template-only
	// geometry retains the old endpoint-label fallback rather than drawing a
	// conductor through a symbol body.
	if index == nil {
		return true
	}
	for _, endpoint := range net.Connect {
		componentID, _, ok := endpoint.Split()
		if !ok {
			return true
		}
		component, exists := components[componentID]
		if !exists {
			return true
		}
		geometry := schematicLayoutGeometry(component, index)
		switch geometry.Source {
		case schematiclayout.GeometrySourceExplicitBody,
			schematiclayout.GeometrySourceExplicitPinEnvelope,
			schematiclayout.GeometrySourceResolverGraphics,
			schematiclayout.GeometrySourceResolverPinEnvelope,
			schematiclayout.GeometrySourceEmbeddedTemplate:
		default:
			return true
		}
	}
	return false
}

func effectiveMinComponentSpacingMM(document Document) float64 {
	spacing := *document.Layout.Rules.MinComponentSpacingMM
	if document.Policy.Repair.AllowGroupSpacingAdjustment && spacing < DefaultMinComponentSpacingMM {
		return DefaultMinComponentSpacingMM
	}
	return spacing
}

func schematicLayoutOrdinals(document Document) map[string]int {
	groups := append([]Group(nil), document.Layout.Groups...)
	for index := range groups {
		groups[index].Members = append([]string(nil), groups[index].Members...)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Rank != groups[j].Rank {
			return groups[i].Rank < groups[j].Rank
		}
		return groups[i].ID < groups[j].ID
	})
	ordinals := map[string]int{}
	next := 0
	for _, group := range groups {
		for _, member := range group.Members {
			if _, exists := ordinals[member]; exists {
				continue
			}
			ordinals[member] = next
			next++
		}
	}
	var remaining []string
	for _, component := range document.Circuit.Components {
		if _, exists := ordinals[component.ID]; !exists {
			remaining = append(remaining, component.ID)
		}
	}
	sort.Strings(remaining)
	for _, componentID := range remaining {
		ordinals[componentID] = next
		next++
	}
	return ordinals
}

func schematicLayoutNetOrdinals(nets []Net) map[string]int {
	names := make([]string, 0, len(nets))
	seen := map[string]struct{}{}
	for _, net := range nets {
		if _, exists := seen[net.Name]; exists {
			continue
		}
		seen[net.Name] = struct{}{}
		names = append(names, net.Name)
	}
	sort.Strings(names)
	ordinals := make(map[string]int, len(names))
	for index, name := range names {
		ordinals[name] = index
	}
	return ordinals
}

func schematicLayoutComponentRole(component Component) string {
	if schematicComponentIsPowerFlag(component) {
		// ERC power flags are boundary annotations, not functional rail blocks.
		// Keep them in the connector flow so they remain close to the rail
		// entry instead of stretching the drawing to the top or bottom margin.
		return "boundary_annotation"
	}
	if component.Role != ComponentRolePowerSymbol {
		return string(component.Role)
	}
	value := strings.ToLower(strings.TrimSpace(component.Value + " " + component.Symbol))
	switch {
	case strings.Contains(value, "gnd"), strings.Contains(value, "ground"):
		return "ground"
	case strings.Contains(value, "vee"), strings.Contains(value, "vss"), strings.HasPrefix(strings.TrimSpace(component.Value), "-"):
		return "negative_rail"
	default:
		return "positive_rail"
	}
}

func schematicComponentIsPowerFlag(component Component) bool {
	if component.Role != ComponentRolePowerSymbol && component.Role != ComponentRoleGroundSymbol {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(component.Value), "PWR_FLAG") ||
		strings.EqualFold(strings.TrimSpace(component.Symbol), "power:PWR_FLAG")
}

func schematicPowerFlagNeighbors(document Document, components map[string]Component) map[string]string {
	powerRailComponent := map[string]bool{}
	for _, net := range document.Circuit.Nets {
		switch net.Role {
		case NetRolePower, NetRolePowerPos, NetRolePowerNeg:
		default:
			continue
		}
		for _, endpoint := range net.Connect {
			componentID, _, ok := endpoint.Split()
			if ok {
				powerRailComponent[componentID] = true
			}
		}
	}
	neighbors := map[string]string{}
	for _, net := range document.Circuit.Nets {
		var flags, candidates []string
		for _, endpoint := range net.Connect {
			componentID, _, ok := endpoint.Split()
			if !ok {
				continue
			}
			component, exists := components[componentID]
			if !exists {
				continue
			}
			if schematicComponentIsPowerFlag(component) {
				flags = append(flags, componentID)
				continue
			}
			switch component.Role {
			case ComponentRoleConnector, ComponentRoleInputConnector, ComponentRoleOutputConnector:
				candidates = append(candidates, componentID)
			}
		}
		if len(flags) == 0 || len(candidates) == 0 {
			continue
		}
		sort.Strings(flags)
		sort.Strings(candidates)
		sort.SliceStable(candidates, func(i, j int) bool {
			leftPower, rightPower := powerRailComponent[candidates[i]], powerRailComponent[candidates[j]]
			if leftPower != rightPower {
				return leftPower
			}
			leftRole, rightRole := components[candidates[i]].Role, components[candidates[j]].Role
			leftInput := leftRole == ComponentRoleInputConnector || leftRole == ComponentRoleConnector
			rightInput := rightRole == ComponentRoleInputConnector || rightRole == ComponentRoleConnector
			if leftInput != rightInput {
				return leftInput
			}
			return candidates[i] < candidates[j]
		})
		for _, flag := range flags {
			neighbors[flag] = candidates[0]
		}
	}
	return neighbors
}

func layoutNeedsLabelRepair(result schematiclayout.Result) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity != schematiclayout.SeverityError && diagnostic.Severity != schematiclayout.SeverityWarning {
			continue
		}
		switch diagnostic.Code {
		case schematiclayout.DiagnosticWireCrossing, schematiclayout.DiagnosticWireSymbolOverlap, schematiclayout.DiagnosticWirePinOverlap, schematiclayout.DiagnosticTextWireOverlap:
			return true
		}
	}
	return false
}

func layoutNeedsSpacingRepair(result schematiclayout.Result) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity != schematiclayout.SeverityError && diagnostic.Severity != schematiclayout.SeverityWarning {
			continue
		}
		switch diagnostic.Code {
		case "label_placement_fallback", "label_overlap", "symbol_overlap", "text_symbol_overlap",
			"text_placement_fallback", schematiclayout.DiagnosticTextWireOverlap,
			schematiclayout.DiagnosticWireSymbolOverlap:
			return true
		}
	}
	return false
}

func layoutDiagnosticScore(result schematiclayout.Result) int {
	score := 0
	for _, diagnostic := range result.Diagnostics {
		switch diagnostic.Severity {
		case schematiclayout.SeverityError:
			score += 100
		case schematiclayout.SeverityWarning:
			score++
		}
	}
	return score
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

type layoutGeometry struct {
	Body   schematiclayout.Rect
	Source schematiclayout.GeometrySource
}

func (geometry layoutGeometry) known() bool {
	return geometry.Source != schematiclayout.GeometrySourceUnknown && geometry.Source != schematiclayout.GeometrySourceConservative
}

func schematicLayoutBody(component Component, index *libraryresolver.LibraryIndex) schematiclayout.Rect {
	return schematicLayoutGeometry(component, index).Body
}

func schematicLayoutBodyKnown(component Component, index *libraryresolver.LibraryIndex) bool {
	return schematicLayoutGeometry(component, index).known()
}

func schematicLayoutGeometry(component Component, index *libraryresolver.LibraryIndex) layoutGeometry {
	if component.Body != nil {
		return layoutGeometry{Body: schematiclayout.Rect{
			MinX: kicadfiles.MM(component.Body.MinXMM),
			MinY: kicadfiles.MM(component.Body.MinYMM),
			MaxX: kicadfiles.MM(component.Body.MaxXMM),
			MaxY: kicadfiles.MM(component.Body.MaxYMM),
		}, Source: schematiclayout.GeometrySourceExplicitBody}
	}
	if index == nil {
		// The embedded template is the writer's exact symbol body when no
		// external resolver is present. Use the same verified geometry during
		// layout so labels and routes cannot be accepted against a smaller
		// fallback envelope and then overlap the serialized body.
		if bounds, ok := schematic.EmbeddedSymbolBodyBounds(component.Symbol); ok {
			return layoutGeometry{Body: schematiclayout.Rect{
				MinX: bounds.Min.X,
				MinY: bounds.Min.Y,
				MaxX: bounds.Max.X,
				MaxY: bounds.Max.Y,
			}, Source: schematiclayout.GeometrySourceEmbeddedTemplate}
		}
		if _, known := schematic.EmbeddedSymbolTemplate(component.Symbol); known {
			return layoutGeometry{Source: schematiclayout.GeometrySourceConservative}
		}
		return fallbackComponentGeometry(component)
	}
	record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol)
	if !ok {
		if bounds, embedded := schematic.EmbeddedSymbolBodyBounds(component.Symbol); embedded {
			return layoutGeometry{Body: schematiclayout.Rect{
				MinX: bounds.Min.X,
				MinY: bounds.Min.Y,
				MaxX: bounds.Max.X,
				MaxY: bounds.Max.Y,
			}, Source: schematiclayout.GeometrySourceEmbeddedTemplate}
		}
		return fallbackComponentGeometry(component)
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
		// Line-bodied units (commonly package power units) are legitimate
		// symbol geometry, but a zero-area rectangle is treated as empty by
		// route scoring. Give only the degenerate axis the normal symbol
		// padding so layout cannot route through geometry that readback later
		// reconstructs and rejects.
		if bounds.MinX == bounds.MaxX {
			bounds.MinX -= defaultComponentPadding
			bounds.MaxX += defaultComponentPadding
		}
		if bounds.MinY == bounds.MaxY {
			bounds.MinY -= defaultComponentPadding
			bounds.MaxY += defaultComponentPadding
		}
		return layoutGeometry{Body: bounds, Source: schematiclayout.GeometrySourceResolverGraphics}
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
		return fallbackComponentGeometry(component)
	}
	// A pin-only unit can still occupy the span between its pins and the
	// symbol origin (multi-unit package power sections are a common example).
	// Include the origin so route planning uses the same conservative body
	// that schematic readback reconstructs.
	pinBounds = unionLayoutRect(pinBounds, schematiclayout.Rect{})
	padding := defaultComponentPadding
	pinBounds.MinX -= padding
	pinBounds.MinY -= padding
	pinBounds.MaxX += padding
	pinBounds.MaxY += padding
	return layoutGeometry{Body: pinBounds, Source: schematiclayout.GeometrySourceResolverPinEnvelope}
}

func resolverPinOffset(index libraryresolver.LibraryIndex, component Component, number string) (kicadfiles.Point, bool) {
	record, ok := libraryresolver.ResolveSymbol(index, component.Symbol)
	if !ok || !resolverGeometryAuthoritative(record, component.Symbol) {
		return kicadfiles.Point{}, false
	}
	offset, ok := resolverPinOffsets(record, component)[strings.TrimSpace(number)]
	return offset, ok
}

func resolverGeometryAuthoritative(record libraryresolver.SymbolRecord, symbolID string) bool {
	if strings.TrimSpace(record.Raw) != "" {
		return true
	}
	_, hasEmbeddedGeometry := schematic.EmbeddedSymbolConnectionPinOffsets(symbolID)
	return !hasEmbeddedGeometry
}

func resolverPinOffsets(record libraryresolver.SymbolRecord, component Component) map[string]kicadfiles.Point {
	unit := maxUnit(componentUnitOrZero(component))
	offsets := map[string]kicadfiles.Point{}
	common := map[string]kicadfiles.Point{}
	for _, pin := range record.Pins {
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			continue
		}
		if pin.Unit == unit {
			for _, member := range libraryresolver.GroupedPinMembers(number) {
				offsets[member] = pin.Position
			}
			continue
		}
		if pin.Unit == 0 {
			for _, member := range libraryresolver.GroupedPinMembers(number) {
				common[member] = pin.Position
			}
		}
	}
	for number, offset := range common {
		if _, exists := offsets[number]; !exists {
			offsets[number] = offset
		}
	}
	return offsets
}

func fallbackComponentGeometry(component Component) layoutGeometry {
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
		return layoutGeometry{Body: schematiclayout.Rect{MinX: -defaultComponentPadding, MinY: -defaultComponentPadding, MaxX: defaultComponentPadding, MaxY: defaultComponentPadding}, Source: schematiclayout.GeometrySourceConservative}
	}
	padding := defaultComponentPadding
	bounds.MinX -= padding
	bounds.MinY -= padding
	bounds.MaxX += padding
	bounds.MaxY += padding
	return layoutGeometry{Body: bounds, Source: schematiclayout.GeometrySourceExplicitPinEnvelope}
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
		for _, member := range libraryresolver.GroupedPinMembers(pin.Number) {
			known[member] = struct{}{}
		}
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
	templatePins, _ := schematic.EmbeddedSymbolConnectionPinOffsets(component.Symbol)
	offsets := map[string]kicadfiles.Point{}
	for _, pin := range templatePins {
		offsets[strings.TrimSpace(pin.Number)] = pin.Offset
	}
	directions := schematicLayoutPinDirections(component, index)
	resolverAuthoritative := false
	if index != nil {
		if record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol); ok && resolverGeometryAuthoritative(record, component.Symbol) {
			resolverAuthoritative = true
			for number, position := range resolverPinOffsets(record, component) {
				offsets[number] = position
			}
		}
	}
	if len(component.Pins) == 0 {
		pins := make([]schematiclayout.Pin, 0, len(templatePins))
		for _, pin := range templatePins {
			pins = append(pins, schematiclayout.Pin{Number: pin.Number, At: offsets[pin.Number], Direction: directions[pin.Number]})
		}
		return pins
	}
	pins := make([]schematiclayout.Pin, 0, len(component.Pins))
	for _, pin := range component.Pins {
		number := strings.TrimSpace(pin.Number)
		offset := offsets[number]
		if explicit, ok := explicitPinOffset(pin, offset); ok && !resolverAuthoritative {
			offset = explicit
		}
		pins = append(pins, schematiclayout.Pin{Number: number, Role: roles[number], At: offset, Direction: directions[number]})
	}
	return pins
}

func schematicLayoutEndpoints(refs []EndpointRef) ([]schematiclayout.Endpoint, []EndpointRef) {
	endpoints := make([]schematiclayout.Endpoint, 0, len(refs))
	var invalid []EndpointRef
	for _, ref := range refs {
		componentID, pin, ok := ref.Split()
		if !ok {
			invalid = append(invalid, ref)
			continue
		}
		endpoints = append(endpoints, schematiclayout.Endpoint{Ref: componentID, Pin: pin})
	}
	return endpoints, invalid
}

// schematicLayoutPinDirections retains the raw library pin's outward-facing
// side while connections use KiCad-calibrated physical anchors. KiCad parses
// raw library Y coordinates into schematic space inverted, hence the Y flip.
func schematicLayoutPinDirections(component Component, index *libraryresolver.LibraryIndex) map[string]kicadfiles.Point {
	directions := map[string]kicadfiles.Point{}
	if templatePins, ok := schematic.EmbeddedSymbolPinOffsets(component.Symbol); ok {
		for _, pin := range templatePins {
			if pin.Direction.X != 0 || pin.Direction.Y != 0 {
				directions[strings.TrimSpace(pin.Number)] = pin.Direction
			}
		}
	}
	if index == nil {
		return directions
	}
	record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol)
	if !ok || !resolverGeometryAuthoritative(record, component.Symbol) {
		return directions
	}
	unit := componentUnitOrZero(component)
	for _, pin := range record.Pins {
		if pin.Unit != 0 && pin.Unit != maxUnit(unit) {
			continue
		}
		pinNumber := strings.TrimSpace(pin.Number)
		if direction, known := schematic.PinDirectionFromOrientation(pin.Orientation); known {
			directions[pinNumber] = direction
		}
	}
	return directions
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

func layoutMirrors(document Document) map[string]Mirror {
	mirrors := map[string]Mirror{}
	for _, placement := range document.Layout.Placements {
		if placement.Target != "" {
			mirrors[placement.Target] = placement.Mirror
		}
	}
	return mirrors
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
	resolverAuthoritative := false
	if templatePins, ok := schematic.EmbeddedSymbolConnectionPinOffsets(component.Symbol); ok {
		for _, pin := range templatePins {
			offsets[strings.TrimSpace(pin.Number)] = pin.Offset
		}
	}
	if index != nil {
		if record, ok := libraryresolver.ResolveSymbol(*index, component.Symbol); ok && resolverGeometryAuthoritative(record, component.Symbol) {
			resolverAuthoritative = true
			for number, position := range resolverPinOffsets(record, component) {
				offsets[number] = position
			}
		}
	}
	out := make([]transactions.PinSpec, 0, len(component.Pins))
	for _, pin := range component.Pins {
		number := strings.TrimSpace(pin.Number)
		offset := offsets[number]
		explicit, ok := explicitPinOffset(pin, offset)
		if ok && !resolverAuthoritative {
			offset = explicit
		}
		out = append(out, transactions.PinSpec{Number: number, XMM: float64(offset.X) / float64(kicadfiles.MM(1)), YMM: float64(offset.Y) / float64(kicadfiles.MM(1)), ExplicitOffset: ok && !resolverAuthoritative})
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

func schematicAnchorsMatch(left, right kicadfiles.Point) bool {
	return iuWithinOne(left.X, right.X) && iuWithinOne(left.Y, right.Y)
}

func iuWithinOne(left, right kicadfiles.IU) bool {
	// Avoid subtraction here: IU is signed and hostile IR coordinates must not
	// turn an extreme-value difference into an apparent near match.
	if left == right {
		return true
	}
	if left > right {
		return left <= right+1
	}
	return right <= left+1
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
		// AI/catalog provenance and part metadata must remain available in the
		// KiCad file without competing with the human-facing reference and value.
		// Reference and Value are replaced with explicitly positioned visible
		// properties by transactionSymbolPropertiesWithLayout below.
		properties = append(properties, transactions.SymbolProperty{Name: trimmedKey, Value: component.Properties[key], Hidden: true})
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

func transactionSymbolPropertiesWithLayout(component Component, reference string, layout layoutTextPlacement, symbolRotation float64, hideValue bool) []transactions.SymbolProperty {
	properties := transactionSymbolProperties(component)
	// KiCad property angles are relative to the parent symbol transform. Cancel
	// quarter-turn symbol rotation so generated reference and value fields stay
	// horizontal and readable on the finished sheet.
	rotation := math.Mod(360-symbolRotation, 360)
	if rotation < 0 {
		rotation += 360
	}
	doNotAutoplace := true
	hideReference := component.Role == ComponentRolePowerSymbol || component.Role == ComponentRoleGroundSymbol
	visible := []transactions.SymbolProperty{
		{Name: "Reference", Value: reference, Hidden: hideReference, At: layout.reference, Rotation: &rotation, DoNotAutoplace: &doNotAutoplace},
	}
	value := component.Value
	hiddenValue := hideValue
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
