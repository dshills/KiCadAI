package designapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"path"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/library"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/routing"
)

// Builder accumulates a KiCad design through ordered mutations. It is not
// safe for concurrent use by multiple goroutines.
type Builder struct {
	name              string
	generator         kicadfiles.DeterministicIDGenerator
	design            kicaddesign.Design
	libraryIndex      *libraryresolver.LibraryIndex
	resolverSymbolIDs map[string]struct{}
	nets              *pcb.NetRegistry
	netParents        map[string]string
	symbols           map[string]*symbolState
	symbolUnits       map[string][]*symbolState
	symbolKeys        map[string]string
	// schematicPinAnchors tracks generated schematic symbol pin coordinates so
	// grid snapping can avoid creating exact anchor collisions between symbols.
	schematicPinAnchors map[kicadfiles.Point]struct{}
	schematicWireEnds   map[kicadfiles.Point]struct{}
	footprints          map[string]int
	pads                map[string]map[string][]int
	routeViaCounts      map[string]int
	hierarchy           *SchematicHierarchy
}

type Options struct {
	Name         string
	DesignID     kicadfiles.UUID
	Seed         string
	Paper        kicadfiles.Paper
	LibraryIndex *libraryresolver.LibraryIndex
}

type SymbolHandle struct {
	Reference string
}

type FootprintHandle struct {
	Reference string
}

type RouteHandle struct {
	NetName string
	Count   int
}

type ZoneHandle struct {
	Name string
}

type BoardOutlineHandle struct {
	SegmentCount int
}

type SymbolOptions struct {
	Reference string
	Unit      int
	Role      string
	Value     string
	LibraryID string
	Position  kicadfiles.Point
	Rotation  kicadfiles.Angle
	Mirror    schematic.SymbolMirror
	Pins      []PinSpec
	// UsePhysicalConnectionAnchors applies KiCad-calibrated template anchors
	// to a transaction-derived pin list. Direct callers keep supplied offsets.
	UsePhysicalConnectionAnchors bool
	Properties                   []schematic.Property
	PreferResolverSymbol         bool
}

type PinSpec struct {
	Number string
	Offset kicadfiles.Point
}

type Endpoint struct {
	Reference string
	Pin       string
	Unit      int
}

// SchematicHierarchy describes child-sheet emission for an oversized
// schematic. References are the emitted KiCad references, not IR component
// IDs, so the builder can apply the hierarchy after symbol creation.
type SchematicHierarchy struct {
	Sheets         []SchematicSheet         `json:"sheets"`
	CrossSheetNets []SchematicCrossSheetNet `json:"cross_sheet_nets,omitempty"`
	Buses          []SchematicBus           `json:"buses,omitempty"`
}

type SchematicSheet struct {
	ID         string
	Name       string
	Filename   string
	References []string
	Symbols    []SchematicSymbolRef
}

type SchematicSymbolRef struct {
	Reference string
	Unit      int
}

type SchematicCrossSheetNet struct {
	Name      string
	Endpoints []Endpoint
}

type SchematicBus struct {
	ID      string
	Name    string
	Points  []kicadfiles.Point
	Entries []SchematicBusEntry
}

type SchematicBusEntry struct {
	Member   string
	Label    string
	Endpoint Endpoint
	At       kicadfiles.Point
	Size     kicadfiles.Point
}

type ConnectOptions struct {
	// UseLabels forces label stubs when true and direct orthogonal wiring when
	// false. Nil preserves the legacy distance- and net-name-based behavior.
	UseLabels *bool
	// SuppressBendLabels prevents automatic labels at orthogonal route bends
	// when the caller explicitly chose local wiring.
	SuppressBendLabels bool
	// SkipFromLabel and SkipToLabel let a caller replace one endpoint's local
	// label with a global or hierarchical label at the same anchor.
	SkipFromLabel bool
	SkipToLabel   bool
	// Waypoints contains the complete absolute orthogonal path, including the
	// source and destination anchors. It is ignored when labels are forced.
	Waypoints   []kicadfiles.Point
	FromLabelAt *kicadfiles.Point
	ToLabelAt   *kicadfiles.Point
}

type LabelOptions struct {
	Rotation kicadfiles.Angle
	Shape    schematic.LabelShape
}

type PlaceFootprintOptions struct {
	Position                      kicadfiles.Point
	Rotation                      kicadfiles.Angle
	Layer                         kicadfiles.BoardLayer
	Description                   string
	Tags                          string
	Attributes                    []string
	MetadataProperties            []pcb.FootprintMetadataProperty
	Properties                    []pcb.FootprintProperty
	Texts                         []pcb.FootprintText
	Graphics                      []pcb.FootprintGraphic
	Models                        []pcb.Model3D
	Pads                          []PadSpec
	AllowUnmatchedUnconnectedPads bool
	// HideDefaultFootprintText hides generated KiCad Reference and Value properties.
	HideDefaultFootprintText bool
}

type PadSpec struct {
	Name   string
	Type   string
	Offset kicadfiles.Point
	Size   kicadfiles.Point
	Drill  kicadfiles.IU
	Shape  string
	Layers []kicadfiles.BoardLayer
	Net    string
}

type RouteOptions struct {
	Layer kicadfiles.BoardLayer
	Width kicadfiles.IU
	Vias  []RouteViaSpec
}

type RouteViaSpec struct {
	At     kicadfiles.Point
	Size   kicadfiles.IU
	Drill  kicadfiles.IU
	Layers []kicadfiles.BoardLayer
}

type RouteBoardOptions struct {
	DryRun bool
}

type ZoneOptions struct {
	Name                 string
	Layers               []kicadfiles.BoardLayer
	ConnectPads          bool
	Clearance            kicadfiles.IU
	MinThickness         kicadfiles.IU
	ThermalGap           kicadfiles.IU
	ThermalBridgeWidth   kicadfiles.IU
	FilledAreasThickness bool
	Priority             int
}

type symbolState struct {
	symbolIndex                  int
	reference                    string
	unit                         int
	libraryID                    string
	position                     kicadfiles.Point
	rotation                     kicadfiles.Angle
	mirror                       schematic.SymbolMirror
	usePhysicalConnectionAnchors bool
	pins                         map[string]kicadfiles.Point
	pinOrder                     []string
	pinNets                      map[string]string
	footprintID                  string
}

const schematicConnectionGrid = kicadfiles.IU(1270000)
const usbCPowerOnlyConnectorLibraryID = "kicadai:USB_C_Receptacle_PowerOnly_6P"
const usbCPowerOnlyFullConnectorLibraryID = "kicadai:usb_c_receptacle_poweronly_full"

func New(options Options) (*Builder, error) {
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = "generated_design"
	}
	seed := strings.TrimSpace(options.Seed)
	if seed == "" {
		seed = name
	}
	generator, err := kicadfiles.NewDeterministicIDGenerator(options.DesignID, seed)
	if err != nil {
		return nil, err
	}
	paper := options.Paper
	if strings.TrimSpace(paper.Name) == "" {
		paper.Name = "A4"
	}
	builder := &Builder{
		name:                name,
		generator:           generator,
		libraryIndex:        options.LibraryIndex,
		resolverSymbolIDs:   map[string]struct{}{},
		nets:                pcb.NewNetRegistry(),
		netParents:          map[string]string{},
		symbols:             map[string]*symbolState{},
		symbolUnits:         map[string][]*symbolState{},
		symbolKeys:          map[string]string{},
		schematicPinAnchors: map[kicadfiles.Point]struct{}{},
		schematicWireEnds:   map[kicadfiles.Point]struct{}{},
		footprints:          map[string]int{},
		pads:                map[string]map[string][]int{},
		routeViaCounts:      map[string]int{},
	}
	builder.design = kicaddesign.Design{
		Name: name,
		Project: project.ProjectFile{
			Name:          name,
			DesignID:      options.DesignID,
			FormatVersion: kicadfiles.KiCadFormatV20260306,
			Generator:     "kicadai",
			PageSettings:  project.PageSettings{Paper: paper},
			NetClasses: []project.NetClass{{
				Name:        "Default",
				Clearance:   kicadfiles.MM(0.2),
				TrackWidth:  kicadfiles.MM(0.25),
				ViaDiameter: kicadfiles.MM(0.8),
				ViaDrill:    kicadfiles.MM(0.4),
			}},
		},
		Schematic: &schematic.SchematicFile{
			Filename:         name + ".kicad_sch",
			Version:          kicadfiles.KiCadSchematicFormatV20260306,
			Generator:        "eeschema",
			GeneratorVersion: "10.0",
			UUID:             generator.New("root.schematic"),
			Paper:            paper,
			SheetInstances:   []schematic.SheetInstance{{Project: name, Path: "/", Page: "1"}},
		},
		PCB: &pcb.PCBFile{
			Version:          kicadfiles.KiCadPCBFormatV20260206,
			Generator:        "pcbnew",
			GeneratorVersion: "10.0",
			General:          pcb.DefaultGeneral(),
			Paper:            paper,
			Layers:           pcb.DefaultTwoLayerStack(),
			Setup:            pcb.DefaultSetup(),
			Nets:             builder.nets.Nets(),
			TitleBlock:       kicadfiles.TitleBlock{Title: name},
		},
	}
	return builder, nil
}

func (builder *Builder) AddSymbol(options SymbolOptions) (SymbolHandle, error) {
	if builder == nil {
		return SymbolHandle{}, fmt.Errorf("builder required")
	}
	reference := strings.TrimSpace(options.Reference)
	if reference == "" {
		return SymbolHandle{}, fmt.Errorf("reference required")
	}
	unit := options.Unit
	if unit <= 0 {
		unit = 1
	}
	stateKey := symbolStateKey(reference, unit)
	if _, ok := builder.symbols[stateKey]; ok {
		return SymbolHandle{}, fmt.Errorf("symbol %s unit %d already exists", reference, unit)
	}
	if states := builder.symbolUnits[referenceKey(reference)]; len(states) > 0 && states[0].reference != reference {
		return SymbolHandle{}, fmt.Errorf("multi-unit reference casing must match %s", states[0].reference)
	}
	if existing, ok := builder.symbolKeys[stateKey]; ok {
		return SymbolHandle{}, fmt.Errorf("symbol %s collides with %s after KiCad key normalization", reference, existing)
	}
	generationKey := symbolGenerationKey(reference, unit)
	libraryID := strings.TrimSpace(options.LibraryID)
	if libraryID == "" {
		return SymbolHandle{}, fmt.Errorf("library id required")
	}
	libraryID = schematic.CanonicalEmbeddedSymbolLibraryID(libraryID)
	if err := builder.ensureSchematicLibrarySymbol(libraryID, options.PreferResolverSymbol); err != nil {
		return SymbolHandle{}, err
	}
	rotation, mirror := schematic.CanonicalSymbolTransform(options.Rotation, options.Mirror)
	value := strings.TrimSpace(options.Value)
	if value == "" {
		value = reference
	}
	usePhysicalConnectionAnchors := options.UsePhysicalConnectionAnchors || len(options.Pins) == 0
	pinSpecs := symbolPinSpecs(libraryID, options.Pins)
	position := builder.safeSchematicSymbolPosition(options.Position, pinSpecs, rotation, mirror)
	symbol := schematic.NewSymbol(builder.generator.New("root.schematic.symbol", generationKey), libraryID, reference, value, position)
	symbol.Unit = unit
	symbol.Rotation = rotation
	symbol.Mirror = mirror
	symbol.Path = "root.component." + generationKey
	if strings.EqualFold(strings.TrimSpace(options.Role), "generated_terminal") {
		inBOM := false
		onBoard := false
		inPositionFile := false
		symbol.InBOM = &inBOM
		symbol.OnBoard = &onBoard
		symbol.InPositionFile = &inPositionFile
	}
	symbol.Properties = schematic.MergeProperties(symbol.Properties, options.Properties)
	symbol.Pins = make([]schematic.SymbolPin, 0, len(pinSpecs))
	pins := make(map[string]kicadfiles.Point, len(pinSpecs))
	pinNets := make(map[string]string, len(pinSpecs))
	pinOrder := make([]string, 0, len(pinSpecs))
	for _, pin := range pinSpecs {
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			return SymbolHandle{}, fmt.Errorf("pin number required")
		}
		if _, ok := pins[number]; ok {
			return SymbolHandle{}, fmt.Errorf("duplicate pin %s on %s", number, reference)
		}
		anchorOffset := pin.Offset
		if usePhysicalConnectionAnchors {
			if offset, ok := schematic.EmbeddedSymbolConnectionPinOffset(libraryID, number); ok {
				anchorOffset = offset
			}
		}
		anchorOffset = schematic.TransformConnectionAnchor(anchorOffset, rotation, mirror)
		anchor := schematicSymbolPinAnchor(position, anchorOffset)
		pins[number] = anchor
		builder.schematicPinAnchors[anchor] = struct{}{}
		pinOrder = append(pinOrder, number)
		symbol.PinAnchors = append(symbol.PinAnchors, anchor)
		symbol.Pins = append(symbol.Pins, schematic.SymbolPin{
			Number: number,
			UUID:   builder.generator.New("root.schematic.symbol.pin", generationKey, number),
		})
	}
	symbol.Instances = []schematic.SymbolInstance{{
		Project:   builder.design.Project.Name,
		Path:      "/" + string(symbol.UUID),
		Reference: symbol.Reference,
		Unit:      unit,
		Value:     symbol.Value,
	}}
	builder.design.Schematic.Symbols = append(builder.design.Schematic.Symbols, symbol)
	builder.symbols[stateKey] = &symbolState{
		symbolIndex:                  len(builder.design.Schematic.Symbols) - 1,
		reference:                    reference,
		unit:                         unit,
		libraryID:                    libraryID,
		position:                     position,
		rotation:                     rotation,
		mirror:                       mirror,
		usePhysicalConnectionAnchors: usePhysicalConnectionAnchors,
		pins:                         pins,
		pinOrder:                     pinOrder,
		pinNets:                      pinNets,
	}
	refKey := referenceKey(reference)
	states := builder.symbolUnits[refKey]
	insertAt := sort.Search(len(states), func(index int) bool {
		return states[index].unit >= unit
	})
	states = append(states, nil)
	copy(states[insertAt+1:], states[insertAt:])
	states[insertAt] = builder.symbols[stateKey]
	builder.symbolUnits[refKey] = states
	builder.symbolKeys[stateKey] = reference
	builder.addKnownSymbolLibrary(libraryID)
	return SymbolHandle{Reference: reference}, nil
}

