package pcb

import (
	"math/big"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

const (
	kicad10LayerFCu      = 0
	kicad10LayerFMask    = 1
	kicad10LayerBCu      = 2
	kicad10LayerBMask    = 3
	kicad10LayerFSilkS   = 5
	kicad10LayerBSilkS   = 7
	kicad10LayerFAdhes   = 9
	kicad10LayerBAdhes   = 11
	kicad10LayerFPaste   = 13
	kicad10LayerBPaste   = 15
	kicad10LayerDwgs     = 17
	kicad10LayerCmts     = 19
	kicad10LayerEco1     = 21
	kicad10LayerEco2     = 23
	kicad10LayerEdge     = 25
	kicad10LayerMargin   = 27
	kicad10LayerBCrtYd   = 29
	kicad10LayerFCrtYd   = 31
	kicad10LayerBFab     = 33
	kicad10LayerFFab     = 35
	kicad10LayerUserBase = 39

	defaultTextSizeMM         = 1.0
	defaultTextThicknessMM    = 0.15
	outlineClosureToleranceIU = kicadfiles.IU(100)

	zoneIslandRemovalAlways = 0
	zoneIslandRemovalNever  = 1
	zoneIslandRemovalArea   = 2
)

func DefaultTwoLayerStack() []LayerDefinition {
	layers := []LayerDefinition{
		{Number: kicad10LayerFCu, Name: kicadfiles.LayerFCu, Kind: "signal"},
		{Number: kicad10LayerBCu, Name: kicadfiles.LayerBCu, Kind: "signal"},
		{Number: kicad10LayerFAdhes, Name: kicadfiles.LayerFAdhes, Kind: "user", DisplayName: "F.Adhesive"},
		{Number: kicad10LayerBAdhes, Name: kicadfiles.LayerBAdhes, Kind: "user", DisplayName: "B.Adhesive"},
		{Number: kicad10LayerFPaste, Name: kicadfiles.LayerFPaste, Kind: "user"},
		{Number: kicad10LayerBPaste, Name: kicadfiles.LayerBPaste, Kind: "user"},
		{Number: kicad10LayerFSilkS, Name: kicadfiles.LayerFSilkS, Kind: "user", DisplayName: "F.Silkscreen"},
		{Number: kicad10LayerBSilkS, Name: kicadfiles.LayerBSilkS, Kind: "user", DisplayName: "B.Silkscreen"},
		{Number: kicad10LayerFMask, Name: kicadfiles.LayerFMask, Kind: "user"},
		{Number: kicad10LayerBMask, Name: kicadfiles.LayerBMask, Kind: "user"},
		{Number: kicad10LayerDwgs, Name: kicadfiles.LayerDwgs, Kind: "user", DisplayName: "User.Drawings"},
		{Number: kicad10LayerCmts, Name: kicadfiles.LayerCmts, Kind: "user", DisplayName: "User.Comments"},
		{Number: kicad10LayerEco1, Name: kicadfiles.LayerEco1, Kind: "user", DisplayName: "User.Eco1"},
		{Number: kicad10LayerEco2, Name: kicadfiles.LayerEco2, Kind: "user", DisplayName: "User.Eco2"},
		{Number: kicad10LayerEdge, Name: kicadfiles.LayerEdge, Kind: "user"},
		{Number: kicad10LayerMargin, Name: kicadfiles.LayerMargin, Kind: "user"},
		{Number: kicad10LayerFCrtYd, Name: kicadfiles.LayerFCrtYd, Kind: "user", DisplayName: "F.Courtyard"},
		{Number: kicad10LayerBCrtYd, Name: kicadfiles.LayerBCrtYd, Kind: "user", DisplayName: "B.Courtyard"},
		{Number: kicad10LayerFFab, Name: kicadfiles.LayerFFab, Kind: "user"},
		{Number: kicad10LayerBFab, Name: kicadfiles.LayerBFab, Kind: "user"},
	}
	for i := 1; i <= 45; i++ {
		layers = append(layers, LayerDefinition{Number: kicad10LayerUserBase + (i-1)*2, Name: kicadfiles.BoardLayer("User." + strconv.Itoa(i)), Kind: "user"})
	}
	return layers
}

func DefaultGeneral() PCBGeneral {
	return PCBGeneral{Thickness: kicadfiles.MM(1.6)}
}

func DefaultSetup() PCBSetup {
	return PCBSetup{
		Stackup:                            PCBStackup{Thickness: kicadfiles.MM(1.6)},
		AllowSoldermaskBridgesInFootprints: false,
		TentingFront:                       true,
		TentingBack:                        true,
		PlotParams:                         DefaultPlotParams(),
	}
}

func DefaultPlotParams() PCBPlotParams {
	return PCBPlotParams{
		LayerSelection:              "0x00000000_00000000_55555555_5755f5ff",
		PlotOnAllLayersSelection:    "0x00000000_00000000_00000000_00000000",
		UseGerberAttributes:         true,
		UseGerberAdvancedAttributes: true,
		CreateGerberJobFile:         true,
		DashedLineDashRatio:         12,
		DashedLineGapRatio:          3,
		SVGPrecision:                4,
		Mode:                        1,
		PDFFrontFPPropertyPopups:    true,
		PDFBackFPPropertyPopups:     true,
		PDFMetadata:                 true,
		DXFPolygonMode:              true,
		DXFImperialUnits:            true,
		DXFUsePcbNewFont:            true,
		PlotBlackAndWhite:           true,
		SketchDNPOnFab:              true,
		CrossoutDNPOnFab:            true,
		OutputFormat:                1,
		DrillShape:                  1,
		ScaleSelection:              1,
	}
}

func NewNetRegistry(names ...string) *NetRegistry {
	registry := &NetRegistry{
		nets:   []Net{{Code: 0, Name: ""}},
		byName: map[string]int{"": 0},
	}
	for _, name := range names {
		registry.EnsureNet(name)
	}
	return registry
}

func (registry *NetRegistry) EnsureNet(name string) Net {
	registry.mustBeUsable()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureInitializedLocked()
	name = strings.TrimSpace(name)
	if code, ok := registry.byName[name]; ok {
		return Net{Code: code, Name: name}
	}
	code := len(registry.nets)
	net := Net{Code: code, Name: name}
	registry.nets = append(registry.nets, net)
	registry.byName[name] = code
	return net
}

func (registry *NetRegistry) NetCode(name string) (int, bool) {
	registry.mustBeUsable()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureInitializedLocked()
	code, ok := registry.byName[strings.TrimSpace(name)]
	return code, ok
}

func (registry *NetRegistry) Nets() []Net {
	registry.mustBeUsable()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.ensureInitializedLocked()
	return slices.Clone(registry.nets)
}

func (registry *NetRegistry) mustBeUsable() {
	if registry == nil {
		panic("nil PCB net registry")
	}
}

func (registry *NetRegistry) ensureInitializedLocked() {
	if registry.byName != nil {
		return
	}
	registry.nets = []Net{{Code: 0, Name: ""}}
	registry.byName = map[string]int{"": 0}
}

func NormalizeNets(nets []Net) []Net {
	ordered := sortedNets(slices.Clone(nets))
	if len(ordered) > 0 && ordered[0].Code == 0 {
		return ordered
	}
	normalized := make([]Net, 0, len(ordered)+1)
	normalized = append(normalized, Net{Code: 0, Name: ""})
	return append(normalized, ordered...)
}

func Validate(board PCBFile) error {
	var errs kicadfiles.ValidationErrors
	if board.Version == "" {
		errs = append(errs, fieldError("version", "required"))
	} else if _, err := strconv.ParseInt(string(board.Version), 10, 64); err != nil {
		errs = append(errs, fieldError("version", "must be numeric"))
	}
	if strings.TrimSpace(board.Generator) == "" {
		errs = append(errs, fieldError("generator", "required"))
	}
	if strings.TrimSpace(board.GeneratorVersion) == "" {
		errs = append(errs, fieldError("generator_version", "required"))
	}
	if strings.TrimSpace(board.Paper.Name) == "" {
		errs = append(errs, fieldError("paper", "required"))
	}
	if board.General.Thickness <= 0 {
		errs = append(errs, fieldError("general.thickness", "must be positive"))
	}
	if board.Setup.HasStackup && board.Setup.Stackup.Thickness <= 0 {
		errs = append(errs, fieldError("setup.stackup.thickness", "must be positive"))
	}
	if board.Setup.SolderMaskMinWidth < 0 {
		errs = append(errs, fieldError("setup.solder_mask_min_width", "must be non-negative"))
	}
	if board.Setup.PadToMaskClearance < 0 {
		errs = append(errs, fieldError("setup.pad_to_mask_clearance", "must be non-negative"))
	}
	if len(board.TitleBlock.Comments) > 9 {
		errs = append(errs, fieldError("title_block.comments", "at most 9 comments allowed"))
	}
	if len(board.Layers) == 0 {
		errs = append(errs, fieldError("layers", "at least one layer required"))
	}
	layerNumbers := make(map[int]struct{}, len(board.Layers))
	layerNames := make(map[kicadfiles.BoardLayer]struct{}, len(board.Layers))
	for i, layer := range board.Layers {
		if _, ok := layerNumbers[layer.Number]; ok {
			errs = append(errs, fieldError(indexed("layers", i, "number"), "duplicate"))
		}
		layerNumbers[layer.Number] = struct{}{}
		if _, ok := layerNames[layer.Name]; ok {
			errs = append(errs, fieldError(indexed("layers", i, "name"), "duplicate"))
		}
		layerNames[layer.Name] = struct{}{}
		if !kicadfiles.IsValidBoardLayer(layer.Name) {
			errs = append(errs, fieldError(indexed("layers", i, "name"), "invalid"))
		}
		if strings.TrimSpace(layer.Kind) == "" {
			errs = append(errs, fieldError(indexed("layers", i, "kind"), "required"))
		}
	}
	netCodes := make(map[int]struct{}, len(board.Nets))
	netNames := make(map[string]struct{}, len(board.Nets))
	for i, net := range board.Nets {
		if net.Code < 0 {
			errs = append(errs, fieldError(indexed("nets", i, "code"), "must be non-negative"))
		}
		if net.Code == 0 && net.Name != "" {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "must be empty for net 0"))
		}
		if net.Code > 0 && strings.TrimSpace(net.Name) == "" {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "required"))
		}
		if _, ok := netCodes[net.Code]; ok {
			errs = append(errs, fieldError(indexed("nets", i, "code"), "duplicate"))
		}
		netCodes[net.Code] = struct{}{}
		if _, ok := netNames[net.Name]; ok {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "duplicate"))
		}
		netNames[net.Name] = struct{}{}
	}
	validNetCodes := netCodeSet(board.Nets)
	validNetNames := netNameMap(board.Nets)
	for i, footprint := range board.Footprints {
		errs = append(errs, validateFootprint(i, footprint, validNetCodes, validNetNames)...)
	}
	for i, drawing := range board.Drawings {
		errs = append(errs, validateDrawing(i, drawing, validNetCodes, validNetNames)...)
	}
	if board.RequireClosedOutline {
		errs = append(errs, validateClosedOutline(board.Drawings)...)
	}
	for i, track := range board.Tracks {
		errs = append(errs, validateTrack(i, track, validNetCodes, validNetNames)...)
	}
	for i, arc := range board.TrackArcs {
		errs = append(errs, validateTrackArc(i, arc, validNetCodes, validNetNames)...)
	}
	for i, via := range board.Vias {
		errs = append(errs, validateVia(i, via, validNetCodes, validNetNames)...)
	}
	for i, zone := range board.Zones {
		errs = append(errs, validateZone(i, zone, validNetCodes, validNetNames)...)
	}
	for i, dimension := range board.Dimensions {
		errs = append(errs, validateDimension(i, dimension)...)
	}
	preservedFamilies := map[string]struct{}{}
	for _, preserved := range board.Preserved {
		raw := strings.TrimSpace(preserved.Raw)
		if !sexpr.ValidRaw(raw) {
			continue
		}
		if family := strings.TrimSpace(preserved.Family); family != "" {
			preservedFamilies[family] = struct{}{}
			continue
		}
		if family := rawRootToken(raw); family != "" {
			preservedFamilies[family] = struct{}{}
		}
	}
	for i, preserved := range board.Preserved {
		raw := strings.TrimSpace(preserved.Raw)
		if raw == "" {
			errs = append(errs, fieldError(indexed("preserved", i, "raw"), "required"))
		} else if !sexpr.ValidRaw(raw) {
			errs = append(errs, fieldError(indexed("preserved", i, "raw"), "invalid s-expression syntax"))
		} else {
			family := rawRootToken(raw)
			explicitFamily := strings.TrimSpace(preserved.Family)
			if explicitFamily != "" {
				family = explicitFamily
			}
			if isModeledSingleInstancePCBNode(family, board) {
				errs = append(errs, fieldError(indexed("preserved", i, "family"), "must not duplicate modeled PCB node "+family))
			}
			if explicitFamily != "" && explicitFamily != preserved.Family {
				errs = append(errs, fieldError(indexed("preserved", i, "family"), "trimmed value required"))
			} else if explicitFamily != "" && rawRootToken(raw) != explicitFamily {
				errs = append(errs, fieldError(indexed("preserved", i, "family"), "must match preserved raw node"))
			}
		}
		if after := strings.TrimSpace(preserved.After); after != preserved.After {
			errs = append(errs, fieldError(indexed("preserved", i, "after"), "trimmed value required"))
		} else if after != "" && !knownPCBTopLevelNode(after) {
			if _, ok := preservedFamilies[after]; ok {
				continue
			}
			errs = append(errs, fieldError(indexed("preserved", i, "after"), "unknown PCB top-level anchor"))
		}
	}
	errs = append(errs, validatePreservedNodeAnchorGraph(board.Preserved)...)
	return errs.Err()
}

