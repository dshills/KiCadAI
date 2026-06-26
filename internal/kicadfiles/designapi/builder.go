package designapi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/library"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/routing"
)

type Builder struct {
	name       string
	generator  kicadfiles.DeterministicIDGenerator
	design     kicaddesign.Design
	nets       *pcb.NetRegistry
	netParents map[string]string
	symbols    map[string]*symbolState
	symbolKeys map[string]string
	footprints map[string]int
	pads       map[string]map[string][]int
}

type Options struct {
	Name     string
	DesignID kicadfiles.UUID
	Seed     string
	Paper    kicadfiles.Paper
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
	Value     string
	LibraryID string
	Position  kicadfiles.Point
	Pins      []PinSpec
}

type PinSpec struct {
	Number string
	Offset kicadfiles.Point
}

type Endpoint struct {
	Reference string
	Pin       string
}

type PlaceFootprintOptions struct {
	Position           kicadfiles.Point
	Rotation           kicadfiles.Angle
	Layer              kicadfiles.BoardLayer
	Description        string
	Tags               string
	Attributes         []string
	MetadataProperties []pcb.FootprintMetadataProperty
	Texts              []pcb.FootprintText
	Graphics           []pcb.FootprintGraphic
	Models             []pcb.Model3D
	Pads               []PadSpec
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
	symbolIndex int
	pins        map[string]kicadfiles.Point
	pinOrder    []string
	pinNets     map[string]string
	footprintID string
}

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
		name:       name,
		generator:  generator,
		nets:       pcb.NewNetRegistry(),
		netParents: map[string]string{},
		symbols:    map[string]*symbolState{},
		symbolKeys: map[string]string{},
		footprints: map[string]int{},
		pads:       map[string]map[string][]int{},
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
	if _, ok := builder.symbols[reference]; ok {
		return SymbolHandle{}, fmt.Errorf("symbol %s already exists", reference)
	}
	key := referenceKey(reference)
	if existing, ok := builder.symbolKeys[key]; ok {
		return SymbolHandle{}, fmt.Errorf("symbol %s collides with %s after KiCad key normalization", reference, existing)
	}
	libraryID := strings.TrimSpace(options.LibraryID)
	if libraryID == "" {
		return SymbolHandle{}, fmt.Errorf("library id required")
	}
	value := strings.TrimSpace(options.Value)
	if value == "" {
		value = reference
	}
	symbol := schematic.NewSymbol(builder.generator.New("root.schematic.symbol", key), libraryID, reference, value, options.Position)
	symbol.Path = "root.component." + key
	symbol.Pins = make([]schematic.SymbolPin, 0, len(options.Pins))
	pins := make(map[string]kicadfiles.Point, len(options.Pins))
	pinNets := make(map[string]string, len(options.Pins))
	pinOrder := make([]string, 0, len(options.Pins))
	for _, pin := range options.Pins {
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			return SymbolHandle{}, fmt.Errorf("pin number required")
		}
		if _, ok := pins[number]; ok {
			return SymbolHandle{}, fmt.Errorf("duplicate pin %s on %s", number, reference)
		}
		anchor := addPoint(options.Position, pin.Offset)
		pins[number] = anchor
		pinOrder = append(pinOrder, number)
		symbol.PinAnchors = append(symbol.PinAnchors, anchor)
		symbol.Pins = append(symbol.Pins, schematic.SymbolPin{
			Number: number,
			UUID:   builder.generator.New("root.schematic.symbol.pin", key, number),
		})
	}
	builder.design.Schematic.Symbols = append(builder.design.Schematic.Symbols, symbol)
	builder.symbols[reference] = &symbolState{
		symbolIndex: len(builder.design.Schematic.Symbols) - 1,
		pins:        pins,
		pinOrder:    pinOrder,
		pinNets:     pinNets,
	}
	builder.symbolKeys[key] = reference
	builder.addKnownSymbolLibrary(libraryID)
	return SymbolHandle{Reference: reference}, nil
}