func (builder *Builder) ensureSchematicLibrarySymbol(libraryID string, preferResolver bool) error {
	if schematic.EmbeddedSymbolPresent(builder.design.Schematic, libraryID) {
		return nil
	}
	if preferResolver && builder.libraryIndex != nil {
		if record, ok := libraryresolver.ResolveSymbolPtr(builder.libraryIndex, libraryID); ok && strings.TrimSpace(record.Raw) != "" {
			if !schematic.EnsureEmbeddedSymbolFromRaw(builder.design.Schematic, libraryID, record.Raw) {
				return fmt.Errorf("symbol library record has malformed KiCad body: %s", libraryID)
			}
			builder.resolverSymbolIDs[libraryID] = struct{}{}
			return nil
		}
	}
	if schematic.EnsureEmbeddedSymbol(builder.design.Schematic, libraryID) || builder.libraryIndex == nil {
		return nil
	}
	record, ok := libraryresolver.ResolveSymbolPtr(builder.libraryIndex, libraryID)
	if !ok {
		return fmt.Errorf("symbol library record not found: %s", libraryID)
	}
	if !schematic.EnsureEmbeddedSymbolFromRaw(builder.design.Schematic, libraryID, record.Raw) {
		// Pin-only resolver records intentionally rely on the qualified library
		// reference when no bundled template exists.
		if strings.TrimSpace(record.Raw) == "" {
			return nil
		}
		return fmt.Errorf("symbol library record has malformed KiCad body: %s", libraryID)
	}
	builder.resolverSymbolIDs[libraryID] = struct{}{}
	return nil
}

// SetSchematicHierarchy schedules deterministic child-sheet emission during
// the next project write. It must be called after all schematic symbols have
// been added.
func (builder *Builder) SetSchematicHierarchy(hierarchy SchematicHierarchy) error {
	if builder == nil {
		return fmt.Errorf("builder required")
	}
	if len(hierarchy.Sheets) < 2 {
		return fmt.Errorf("schematic hierarchy requires at least two sheets")
	}
	clone := hierarchy
	clone.Sheets = make([]SchematicSheet, len(hierarchy.Sheets))
	for index, sheet := range hierarchy.Sheets {
		clone.Sheets[index] = sheet
		clone.Sheets[index].References = append([]string(nil), sheet.References...)
		clone.Sheets[index].Symbols = append([]SchematicSymbolRef(nil), sheet.Symbols...)
	}
	seenSheets := make(map[string]struct{}, len(hierarchy.Sheets))
	seenFiles := make(map[string]struct{}, len(hierarchy.Sheets))
	seenRefs := make(map[string]string)
	seenLegacyRefs := make(map[string]string)
	for index := range clone.Sheets {
		sheet := &clone.Sheets[index]
		if strings.TrimSpace(sheet.ID) == "" || strings.TrimSpace(sheet.Name) == "" {
			return fmt.Errorf("hierarchy sheet %d requires id and name", index)
		}
		if _, exists := seenSheets[sheet.ID]; exists {
			return fmt.Errorf("duplicate hierarchy sheet %s", sheet.ID)
		}
		seenSheets[sheet.ID] = struct{}{}
		if strings.TrimSpace(sheet.Filename) == "" {
			sheet.Filename = "sch/" + sheet.ID + ".kicad_sch"
		}
		if err := validHierarchyFilename(sheet.Filename); err != nil {
			return err
		}
		fileKey := strings.ToLower(path.Clean(sheet.Filename))
		if _, exists := seenFiles[fileKey]; exists {
			return fmt.Errorf("duplicate hierarchy sheet filename %s", sheet.Filename)
		}
		seenFiles[fileKey] = struct{}{}
		for _, symbol := range hierarchySheetSymbols(*sheet) {
			key := symbolStateKey(symbol.Reference, symbol.Unit)
			if _, exists := seenRefs[key]; exists {
				return fmt.Errorf("reference %s unit %d assigned to multiple hierarchy sheets", symbol.Reference, symbol.Unit)
			}
			seenRefs[key] = sheet.ID
		}
		for _, reference := range sheet.References {
			key := referenceKey(reference)
			if _, exists := seenLegacyRefs[key]; exists {
				return fmt.Errorf("reference %s assigned to multiple hierarchy sheets", reference)
			}
			seenLegacyRefs[key] = sheet.ID
		}
	}
	if builder.design.Schematic == nil {
		return fmt.Errorf("schematic required")
	}
	for _, symbol := range builder.design.Schematic.Symbols {
		if _, ok := seenRefs[symbolStateKey(symbol.Reference, symbol.Unit)]; !ok {
			if _, legacy := seenLegacyRefs[referenceKey(symbol.Reference)]; legacy {
				continue
			}
			return fmt.Errorf("reference %s is not assigned to a hierarchy sheet", symbol.Reference)
		}
	}
	clone.CrossSheetNets = append([]SchematicCrossSheetNet(nil), hierarchy.CrossSheetNets...)
	for index := range clone.CrossSheetNets {
		clone.CrossSheetNets[index].Endpoints = append([]Endpoint(nil), clone.CrossSheetNets[index].Endpoints...)
	}
	builder.hierarchy = &clone
	return nil
}

func symbolPinSpecs(libraryID string, explicit []PinSpec) []PinSpec {
	if len(explicit) > 0 {
		return explicit
	}
	templatePins, ok := schematic.EmbeddedSymbolConnectionPinOffsets(libraryID)
	if !ok {
		return nil
	}
	pins := make([]PinSpec, 0, len(templatePins))
	for _, pin := range templatePins {
		pins = append(pins, PinSpec{Number: pin.Number, Offset: pin.Offset})
	}
	return pins
}

func (builder *Builder) Connect(from, to Endpoint, netName string) error {
	return builder.ConnectWithOptions(from, to, netName, ConnectOptions{})
}

func (builder *Builder) ConnectWithOptions(from, to Endpoint, netName string, options ConnectOptions) error {
	if builder == nil {
		return fmt.Errorf("builder required")
	}
	start, err := builder.pinAnchor(from)
	if err != nil {
		return err
	}
	end, err := builder.pinAnchor(to)
	if err != nil {
		return err
	}
	if options.UseLabels == nil || !*options.UseLabels {
		if err := validateSchematicWaypoints(options.Waypoints, start, end); err != nil {
			return err
		}
	} else {
		if err := validateSchematicLabelPoint(options.FromLabelAt, start); err != nil {
			return err
		}
		if err := validateSchematicLabelPoint(options.ToLabelAt, end); err != nil {
			return err
		}
	}
	fromNet := builder.assignedPinNet(from)
	toNet := builder.assignedPinNet(to)
	netName = strings.TrimSpace(netName)
	if netName == "" {
		switch {
		case fromNet != "" && toNet != "" && fromNet != toNet:
			netName = fromNet
			builder.mergeNet(toNet, netName)
		case fromNet != "":
			netName = fromNet
		case toNet != "":
			netName = toNet
		default:
			netName = "NET_" + referenceKey(from.Reference) + "_" + strings.TrimSpace(from.Pin) + "_" + referenceKey(to.Reference) + "_" + strings.TrimSpace(to.Pin)
		}
	} else {
		if fromNet != "" && fromNet != netName {
			builder.mergeNet(fromNet, netName)
		}
		if toNet != "" && toNet != netName {
			builder.mergeNet(toNet, netName)
		}
	}
	netName = builder.canonicalNet(netName)
	builder.nets.EnsureNet(netName)
	builder.assignPinNet(from, netName)
	builder.assignPinNet(to, netName)
	builder.design.ExpectedNets = appendUniqueNet(builder.design.ExpectedNets, netName)
	if options.UseLabels != nil && *options.UseLabels {
		if !options.SkipFromLabel {
			builder.addSchematicLabelConnection(netName, from, start, end, options.FromLabelAt)
		}
		if !options.SkipToLabel {
			builder.addSchematicLabelConnection(netName, to, end, start, options.ToLabelAt)
		}
	} else if len(options.Waypoints) != 0 {
		builder.addSchematicWirePointsWithOptions(netName, from, to, options.Waypoints, options.SuppressBendLabels)
	} else if options.UseLabels != nil {
		builder.addSchematicWireWithOptions(netName, from, to, start, end, options.SuppressBendLabels)
	} else if builder.schematicConnectionShouldUseDirectLabels(from, to) {
		if err := builder.AddLabel(netName, start, schematic.LabelLocal); err != nil {
			return err
		}
		if err := builder.AddLabel(netName, end, schematic.LabelLocal); err != nil {
			return err
		}
	} else if schematicConnectionShouldUseLabels(netName, start, end) {
		builder.addSchematicLabelStub(netName, from, start, builder.labelStubOffset(from, start, end))
		builder.addSchematicLabelStub(netName, to, end, builder.labelStubOffset(to, end, start))
	} else {
		builder.addSchematicWire(netName, from, to, start, end)
	}
	builder.syncPCBNets()
	return nil
}

func (builder *Builder) addSchematicLabelConnection(netName string, endpoint Endpoint, anchor, other kicadfiles.Point, requested *kicadfiles.Point) {
	if requested == nil {
		builder.addSchematicLabelStub(netName, endpoint, anchor, builder.labelStubOffset(endpoint, anchor, other))
		return
	}
	position := *requested
	if position != anchor {
		builder.addSchematicWirePoints(netName, endpoint, endpoint, []kicadfiles.Point{anchor, position})
	}
	_ = builder.AddLabel(netName, position, schematic.LabelLocal)
}

func (builder *Builder) schematicConnectionShouldUseDirectLabels(from, to Endpoint) bool {
	return builder.endpointIsUSBCPowerOnlyCC(from) || builder.endpointIsUSBCPowerOnlyCC(to)
}

func (builder *Builder) endpointIsUSBCPowerOnlyCC(endpoint Endpoint) bool {
	pin := strings.TrimSpace(endpoint.Pin)
	if !strings.EqualFold(pin, "A5") && !strings.EqualFold(pin, "B5") {
		return false
	}
	state, err := builder.symbolStateForEndpoint(endpoint)
	if err != nil {
		return false
	}
	libraryID := strings.TrimSpace(state.libraryID)
	return strings.EqualFold(libraryID, usbCPowerOnlyConnectorLibraryID) || strings.EqualFold(libraryID, usbCPowerOnlyFullConnectorLibraryID)
}

func schematicConnectionShouldUseLabels(netName string, start, end kicadfiles.Point) bool {
	const generatedIOAliasPrefix = "io_"
	longSchematicWireLabelThreshold := kicadfiles.MM(40)
	if strings.HasPrefix(strings.TrimSpace(netName), generatedIOAliasPrefix) {
		return true
	}
	dx := start.X - end.X
	if dx < 0 {
		dx = -dx
	}
	dy := start.Y - end.Y
	if dy < 0 {
		dy = -dy
	}
	return dx+dy > longSchematicWireLabelThreshold
}

func schematicConnectionSuppressesBendLabels(netName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(netName))
	return strings.HasSuffix(normalized, "_vbus_connector")
}

func (builder *Builder) labelStubOffset(endpoint Endpoint, from, to kicadfiles.Point) kicadfiles.Point {
	if builder != nil {
		if offset, ok := builder.pinAnchorOffset(endpoint); ok {
			grid := kicadfiles.MM(1.27)
			switch {
			case offset.X < 0:
				return kicadfiles.Point{X: -grid}
			case offset.X > 0:
				return kicadfiles.Point{X: grid}
			case offset.Y < 0:
				return kicadfiles.Point{Y: -grid}
			case offset.Y > 0:
				return kicadfiles.Point{Y: grid}
			}
		}
	}
	if to.X >= from.X {
		return kicadfiles.Point{Y: -kicadfiles.MM(1.27)}
	}
	return kicadfiles.Point{Y: kicadfiles.MM(1.27)}
}

func (builder *Builder) AddNoConnect(endpoint Endpoint) error {
	if builder == nil {
		return fmt.Errorf("builder required")
	}
	anchor, err := builder.pinAnchor(endpoint)
	if err != nil {
		return err
	}
	if hasNoConnect(builder.design.Schematic.NoConnects, anchor) {
		return nil
	}
	builder.design.Schematic.NoConnects = append(builder.design.Schematic.NoConnects, schematic.NewNoConnect(
		builder.generator.New("root.schematic.no_connect", endpoint.Reference, endpoint.Pin),
		anchor,
	))
	return nil
}

func (builder *Builder) AddLabel(text string, position kicadfiles.Point, kind schematic.LabelKind) error {
	return builder.AddLabelWithOptions(text, position, kind, LabelOptions{})
}

func (builder *Builder) AddLabelWithOptions(text string, position kicadfiles.Point, kind schematic.LabelKind, options LabelOptions) error {
	if builder == nil {
		return fmt.Errorf("builder required")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("label text required")
	}
	if kind == "" {
		kind = schematic.LabelLocal
	}
	if hasSchematicLabel(builder.design.Schematic.Labels, text, position) {
		return nil
	}
	label := schematic.NewLabel(
		builder.generator.New("root.schematic.label", text, formatPoint(position)),
		text,
		kind,
		position,
	)
	label.Rotation = options.Rotation
	label.Shape = options.Shape
	builder.design.Schematic.Labels = append(builder.design.Schematic.Labels, label)
	return nil
}