func validateFootprint(index int, footprint Footprint, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("footprints", index, field) }
	if !footprint.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(footprint.Path) == "" {
		errs = append(errs, fieldError(prefix("path"), "required"))
	}
	if strings.TrimSpace(footprint.LibraryID) == "" {
		errs = append(errs, fieldError(prefix("library_id"), "required"))
	}
	if strings.TrimSpace(footprint.Reference) == "" {
		errs = append(errs, fieldError(prefix("reference"), "required"))
	}
	if !kicadfiles.IsValidBoardLayer(footprint.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "invalid"))
	}
	referenceProperty := ""
	valueProperty := ""
	hasReferenceProperty := false
	hasValueProperty := false
	propertyNames := map[string]struct{}{}
	for propertyIndex, property := range footprint.Properties {
		errs = append(errs, validateFootprintProperty(prefix("properties"), propertyIndex, property)...)
		propertyName := strings.TrimSpace(property.Name)
		if _, exists := propertyNames[propertyName]; exists {
			errs = append(errs, fieldError(indexed(prefix("properties"), propertyIndex, "name"), "duplicate"))
		}
		propertyNames[propertyName] = struct{}{}
		switch property.Name {
		case "Reference":
			referenceProperty = property.Value
			hasReferenceProperty = true
		case "Value":
			valueProperty = property.Value
			hasValueProperty = true
		}
	}
	for propertyIndex, property := range footprint.MetadataProperties {
		errs = append(errs, validateFootprintMetadataProperty(prefix("metadata_properties"), propertyIndex, property)...)
		propertyName := strings.TrimSpace(property.Name)
		if _, exists := propertyNames[propertyName]; exists {
			errs = append(errs, fieldError(indexed(prefix("metadata_properties"), propertyIndex, "name"), "duplicate"))
		}
		propertyNames[propertyName] = struct{}{}
	}
	attributes := map[string]struct{}{}
	for attrIndex, attr := range footprint.Attributes {
		trimmedAttr := strings.TrimSpace(attr)
		if trimmedAttr == "" {
			errs = append(errs, fieldError(indexedValue(prefix("attributes"), attrIndex), "required"))
		}
		if attr != trimmedAttr {
			errs = append(errs, fieldError(indexedValue(prefix("attributes"), attrIndex), "trimmed value required"))
		}
		if _, exists := attributes[trimmedAttr]; exists {
			errs = append(errs, fieldError(indexedValue(prefix("attributes"), attrIndex), "duplicate"))
		}
		attributes[trimmedAttr] = struct{}{}
	}
	unitNames := map[string]struct{}{}
	for unitIndex, unit := range footprint.Units {
		errs = append(errs, validateFootprintUnit(prefix("units"), unitIndex, unit)...)
		unitName := strings.TrimSpace(unit.Name)
		if _, exists := unitNames[unitName]; exists {
			errs = append(errs, fieldError(indexed(prefix("units"), unitIndex, "name"), "duplicate"))
		}
		unitNames[unitName] = struct{}{}
	}
	netTiePadGroups := map[string]struct{}{}
	for groupIndex, group := range footprint.NetTiePadGroups {
		trimmedGroup := strings.TrimSpace(group)
		if trimmedGroup == "" {
			errs = append(errs, fieldError(indexedValue(prefix("net_tie_pad_groups"), groupIndex), "required"))
		}
		if group != trimmedGroup {
			errs = append(errs, fieldError(indexedValue(prefix("net_tie_pad_groups"), groupIndex), "trimmed value required"))
		}
		if _, exists := netTiePadGroups[trimmedGroup]; exists {
			errs = append(errs, fieldError(indexedValue(prefix("net_tie_pad_groups"), groupIndex), "duplicate"))
		}
		netTiePadGroups[trimmedGroup] = struct{}{}
	}
	textKinds := map[string]string{}
	for textIndex, text := range footprint.Texts {
		errs = append(errs, validateFootprintText(prefix("texts"), textIndex, text)...)
		textKinds[text.Kind] = text.Text
	}
	if len(footprint.Properties) > 0 {
		if !hasReferenceProperty {
			referenceProperty = textKinds["reference"]
		}
		if !hasValueProperty {
			valueProperty = textKinds["value"]
		}
		if referenceProperty != footprint.Reference {
			errs = append(errs, fieldError(prefix("properties.Reference"), "must match footprint reference"))
		}
		if valueProperty != footprint.Value {
			errs = append(errs, fieldError(prefix("properties.Value"), "must match footprint value"))
		}
	} else {
		referenceText, hasReferenceText := textKinds["reference"]
		if !hasReferenceText || referenceText != footprint.Reference {
			errs = append(errs, fieldError(prefix("texts.reference"), "must match footprint reference"))
		}
		valueText, hasValueText := textKinds["value"]
		if !hasValueText || valueText != footprint.Value {
			errs = append(errs, fieldError(prefix("texts.value"), "must match footprint value"))
		}
	}
	padNames := make(map[string]struct{}, len(footprint.Pads))
	for padIndex, pad := range footprint.Pads {
		errs = append(errs, validatePad(prefix("pads"), padIndex, pad, netCodes, netNames)...)
		if _, ok := padNames[pad.Name]; ok {
			errs = append(errs, fieldError(indexed(prefix("pads"), padIndex, "name"), "duplicate"))
		}
		padNames[pad.Name] = struct{}{}
	}
	for graphicIndex, graphic := range footprint.Graphics {
		errs = append(errs, validateGraphic(indexedValue(prefix("graphics"), graphicIndex), Drawing(graphic))...)
	}
	for modelIndex, model := range footprint.Models {
		if strings.TrimSpace(model.Path) == "" {
			errs = append(errs, fieldError(indexed(prefix("models"), modelIndex, "path"), "required"))
		}
	}
	return errs
}