func (builder *Builder) Connect(from, to Endpoint, netName string) error {
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
	builder.addSchematicWire(netName, from, to, start, end)
	builder.syncPCBNets()
	return nil
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

func (builder *Builder) addSchematicWire(netName string, from, to Endpoint, start, end kicadfiles.Point) {
	if builder == nil || hasSchematicWire(builder.design.Schematic.Wires, start, end) {
		return
	}
	wireOffset := len(builder.design.Schematic.Wires)
	builder.design.Schematic.Wires = append(builder.design.Schematic.Wires, schematic.NewWire(
		builder.generator.New("root.schematic.wire", netName, fmt.Sprintf("%d", wireOffset), from.Reference, from.Pin, to.Reference, to.Pin),
		start,
		end,
	))
}

func hasNoConnect(noConnects []schematic.NoConnect, position kicadfiles.Point) bool {
	for _, noConnect := range noConnects {
		if noConnect.Position == position {
			return true
		}
	}
	return false
}

func hasSchematicWire(wires []schematic.Wire, start, end kicadfiles.Point) bool {
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

func samePoint(first, second kicadfiles.Point) bool {
	// KiCad file coordinates are normalized to integer internal units before
	// they reach the design builder, so exact comparison is intentional here.
	return first.X == second.X && first.Y == second.Y
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
	state, err := builder.symbolState(reference)
	if err != nil {
		return err
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return fmt.Errorf("footprint library id required")
	}
	state.footprintID = libraryID
	builder.setSymbolProperty(state, "Footprint", libraryID)
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
	if err := builder.validatePadSpecs(reference, state, padSpecs); err != nil {
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
		Properties:         builder.footprintProperties(reference, symbol.Reference, symbol.Value),
		Texts:              builder.footprintTextsFromOptions(reference, options.Texts),
		Graphics:           builder.footprintGraphicsFromOptions(reference, options.Graphics),
		Models:             cloneModels(options.Models),
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
	if len(points) < 2 {
		return RouteHandle{}, fmt.Errorf("route requires at least two points")
	}
	if options.Layer == "" {
		options.Layer = kicadfiles.LayerFCu
	}
	if options.Width == 0 {
		options.Width = kicadfiles.MM(0.25)
	}
	net := builder.nets.EnsureNet(netName)
	added := 0
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
		for _, via := range route.Vias {
			position := pointFromRoutingPoint(via.At)
			layers := make([]kicadfiles.BoardLayer, 0, len(via.Layers))
			for _, layer := range via.Layers {
				layers = append(layers, boardLayerFromRouting(layer))
			}
			builder.design.PCB.Vias = append(builder.design.PCB.Vias, pcb.Via{
				UUID:     builder.generator.New("root.pcb.route_board.via", route.Net, formatPoint(position), formatLayers(layers)),
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
	return design
}

func (builder *Builder) WriteProject(root string, options kicaddesign.WriteOptions) (kicaddesign.WriteResult, error) {
	if builder == nil {
		return kicaddesign.WriteResult{}, fmt.Errorf("builder required")
	}
	return kicaddesign.WriteProjectDirectory(root, builder.Design(), options)
}

func (builder *Builder) pinAnchor(endpoint Endpoint) (kicadfiles.Point, error) {
	state, err := builder.symbolState(endpoint.Reference)
	if err != nil {
		return kicadfiles.Point{}, err
	}
	pin := strings.TrimSpace(endpoint.Pin)
	if pin == "" {
		return kicadfiles.Point{}, fmt.Errorf("pin required")
	}
	anchor, ok := state.pins[pin]
	if !ok {
		return kicadfiles.Point{}, fmt.Errorf("symbol %s has no pin %s", endpoint.Reference, pin)
	}
	return anchor, nil
}

func (builder *Builder) assignedPinNet(endpoint Endpoint) string {
	reference := strings.TrimSpace(endpoint.Reference)
	pin := strings.TrimSpace(endpoint.Pin)
	if state, ok := builder.symbols[reference]; ok {
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
	state, ok := builder.symbols[reference]
	if !ok {
		return nil, fmt.Errorf("unknown symbol %s", reference)
	}
	return state, nil
}

func (builder *Builder) assignPinNet(endpoint Endpoint, netName string) {
	reference := strings.TrimSpace(endpoint.Reference)
	if state, ok := builder.symbols[reference]; ok {
		pin := strings.TrimSpace(endpoint.Pin)
		netName = builder.canonicalNet(netName)
		state.pinNets[pin] = netName
		if footprint := builder.footprint(reference); footprint != nil {
			pads, padsOK := builder.pads[reference]
			if padIndexes, ok := pads[pin]; padsOK && ok {
				net := builder.nets.EnsureNet(netName)
				for _, padIndex := range padIndexes {
					footprint.Pads[padIndex].NetCode = net.Code
					footprint.Pads[padIndex].NetName = net.Name
				}
			}
		}
	}
}

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

func (builder *Builder) validatePadSpecs(reference string, state *symbolState, padSpecs []PadSpec) error {
	seen := make(map[string]struct{}, len(padSpecs))
	for _, padSpec := range padSpecs {
		name := strings.TrimSpace(padSpec.Name)
		if name == "" {
			return fmt.Errorf("pad name required")
		}
		seen[name] = struct{}{}
		if _, ok := state.pins[name]; !ok {
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

func (builder *Builder) footprintProperties(key, reference, value string) []pcb.FootprintProperty {
	return []pcb.FootprintProperty{
		builder.footprintProperty(key, "Reference", reference, kicadfiles.Point{X: 0, Y: kicadfiles.MM(-1.5)}, kicadfiles.LayerFSilkS, false),
		builder.footprintProperty(key, "Value", value, kicadfiles.Point{X: 0, Y: kicadfiles.MM(1.5)}, kicadfiles.LayerFSilkS, false),
		builder.footprintProperty(key, "Datasheet", "", kicadfiles.Point{}, kicadfiles.LayerFFab, true),
		builder.footprintProperty(key, "Description", "", kicadfiles.Point{}, kicadfiles.LayerFFab, true),
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
	return before
}

func referenceKey(reference string) string {
	reference = strings.TrimSpace(reference)
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return strings.ToLower(replacer.Replace(reference))
}

func addPoint(point, offset kicadfiles.Point) kicadfiles.Point {
	return kicadfiles.Point{X: point.X + offset.X, Y: point.Y + offset.Y}
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
	clone.Properties = append([]schematic.Property(nil), source.Properties...)
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