// AddBus appends one KiCad vector-bus spine. Bus membership remains expressed
// by scalar member wires and labels emitted separately by the transaction layer.
func (builder *Builder) AddBus(points []kicadfiles.Point) error {
	if builder == nil || builder.design.Schematic == nil {
		return fmt.Errorf("schematic required")
	}
	if len(points) < 2 {
		return fmt.Errorf("bus requires at least two points")
	}
	for index := 1; index < len(points); index++ {
		if samePoint(points[index-1], points[index]) {
			return fmt.Errorf("bus contains a zero-length segment")
		}
		if points[index-1].X != points[index].X && points[index-1].Y != points[index].Y {
			return fmt.Errorf("bus segments must be orthogonal")
		}
	}
	for _, bus := range builder.design.Schematic.Buses {
		if samePointList(bus.Points, points) {
			return nil
		}
	}
	builder.design.Schematic.Buses = append(builder.design.Schematic.Buses, schematic.Bus{
		UUID:   builder.generator.New("root.schematic.bus", formatPoint(points[0]), formatPoint(points[len(points)-1])),
		Points: append([]kicadfiles.Point(nil), points...),
	})
	return nil
}

// AddBusEntry appends a native KiCad bus entry. The member wire endpoint is
// conventionally position plus size, matching KiCad's S-expression format.
func (builder *Builder) AddBusEntry(position, size kicadfiles.Point) error {
	if builder == nil || builder.design.Schematic == nil {
		return fmt.Errorf("schematic required")
	}
	if size.X == 0 || size.Y == 0 {
		return fmt.Errorf("bus entry size must be non-zero on both axes")
	}
	for _, entry := range builder.design.Schematic.BusEntries {
		if entry.Position == position && entry.Size == size {
			return nil
		}
	}
	builder.design.Schematic.BusEntries = append(builder.design.Schematic.BusEntries, schematic.BusEntry{
		UUID:     builder.generator.New("root.schematic.bus_entry", formatPoint(position), formatPoint(size)),
		Position: position,
		Size:     size,
	})
	return nil
}

// AddSchematicWireWithLabel emits a raw orthogonal scalar member wire and an
// optional local label. It is intentionally separate from Connect because one
// endpoint may be a bus entry rather than a symbol pin.
func (builder *Builder) AddSchematicWireWithLabel(netName string, points []kicadfiles.Point, label string, labelAt *kicadfiles.Point, rotation kicadfiles.Angle) error {
	if builder == nil || builder.design.Schematic == nil {
		return fmt.Errorf("schematic required")
	}
	if strings.TrimSpace(netName) == "" {
		return fmt.Errorf("schematic wire net name required")
	}
	if len(points) < 2 {
		return fmt.Errorf("schematic wire requires at least two points")
	}
	for index := 1; index < len(points); index++ {
		if samePoint(points[index-1], points[index]) {
			return fmt.Errorf("schematic wire contains a zero-length segment")
		}
		if points[index-1].X != points[index].X && points[index-1].Y != points[index].Y {
			return fmt.Errorf("schematic wire segments must be orthogonal")
		}
	}
	builder.design.ExpectedNets = appendUniqueNet(builder.design.ExpectedNets, netName)
	builder.nets.EnsureNet(netName)
	builder.addSchematicWirePointsWithOptions(netName, Endpoint{}, Endpoint{}, points, true)
	if strings.TrimSpace(label) != "" {
		position := points[len(points)-1]
		if labelAt != nil {
			position = *labelAt
		}
		if err := builder.AddLabelWithOptions(label, position, schematic.LabelLocal, LabelOptions{Rotation: rotation}); err != nil {
			return err
		}
	}
	return nil
}

func (builder *Builder) addSchematicLabelStub(netName string, endpoint Endpoint, anchor kicadfiles.Point, offset kicadfiles.Point) {
	if builder == nil {
		return
	}
	if builder.schematicWireEndpointExists(anchor) {
		return
	}
	if offset.X == 0 && offset.Y == 0 {
		offset.X = kicadfiles.MM(1.27)
	}
	offset = builder.safeSchematicLabelStubOffset(anchor, offset)
	labelPoint := kicadfiles.Point{X: anchor.X + offset.X, Y: anchor.Y + offset.Y}
	builder.addSchematicWire(netName, endpoint, endpoint, anchor, labelPoint)
	_ = builder.AddLabel(netName, labelPoint, schematic.LabelLocal)
}

func (builder *Builder) safeSchematicLabelStubOffset(anchor kicadfiles.Point, preferred kicadfiles.Point) kicadfiles.Point {
	if builder == nil {
		return preferred
	}
	grid := kicadfiles.MM(1.27)
	directions := []kicadfiles.Point{preferred}
	for _, direction := range []kicadfiles.Point{
		{X: grid},
		{X: -grid},
		{Y: -grid},
		{Y: grid},
	} {
		duplicate := false
		for _, candidate := range directions {
			if candidate == direction {
				duplicate = true
				break
			}
		}
		if !duplicate {
			directions = append(directions, direction)
		}
	}
	for _, scale := range []kicadfiles.IU{1, 2, 4} {
		for _, direction := range directions {
			candidate := kicadfiles.Point{X: direction.X * scale, Y: direction.Y * scale}
			labelPoint := kicadfiles.Point{X: anchor.X + candidate.X, Y: anchor.Y + candidate.Y}
			if !schematicStubTouchesExistingWire(anchor, labelPoint, builder.design.Schematic.Wires) &&
				!builder.schematicSegmentTouchesOtherPinAnchor(anchor, labelPoint, anchor, anchor) &&
				!schematicLabelPositionOccupied(labelPoint, builder.design.Schematic.Labels) {
				return candidate
			}
		}
	}
	return preferred
}

func schematicLabelPositionOccupied(position kicadfiles.Point, labels []schematic.Label) bool {
	for _, label := range labels {
		if samePoint(position, label.Position) {
			return true
		}
	}
	return false
}

func schematicStubTouchesExistingWire(anchor kicadfiles.Point, labelPoint kicadfiles.Point, wires []schematic.Wire) bool {
	for _, wire := range wires {
		for index := 1; index < len(wire.Points); index++ {
			a := wire.Points[index-1]
			b := wire.Points[index]
			if pointOnSchematicSegment(labelPoint, a, b) && !samePoint(labelPoint, anchor) {
				return true
			}
			if pointOnSchematicSegment(a, anchor, labelPoint) && !samePoint(a, anchor) {
				return true
			}
			if pointOnSchematicSegment(b, anchor, labelPoint) && !samePoint(b, anchor) {
				return true
			}
		}
	}
	return false
}

func pointOnSchematicSegment(point kicadfiles.Point, a kicadfiles.Point, b kicadfiles.Point) bool {
	dxSegment := b.X - a.X
	dySegment := b.Y - a.Y
	dxPoint := point.X - a.X
	dyPoint := point.Y - a.Y
	if !sameSchematicCrossProduct(dxPoint, dySegment, dyPoint, dxSegment) {
		return false
	}
	return betweenSchematicInclusive(point.X, a.X, b.X) &&
		betweenSchematicInclusive(point.Y, a.Y, b.Y)
}

func sameSchematicCrossProduct(a kicadfiles.IU, b kicadfiles.IU, c kicadfiles.IU, d kicadfiles.IU) bool {
	var left big.Int
	left.Mul(big.NewInt(int64(a)), big.NewInt(int64(b)))
	var right big.Int
	right.Mul(big.NewInt(int64(c)), big.NewInt(int64(d)))
	return left.Cmp(&right) == 0
}

func betweenSchematicInclusive(value kicadfiles.IU, a kicadfiles.IU, b kicadfiles.IU) bool {
	if a > b {
		a, b = b, a
	}
	return value >= a && value <= b
}

func (builder *Builder) addSchematicWire(netName string, from, to Endpoint, start, end kicadfiles.Point) {
	builder.addSchematicWireWithOptions(netName, from, to, start, end, false)
}

func (builder *Builder) addSchematicWireWithOptions(netName string, from, to Endpoint, start, end kicadfiles.Point, suppressBendLabels bool) {
	if builder == nil {
		return
	}
	points := builder.orthogonalSchematicWirePoints(start, end)
	builder.addSchematicWirePointsWithOptions(netName, from, to, points, suppressBendLabels)
}

func (builder *Builder) addSchematicWirePoints(netName string, from, to Endpoint, points []kicadfiles.Point) {
	builder.addSchematicWirePointsWithOptions(netName, from, to, points, false)
}

func (builder *Builder) addSchematicWirePointsWithOptions(netName string, from, to Endpoint, points []kicadfiles.Point, suppressBendLabels bool) {
	if builder == nil || len(points) < 2 {
		return
	}
	for index := 0; index < len(points)-1; index++ {
		if samePoint(points[index], points[index+1]) || hasSchematicWireSegment(builder.design.Schematic.Wires, points[index], points[index+1]) {
			continue
		}
		builder.addSchematicEndpointJunction(netName, points[index])
		builder.addSchematicEndpointJunction(netName, points[index+1])
		wireOffset := len(builder.design.Schematic.Wires)
		builder.design.Schematic.Wires = append(builder.design.Schematic.Wires, schematic.NewWire(
			builder.generator.New("root.schematic.wire", netName, fmt.Sprintf("%d", wireOffset), fmt.Sprintf("%d", index), from.Reference, from.Pin, to.Reference, to.Pin),
			points[index],
			points[index+1],
		))
		builder.indexSchematicWireEndpoint(points[index])
		builder.indexSchematicWireEndpoint(points[index+1])
	}
	suppressBendLabelsForNet := schematicConnectionSuppressesBendLabels(netName)
	for index := 1; index < len(points)-1; index++ {
		if !hasSchematicJunction(builder.design.Schematic.Junctions, points[index]) {
			builder.design.Schematic.Junctions = append(builder.design.Schematic.Junctions, schematic.Junction{
				UUID:     builder.generator.New("root.schematic.junction", netName, fmt.Sprintf("%d", index), formatPoint(points[index])),
				Position: points[index],
			})
		}
		if suppressBendLabels || suppressBendLabelsForNet {
			continue
		}
		if hasSchematicLabel(builder.design.Schematic.Labels, netName, points[index]) {
			continue
		}
		builder.design.Schematic.Labels = append(builder.design.Schematic.Labels, schematic.NewLabel(
			builder.generator.New("root.schematic.label", netName, fmt.Sprintf("%d", index), formatPoint(points[index])),
			netName,
			schematic.LabelLocal,
			points[index],
		))
	}
}

func validateSchematicWaypoints(points []kicadfiles.Point, start, end kicadfiles.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 2 {
		return fmt.Errorf("schematic waypoints require at least source and destination")
	}
	if !samePoint(points[0], start) || !samePoint(points[len(points)-1], end) {
		return fmt.Errorf("schematic waypoints must start and end at connected pin anchors")
	}
	for index := 1; index < len(points); index++ {
		if samePoint(points[index-1], points[index]) {
			return fmt.Errorf("schematic waypoints contain duplicate adjacent point")
		}
		if points[index-1].X != points[index].X && points[index-1].Y != points[index].Y {
			return fmt.Errorf("schematic waypoints must be orthogonal")
		}
	}
	return nil
}

func validateSchematicLabelPoint(position *kicadfiles.Point, anchor kicadfiles.Point) error {
	if position == nil || *position == anchor {
		return nil
	}
	if position.X != anchor.X && position.Y != anchor.Y {
		return fmt.Errorf("schematic label stub must be orthogonal")
	}
	return nil
}

func (builder *Builder) orthogonalSchematicWirePoints(start, end kicadfiles.Point) []kicadfiles.Point {
	points := orthogonalSchematicWirePoints(start, end)
	if builder == nil || !builder.schematicPathTouchesOtherPinAnchor(points, start, end) {
		return points
	}
	grid := kicadfiles.MM(1.27)
	var candidates [][]kicadfiles.Point
	if start.X != end.X && start.Y != end.Y {
		candidates = append(candidates, []kicadfiles.Point{
			start,
			{X: end.X, Y: start.Y},
			end,
		})
		xOffset := grid
		if end.X < start.X {
			xOffset = -xOffset
		}
		candidates = append(candidates, []kicadfiles.Point{
			start,
			{X: start.X + xOffset, Y: start.Y},
			{X: start.X + xOffset, Y: end.Y},
			end,
		})
	} else if start.X == end.X {
		for _, xOffset := range []kicadfiles.IU{grid, -grid} {
			candidates = append(candidates, []kicadfiles.Point{
				start,
				{X: start.X + xOffset, Y: start.Y},
				{X: end.X + xOffset, Y: end.Y},
				end,
			})
		}
	} else {
		for _, yOffset := range []kicadfiles.IU{grid, -grid} {
			candidates = append(candidates, []kicadfiles.Point{
				start,
				{X: start.X, Y: start.Y + yOffset},
				{X: end.X, Y: end.Y + yOffset},
				end,
			})
		}
	}
	for _, candidate := range candidates {
		if !builder.schematicPathTouchesOtherPinAnchor(candidate, start, end) {
			return candidate
		}
	}
	return points
}

func (builder *Builder) schematicPathTouchesOtherPinAnchor(points []kicadfiles.Point, start, end kicadfiles.Point) bool {
	for index := 1; index < len(points); index++ {
		if builder.schematicSegmentTouchesOtherPinAnchor(points[index-1], points[index], start, end) {
			return true
		}
	}
	return false
}

func (builder *Builder) schematicSegmentTouchesOtherPinAnchor(a, b, start, end kicadfiles.Point) bool {
	if builder == nil {
		return false
	}
	for anchor := range builder.schematicPinAnchors {
		if samePoint(anchor, start) || samePoint(anchor, end) {
			continue
		}
		if pointOnSchematicSegment(anchor, a, b) {
			return true
		}
	}
	return false
}