func validateFootprintMetadataProperty(collection string, index int, property FootprintMetadataProperty) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	trimmedName := strings.TrimSpace(property.Name)
	if trimmedName == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	if property.Name != trimmedName {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "trimmed value required"))
	}
	return errs
}

func validateFootprintUnit(collection string, index int, unit FootprintUnit) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	trimmedName := strings.TrimSpace(unit.Name)
	if trimmedName == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	if unit.Name != trimmedName {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "trimmed value required"))
	}
	if len(unit.Pins) == 0 {
		errs = append(errs, fieldError(indexed(collection, index, "pins"), "required"))
	}
	pins := map[string]struct{}{}
	for pinIndex, pin := range unit.Pins {
		trimmedPin := strings.TrimSpace(pin)
		if trimmedPin == "" {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "pins"), pinIndex), "required"))
		}
		if pin != trimmedPin {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "pins"), pinIndex), "trimmed value required"))
		}
		if _, exists := pins[trimmedPin]; exists {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "pins"), pinIndex), "duplicate"))
		}
		pins[trimmedPin] = struct{}{}
	}
	return errs
}

func validateFootprintProperty(collection string, index int, property FootprintProperty) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !property.UUID.Valid() {
		errs = append(errs, fieldError(indexed(collection, index, "uuid"), "valid UUID required"))
	}
	trimmedName := strings.TrimSpace(property.Name)
	if trimmedName == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	if property.Name != trimmedName {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "trimmed value required"))
	}
	if !kicadfiles.IsValidBoardLayer(property.Layer) {
		errs = append(errs, fieldError(indexed(collection, index, "layer"), "invalid"))
	}
	return errs
}