func (builder *Builder) addSchematicEndpointJunction(netName string, position kicadfiles.Point) {
	if builder == nil || hasSchematicJunction(builder.design.Schematic.Junctions, position) {
		return
	}
	if !builder.schematicWireEndpointExists(position) {
		return
	}
	builder.design.Schematic.Junctions = append(builder.design.Schematic.Junctions, schematic.Junction{
		UUID:     builder.generator.New("root.schematic.endpoint_junction", netName, formatPoint(position)),
		Position: position,
	})
}

func (builder *Builder) schematicWireEndpointExists(position kicadfiles.Point) bool {
	if builder == nil {
		return false
	}
	_, ok := builder.schematicWireEnds[position]
	return ok
}

func (builder *Builder) indexSchematicWireEndpoint(position kicadfiles.Point) {
	if builder == nil {
		return
	}
	if builder.schematicWireEnds == nil {
		builder.schematicWireEnds = map[kicadfiles.Point]struct{}{}
	}
	builder.schematicWireEnds[position] = struct{}{}
}

func hasNoConnect(noConnects []schematic.NoConnect, position kicadfiles.Point) bool {
	for _, noConnect := range noConnects {
		if noConnect.Position == position {
			return true
		}
	}
	return false
}

func hasSchematicWireSegment(wires []schematic.Wire, start, end kicadfiles.Point) bool {
	for _, wire := range wires {
		if len(wire.Points) != 2 {
			continue
		}
		if samePoint(wire.Points[0], start) && samePoint(wire.Points[1], end) {
			return true
		}
		if samePoint(wire.Points[0], end) && samePoint(wire.Points[1], start) {
			return true
		}
	}
	return false
}

func hasSchematicJunction(junctions []schematic.Junction, position kicadfiles.Point) bool {
	for _, junction := range junctions {
		if samePoint(junction.Position, position) {
			return true
		}
	}
	return false
}

func hasSchematicLabel(labels []schematic.Label, text string, position kicadfiles.Point) bool {
	for _, label := range labels {
		if label.Kind == schematic.LabelLocal && label.Text == text && samePoint(label.Position, position) {
			return true
		}
	}
	return false
}

func samePoint(first, second kicadfiles.Point) bool {
	// KiCad file coordinates are normalized to integer internal units before
	// they reach the design builder, so exact comparison is intentional here.
	return first.X == second.X && first.Y == second.Y
}

func samePointList(left, right []kicadfiles.Point) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !samePoint(left[index], right[index]) {
			return false
		}
	}
	return true
}

func orthogonalSchematicWirePoints(start, end kicadfiles.Point) []kicadfiles.Point {
	if start.X == end.X || start.Y == end.Y {
		return []kicadfiles.Point{start, end}
	}
	return []kicadfiles.Point{
		start,
		{X: start.X, Y: end.Y},
		end,
	}
}

func (builder *Builder) mergeNet(oldName, newName string) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if builder == nil || oldName == "" || newName == "" || oldName == newName {
		return
	}
	oldRoot := builder.canonicalNet(oldName)
	newRoot := builder.canonicalNet(newName)
	if oldRoot == "" || newRoot == "" || oldRoot == newRoot {
		return
	}
	builder.netParents[oldRoot] = newRoot
	builder.nets.EnsureNet(newRoot)
	builder.design.ExpectedNets = removeString(builder.design.ExpectedNets, oldName)
	builder.design.ExpectedNets = removeString(builder.design.ExpectedNets, oldRoot)
	builder.design.ExpectedNets = appendUniqueNet(builder.design.ExpectedNets, newRoot)
}

func (builder *Builder) AssignFootprint(reference, libraryID string) error {
	states := builder.symbolStates(reference)
	if len(states) == 0 {
		return fmt.Errorf("unknown symbol %s", strings.TrimSpace(reference))
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return fmt.Errorf("footprint library id required")
	}
	for _, state := range states {
		state.footprintID = libraryID
		builder.setSymbolProperty(state, "Footprint", libraryID)
	}
	if footprint := builder.footprint(reference); footprint != nil {
		footprint.LibraryID = libraryID
	}
	builder.addKnownFootprintLibrary(libraryID)
	return nil
}

func (builder *Builder) PlaceFootprint(reference string, options PlaceFootprintOptions) (FootprintHandle, error) {
	if builder == nil {
		return FootprintHandle{}, fmt.Errorf("builder required")
	}
	state, err := builder.symbolState(reference)
	if err != nil {
		return FootprintHandle{}, err
	}
	symbol := builder.design.Schematic.Symbols[state.symbolIndex]
	if state.footprintID == "" {
		return FootprintHandle{}, fmt.Errorf("symbol %s has no assigned footprint", reference)
	}
	if options.Layer == "" {
		options.Layer = kicadfiles.LayerFCu
	}
	attributes := trimNonEmpty(options.Attributes)
	if len(attributes) == 0 {
		attributes = []string{"smd"}
	}
	defaultPadType := padTypeForAttributes(attributes)
	padSpecs := options.Pads
	if len(padSpecs) == 0 {
		padSpecs = builder.defaultPadSpecs(state, options.Layer, defaultPadType)
	}
	if err := builder.validatePadSpecs(reference, state, padSpecs, options.AllowUnmatchedUnconnectedPads); err != nil {
		return FootprintHandle{}, err
	}
	footprint := pcb.Footprint{
		UUID:               builder.generator.New("root.pcb.footprint", reference),
		Path:               symbol.Path,
		LibraryID:          state.footprintID,
		Reference:          symbol.Reference,
		Value:              symbol.Value,
		Description:        strings.TrimSpace(options.Description),
		Tags:               strings.TrimSpace(options.Tags),
		Position:           options.Position,
		Rotation:           options.Rotation,
		Layer:              options.Layer,
		Attributes:         attributes,
		MetadataProperties: cloneFootprintMetadataProperties(options.MetadataProperties),
		Properties:         builder.footprintProperties(reference, symbol.Reference, symbol.Value, options.Layer, options.HideDefaultFootprintText),
		Texts:              builder.footprintTextsFromOptions(reference, options.Texts),
		Graphics:           builder.footprintGraphicsFromOptions(reference, options.Graphics),
		Models:             cloneModels(options.Models),
	}
	for index, property := range options.Properties {
		property.UUID = builder.generator.New("root.pcb.footprint.property", reference, property.Name, fmt.Sprintf("%d", index))
		property.Layer = kicadfiles.BoardLayerForPlacement(property.Layer, options.Layer)
		footprint.Properties = append(footprint.Properties, property)
	}
	padOccurrences := map[string]int{}
	for _, padSpec := range padSpecs {
		spec := padSpec
		padName := strings.TrimSpace(spec.Name)
		padOccurrence := padOccurrences[padName]
		padOccurrences[padName]++
		if strings.TrimSpace(spec.Net) == "" {
			spec.Net = state.pinNets[padName]
		}
		pad, err := builder.padFromSpec(reference, padOccurrence, spec, defaultPadType, options.Layer)
		if err != nil {
			return FootprintHandle{}, err
		}
		footprint.Pads = append(footprint.Pads, pad)
	}
	if index, ok := builder.footprints[reference]; ok {
		builder.design.PCB.Footprints[index] = footprint
	} else {
		builder.design.PCB.Footprints = append(builder.design.PCB.Footprints, footprint)
		builder.footprints[reference] = len(builder.design.PCB.Footprints) - 1
	}
	builder.pads[reference] = map[string][]int{}
	for i, pad := range footprint.Pads {
		builder.pads[reference][pad.Name] = append(builder.pads[reference][pad.Name], i)
	}
	builder.syncPCBNets()
	return FootprintHandle{Reference: reference}, nil
}

func (builder *Builder) footprintTextsFromOptions(reference string, texts []pcb.FootprintText) []pcb.FootprintText {
	if len(texts) == 0 {
		return nil
	}
	result := make([]pcb.FootprintText, 0, len(texts))
	for i, text := range texts {
		item := text
		if !item.UUID.Valid() {
			item.UUID = builder.generator.New("root.pcb.footprint.text", reference, item.Kind, strconv.Itoa(i))
		}
		result = append(result, item)
	}
	return result
}

func (builder *Builder) footprintGraphicsFromOptions(reference string, graphics []pcb.FootprintGraphic) []pcb.FootprintGraphic {
	if len(graphics) == 0 {
		return nil
	}
	result := make([]pcb.FootprintGraphic, 0, len(graphics))
	for i, graphic := range graphics {
		drawing := pcb.Drawing(graphic)
		if !drawing.UUID.Valid() {
			drawing.UUID = builder.generator.New("root.pcb.footprint.graphic", reference, drawing.Kind, strconv.Itoa(i))
		}
		result = append(result, pcb.FootprintGraphic(drawing))
	}
	return result
}

func cloneModels(models []pcb.Model3D) []pcb.Model3D {
	if len(models) == 0 {
		return nil
	}
	return append([]pcb.Model3D(nil), models...)
}

func (builder *Builder) Route(netName string, points []kicadfiles.Point, options RouteOptions) (RouteHandle, error) {
	if builder == nil {
		return RouteHandle{}, fmt.Errorf("builder required")
	}
	netName = strings.TrimSpace(netName)
	if netName == "" {
		return RouteHandle{}, fmt.Errorf("net name required")
	}
	netName = builder.canonicalNet(netName)
	if len(points) == 1 {
		return RouteHandle{}, fmt.Errorf("route requires at least two points when points are provided")
	}
	if len(points) == 0 && len(options.Vias) == 0 {
		return RouteHandle{}, fmt.Errorf("route requires at least two points or at least one via")
	}
	if options.Layer == "" {
		options.Layer = kicadfiles.LayerFCu
	}
	if options.Width == 0 {
		options.Width = kicadfiles.MM(0.25)
	}
	net := builder.nets.EnsureNet(netName)
	added := 0
	// Track UUIDs use the existing board track count so repeated Route calls
	// cannot reuse the same per-call segment index.
	trackOffset := len(builder.design.PCB.Tracks)
	for i := 0; i < len(points)-1; i++ {
		if points[i] == points[i+1] {
			return RouteHandle{}, fmt.Errorf("route segment %d has identical endpoints", i)
		}
		builder.design.PCB.Tracks = append(builder.design.PCB.Tracks, pcb.Track{
			UUID:    builder.generator.New("root.pcb.route", netName, string(options.Layer), fmt.Sprintf("%d", trackOffset+i), formatPoint(points[i]), formatPoint(points[i+1])),
			Start:   points[i],
			End:     points[i+1],
			Width:   options.Width,
			Layer:   options.Layer,
			NetCode: net.Code,
			NetName: net.Name,
		})
		added++
	}
	routeSeed := formatPoints(points)
	for _, via := range options.Vias {
		viaOrdinal := builder.routeViaCounts[net.Name]
		builder.routeViaCounts[net.Name] = viaOrdinal + 1
		layers := append([]kicadfiles.BoardLayer(nil), via.Layers...)
		if len(layers) == 0 {
			layers = []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu}
		}
		layers = canonicalRouteViaLayers(layers)
		size := via.Size
		if size == 0 {
			size = kicadfiles.MM(0.6)
		}
		drill := via.Drill
		if drill == 0 {
			drill = kicadfiles.MM(0.3)
		}
		builder.design.PCB.Vias = append(builder.design.PCB.Vias, pcb.Via{
			UUID:     builder.generator.New("root.pcb.route.via", net.Name, fmt.Sprintf("%d", viaOrdinal), routeSeed, formatPoint(via.At), formatLayers(layers)),
			Position: via.At,
			Size:     size,
			Drill:    drill,
			NetCode:  net.Code,
			NetName:  net.Name,
			Layers:   layers,
		})
	}
	builder.syncPCBNets()
	return RouteHandle{NetName: net.Name, Count: added}, nil
}

func (builder *Builder) RouteBoard(request routing.Request, options RouteBoardOptions) (routing.Result, error) {
	if builder == nil {
		return routing.Result{}, fmt.Errorf("builder required")
	}
	if builder.design.PCB == nil {
		return routing.Result{}, fmt.Errorf("PCB required")
	}
	result := routing.RouteRequest(request)
	if options.DryRun {
		return result, nil
	}
	for _, route := range result.Routes {
		if len(route.Segments) == 0 && len(route.Vias) == 0 {
			continue
		}
		net := builder.nets.EnsureNet(route.Net)
		for index, segment := range route.Segments {
			start := pointFromRoutingPoint(segment.Start)
			end := pointFromRoutingPoint(segment.End)
			layer := boardLayerFromRouting(segment.Layer)
			builder.design.PCB.Tracks = append(builder.design.PCB.Tracks, pcb.Track{
				UUID:    builder.generator.New("root.pcb.route_board", route.Net, string(layer), fmt.Sprintf("%d", index), formatPoint(start), formatPoint(end)),
				Start:   start,
				End:     end,
				Width:   kicadfiles.MM(segment.WidthMM),
				Layer:   layer,
				NetCode: net.Code,
				NetName: net.Name,
			})
		}
		for viaIndex, via := range route.Vias {
			position := pointFromRoutingPoint(via.At)
			layers := make([]kicadfiles.BoardLayer, 0, len(via.Layers))
			for _, layer := range via.Layers {
				layers = append(layers, boardLayerFromRouting(layer))
			}
			layers = canonicalRouteViaLayers(layers)
			builder.design.PCB.Vias = append(builder.design.PCB.Vias, pcb.Via{
				UUID:     builder.generator.New("root.pcb.route_board.via", route.Net, strconv.Itoa(viaIndex), formatPoint(position), formatLayers(layers)),
				Position: position,
				Size:     kicadfiles.MM(via.DiameterMM),
				Drill:    kicadfiles.MM(via.DrillMM),
				NetCode:  net.Code,
				NetName:  net.Name,
				Layers:   layers,
			})
		}
	}
	builder.syncPCBNets()
	return result, nil
}

func pointFromRoutingPoint(point routing.Point) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(point.XMM), Y: kicadfiles.MM(point.YMM)}
}

func boardLayerFromRouting(layer string) kicadfiles.BoardLayer {
	switch strings.ToUpper(strings.TrimSpace(layer)) {
	case "F.CU":
		return kicadfiles.LayerFCu
	case "B.CU":
		return kicadfiles.LayerBCu
	default:
		return kicadfiles.BoardLayer(strings.TrimSpace(layer))
	}
}

func (builder *Builder) AddZone(netName string, polygon []kicadfiles.Point, options ZoneOptions) (ZoneHandle, error) {
	if builder == nil {
		return ZoneHandle{}, fmt.Errorf("builder required")
	}
	netName = strings.TrimSpace(netName)
	if netName == "" {
		return ZoneHandle{}, fmt.Errorf("net name required")
	}
	netName = builder.canonicalNet(netName)
	if countDistinctPoints(polygon) < 3 {
		return ZoneHandle{}, fmt.Errorf("zone polygon requires at least three distinct points")
	}
	layers := options.Layers
	if len(layers) == 0 {
		layers = []kicadfiles.BoardLayer{kicadfiles.LayerFCu}
	}
	net := builder.nets.EnsureNet(netName)
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = net.Name
	}
	zoneOffset := len(builder.design.PCB.Zones)
	builder.design.PCB.Zones = append(builder.design.PCB.Zones, pcb.Zone{
		UUID:                 builder.generator.New("root.pcb.zone", name, net.Name, fmt.Sprintf("%d", zoneOffset), formatLayers(layers), formatPoints(polygon)),
		NetCode:              net.Code,
		NetName:              net.Name,
		Name:                 name,
		Layers:               append([]kicadfiles.BoardLayer(nil), layers...),
		Polygons:             [][]kicadfiles.Point{append([]kicadfiles.Point(nil), polygon...)},
		ConnectPads:          options.ConnectPads,
		Clearance:            options.Clearance,
		MinThickness:         defaultIU(options.MinThickness, kicadfiles.MM(0.25)),
		FilledAreasThickness: options.FilledAreasThickness,
		Priority:             options.Priority,
		Fill: pcb.ZoneFillSettings{
			ThermalGap:         options.ThermalGap,
			ThermalBridgeWidth: options.ThermalBridgeWidth,
			IslandRemovalMode:  1,
		},
	})
	builder.syncPCBNets()
	return ZoneHandle{Name: name}, nil
}

func (builder *Builder) SetBoardOutline(points []kicadfiles.Point) (BoardOutlineHandle, error) {
	if builder == nil {
		return BoardOutlineHandle{}, fmt.Errorf("builder required")
	}
	if countDistinctPoints(points) < 3 {
		return BoardOutlineHandle{}, fmt.Errorf("board outline requires at least three distinct points")
	}
	outline := append([]kicadfiles.Point(nil), points...)
	if outline[0] == outline[len(outline)-1] {
		outline = outline[:len(outline)-1]
	}
	if len(outline) < 3 {
		return BoardOutlineHandle{}, fmt.Errorf("board outline requires at least three distinct points")
	}
	drawings := make([]pcb.Drawing, 0, len(outline))
	for i := range outline {
		start := outline[i]
		end := outline[(i+1)%len(outline)]
		if start == end {
			return BoardOutlineHandle{}, fmt.Errorf("board outline segment %d has identical endpoints", i)
		}
		drawings = append(drawings, pcb.Drawing{
			UUID:  builder.generator.New("root.pcb.outline", fmt.Sprintf("%d", i), formatPoint(start), formatPoint(end)),
			Layer: kicadfiles.LayerEdge,
			Kind:  "line",
			Line:  &pcb.LineDrawing{Start: start, End: end, Width: kicadfiles.MM(0.1)},
		})
	}
	var preserved []pcb.Drawing
	for _, drawing := range builder.design.PCB.Drawings {
		if drawing.Layer != kicadfiles.LayerEdge {
			preserved = append(preserved, drawing)
		}
	}
	builder.design.PCB.Drawings = append(preserved, drawings...)
	builder.design.PCB.RequireClosedOutline = true
	return BoardOutlineHandle{SegmentCount: len(drawings)}, nil
}

func (builder *Builder) SetRectangularBoardOutline(width, height kicadfiles.IU) (BoardOutlineHandle, error) {
	if width <= 0 || height <= 0 {
		return BoardOutlineHandle{}, fmt.Errorf("board width and height must be positive")
	}
	return builder.SetBoardOutline([]kicadfiles.Point{
		{X: 0, Y: 0},
		{X: width, Y: 0},
		{X: width, Y: height},
		{X: 0, Y: height},
	})
}

func (builder *Builder) Design() kicaddesign.Design {
	if builder == nil {
		return kicaddesign.Design{}
	}
	builder.syncPCBNets()
	design := cloneDesign(builder.design)
	builder.resolveDesignNets(&design)
	builder.addSameNetPadAliasTies(&design)
	return design
}

func (builder *Builder) addSameNetPadAliasTies(design *kicaddesign.Design) {
	if builder == nil || design == nil || design.PCB == nil {
		return
	}
	existingTracks := padAliasTieKeySet(design.PCB.Tracks)
	for footprintIndex := range design.PCB.Footprints {
		footprint := &design.PCB.Footprints[footprintIndex]
		seen := map[string]int{}
		for padIndex := range footprint.Pads {
			pad := &footprint.Pads[padIndex]
			name := strings.TrimSpace(pad.Name)
			if name == "" || strings.TrimSpace(pad.NetName) == "" {
				continue
			}
			if firstIndex, ok := seen[name]; ok {
				first := &footprint.Pads[firstIndex]
				if first.NetCode == pad.NetCode && first.NetName == pad.NetName {
					builder.addPadAliasTie(design.PCB, footprint, first, pad, existingTracks)
				}
				continue
			}
			seen[name] = padIndex
		}
	}
}

func (builder *Builder) addPadAliasTie(board *pcb.PCBFile, footprint *pcb.Footprint, first *pcb.Pad, second *pcb.Pad, existingTracks map[padAliasTrackKey]struct{}) {
	layer, ok := sharedPadTieLayer(first, second)
	if !ok {
		return
	}
	start := footprintPadCenter(*footprint, *first)
	end := footprintPadCenter(*footprint, *second)
	key := padAliasTieKey(first.NetCode, layer, start, end)
	if start == end {
		return
	}
	if _, exists := existingTracks[key]; exists {
		return
	}
	width := kicadfiles.MM(0.25)
	board.Tracks = append(board.Tracks, pcb.Track{
		UUID:    builder.generator.New("root.pcb.pad_alias_tie", footprint.Reference, first.Name, first.NetName, formatPoint(start), formatPoint(end)),
		Start:   start,
		End:     end,
		Width:   width,
		Layer:   layer,
		NetCode: first.NetCode,
		NetName: first.NetName,
	})
	existingTracks[key] = struct{}{}
}

func sharedPadTieLayer(first *pcb.Pad, second *pcb.Pad) (kicadfiles.BoardLayer, bool) {
	for _, preferred := range []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu} {
		if padHasLayer(*first, preferred) && padHasLayer(*second, preferred) {
			return preferred, true
		}
	}
	if padHasLayer(*first, kicadfiles.LayerAllCu) {
		return firstCopperLayer(second.Layers)
	}
	if padHasLayer(*second, kicadfiles.LayerAllCu) {
		return firstCopperLayer(first.Layers)
	}
	for _, layer := range first.Layers {
		if isPadTieCopperLayer(layer) && padHasLayer(*second, layer) {
			return layer, true
		}
	}
	return "", false
}

func firstCopperLayer(layers []kicadfiles.BoardLayer) (kicadfiles.BoardLayer, bool) {
	for _, layer := range layers {
		if layer == kicadfiles.LayerAllCu {
			return kicadfiles.LayerFCu, true
		}
		if isPadTieCopperLayer(layer) {
			return layer, true
		}
	}
	return "", false
}

func isPadTieCopperLayer(layer kicadfiles.BoardLayer) bool {
	name := string(layer)
	return strings.HasSuffix(name, ".Cu")
}

func padHasLayer(pad pcb.Pad, layer kicadfiles.BoardLayer) bool {
	for _, candidate := range pad.Layers {
		if candidate == layer || candidate == kicadfiles.LayerAllCu {
			return true
		}
	}
	return false
}

func footprintPadCenter(footprint pcb.Footprint, pad pcb.Pad) kicadfiles.Point {
	rotated := rotatePadOffset(pad.Position, footprint.Rotation)
	return kicadfiles.Point{X: footprint.Position.X + rotated.X, Y: footprint.Position.Y + rotated.Y}
}

func rotatePadOffset(point kicadfiles.Point, angle kicadfiles.Angle) kicadfiles.Point {
	if angle == 0 {
		return point
	}
	theta := float64(angle) * math.Pi / 180
	sin, cos := math.Sincos(theta)
	x := float64(point.X)
	y := float64(point.Y)
	return kicadfiles.Point{
		X: kicadfiles.IU(math.Round(x*cos - y*sin)),
		Y: kicadfiles.IU(math.Round(x*sin + y*cos)),
	}
}

func padAliasTieKeySet(tracks []pcb.Track) map[padAliasTrackKey]struct{} {
	keys := make(map[padAliasTrackKey]struct{}, len(tracks))
	for _, track := range tracks {
		keys[padAliasTieKey(track.NetCode, track.Layer, track.Start, track.End)] = struct{}{}
	}
	return keys
}

type padAliasTrackKey struct {
	netCode int
	layer   kicadfiles.BoardLayer
	start   kicadfiles.Point
	end     kicadfiles.Point
}

func padAliasTieKey(netCode int, layer kicadfiles.BoardLayer, start kicadfiles.Point, end kicadfiles.Point) padAliasTrackKey {
	if pointLess(end, start) {
		start, end = end, start
	}
	return padAliasTrackKey{netCode: netCode, layer: layer, start: start, end: end}
}

func pointLess(first, second kicadfiles.Point) bool {
	if first.X != second.X {
		return first.X < second.X
	}
	return first.Y < second.Y
}

func (builder *Builder) WriteProject(root string, options kicaddesign.WriteOptions) (kicaddesign.WriteResult, error) {
	if builder == nil {
		return kicaddesign.WriteResult{}, fmt.Errorf("builder required")
	}
	design := builder.Design()
	if err := builder.applySchematicHierarchy(&design); err != nil {
		return kicaddesign.WriteResult{}, err
	}
	if err := ensureGeneratedLocalSymbolLibraries(&design, builder.libraryIndex, builder.resolverSymbolIDs); err != nil {
		return kicaddesign.WriteResult{}, err
	}
	if err := ensureGeneratedLocalFootprintLibraries(&design); err != nil {
		return kicaddesign.WriteResult{}, err
	}
	return kicaddesign.WriteProjectDirectory(root, design, options)
}

func (builder *Builder) WriteSchematicProject(root string, options kicaddesign.WriteOptions) (kicaddesign.WriteResult, error) {
	if builder == nil {
		return kicaddesign.WriteResult{}, fmt.Errorf("builder required")
	}
	// Design returns a cloned design, so omitting PCB data here does not mutate
	// the builder's accumulated board state.
	design := builder.Design()
	if err := builder.applySchematicHierarchy(&design); err != nil {
		return kicaddesign.WriteResult{}, err
	}
	design.PCB = nil
	design.ExpectedNets = nil
	if err := ensureGeneratedLocalSymbolLibraries(&design, builder.libraryIndex, builder.resolverSymbolIDs); err != nil {
		return kicaddesign.WriteResult{}, err
	}
	return kicaddesign.WriteProjectDirectory(root, design, options)
}

func ensureGeneratedLocalSymbolLibraries(design *kicaddesign.Design, index *libraryresolver.LibraryIndex, resolverSymbolIDs map[string]struct{}) error {
	if design == nil || design.Schematic == nil {
		return nil
	}
	seenLibraries := map[string]library.TableEntry{}
	for _, entry := range design.SymbolTables {
		seenLibraries[strings.ToLower(strings.TrimSpace(entry.Name))] = entry
	}
	existingAssets := map[string][]byte{}
	for _, artifact := range design.AssetFiles {
		existingAssets[strings.ToLower(strings.TrimSpace(artifact.Path))] = artifact.Contents
	}
	libraryIDsByNickname := map[string][]string{}
	var nicknameOrder []string
	for _, libraryID := range generatedLocalSymbolLibraryIDs(design.Schematic) {
		if _, resolverOwned := resolverSymbolIDs[libraryID]; resolverOwned {
			continue
		}
		nickname := libraryNickname(libraryID)
		if nickname == "" {
			continue
		}
		nicknameKey := strings.ToLower(nickname)
		if _, ok := libraryIDsByNickname[nicknameKey]; !ok {
			nicknameOrder = append(nicknameOrder, nickname)
		}
		libraryIDsByNickname[nicknameKey] = appendUnique(libraryIDsByNickname[nicknameKey], libraryID)
	}
	for _, nickname := range nicknameOrder {
		nicknameKey := strings.ToLower(nickname)
		assetPath := "lib/kicadai_" + strings.ToLower(nickname) + ".kicad_sym"
		contents, ok := schematic.LocalSymbolLibraryForIDs(libraryIDsByNickname[nicknameKey])
		if !ok {
			continue
		}
		if _, ok := seenLibraries[strings.ToLower(nickname)]; !ok {
			design.SymbolTables = append(design.SymbolTables, library.TableEntry{
				Name:        nickname,
				Type:        "KiCad",
				URI:         "${KIPRJMOD}/" + assetPath,
				Description: "Generated symbols for " + nickname,
			})
			seenLibraries[strings.ToLower(nickname)] = design.SymbolTables[len(design.SymbolTables)-1]
		}
		assetKey := strings.ToLower(strings.TrimSpace(assetPath))
		if _, ok := existingAssets[assetKey]; !ok {
			design.AssetFiles = append(design.AssetFiles, kicaddesign.TextArtifact{
				Path:     assetPath,
				Contents: contents,
			})
			existingAssets[assetKey] = contents
		}
	}
	return ensureResolverLocalSymbolLibraries(design, index, resolverSymbolIDs, seenLibraries, existingAssets)
}