func validateFootprintText(collection string, index int, text FootprintText) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if text.Kind != "reference" && text.Kind != "value" && text.Kind != "user" {
		errs = append(errs, fieldError(indexed(collection, index, "kind"), "invalid"))
	}
	if strings.TrimSpace(text.Text) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "text"), "required"))
	}
	if !text.UUID.Valid() {
		errs = append(errs, fieldError(indexed(collection, index, "uuid"), "valid UUID required"))
	}
	if !kicadfiles.IsValidBoardLayer(text.Layer) {
		errs = append(errs, fieldError(indexed(collection, index, "layer"), "invalid"))
	}
	return errs
}

func validatePad(collection string, index int, pad Pad, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if strings.TrimSpace(pad.Name) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "name"), "required"))
	}
	padType := padType(pad)
	if !isValidPadType(padType) {
		errs = append(errs, fieldError(indexed(collection, index, "type"), "invalid"))
	}
	if strings.TrimSpace(pad.Shape) == "" {
		errs = append(errs, fieldError(indexed(collection, index, "shape"), "required"))
	}
	if !isValidPadShape(pad.Shape) {
		errs = append(errs, fieldError(indexed(collection, index, "shape"), "invalid"))
	}
	if pad.Shape == "roundrect" && (pad.RoundRectRRatio < 0 || pad.RoundRectRRatio > 1) {
		errs = append(errs, fieldError(indexed(collection, index, "roundrect_rratio"), "must be between 0 and 1"))
	}
	if pad.Size.X <= 0 || pad.Size.Y <= 0 {
		errs = append(errs, fieldError(indexed(collection, index, "size"), "must be positive"))
	}
	if pad.Drill < 0 {
		errs = append(errs, fieldError(indexed(collection, index, "drill"), "must be non-negative"))
	}
	if len(pad.Layers) == 0 {
		errs = append(errs, fieldError(indexed(collection, index, "layers"), "required"))
	}
	padLayers := map[kicadfiles.BoardLayer]struct{}{}
	for layerIndex, layer := range pad.Layers {
		if !kicadfiles.IsValidBoardLayer(layer) {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "layers"), layerIndex), "invalid"))
		}
		if _, exists := padLayers[layer]; exists {
			errs = append(errs, fieldError(indexedValue(indexed(collection, index, "layers"), layerIndex), "duplicate"))
		}
		padLayers[layer] = struct{}{}
	}
	if (padType == "thru_hole" || padType == "np_thru_hole") && pad.Drill <= 0 {
		errs = append(errs, fieldError(indexed(collection, index, "drill"), "required for through-hole pads"))
	}
	if padType == "smd" && pad.Drill > 0 {
		errs = append(errs, fieldError(indexed(collection, index, "drill"), "not allowed for SMD pads"))
	}
	if padType == "smd" && !validSMDPadLayers(pad.Layers) {
		errs = append(errs, fieldError(indexed(collection, index, "layers"), "SMD pads require a single copper side with matching mask/paste side"))
	}
	if pad.Drill > 0 && !validDrilledPadLayers(pad.Layers) {
		errs = append(errs, fieldError(indexed(collection, index, "layers"), "drilled pads require through copper and mask layers"))
	}
	if _, ok := netCodes[pad.NetCode]; !ok {
		errs = append(errs, fieldError(indexed(collection, index, "net_code"), "unknown"))
	}
	if strings.TrimSpace(pad.NetName) != "" && pad.NetName != netNames[pad.NetCode] {
		errs = append(errs, fieldError(indexed(collection, index, "net_name"), "must match net code"))
	}
	return errs
}

func validateDrawing(index int, drawing Drawing, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	errs := validateGraphic(indexedValue("drawings", index), drawing)
	if drawing.NetCode > 0 {
		if drawing.Poly == nil {
			errs = append(errs, fieldError(indexed("drawings", index, "net_code"), "only supported for copper polygons"))
		}
		if _, ok := netCodes[drawing.NetCode]; !ok {
			errs = append(errs, fieldError(indexed("drawings", index, "net_code"), "unknown"))
		}
		if strings.TrimSpace(drawing.NetName) != "" && drawing.NetName != netNames[drawing.NetCode] {
			errs = append(errs, fieldError(indexed("drawings", index, "net_name"), "must match net code"))
		}
	}
	return errs
}

func validateGraphic(prefix string, drawing Drawing) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !drawing.UUID.Valid() {
		errs = append(errs, fieldError(prefix+".uuid", "valid UUID required"))
	}
	if !kicadfiles.IsValidBoardLayer(drawing.Layer) {
		errs = append(errs, fieldError(prefix+".layer", "invalid"))
	}
	shapes := countShapes(drawing)
	if shapes != 1 {
		errs = append(errs, fieldError(prefix, "exactly one shape required"))
		return errs
	}
	switch {
	case drawing.Line != nil:
		if drawing.Line.Width <= 0 {
			errs = append(errs, fieldError(prefix+".line.width", "must be positive"))
		}
		if drawing.Line.Start == drawing.Line.End {
			errs = append(errs, fieldError(prefix+".line", "must have non-zero length"))
		}
	case drawing.Rect != nil:
		if drawing.Rect.Width < 0 {
			errs = append(errs, fieldError(prefix+".rect.width", "must be non-negative"))
		}
		if drawing.Rect.Start.X == drawing.Rect.End.X || drawing.Rect.Start.Y == drawing.Rect.End.Y {
			errs = append(errs, fieldError(prefix+".rect", "must have non-zero area"))
		}
	case drawing.Circle != nil:
		if drawing.Circle.Width < 0 {
			errs = append(errs, fieldError(prefix+".circle.width", "must be non-negative"))
		}
		if drawing.Circle.Center == drawing.Circle.End {
			errs = append(errs, fieldError(prefix+".circle", "must have non-zero radius"))
		}
	case drawing.Arc != nil:
		if drawing.Arc.Width <= 0 {
			errs = append(errs, fieldError(prefix+".arc.width", "must be positive"))
		}
		if drawing.Arc.Start == drawing.Arc.Mid || drawing.Arc.Mid == drawing.Arc.End || drawing.Arc.Start == drawing.Arc.End {
			errs = append(errs, fieldError(prefix+".arc", "points must be distinct"))
		}
	case drawing.Poly != nil:
		if drawing.Poly.Width < 0 {
			errs = append(errs, fieldError(prefix+".poly.width", "must be non-negative"))
		}
		if countDistinctPoints(drawing.Poly.Points) < 2 {
			errs = append(errs, fieldError(prefix+".poly.points", "at least two distinct points required"))
		}
	case drawing.Curve != nil:
		if drawing.Curve.Width < 0 {
			errs = append(errs, fieldError(prefix+".curve.width", "must be non-negative"))
		}
		if len(drawing.Curve.Points) != 4 {
			errs = append(errs, fieldError(prefix+".curve.points", "exactly four points required"))
		}
		if countDistinctPoints(drawing.Curve.Points) < 2 {
			errs = append(errs, fieldError(prefix+".curve.points", "at least two distinct points required"))
		}
	case drawing.Text != nil:
		if strings.TrimSpace(drawing.Text.Text) == "" {
			errs = append(errs, fieldError(prefix+".text", "required"))
		}
	}
	return errs
}