func ensureResolverLocalSymbolLibraries(design *kicaddesign.Design, index *libraryresolver.LibraryIndex, resolverSymbolIDs map[string]struct{}, seenLibraries map[string]library.TableEntry, existingAssets map[string][]byte) error {
	if design == nil || index == nil || len(resolverSymbolIDs) == 0 {
		return nil
	}
	rawByNickname := map[string][]string{}
	var nicknameOrder []string
	libraryIDs := make([]string, 0, len(resolverSymbolIDs))
	for libraryID := range resolverSymbolIDs {
		libraryIDs = append(libraryIDs, libraryID)
	}
	sort.Strings(libraryIDs)
	for _, libraryID := range libraryIDs {
		record, ok := libraryresolver.ResolveSymbolPtr(index, libraryID)
		if !ok {
			return fmt.Errorf("resolver symbol record not found during materialization: %s", libraryID)
		}
		if strings.TrimSpace(record.Raw) == "" {
			return fmt.Errorf("resolver symbol %s has no materializable KiCad body", libraryID)
		}
		nickname := strings.TrimSpace(record.LibraryNickname)
		if nickname == "" {
			nickname = libraryNickname(libraryID)
		}
		if nickname == "" {
			return fmt.Errorf("resolver symbol %s has no library nickname", libraryID)
		}
		nicknameKey := strings.ToLower(nickname)
		if _, ok := rawByNickname[nicknameKey]; !ok {
			nicknameOrder = append(nicknameOrder, nickname)
		}
		rawByNickname[nicknameKey] = append(rawByNickname[nicknameKey], record.Raw)
	}
	for _, nickname := range nicknameOrder {
		nicknameKey := strings.ToLower(nickname)
		contents, ok := schematic.LocalSymbolLibraryForRaw(rawByNickname[nicknameKey])
		if !ok {
			return fmt.Errorf("resolver symbol library %s has no materializable symbol bodies", nickname)
		}
		safeNickname, ok := validateGeneratedPathComponent(nickname)
		if !ok {
			return fmt.Errorf("invalid resolver symbol library nickname %q", nickname)
		}
		assetPath := "lib/kicadai_resolved_" + safeNickname + ".kicad_sym"
		if existing, exists := seenLibraries[nicknameKey]; exists {
			wantURI := "${KIPRJMOD}/" + assetPath
			if strings.TrimSpace(existing.URI) != wantURI {
				return fmt.Errorf("resolver symbol library %s conflicts with existing project table URI %q", nickname, existing.URI)
			}
		} else {
			design.SymbolTables = append(design.SymbolTables, library.TableEntry{
				Name:        nickname,
				Type:        "KiCad",
				URI:         "${KIPRJMOD}/" + assetPath,
				Description: "Resolver-backed symbols for " + nickname,
			})
			seenLibraries[nicknameKey] = design.SymbolTables[len(design.SymbolTables)-1]
		}
		assetKey := strings.ToLower(assetPath)
		if existing, found := existingAssets[assetKey]; found {
			if !bytes.Equal(existing, contents) {
				return fmt.Errorf("resolver symbol library asset %s already exists with different contents", assetPath)
			}
		} else {
			design.AssetFiles = append(design.AssetFiles, kicaddesign.TextArtifact{Path: assetPath, Contents: contents})
			existingAssets[assetKey] = contents
		}
	}
	return nil
}

func generatedLocalSymbolLibraryIDs(file *schematic.SchematicFile) []string {
	if file == nil {
		return nil
	}
	var ids []string
	for _, symbol := range file.Symbols {
		libraryID := strings.TrimSpace(symbol.LibraryID)
		if _, ok := schematic.LocalSymbolLibrary(libraryID); ok {
			ids = appendUnique(ids, libraryID)
		}
	}
	return ids
}

func ensureGeneratedLocalFootprintLibraries(design *kicaddesign.Design) error {
	if design == nil || design.PCB == nil {
		return nil
	}
	seenLibraries := map[string]struct{}{}
	for _, entry := range design.FootprintTables {
		seenLibraries[strings.ToLower(strings.TrimSpace(entry.Name))] = struct{}{}
	}
	existingArtifacts := map[string]struct{}{}
	for _, artifact := range design.AssetFiles {
		existingArtifacts[strings.ToLower(strings.TrimSpace(artifact.Path))] = struct{}{}
	}
	footprintsByNickname := map[string][]generatedLocalFootprint{}
	var nicknameOrder []string
	for i := range design.PCB.Footprints {
		footprint := &design.PCB.Footprints[i]
		rawNickname := libraryNickname(footprint.LibraryID)
		rawName := footprintLibraryName(footprint.LibraryID)
		nickname := rawNickname
		name := rawName
		if nickname == "" || name == "" {
			continue
		}
		var ok bool
		nickname, ok = cleanGeneratedFootprintPathComponent(nickname)
		if !ok {
			return fmt.Errorf("invalid footprint library nickname %q", rawNickname)
		}
		name, ok = cleanGeneratedFootprintPathComponent(name)
		if !ok {
			return fmt.Errorf("invalid footprint library name %q", rawName)
		}
		nicknameKey := strings.ToLower(nickname)
		if _, ok := footprintsByNickname[nicknameKey]; !ok {
			nicknameOrder = append(nicknameOrder, nickname)
		}
		footprintsByNickname[nicknameKey] = append(footprintsByNickname[nicknameKey], generatedLocalFootprint{
			Name:      name,
			Footprint: footprint,
		})
	}
	renderedModules := map[string][]byte{}
	for _, nickname := range nicknameOrder {
		nicknameKey := strings.ToLower(nickname)
		libraryPath := "footprints/" + nickname + ".pretty"
		if _, ok := seenLibraries[nicknameKey]; !ok {
			design.FootprintTables = append(design.FootprintTables, library.TableEntry{
				Name:        nickname,
				Type:        "KiCad",
				URI:         "${KIPRJMOD}/" + libraryPath,
				Description: "Generated footprints for " + nickname,
			})
			seenLibraries[nicknameKey] = struct{}{}
		}
		for _, module := range footprintsByNickname[nicknameKey] {
			footprint := module.Footprint
			name := module.Name
			assetPath := libraryPath + "/" + name + ".kicad_mod"
			assetKey := strings.ToLower(assetPath)
			if _, ok := existingArtifacts[assetKey]; ok {
				continue
			}
			var contents bytes.Buffer
			if err := pcb.WriteFootprintLibraryModule(&contents, footprint, name); err != nil {
				return fmt.Errorf("write generated footprint module %s: %w", footprint.LibraryID, err)
			}
			if previous, ok := renderedModules[assetKey]; ok {
				if !bytes.Equal(previous, contents.Bytes()) {
					return fmt.Errorf("footprint %s has inconsistent generated geometry for local library module %s", footprint.LibraryID, assetPath)
				}
				continue
			}
			moduleContents := append([]byte(nil), contents.Bytes()...)
			renderedModules[assetKey] = moduleContents
			design.AssetFiles = append(design.AssetFiles, kicaddesign.TextArtifact{
				Path:     assetPath,
				Contents: moduleContents,
			})
		}
	}
	return nil
}

type generatedLocalFootprint struct {
	Name      string
	Footprint *pcb.Footprint
}

func (builder *Builder) pinAnchor(endpoint Endpoint) (kicadfiles.Point, error) {
	state, err := builder.symbolStateForEndpoint(endpoint)
	if err != nil {
		return kicadfiles.Point{}, err
	}
	pin := strings.TrimSpace(endpoint.Pin)
	if pin == "" {
		return kicadfiles.Point{}, fmt.Errorf("pin required")
	}
	return builder.pinAnchorForState(state, endpoint.Reference, pin)
}

func (builder *Builder) pinAnchorForState(state *symbolState, reference string, pin string) (kicadfiles.Point, error) {
	pin = strings.TrimSpace(pin)
	if pin == "" {
		return kicadfiles.Point{}, fmt.Errorf("pin required")
	}
	anchor, ok := state.pins[pin]
	if !ok {
		return kicadfiles.Point{}, fmt.Errorf("symbol %s has no pin %s", reference, pin)
	}
	if state.usePhysicalConnectionAnchors {
		if offset, ok := schematic.EmbeddedSymbolConnectionPinOffset(state.libraryID, pin); ok {
			return schematicSymbolPinAnchor(state.position, schematic.TransformConnectionAnchor(offset, state.rotation, state.mirror)), nil
		}
	}
	return anchor, nil
}

func (builder *Builder) pinAnchorOffset(endpoint Endpoint) (kicadfiles.Point, bool) {
	state, err := builder.symbolStateForEndpoint(endpoint)
	if err != nil {
		return kicadfiles.Point{}, false
	}
	pin := strings.TrimSpace(endpoint.Pin)
	if pin == "" {
		return kicadfiles.Point{}, false
	}
	anchor, err := builder.pinAnchorForState(state, endpoint.Reference, pin)
	if err != nil {
		return kicadfiles.Point{}, false
	}
	return kicadfiles.Point{X: anchor.X - state.position.X, Y: anchor.Y - state.position.Y}, true
}

func (builder *Builder) assignedPinNet(endpoint Endpoint) string {
	pin := strings.TrimSpace(endpoint.Pin)
	if state, err := builder.symbolStateForEndpoint(endpoint); err == nil {
		return builder.canonicalNet(state.pinNets[pin])
	}
	return ""
}

func (builder *Builder) symbolState(reference string) (*symbolState, error) {
	if builder == nil {
		return nil, fmt.Errorf("builder required")
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, fmt.Errorf("reference required")
	}
	// Reference-only operations address one physical footprint. Preserve the
	// legacy default of unit 1 when that unit exists; endpoint operations use
	// symbolStateForEndpoint and never rely on this fallback.
	if state, ok := builder.symbols[symbolStateKey(reference, 1)]; ok {
		return state, nil
	}
	states := builder.symbolStates(reference)
	if len(states) == 1 {
		return states[0], nil
	}
	if len(states) > 1 {
		return nil, fmt.Errorf("symbol %s has multiple units; unit required", reference)
	}
	return nil, fmt.Errorf("unknown symbol %s", reference)
}

func (builder *Builder) symbolStateForEndpoint(endpoint Endpoint) (*symbolState, error) {
	if builder == nil {
		return nil, fmt.Errorf("builder required")
	}
	reference := strings.TrimSpace(endpoint.Reference)
	if reference == "" {
		return nil, fmt.Errorf("reference required")
	}
	unit := endpoint.Unit
	if unit <= 0 {
		unit = 1
	}
	state, ok := builder.symbols[symbolStateKey(reference, unit)]
	if !ok {
		return nil, fmt.Errorf("unknown symbol %s unit %d", reference, unit)
	}
	return state, nil
}

func (builder *Builder) symbolStates(reference string) []*symbolState {
	if builder == nil {
		return nil
	}
	want := referenceKey(reference)
	return builder.symbolUnits[want]
}

func symbolStateKey(reference string, unit int) string {
	if unit <= 0 {
		unit = 1
	}
	return referenceKey(reference) + "#" + strconv.Itoa(unit)
}

func symbolGenerationKey(reference string, unit int) string {
	if unit <= 1 {
		return referenceKey(reference)
	}
	return symbolStateKey(reference, unit)
}

func (builder *Builder) assignPinNet(endpoint Endpoint, netName string) {
	state, err := builder.symbolStateForEndpoint(endpoint)
	if err != nil {
		return
	}
	pin := strings.TrimSpace(endpoint.Pin)
	netName = builder.canonicalNet(netName)
	state.pinNets[pin] = netName
	if footprint := builder.footprint(state.reference); footprint != nil {
		pads, padsOK := builder.pads[state.reference]
		if padIndexes, ok := pads[pin]; padsOK && ok {
			net := builder.nets.EnsureNet(netName)
			for _, padIndex := range padIndexes {
				footprint.Pads[padIndex].NetCode = net.Code
				footprint.Pads[padIndex].NetName = net.Name
			}
		}
	}
}

/*
The reference-only lookup is intentionally retained for legacy operations
that address one physical footprint. Endpoint operations must use the unit-
aware lookup above so multi-unit symbols cannot silently connect to unit 1.
*/
func (builder *Builder) footprint(reference string) *pcb.Footprint {
	reference = strings.TrimSpace(reference)
	index, ok := builder.footprints[reference]
	if !ok || index < 0 || index >= len(builder.design.PCB.Footprints) {
		return nil
	}
	return &builder.design.PCB.Footprints[index]
}

func (builder *Builder) defaultPadSpecs(state *symbolState, layer kicadfiles.BoardLayer, padType string) []PadSpec {
	specs := make([]PadSpec, 0, len(state.pinOrder))
	for _, pin := range state.pinOrder {
		specs = append(specs, PadSpec{
			Name:   pin,
			Type:   padType,
			Offset: kicadfiles.Point{},
			Size:   defaultPadSize(padType),
			Drill:  defaultPadDrill(padType),
			Shape:  defaultPadShape(padType),
			Layers: defaultPadLayers(padType, layer),
			Net:    state.pinNets[pin],
		})
	}
	return specs
}

func (builder *Builder) validatePadSpecs(reference string, state *symbolState, padSpecs []PadSpec, allowUnmatchedUnconnected bool) error {
	seen := make(map[string]struct{}, len(padSpecs))
	for _, padSpec := range padSpecs {
		name := strings.TrimSpace(padSpec.Name)
		if name == "" {
			return fmt.Errorf("pad name required")
		}
		seen[name] = struct{}{}
		netted := strings.TrimSpace(padSpec.Net) != ""
		if _, ok := state.pins[name]; !ok && (netted || !allowUnmatchedUnconnected) {
			return fmt.Errorf("pad %s on %s does not match a symbol pin", name, reference)
		}
	}
	for _, pin := range state.pinOrder {
		if _, ok := seen[pin]; !ok {
			return fmt.Errorf("footprint %s missing pad for symbol pin %s", reference, pin)
		}
	}
	return nil
}