func validateClosedOutline(drawings []Drawing) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	degrees := map[kicadfiles.Point]int{}
	outlinePoints := map[outlineCell][]kicadfiles.Point{}
	hasClosedShape := false
	for _, drawing := range drawings {
		if drawing.Layer == kicadfiles.LayerEdge && (drawing.Rect != nil || drawing.Circle != nil || drawing.Poly != nil) {
			hasClosedShape = true
			continue
		}
		if drawing.Layer != kicadfiles.LayerEdge || drawing.Line == nil {
			continue
		}
		start := canonicalOutlinePoint(drawing.Line.Start, outlinePoints)
		end := canonicalOutlinePoint(drawing.Line.End, outlinePoints)
		degrees[start]++
		degrees[end]++
	}
	if len(degrees) == 0 {
		if hasClosedShape {
			return nil
		}
		return append(errs, fieldError("drawings.edge_cuts", "closed outline required"))
	}
	for point, degree := range degrees {
		if degree != 2 {
			errs = append(errs, fieldError("drawings.edge_cuts", "outline endpoint "+kicadfiles.ToMMString(point.X)+","+kicadfiles.ToMMString(point.Y)+" is not closed"))
		}
	}
	return errs
}

type outlineCell struct {
	x int64
	y int64
}

func canonicalOutlinePoint(point kicadfiles.Point, grid map[outlineCell][]kicadfiles.Point) kicadfiles.Point {
	cell := pointOutlineCell(point)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, existing := range grid[outlineCell{x: cell.x + dx, y: cell.y + dy}] {
				xDelta := absIU(point.X - existing.X)
				if xDelta > outlineClosureToleranceIU {
					continue
				}
				yDelta := absIU(point.Y - existing.Y)
				if yDelta > outlineClosureToleranceIU {
					continue
				}
				if xDelta*xDelta+yDelta*yDelta <= outlineClosureToleranceIU*outlineClosureToleranceIU {
					return existing
				}
			}
		}
	}
	grid[cell] = append(grid[cell], point)
	return point
}

func pointOutlineCell(point kicadfiles.Point) outlineCell {
	return outlineCell{
		x: floorDivIU(point.X, outlineClosureToleranceIU),
		y: floorDivIU(point.Y, outlineClosureToleranceIU),
	}
}

func floorDivIU(value, divisor kicadfiles.IU) int64 {
	quotient := int64(value / divisor)
	if value < 0 && value%divisor != 0 {
		quotient--
	}
	return quotient
}

func absIU(value kicadfiles.IU) kicadfiles.IU {
	if value < 0 {
		return -value
	}
	return value
}

func validateTrack(index int, track Track, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("tracks", index, field) }
	if !track.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if track.Start == track.End {
		errs = append(errs, fieldError(prefix("end"), "must differ from start"))
	}
	if track.Width <= 0 {
		errs = append(errs, fieldError(prefix("width"), "must be positive"))
	}
	if !isCopperLayer(track.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "must be copper"))
	}
	errs = append(errs, validateRoutedNet(prefix, track.NetCode, track.NetName, netCodes, netNames)...)
	return errs
}

func validateTrackArc(index int, arc TrackArc, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("track_arcs", index, field) }
	if !arc.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if arc.Start == arc.Mid || arc.Mid == arc.End || arc.Start == arc.End {
		errs = append(errs, fieldError(prefix("points"), "start, mid, and end must be distinct"))
	}
	if collinear(arc.Start, arc.Mid, arc.End) {
		errs = append(errs, fieldError(prefix("points"), "start, mid, and end must not be collinear"))
	}
	if arc.Width <= 0 {
		errs = append(errs, fieldError(prefix("width"), "must be positive"))
	}
	if !isCopperLayer(arc.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "must be copper"))
	}
	errs = append(errs, validateRoutedNet(prefix, arc.NetCode, arc.NetName, netCodes, netNames)...)
	return errs
}

func collinear(a, b, c kicadfiles.Point) bool {
	left := new(big.Int).Mul(big.NewInt(int64(b.Y-a.Y)), big.NewInt(int64(c.X-b.X)))
	right := new(big.Int).Mul(big.NewInt(int64(c.Y-b.Y)), big.NewInt(int64(b.X-a.X)))
	return left.Cmp(right) == 0
}

func validateVia(index int, via Via, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("vias", index, field) }
	if !via.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if via.Size <= 0 {
		errs = append(errs, fieldError(prefix("size"), "must be positive"))
	}
	if via.Drill <= 0 {
		errs = append(errs, fieldError(prefix("drill"), "must be positive"))
	}
	if via.Drill >= via.Size {
		errs = append(errs, fieldError(prefix("drill"), "must be less than size"))
	}
	errs = append(errs, validateRoutedNet(prefix, via.NetCode, via.NetName, netCodes, netNames)...)
	copperLayerCount := 0
	for layerIndex, layer := range via.Layers {
		duplicateLayer := false
		for previousIndex := 0; previousIndex < layerIndex; previousIndex++ {
			if via.Layers[previousIndex] == layer {
				errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "duplicate"))
				duplicateLayer = true
				break
			}
		}
		if !isCopperLayer(layer) {
			errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "must be copper"))
		} else if !duplicateLayer {
			copperLayerCount++
		}
	}
	if copperLayerCount < 2 {
		errs = append(errs, fieldError(prefix("layers"), "at least two copper layers required"))
	}
	return errs
}

func validateRoutedNet(prefix func(string) string, code int, name string, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if _, ok := netCodes[code]; !ok {
		return append(errs, fieldError(prefix("net_code"), "unknown"))
	}
	return validateExplicitNetName(prefix("net_name"), code, name, netNames)
}

func validateExplicitNetName(field string, code int, name string, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	trimmed := strings.TrimSpace(name)
	if name != trimmed {
		errs = append(errs, fieldError(field, "trimmed value required"))
		return errs
	}
	if name != "" && name != netNames[code] {
		errs = append(errs, fieldError(field, "must match net code"))
	}
	return errs
}

func validateZone(index int, zone Zone, netCodes map[int]struct{}, netNames map[int]string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("zones", index, field) }
	if !zone.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if _, ok := netCodes[zone.NetCode]; !ok {
		errs = append(errs, fieldError(prefix("net_code"), "unknown"))
	}
	if expected, ok := netNames[zone.NetCode]; ok && strings.TrimSpace(zone.NetName) != "" && zone.NetName != expected {
		errs = append(errs, fieldError(prefix("net_name"), "must match net code"))
	}
	if zone.Keepout != nil {
		if zone.NetCode != 0 {
			errs = append(errs, fieldError(prefix("net_code"), "must be 0 for keepout zones"))
		}
		if strings.TrimSpace(zone.NetName) != "" {
			errs = append(errs, fieldError(prefix("net_name"), "must be empty for keepout zones"))
		}
		errs = append(errs, validateZoneKeepout(prefix, *zone.Keepout)...)
	}
	if len(zone.Layers) == 0 {
		errs = append(errs, fieldError(prefix("layers"), "required"))
	}
	zoneLayers := make(map[kicadfiles.BoardLayer]struct{}, len(zone.Layers))
	for layerIndex, layer := range zone.Layers {
		if zone.Keepout != nil {
			if !kicadfiles.IsValidBoardLayer(layer) {
				errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "invalid"))
			}
		} else if !isCopperLayer(layer) {
			errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "must be copper"))
		}
		if _, ok := zoneLayers[layer]; ok {
			errs = append(errs, fieldError(indexedValue(prefix("layers"), layerIndex), "duplicate"))
		}
		zoneLayers[layer] = struct{}{}
	}
	if len(zone.Polygons) == 0 {
		errs = append(errs, fieldError(prefix("polygons"), "required"))
	}
	for polygonIndex, polygon := range zone.Polygons {
		if countDistinctPoints(polygon) < 3 {
			errs = append(errs, fieldError(indexed(prefix("polygons"), polygonIndex, "points"), "at least three distinct points required"))
		}
	}
	if zone.Keepout != nil && len(zone.FilledPolygons) > 0 {
		errs = append(errs, fieldError(prefix("filled_polygons"), "not allowed for keepout zones"))
	}
	for polygonIndex, polygon := range zone.FilledPolygons {
		if !isCopperLayer(polygon.Layer) {
			errs = append(errs, fieldError(indexed(prefix("filled_polygons"), polygonIndex, "layer"), "must be copper"))
		}
		if _, ok := zoneLayers[polygon.Layer]; !ok {
			errs = append(errs, fieldError(indexed(prefix("filled_polygons"), polygonIndex, "layer"), "must be declared in zone layers"))
		}
		if countDistinctPoints(polygon.Points) < 3 {
			errs = append(errs, fieldError(indexed(prefix("filled_polygons"), polygonIndex, "points"), "at least three distinct points required"))
		}
	}
	if zone.Priority < 0 {
		errs = append(errs, fieldError(prefix("priority"), "must be non-negative"))
	}
	if !isValidZoneConnectMode(zoneConnectMode(zone)) {
		errs = append(errs, fieldError(prefix("connect_pads"), "invalid"))
	}
	if zone.MinThickness < 0 {
		errs = append(errs, fieldError(prefix("min_thickness"), "must be non-negative"))
	}
	if zone.Clearance < 0 {
		errs = append(errs, fieldError(prefix("clearance"), "must be non-negative"))
	}
	if zone.HatchPitch < 0 {
		errs = append(errs, fieldError(prefix("hatch_pitch"), "must be non-negative"))
	}
	if zone.Fill.ThermalGap < 0 {
		errs = append(errs, fieldError(prefix("fill.thermal_gap"), "must be non-negative"))
	}
	if zone.Fill.ThermalBridgeWidth < 0 {
		errs = append(errs, fieldError(prefix("fill.thermal_bridge_width"), "must be non-negative"))
	}
	if zone.Fill.IslandRemovalMode < zoneIslandRemovalAlways || zone.Fill.IslandRemovalMode > zoneIslandRemovalArea {
		errs = append(errs, fieldError(prefix("fill.island_removal_mode"), "must be 0, 1, or 2"))
	}
	if zone.Fill.IslandAreaMin < 0 {
		errs = append(errs, fieldError(prefix("fill.island_area_min"), "must be non-negative"))
	}
	return errs
}

func validateZoneKeepout(prefix func(string) string, keepout ZoneKeepout) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	for _, permission := range []struct {
		field string
		value string
	}{
		{field: "tracks", value: keepout.Tracks},
		{field: "vias", value: keepout.Vias},
		{field: "pads", value: keepout.Pads},
		{field: "copperpour", value: keepout.CopperPour},
		{field: "footprints", value: keepout.Footprints},
	} {
		if strings.TrimSpace(permission.value) != permission.value {
			errs = append(errs, fieldError(prefix("keepout."+permission.field), "trimmed value required"))
			continue
		}
		if !isValidKeepoutPermission(defaultString(permission.value, "allowed")) {
			errs = append(errs, fieldError(prefix("keepout."+permission.field), "invalid"))
		}
	}
	return errs
}

func isValidKeepoutPermission(value string) bool {
	switch value {
	case "allowed", "not_allowed":
		return true
	default:
		return false
	}
}

func isValidZoneConnectMode(value string) bool {
	switch value {
	case "yes", "no", "thru_hole_only":
		return true
	default:
		return false
	}
}