func (builder *Builder) padFromSpec(reference string, occurrence int, spec PadSpec, defaultType string, footprintLayer kicadfiles.BoardLayer) (pcb.Pad, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return pcb.Pad{}, fmt.Errorf("pad name required")
	}
	padType := strings.TrimSpace(spec.Type)
	if padType == "" {
		padType = defaultType
	}
	if padType == "" {
		padType = "smd"
	}
	size := spec.Size
	if size == (kicadfiles.Point{}) {
		size = defaultPadSize(padType)
	}
	drill := spec.Drill
	if drill == 0 {
		drill = defaultPadDrill(padType)
	}
	shape := strings.TrimSpace(spec.Shape)
	if shape == "" {
		shape = defaultPadShape(padType)
	}
	layers := spec.Layers
	if len(layers) == 0 {
		layers = defaultPadLayers(padType, footprintLayer)
	}
	var net pcb.Net
	if strings.TrimSpace(spec.Net) != "" {
		net = builder.nets.EnsureNet(builder.canonicalNet(spec.Net))
	}
	return pcb.Pad{
		UUID:     builder.generator.New("root.pcb.footprint.pad", reference, name, strconv.Itoa(occurrence)),
		Name:     name,
		Type:     padType,
		NetCode:  net.Code,
		NetName:  net.Name,
		Shape:    shape,
		Position: spec.Offset,
		Size:     size,
		Drill:    drill,
		Layers:   append([]kicadfiles.BoardLayer(nil), layers...),
	}, nil
}

func (builder *Builder) footprintProperties(key, reference, value string, placementLayer kicadfiles.BoardLayer, hideDefaultFootprintText bool) []pcb.FootprintProperty {
	return []pcb.FootprintProperty{
		builder.footprintProperty(key, "Reference", reference, kicadfiles.DefaultFootprintPropertyPosition("Reference"), kicadfiles.BoardLayerForPlacement(kicadfiles.LayerFSilkS, placementLayer), hideDefaultFootprintText),
		builder.footprintProperty(key, "Value", value, kicadfiles.DefaultFootprintPropertyPosition("Value"), kicadfiles.BoardLayerForPlacement(kicadfiles.LayerFSilkS, placementLayer), hideDefaultFootprintText),
		builder.footprintProperty(key, "Datasheet", "", kicadfiles.DefaultFootprintPropertyPosition("Datasheet"), kicadfiles.BoardLayerForPlacement(kicadfiles.LayerFFab, placementLayer), true),
		builder.footprintProperty(key, "Description", "", kicadfiles.DefaultFootprintPropertyPosition("Description"), kicadfiles.BoardLayerForPlacement(kicadfiles.LayerFFab, placementLayer), true),
	}
}

func (builder *Builder) footprintProperty(key, name, value string, position kicadfiles.Point, layer kicadfiles.BoardLayer, hide bool) pcb.FootprintProperty {
	return pcb.FootprintProperty{
		UUID:     builder.generator.New("root.pcb.footprint.property", key, name),
		Name:     name,
		Value:    value,
		Position: position,
		Layer:    layer,
		Hide:     hide,
		Unlocked: true,
		Effects: pcb.TextEffects{
			FontSize:          kicadfiles.Point{X: kicadfiles.MM(1.27), Y: kicadfiles.MM(1.27)},
			OmitFontThickness: hide,
		},
	}
}

func (builder *Builder) syncPCBNets() {
	if builder != nil && builder.design.PCB != nil {
		builder.design.PCB.Nets = builder.nets.Nets()
	}
}

func (builder *Builder) canonicalNet(name string) string {
	name = strings.TrimSpace(name)
	if builder == nil || name == "" {
		return name
	}
	seen := map[string]struct{}{}
	current := name
	for {
		parent := strings.TrimSpace(builder.netParents[current])
		if parent == "" || parent == current {
			break
		}
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}
		current = parent
	}
	if current != name {
		builder.netParents[name] = current
	}
	return current
}

func (builder *Builder) resolveDesignNets(design *kicaddesign.Design) {
	if builder == nil || design == nil || design.PCB == nil {
		return
	}
	resolve := func(netName string) pcb.Net {
		netName = builder.canonicalNet(netName)
		if netName == "" {
			return pcb.Net{}
		}
		return builder.nets.EnsureNet(netName)
	}
	for footprintIndex := range design.PCB.Footprints {
		footprint := &design.PCB.Footprints[footprintIndex]
		for padIndex := range footprint.Pads {
			if net := resolve(footprint.Pads[padIndex].NetName); net.Name != "" {
				footprint.Pads[padIndex].NetCode = net.Code
				footprint.Pads[padIndex].NetName = net.Name
			}
		}
	}
	for i := range design.PCB.Tracks {
		if net := resolve(design.PCB.Tracks[i].NetName); net.Name != "" {
			design.PCB.Tracks[i].NetCode = net.Code
			design.PCB.Tracks[i].NetName = net.Name
		}
	}
	for i := range design.PCB.TrackArcs {
		if net := resolve(design.PCB.TrackArcs[i].NetName); net.Name != "" {
			design.PCB.TrackArcs[i].NetCode = net.Code
			design.PCB.TrackArcs[i].NetName = net.Name
		}
	}
	for i := range design.PCB.Vias {
		if net := resolve(design.PCB.Vias[i].NetName); net.Name != "" {
			design.PCB.Vias[i].NetCode = net.Code
			design.PCB.Vias[i].NetName = net.Name
		}
	}
	for i := range design.PCB.Zones {
		if net := resolve(design.PCB.Zones[i].NetName); net.Name != "" {
			design.PCB.Zones[i].NetCode = net.Code
			design.PCB.Zones[i].NetName = net.Name
		}
	}
	expectedNets := make([]string, 0, len(design.ExpectedNets))
	for _, netName := range design.ExpectedNets {
		expectedNets = appendUniqueNet(expectedNets, builder.canonicalNet(netName))
	}
	design.ExpectedNets = expectedNets
	design.PCB.Nets = builder.nets.Nets()
}

func (builder *Builder) addKnownSymbolLibrary(libraryID string) {
	builder.design.KnownSymbolLibraries = appendUnique(builder.design.KnownSymbolLibraries, libraryNickname(libraryID))
}

func (builder *Builder) addKnownFootprintLibrary(libraryID string) {
	builder.design.KnownFootprintLibraries = appendUnique(builder.design.KnownFootprintLibraries, libraryNickname(libraryID))
}

func (builder *Builder) setSymbolProperty(state *symbolState, name, value string) {
	symbol := &builder.design.Schematic.Symbols[state.symbolIndex]
	for i := range symbol.Properties {
		if strings.EqualFold(strings.TrimSpace(symbol.Properties[i].Name), name) {
			symbol.Properties[i].Name = name
			symbol.Properties[i].Value = value
			return
		}
	}
	symbol.Properties = append(symbol.Properties, schematic.Property{
		Name:     name,
		Value:    value,
		Hidden:   true,
		Position: symbol.Position,
		Rotation: symbol.Rotation,
	})
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), value) {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueNet(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func removeString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || len(values) == 0 {
		return values
	}
	filtered := values[:0]
	for _, existing := range values {
		if strings.TrimSpace(existing) != value {
			filtered = append(filtered, existing)
		}
	}
	return filtered
}

func libraryNickname(libraryID string) string {
	libraryID = strings.TrimSpace(libraryID)
	before, _, ok := strings.Cut(libraryID, ":")
	if !ok {
		return libraryID
	}
	return strings.TrimSpace(before)
}

func footprintLibraryName(libraryID string) string {
	libraryID = strings.TrimSpace(libraryID)
	_, after, ok := strings.Cut(libraryID, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}

func cleanGeneratedFootprintPathComponent(value string) (string, bool) {
	return validateGeneratedPathComponent(value)
}

func validateGeneratedPathComponent(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\:<>|*?"`) {
		return "", false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	return value, true
}

func referenceKey(reference string) string {
	reference = strings.TrimSpace(reference)
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return strings.ToLower(replacer.Replace(reference))
}

func addPoint(point, offset kicadfiles.Point) kicadfiles.Point {
	return kicadfiles.Point{X: point.X + offset.X, Y: point.Y + offset.Y}
}

func snapSchematicPointToConnectionGrid(point kicadfiles.Point) kicadfiles.Point {
	return kicadfiles.Point{
		X: snapSchematicIUToConnectionGrid(point.X),
		Y: snapSchematicIUToConnectionGrid(point.Y),
	}
}

func (builder *Builder) safeSchematicSymbolPosition(requested kicadfiles.Point, pins []PinSpec, rotation kicadfiles.Angle, mirror schematic.SymbolMirror) kicadfiles.Point {
	position := snapSchematicPointToConnectionGrid(requested)
	if builder == nil {
		return position
	}
	occupied := builder.schematicPinAnchors
	if !schematicSymbolPinAnchorsCollide(position, pins, rotation, mirror, occupied) {
		return position
	}
	for radius := kicadfiles.IU(1); radius <= 8; radius++ {
		for _, offset := range schematicGridPerimeterOffsets(radius) {
			candidate := addPoint(position, offset)
			if !schematicSymbolPinAnchorsCollide(candidate, pins, rotation, mirror, occupied) {
				return candidate
			}
		}
	}
	return position
}

func schematicSymbolPinAnchorsCollide(position kicadfiles.Point, pins []PinSpec, rotation kicadfiles.Angle, mirror schematic.SymbolMirror, occupied map[kicadfiles.Point]struct{}) bool {
	if len(pins) == 0 || len(occupied) == 0 {
		return false
	}
	for _, pin := range pins {
		if _, ok := occupied[schematicSymbolPinAnchor(position, schematic.TransformConnectionAnchor(pin.Offset, rotation, mirror))]; ok {
			return true
		}
	}
	return false
}

func schematicGridPerimeterOffsets(radius kicadfiles.IU) []kicadfiles.Point {
	if radius <= 0 {
		return nil
	}
	offsets := make([]kicadfiles.Point, 0, int(radius)*8)
	step := schematicConnectionGrid
	addOffset := func(x, y kicadfiles.IU) {
		offsets = append(offsets, kicadfiles.Point{X: x * step, Y: y * step})
	}
	for x := -radius; x <= radius; x++ {
		addOffset(x, -radius)
	}
	for y := -radius + 1; y <= radius; y++ {
		addOffset(radius, y)
	}
	for x := radius - 1; x >= -radius; x-- {
		addOffset(x, radius)
	}
	for y := radius - 1; y > -radius; y-- {
		addOffset(-radius, y)
	}
	return offsets
}

func schematicSymbolPinAnchor(position, offset kicadfiles.Point) kicadfiles.Point {
	return snapSchematicPointToConnectionGrid(addPoint(position, offset))
}

func snapSchematicIUToConnectionGrid(value kicadfiles.IU) kicadfiles.IU {
	remainder := value % schematicConnectionGrid
	if remainder == 0 {
		return value
	}
	down := value - remainder
	up := down + schematicConnectionGrid
	if value < 0 {
		up = down - schematicConnectionGrid
	}
	if absSchematicIU(value-down) <= absSchematicIU(up-value) {
		return down
	}
	return up
}

func absSchematicIU(value kicadfiles.IU) kicadfiles.IU {
	if value < 0 {
		return -value
	}
	return value
}

func defaultIU(value, fallback kicadfiles.IU) kicadfiles.IU {
	if value == 0 {
		return fallback
	}
	return value
}

func smdPadLayers(layer kicadfiles.BoardLayer) []kicadfiles.BoardLayer {
	if layer == kicadfiles.LayerBCu {
		return []kicadfiles.BoardLayer{kicadfiles.LayerBCu, kicadfiles.LayerBMask, kicadfiles.LayerBPaste}
	}
	return []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask, kicadfiles.LayerFPaste}
}

func throughHolePadLayers() []kicadfiles.BoardLayer {
	return []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}
}

func padTypeForAttributes(attributes []string) string {
	for _, attribute := range attributes {
		switch strings.ToLower(strings.TrimSpace(attribute)) {
		case "through_hole", "thru_hole", "np_thru_hole":
			return "thru_hole"
		}
	}
	return "smd"
}

func defaultPadLayers(padType string, layer kicadfiles.BoardLayer) []kicadfiles.BoardLayer {
	switch strings.TrimSpace(padType) {
	case "thru_hole", "np_thru_hole":
		return throughHolePadLayers()
	default:
		return smdPadLayers(layer)
	}
}

func defaultPadSize(padType string) kicadfiles.Point {
	switch strings.TrimSpace(padType) {
	case "thru_hole", "np_thru_hole":
		return kicadfiles.Point{X: kicadfiles.MM(1.6), Y: kicadfiles.MM(1.6)}
	default:
		return kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)}
	}
}

func defaultPadDrill(padType string) kicadfiles.IU {
	switch strings.TrimSpace(padType) {
	case "thru_hole", "np_thru_hole":
		return kicadfiles.MM(0.8)
	default:
		return 0
	}
}

func defaultPadShape(padType string) string {
	switch strings.TrimSpace(padType) {
	case "thru_hole", "np_thru_hole":
		return "circle"
	default:
		return "roundrect"
	}
}

func trimNonEmpty(values []string) []string {
	var trimmed []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func countDistinctPoints(points []kicadfiles.Point) int {
	distinct := map[kicadfiles.Point]struct{}{}
	for _, point := range points {
		distinct[point] = struct{}{}
	}
	return len(distinct)
}

func formatPoint(point kicadfiles.Point) string {
	return fmt.Sprintf("%d,%d", point.X, point.Y)
}

func formatPoints(points []kicadfiles.Point) string {
	formatted := make([]string, 0, len(points))
	for _, point := range points {
		formatted = append(formatted, formatPoint(point))
	}
	return strings.Join(formatted, ";")
}

func formatLayers(layers []kicadfiles.BoardLayer) string {
	formatted := make([]string, 0, len(layers))
	for _, layer := range layers {
		formatted = append(formatted, string(layer))
	}
	return strings.Join(formatted, ",")
}

func canonicalRouteViaLayers(layers []kicadfiles.BoardLayer) []kicadfiles.BoardLayer {
	if len(layers) < 2 {
		return layers
	}
	ordered := append([]kicadfiles.BoardLayer(nil), layers...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return routeViaLayerRank(ordered[left]) < routeViaLayerRank(ordered[right])
	})
	return ordered
}