func validateDimension(index int, dimension Dimension) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("dimensions", index, field) }
	if !dimension.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(dimension.Type) != dimension.Type {
		errs = append(errs, fieldError(prefix("type"), "trimmed value required"))
	} else if !isValidDimensionType(dimension.Type) {
		errs = append(errs, fieldError(prefix("type"), "invalid"))
	}
	if !kicadfiles.IsValidBoardLayer(dimension.Layer) {
		errs = append(errs, fieldError(prefix("layer"), "invalid"))
	}
	if countDistinctPoints(dimension.Points) < 2 {
		errs = append(errs, fieldError(prefix("points"), "at least two distinct points required"))
	}
	if dimension.Height == 0 {
		errs = append(errs, fieldError(prefix("height"), "must be non-zero"))
	}
	if strings.TrimSpace(dimension.Text) == "" {
		errs = append(errs, fieldError(prefix("text"), "required"))
	}
	return errs
}

func isValidDimensionType(value string) bool {
	switch value {
	case "aligned", "orthogonal", "radial", "leader":
		return true
	default:
		return false
	}
}

func countShapes(drawing Drawing) int {
	count := 0
	if drawing.Line != nil {
		count++
	}
	if drawing.Rect != nil {
		count++
	}
	if drawing.Circle != nil {
		count++
	}
	if drawing.Arc != nil {
		count++
	}
	if drawing.Poly != nil {
		count++
	}
	if drawing.Curve != nil {
		count++
	}
	if drawing.Text != nil {
		count++
	}
	return count
}

func drawingKind(drawing Drawing) string {
	switch {
	case drawing.Line != nil:
		return "line"
	case drawing.Rect != nil:
		return "rect"
	case drawing.Circle != nil:
		return "circle"
	case drawing.Arc != nil:
		return "arc"
	case drawing.Poly != nil:
		return "poly"
	case drawing.Curve != nil:
		return "curve"
	case drawing.Text != nil:
		return "text"
	default:
		return drawing.Kind
	}
}

func countDistinctPoints(points []kicadfiles.Point) int {
	seen := make(map[kicadfiles.Point]struct{}, len(points))
	for _, point := range points {
		seen[point] = struct{}{}
	}
	return len(seen)
}

func countDistinctCopperLayers(layers []kicadfiles.BoardLayer) int {
	seen := map[kicadfiles.BoardLayer]struct{}{}
	for _, layer := range layers {
		if isCopperLayer(layer) {
			seen[layer] = struct{}{}
		}
	}
	return len(seen)
}

func isCopperLayer(layer kicadfiles.BoardLayer) bool {
	name := string(layer)
	return layer == kicadfiles.LayerFCu || layer == kicadfiles.LayerBCu || strings.HasPrefix(name, "In") && strings.HasSuffix(name, ".Cu") && kicadfiles.IsValidBoardLayer(layer)
}

func padType(pad Pad) string {
	if strings.TrimSpace(pad.Type) != "" {
		return pad.Type
	}
	if pad.Drill > 0 {
		return "thru_hole"
	}
	return "smd"
}

func isValidPadType(value string) bool {
	switch value {
	case "smd", "thru_hole", "np_thru_hole", "connect":
		return true
	default:
		return false
	}
}

func isValidPadShape(value string) bool {
	switch value {
	case "rect", "circle", "oval", "trapezoid", "roundrect", "custom":
		return true
	default:
		return false
	}
}

func roundRectRRatio(pad Pad) float64 {
	if pad.RoundRectRRatio == 0 {
		return 0.25
	}
	return pad.RoundRectRRatio
}

func hasPadLayerSet(layers []kicadfiles.BoardLayer, required ...kicadfiles.BoardLayer) bool {
	seen := make(map[kicadfiles.BoardLayer]struct{}, len(layers))
	for _, layer := range layers {
		seen[layer] = struct{}{}
	}
	for _, layer := range required {
		if _, ok := seen[layer]; !ok {
			return false
		}
	}
	return true
}

func validDrilledPadLayers(layers []kicadfiles.BoardLayer) bool {
	if hasPadLayerSet(layers, kicadfiles.LayerAllCu, kicadfiles.LayerAllMask) {
		return true
	}
	return countDistinctCopperLayers(layers) >= 2 && hasAnyMaskLayer(layers)
}

func validSMDPadLayers(layers []kicadfiles.BoardLayer) bool {
	if hasPadLayerSet(layers, kicadfiles.LayerAllCu) || hasPadLayerSet(layers, kicadfiles.LayerAllMask) {
		return false
	}
	hasFrontCopper := hasPadLayerSet(layers, kicadfiles.LayerFCu)
	hasBackCopper := hasPadLayerSet(layers, kicadfiles.LayerBCu)
	if hasFrontCopper == hasBackCopper {
		return false
	}
	if countDistinctCopperLayers(layers) != 1 {
		return false
	}
	if hasFrontCopper {
		return !hasPadLayerSet(layers, kicadfiles.LayerBMask) && !hasPadLayerSet(layers, kicadfiles.LayerBPaste)
	}
	return !hasPadLayerSet(layers, kicadfiles.LayerFMask) && !hasPadLayerSet(layers, kicadfiles.LayerFPaste)
}

func hasAnyMaskLayer(layers []kicadfiles.BoardLayer) bool {
	for _, layer := range layers {
		if layer == kicadfiles.LayerAllMask || layer == kicadfiles.LayerFMask || layer == kicadfiles.LayerBMask {
			return true
		}
	}
	return false
}

func fieldError(field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{Section: "pcb", Field: field, Message: message}
}

func indexed(collection string, index int, field string) string {
	return collection + "[" + strconv.Itoa(index) + "]." + field
}

func indexedValue(collection string, index int) string {
	return collection + "[" + strconv.Itoa(index) + "]"
}