func routeViaLayerRank(layer kicadfiles.BoardLayer) int {
	switch layer {
	case kicadfiles.LayerFCu:
		return 0
	case kicadfiles.LayerBCu:
		return 1000
	}
	name := string(layer)
	if strings.HasPrefix(name, "In") && strings.HasSuffix(name, ".Cu") {
		index, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "In"), ".Cu"))
		if err == nil {
			return index
		}
	}
	return 2000
}

func cloneDesign(source kicaddesign.Design) kicaddesign.Design {
	clone := source
	clone.Project = cloneProject(source.Project)
	clone.Schematic = cloneSchematic(source.Schematic)
	clone.SheetFiles = cloneSchematicFiles(source.SheetFiles)
	clone.PCB = clonePCB(source.PCB)
	clone.SymbolTables = append([]library.TableEntry(nil), source.SymbolTables...)
	clone.FootprintTables = append([]library.TableEntry(nil), source.FootprintTables...)
	clone.KnownSymbolLibraries = append([]string(nil), source.KnownSymbolLibraries...)
	clone.KnownFootprintLibraries = append([]string(nil), source.KnownFootprintLibraries...)
	clone.RuleFiles = cloneTextArtifacts(source.RuleFiles)
	clone.WorksheetFiles = cloneTextArtifacts(source.WorksheetFiles)
	clone.AssetFiles = cloneTextArtifacts(source.AssetFiles)
	clone.ExpectedNets = append([]string(nil), source.ExpectedNets...)
	return clone
}

func cloneProject(source project.ProjectFile) project.ProjectFile {
	clone := source
	clone.TextVariables = cloneStringMap(source.TextVariables)
	clone.Preserved = cloneRawJSONMap(source.Preserved)
	clone.PreservedSections = cloneRawJSONSectionMap(source.PreservedSections)
	clone.NetClasses = cloneNetClasses(source.NetClasses)
	clone.Sheets = append([]project.Sheet(nil), source.Sheets...)
	return clone
}

func cloneNetClasses(source []project.NetClass) []project.NetClass {
	if source == nil {
		return nil
	}
	clone := make([]project.NetClass, len(source))
	for i := range source {
		clone[i] = source[i]
	}
	return clone
}

func cloneSchematicFiles(source []*schematic.SchematicFile) []*schematic.SchematicFile {
	if source == nil {
		return nil
	}
	clone := make([]*schematic.SchematicFile, len(source))
	for i, file := range source {
		clone[i] = cloneSchematic(file)
	}
	return clone
}

func cloneSchematic(source *schematic.SchematicFile) *schematic.SchematicFile {
	if source == nil {
		return nil
	}
	clone := *source
	clone.TitleBlock.Comments = append([]string(nil), source.TitleBlock.Comments...)
	clone.LibSymbols = cloneEmbeddedSymbols(source.LibSymbols)
	clone.Symbols = append([]schematic.SchematicSymbol(nil), source.Symbols...)
	for i := range clone.Symbols {
		clone.Symbols[i] = cloneSchematicSymbol(source.Symbols[i])
	}
	clone.Wires = append([]schematic.Wire(nil), source.Wires...)
	for i := range clone.Wires {
		clone.Wires[i].Points = append([]kicadfiles.Point(nil), source.Wires[i].Points...)
	}
	clone.NoConnects = append([]schematic.NoConnect(nil), source.NoConnects...)
	clone.Labels = append([]schematic.Label(nil), source.Labels...)
	for i := range clone.Labels {
		clone.Labels[i].Fields = append([]schematic.Field(nil), source.Labels[i].Fields...)
	}
	clone.Junctions = append([]schematic.Junction(nil), source.Junctions...)
	clone.Buses = append([]schematic.Bus(nil), source.Buses...)
	for i := range clone.Buses {
		clone.Buses[i].Points = append([]kicadfiles.Point(nil), source.Buses[i].Points...)
	}
	clone.Polylines = append([]schematic.Polyline(nil), source.Polylines...)
	for i := range clone.Polylines {
		clone.Polylines[i].Points = append([]kicadfiles.Point(nil), source.Polylines[i].Points...)
	}
	clone.BusEntries = append([]schematic.BusEntry(nil), source.BusEntries...)
	clone.Texts = append([]schematic.Text(nil), source.Texts...)
	clone.Sheets = append([]schematic.Sheet(nil), source.Sheets...)
	for i := range clone.Sheets {
		clone.Sheets[i].Properties = append([]schematic.Property(nil), source.Sheets[i].Properties...)
		clone.Sheets[i].Pins = append([]schematic.SheetPin(nil), source.Sheets[i].Pins...)
		clone.Sheets[i].Instances = append([]schematic.SheetInstance(nil), source.Sheets[i].Instances...)
	}
	clone.RawItems = append([]schematic.RawSchematicItem(nil), source.RawItems...)
	clone.Instances = append([]schematic.SymbolInstance(nil), source.Instances...)
	clone.SheetInstances = append([]schematic.SheetInstance(nil), source.SheetInstances...)
	return &clone
}

func cloneEmbeddedSymbols(source []schematic.EmbeddedSymbol) []schematic.EmbeddedSymbol {
	if source == nil {
		return nil
	}
	clone := make([]schematic.EmbeddedSymbol, len(source))
	for i := range source {
		clone[i] = source[i]
		clone[i].Body = cloneSexprList(source[i].Body)
	}
	return clone
}

func cloneSexprList(source sexpr.List) sexpr.List {
	if source == nil {
		return nil
	}
	clone := make(sexpr.List, len(source))
	for i, node := range source {
		clone[i] = cloneSexprNode(node)
	}
	return clone
}

func cloneSexprNode(source sexpr.Node) sexpr.Node {
	switch value := source.(type) {
	case sexpr.List:
		return cloneSexprList(value)
	default:
		return value
	}
}

func cloneSchematicSymbol(source schematic.SchematicSymbol) schematic.SchematicSymbol {
	clone := source
	clone.Properties = schematic.CloneProperties(source.Properties)
	clone.Fields = append([]schematic.Field(nil), source.Fields...)
	clone.Pins = append([]schematic.SymbolPin(nil), source.Pins...)
	clone.PinAnchors = append([]kicadfiles.Point(nil), source.PinAnchors...)
	clone.Instances = append([]schematic.SymbolInstance(nil), source.Instances...)
	return clone
}

func clonePCB(source *pcb.PCBFile) *pcb.PCBFile {
	if source == nil {
		return nil
	}
	clone := *source
	clone.TitleBlock.Comments = append([]string(nil), source.TitleBlock.Comments...)
	clone.Layers = append([]pcb.LayerDefinition(nil), source.Layers...)
	clone.Nets = append([]pcb.Net(nil), source.Nets...)
	clone.Footprints = append([]pcb.Footprint(nil), source.Footprints...)
	for i := range clone.Footprints {
		clone.Footprints[i] = cloneFootprint(source.Footprints[i])
	}
	clone.Tracks = append([]pcb.Track(nil), source.Tracks...)
	clone.TrackArcs = append([]pcb.TrackArc(nil), source.TrackArcs...)
	clone.Vias = append([]pcb.Via(nil), source.Vias...)
	for i := range clone.Vias {
		clone.Vias[i].Layers = append([]kicadfiles.BoardLayer(nil), source.Vias[i].Layers...)
	}
	clone.Drawings = append([]pcb.Drawing(nil), source.Drawings...)
	for i := range clone.Drawings {
		clone.Drawings[i] = cloneDrawing(source.Drawings[i])
	}
	clone.Zones = append([]pcb.Zone(nil), source.Zones...)
	for i := range clone.Zones {
		clone.Zones[i] = cloneZone(source.Zones[i])
	}
	clone.Dimensions = append([]pcb.Dimension(nil), source.Dimensions...)
	for i := range clone.Dimensions {
		clone.Dimensions[i].Points = append([]kicadfiles.Point(nil), source.Dimensions[i].Points...)
		clone.Dimensions[i].Effects.Justify = append([]string(nil), source.Dimensions[i].Effects.Justify...)
	}
	clone.Preserved = append([]pcb.PreservedNode(nil), source.Preserved...)
	return &clone
}

func cloneFootprint(source pcb.Footprint) pcb.Footprint {
	clone := source
	clone.Attributes = append([]string(nil), source.Attributes...)
	clone.Properties = append([]pcb.FootprintProperty(nil), source.Properties...)
	for i := range clone.Properties {
		clone.Properties[i].Effects.Justify = append([]string(nil), source.Properties[i].Effects.Justify...)
	}
	clone.MetadataProperties = cloneFootprintMetadataProperties(source.MetadataProperties)
	clone.Units = append([]pcb.FootprintUnit(nil), source.Units...)
	for i := range clone.Units {
		clone.Units[i].Pins = append([]string(nil), source.Units[i].Pins...)
	}
	clone.NetTiePadGroups = append([]string(nil), source.NetTiePadGroups...)
	clone.Texts = cloneFootprintTexts(source.Texts)
	clone.Pads = append([]pcb.Pad(nil), source.Pads...)
	for i := range clone.Pads {
		clone.Pads[i].Layers = append([]kicadfiles.BoardLayer(nil), source.Pads[i].Layers...)
		if source.Pads[i].Teardrops != nil {
			teardrops := *source.Pads[i].Teardrops
			clone.Pads[i].Teardrops = &teardrops
		}
	}
	clone.Graphics = append([]pcb.FootprintGraphic(nil), source.Graphics...)
	for i := range clone.Graphics {
		clone.Graphics[i] = pcb.FootprintGraphic(cloneDrawing(pcb.Drawing(source.Graphics[i])))
	}
	clone.Models = append([]pcb.Model3D(nil), source.Models...)
	return clone
}

func cloneFootprintMetadataProperties(source []pcb.FootprintMetadataProperty) []pcb.FootprintMetadataProperty {
	if source == nil {
		return nil
	}
	clone := make([]pcb.FootprintMetadataProperty, len(source))
	for i := range source {
		clone[i] = source[i]
	}
	return clone
}

func cloneFootprintTexts(source []pcb.FootprintText) []pcb.FootprintText {
	if source == nil {
		return nil
	}
	clone := make([]pcb.FootprintText, len(source))
	for i := range source {
		clone[i] = source[i]
		clone[i].Effects.Justify = append([]string(nil), source[i].Effects.Justify...)
	}
	return clone
}

func cloneDrawing(source pcb.Drawing) pcb.Drawing {
	clone := source
	if source.Line != nil {
		line := *source.Line
		clone.Line = &line
	}
	if source.Rect != nil {
		rect := *source.Rect
		clone.Rect = &rect
	}
	if source.Circle != nil {
		circle := *source.Circle
		clone.Circle = &circle
	}
	if source.Arc != nil {
		arc := *source.Arc
		clone.Arc = &arc
	}
	if source.Poly != nil {
		poly := *source.Poly
		poly.Points = append([]kicadfiles.Point(nil), source.Poly.Points...)
		clone.Poly = &poly
	}
	if source.Curve != nil {
		curve := *source.Curve
		curve.Points = append([]kicadfiles.Point(nil), source.Curve.Points...)
		clone.Curve = &curve
	}
	if source.Text != nil {
		text := *source.Text
		text.Effects.Justify = append([]string(nil), source.Text.Effects.Justify...)
		clone.Text = &text
	}
	return clone
}

func cloneZone(source pcb.Zone) pcb.Zone {
	clone := source
	clone.Layers = append([]kicadfiles.BoardLayer(nil), source.Layers...)
	clone.Polygons = clonePointPolygons(source.Polygons)
	clone.FilledPolygons = append([]pcb.ZoneFilledPolygon(nil), source.FilledPolygons...)
	for i := range clone.FilledPolygons {
		clone.FilledPolygons[i].Points = append([]kicadfiles.Point(nil), source.FilledPolygons[i].Points...)
	}
	if source.Keepout != nil {
		keepout := *source.Keepout
		clone.Keepout = &keepout
	}
	clone.Attributes = append([]pcb.ZoneAttribute(nil), source.Attributes...)
	for i := range clone.Attributes {
		clone.Attributes[i].Values = cloneStringMap(source.Attributes[i].Values)
	}
	return clone
}

func cloneTextArtifacts(source []kicaddesign.TextArtifact) []kicaddesign.TextArtifact {
	if source == nil {
		return nil
	}
	clone := append([]kicaddesign.TextArtifact(nil), source...)
	for i := range clone {
		clone[i].Contents = append([]byte(nil), source[i].Contents...)
	}
	return clone
}

func clonePointPolygons(source [][]kicadfiles.Point) [][]kicadfiles.Point {
	if source == nil {
		return nil
	}
	clone := make([][]kicadfiles.Point, len(source))
	for i := range source {
		clone[i] = append([]kicadfiles.Point(nil), source[i]...)
	}
	return clone
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneRawJSONMap(source map[string]json.RawMessage) map[string]json.RawMessage {
	if source == nil {
		return nil
	}
	clone := make(map[string]json.RawMessage, len(source))
	for key, value := range source {
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
}

func cloneRawJSONSectionMap(source map[string]map[string]json.RawMessage) map[string]map[string]json.RawMessage {
	if source == nil {
		return nil
	}
	clone := make(map[string]map[string]json.RawMessage, len(source))
	for key, section := range source {
		clone[key] = cloneRawJSONMap(section)
	}
	return clone
}
