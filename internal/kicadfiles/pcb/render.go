package pcb

import (
	"cmp"
	"io"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

func Write(w io.Writer, board PCBFile) error {
	if err := validateNetZeroForNormalization(board.Nets); err != nil {
		return err
	}
	board.Nets = NormalizeNets(board.Nets)
	if err := Validate(board); err != nil {
		return err
	}
	node, err := render(board)
	if err != nil {
		return err
	}
	return sexpr.Render(w, node)
}

func validateNetZeroForNormalization(nets []Net) error {
	for i, net := range nets {
		if net.Code == 0 && net.Name != "" {
			return fieldError(indexed("nets", i, "name"), "must be empty for net 0")
		}
	}
	return nil
}

func render(board PCBFile) (sexpr.List, error) {
	version, err := strconv.ParseInt(string(board.Version), 10, 64)
	if err != nil {
		return nil, err
	}
	nodes := []sexpr.Node{
		sexpr.A("kicad_pcb"),
		sexpr.L(sexpr.A("version"), sexpr.I(version)),
		sexpr.L(sexpr.A("generator"), sexpr.S(strings.TrimSpace(board.Generator))),
		sexpr.L(sexpr.A("generator_version"), sexpr.S(strings.TrimSpace(board.GeneratorVersion))),
		renderGeneral(board.General),
		sexpr.L(sexpr.A("paper"), sexpr.S(strings.TrimSpace(board.Paper.Name))),
	}
	if title := renderTitleBlock(board.TitleBlock); len(title) > 1 {
		nodes = append(nodes, title)
	}
	nodes = append(nodes, renderLayers(board.Layers), renderSetup(board.Setup))
	netNames := netNameMap(board.Nets)
	layerNumbers := layerNumberMap(board.Layers)
	footprints := sortedFootprints(board.Footprints)
	routeNetCodes := routedNetSortCodes(footprints, netNames)
	nodes = append(nodes, renderNets(board.Nets)...)
	for _, footprint := range footprints {
		nodes = append(nodes, renderFootprint(footprint, netNames))
	}
	for _, drawing := range sortedDrawings(board.Drawings, layerNumbers) {
		nodes = append(nodes, renderDrawing(drawing))
	}
	for _, item := range sortedRoutedItems(board.Tracks, board.TrackArcs, board.Vias, layerNumbers, netNames, routeNetCodes) {
		nodes = append(nodes, item.render(netNames))
	}
	for _, zone := range board.Zones {
		nodes = append(nodes, renderZone(zone, netNames))
	}
	for _, dimension := range board.Dimensions {
		nodes = append(nodes, renderDimension(dimension))
	}
	if board.EmbeddedFonts != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("embedded_fonts"), yesNo(*board.EmbeddedFonts)))
	}
	nodes = insertPreservedNodes(nodes, board.Preserved)
	return sexpr.L(nodes...), nil
}

func renderNets(nets []Net) []sexpr.Node {
	nodes := make([]sexpr.Node, 0, len(nets))
	for _, net := range nets {
		nodes = append(nodes, sexpr.L(sexpr.A("net"), sexpr.I(int64(net.Code)), sexpr.S(net.Name)))
	}
	return nodes
}

func renderGeneral(general PCBGeneral) sexpr.List {
	return sexpr.L(
		sexpr.A("general"),
		sexpr.L(sexpr.A("thickness"), fixed(general.Thickness)),
		sexpr.L(sexpr.A("legacy_teardrops"), yesNo(general.LegacyTeardrops)),
	)
}

func renderLayers(layers []LayerDefinition) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("layers")}
	for _, layer := range layers {
		layerNodes := []sexpr.Node{sexpr.I(int64(layer.Number)), sexpr.S(string(layer.Name)), sexpr.A(layer.Kind)}
		if strings.TrimSpace(layer.DisplayName) != "" {
			layerNodes = append(layerNodes, sexpr.S(layer.DisplayName))
		}
		nodes = append(nodes, sexpr.L(layerNodes...))
	}
	return sexpr.L(nodes...)
}

func renderSetup(setup PCBSetup) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("setup")}
	if setup.HasStackup {
		nodes = append(nodes, sexpr.L(sexpr.A("stackup"), sexpr.L(sexpr.A("thickness"), fixed(setup.Stackup.Thickness))))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("pad_to_mask_clearance"), fixed(setup.PadToMaskClearance)),
	)
	if setup.SolderMaskMinWidth > 0 {
		nodes = append(nodes, sexpr.L(sexpr.A("solder_mask_min_width"), fixed(setup.SolderMaskMinWidth)))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("allow_soldermask_bridges_in_footprints"), yesNo(setup.AllowSoldermaskBridgesInFootprints)),
		renderSidePair("tenting", setup.TentingFront, setup.TentingBack),
		renderSidePair("covering", setup.CoveringFront, setup.CoveringBack),
		renderSidePair("plugging", setup.PluggingFront, setup.PluggingBack),
		sexpr.L(sexpr.A("capping"), yesNo(setup.Capping)),
		sexpr.L(sexpr.A("filling"), yesNo(setup.Filling)),
		renderPlotParams(setup.PlotParams),
	)
	return sexpr.L(nodes...)
}

func renderSidePair(name string, front, back bool) sexpr.List {
	return sexpr.L(
		sexpr.A(name),
		sexpr.L(sexpr.A("front"), yesNo(front)),
		sexpr.L(sexpr.A("back"), yesNo(back)),
	)
}

func renderPlotParams(params PCBPlotParams) sexpr.List {
	params = normalizePlotParams(params)
	return sexpr.L(
		sexpr.A("pcbplotparams"),
		sexpr.L(sexpr.A("layerselection"), sexpr.A(params.LayerSelection)),
		sexpr.L(sexpr.A("plot_on_all_layers_selection"), sexpr.A(params.PlotOnAllLayersSelection)),
		sexpr.L(sexpr.A("disableapertmacros"), yesNo(params.DisableApertureMacros)),
		sexpr.L(sexpr.A("usegerberextensions"), yesNo(params.UseGerberExtensions)),
		sexpr.L(sexpr.A("usegerberattributes"), yesNo(params.UseGerberAttributes)),
		sexpr.L(sexpr.A("usegerberadvancedattributes"), yesNo(params.UseGerberAdvancedAttributes)),
		sexpr.L(sexpr.A("creategerberjobfile"), yesNo(params.CreateGerberJobFile)),
		sexpr.L(sexpr.A("dashed_line_dash_ratio"), sexpr.I(int64(params.DashedLineDashRatio))),
		sexpr.L(sexpr.A("dashed_line_gap_ratio"), sexpr.I(int64(params.DashedLineGapRatio))),
		sexpr.L(sexpr.A("svgprecision"), sexpr.I(int64(params.SVGPrecision))),
		sexpr.L(sexpr.A("plotframeref"), yesNo(params.PlotFrameRef)),
		sexpr.L(sexpr.A("mode"), sexpr.I(int64(params.Mode))),
		sexpr.L(sexpr.A("useauxorigin"), yesNo(params.UseAuxOrigin)),
		sexpr.L(sexpr.A("pdf_front_fp_property_popups"), yesNo(params.PDFFrontFPPropertyPopups)),
		sexpr.L(sexpr.A("pdf_back_fp_property_popups"), yesNo(params.PDFBackFPPropertyPopups)),
		sexpr.L(sexpr.A("pdf_metadata"), yesNo(params.PDFMetadata)),
		sexpr.L(sexpr.A("pdf_single_document"), yesNo(params.PDFSingleDocument)),
		sexpr.L(sexpr.A("dxfpolygonmode"), yesNo(params.DXFPolygonMode)),
		sexpr.L(sexpr.A("dxfimperialunits"), yesNo(params.DXFImperialUnits)),
		sexpr.L(sexpr.A("dxfusepcbnewfont"), yesNo(params.DXFUsePcbNewFont)),
		sexpr.L(sexpr.A("psnegative"), yesNo(params.PSNegative)),
		sexpr.L(sexpr.A("psa4output"), yesNo(params.PSA4Output)),
		sexpr.L(sexpr.A("plot_black_and_white"), yesNo(params.PlotBlackAndWhite)),
		sexpr.L(sexpr.A("sketchpadsonfab"), yesNo(params.SketchPadsOnFab)),
		sexpr.L(sexpr.A("plotpadnumbers"), yesNo(params.PlotPadNumbers)),
		sexpr.L(sexpr.A("hidednponfab"), yesNo(params.HideDNPOnFab)),
		sexpr.L(sexpr.A("sketchdnponfab"), yesNo(params.SketchDNPOnFab)),
		sexpr.L(sexpr.A("crossoutdnponfab"), yesNo(params.CrossoutDNPOnFab)),
		sexpr.L(sexpr.A("subtractmaskfromsilk"), yesNo(params.SubtractMaskFromSilk)),
		sexpr.L(sexpr.A("outputformat"), sexpr.I(int64(params.OutputFormat))),
		sexpr.L(sexpr.A("mirror"), yesNo(params.Mirror)),
		sexpr.L(sexpr.A("drillshape"), sexpr.I(int64(params.DrillShape))),
		sexpr.L(sexpr.A("scaleselection"), sexpr.I(int64(params.ScaleSelection))),
		sexpr.L(sexpr.A("outputdirectory"), sexpr.S(params.OutputDirectory)),
	)
}

func normalizePlotParams(params PCBPlotParams) PCBPlotParams {
	if params == (PCBPlotParams{}) {
		return DefaultPlotParams()
	}
	if strings.TrimSpace(params.LayerSelection) == "" {
		params.LayerSelection = "0x00000000_00000000_00000000_00000000"
	}
	if strings.TrimSpace(params.PlotOnAllLayersSelection) == "" {
		params.PlotOnAllLayersSelection = "0x00000000_00000000_00000000_00000000"
	}
	return params
}

func renderTitleBlock(title kicadfiles.TitleBlock) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("title_block")}
	if title.Title != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("title"), sexpr.S(title.Title)))
	}
	if title.Date != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("date"), sexpr.S(title.Date)))
	}
	if title.Revision != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("rev"), sexpr.S(title.Revision)))
	}
	if title.Company != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("company"), sexpr.S(title.Company)))
	}
	for i, comment := range title.Comments {
		nodes = append(nodes, sexpr.L(sexpr.A("comment"), sexpr.I(int64(i+1)), sexpr.S(comment)))
	}
	return sexpr.L(nodes...)
}

func renderFootprint(footprint Footprint, netNames map[int]string) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("footprint"),
		sexpr.S(footprint.LibraryID),
	}
	if footprint.Locked {
		nodes = append(nodes, sexpr.L(sexpr.A("locked"), sexpr.A("yes")))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("layer"), sexpr.S(string(footprint.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(footprint.UUID))),
		renderAt(footprint.Position, footprint.Rotation),
	)
	if strings.TrimSpace(footprint.Description) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("descr"), sexpr.S(footprint.Description)))
	}
	if strings.TrimSpace(footprint.Tags) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("tags"), sexpr.S(footprint.Tags)))
	}
	for _, property := range footprint.Properties {
		nodes = append(nodes, renderFootprintProperty(property))
	}
	for _, property := range footprint.MetadataProperties {
		nodes = append(nodes, renderFootprintMetadataProperty(property))
	}
	if strings.TrimSpace(footprint.Path) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("path"), sexpr.S(footprint.Path)))
	}
	if strings.TrimSpace(footprint.SheetName) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("sheetname"), sexpr.S(footprint.SheetName)))
	}
	if strings.TrimSpace(footprint.SheetFile) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("sheetfile"), sexpr.S(footprint.SheetFile)))
	}
	if len(footprint.Units) > 0 {
		nodes = append(nodes, renderFootprintUnits(footprint.Units))
	}
	if len(footprint.Attributes) > 0 {
		attrNodes := []sexpr.Node{sexpr.A("attr")}
		for _, attr := range footprint.Attributes {
			attrNodes = append(attrNodes, sexpr.A(attr))
		}
		nodes = append(nodes, sexpr.L(attrNodes...))
	}
	if len(footprint.NetTiePadGroups) > 0 {
		groupNodes := []sexpr.Node{sexpr.A("net_tie_pad_groups")}
		for _, group := range footprint.NetTiePadGroups {
			groupNodes = append(groupNodes, sexpr.S(group))
		}
		nodes = append(nodes, sexpr.L(groupNodes...))
	}
	if footprint.DuplicatePadNumbersAreJumpers != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("duplicate_pad_numbers_are_jumpers"), yesNo(*footprint.DuplicatePadNumbersAreJumpers)))
	}
	for _, text := range footprint.Texts {
		nodes = append(nodes, renderFootprintText(text))
	}
	for _, graphic := range footprint.Graphics {
		nodes = append(nodes, renderFootprintGraphic(graphic))
	}
	for _, pad := range footprint.Pads {
		nodes = append(nodes, renderPad(pad, netNames[pad.NetCode]))
	}
	if footprint.EmbeddedFonts != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("embedded_fonts"), yesNo(*footprint.EmbeddedFonts)))
	}
	for _, model := range footprint.Models {
		nodes = append(nodes, renderModel3D(model))
	}
	return sexpr.L(nodes...)
}

func renderFootprintMetadataProperty(property FootprintMetadataProperty) sexpr.List {
	return sexpr.L(sexpr.A("property"), sexpr.S(property.Name), sexpr.S(property.Value))
}

func renderFootprintUnits(units []FootprintUnit) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("units")}
	for _, unit := range units {
		unitNodes := []sexpr.Node{
			sexpr.A("unit"),
			sexpr.L(sexpr.A("name"), sexpr.S(unit.Name)),
		}
		pinNodes := []sexpr.Node{sexpr.A("pins")}
		for _, pin := range unit.Pins {
			pinNodes = append(pinNodes, sexpr.S(pin))
		}
		unitNodes = append(unitNodes, sexpr.L(pinNodes...))
		nodes = append(nodes, sexpr.L(unitNodes...))
	}
	return sexpr.L(nodes...)
}

func renderFootprintProperty(property FootprintProperty) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("property"),
		sexpr.S(property.Name),
		sexpr.S(property.Value),
		renderAtWithRotation(property.Position, property.Rotation),
	}
	if property.Unlocked {
		nodes = append(nodes, sexpr.L(sexpr.A("unlocked"), sexpr.A("yes")))
	}
	nodes = append(nodes, sexpr.L(sexpr.A("layer"), sexpr.S(string(property.Layer))))
	if property.Hide {
		nodes = append(nodes, sexpr.L(sexpr.A("hide"), sexpr.A("yes")))
	}
	nodes = append(nodes, sexpr.L(sexpr.A("uuid"), sexpr.S(string(property.UUID))), renderEffects(property.Effects))
	return sexpr.L(nodes...)
}

func renderEffects(effects TextEffects) sexpr.List {
	size := effects.FontSize
	if size.X == 0 {
		size.X = kicadfiles.MM(defaultTextSizeMM)
	}
	if size.Y == 0 {
		size.Y = kicadfiles.MM(defaultTextSizeMM)
	}
	fontNodes := []sexpr.Node{
		sexpr.A("font"),
		sexpr.L(sexpr.A("size"), fixed(size.X), fixed(size.Y)),
	}
	if !effects.OmitFontThickness {
		thickness := effects.FontThickness
		if thickness == 0 {
			thickness = kicadfiles.MM(defaultTextThicknessMM)
		}
		fontNodes = append(fontNodes, sexpr.L(sexpr.A("thickness"), fixed(thickness)))
	}
	font := sexpr.L(fontNodes...)
	nodes := []sexpr.Node{sexpr.A("effects"), font}
	if len(effects.Justify) > 0 {
		justify := []sexpr.Node{sexpr.A("justify")}
		for _, value := range effects.Justify {
			justify = append(justify, sexpr.A(value))
		}
		nodes = append(nodes, sexpr.L(justify...))
	}
	return sexpr.L(nodes...)
}

func renderModel3D(model Model3D) sexpr.List {
	return sexpr.L(
		sexpr.A("model"),
		sexpr.S(model.Path),
		sexpr.L(sexpr.A("offset"), renderXYZ(model.Offset)),
		sexpr.L(sexpr.A("scale"), renderXYZ(defaultScale(model.Scale))),
		sexpr.L(sexpr.A("rotate"), renderXYZ(model.Rotate)),
	)
}

func renderXYZ(value XYZ) sexpr.List {
	return sexpr.L(sexpr.A("xyz"), sexpr.F(value.X), sexpr.F(value.Y), sexpr.F(value.Z))
}

func defaultScale(value XYZ) XYZ {
	if value.X == 0 {
		value.X = 1
	}
	if value.Y == 0 {
		value.Y = 1
	}
	if value.Z == 0 {
		value.Z = 1
	}
	return value
}

func renderFootprintText(text FootprintText) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("fp_text"),
		sexpr.A(text.Kind),
		sexpr.S(text.Text),
		renderAt(text.Position, text.Rotation),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(text.Layer))),
	}
	if text.UUID.Valid() {
		nodes = append(nodes, sexpr.L(sexpr.A("uuid"), sexpr.S(string(text.UUID))))
	}
	return sexpr.L(nodes...)
}

func renderPad(pad Pad, netName string) sexpr.List {
	if strings.TrimSpace(pad.NetName) != "" {
		netName = pad.NetName
	}
	nodes := []sexpr.Node{
		sexpr.A("pad"),
		sexpr.S(pad.Name),
		sexpr.A(padType(pad)),
		sexpr.A(pad.Shape),
		renderAt(pad.Position, pad.Rotation),
		sexpr.L(sexpr.A("size"), fixed(pad.Size.X), fixed(pad.Size.Y)),
		renderLayerList("layers", pad.Layers),
	}
	if pad.Drill > 0 {
		nodes = append(nodes, sexpr.L(sexpr.A("drill"), fixed(pad.Drill)))
	}
	if pad.RemoveUnusedLayers != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("remove_unused_layers"), yesNo(*pad.RemoveUnusedLayers)))
	}
	if strings.TrimSpace(pad.PinFunction) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("pinfunction"), sexpr.S(pad.PinFunction)))
	}
	if strings.TrimSpace(pad.PinType) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("pintype"), sexpr.S(pad.PinType)))
	}
	if pad.ThermalBridgeAngle != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("thermal_bridge_angle"), sexpr.F(*pad.ThermalBridgeAngle)))
	}
	if pad.Shape == "roundrect" {
		nodes = append(nodes, sexpr.L(sexpr.A("roundrect_rratio"), sexpr.F(roundRectRRatio(pad))))
	}
	if pad.NetCode > 0 || strings.TrimSpace(pad.NetName) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("net"), sexpr.I(int64(pad.NetCode)), sexpr.S(netName)))
	}
	if pad.UUID.Valid() {
		nodes = append(nodes, sexpr.L(sexpr.A("uuid"), sexpr.S(string(pad.UUID))))
	}
	if pad.Teardrops != nil {
		nodes = append(nodes, renderTeardrops(*pad.Teardrops))
	}
	return sexpr.L(nodes...)
}

func renderTeardrops(teardrops TeardropSettings) sexpr.List {
	return sexpr.L(
		sexpr.A("teardrops"),
		sexpr.L(sexpr.A("best_length_ratio"), sexpr.F(teardrops.BestLengthRatio)),
		sexpr.L(sexpr.A("max_length"), fixed(teardrops.MaxLength)),
		sexpr.L(sexpr.A("best_width_ratio"), sexpr.F(teardrops.BestWidthRatio)),
		sexpr.L(sexpr.A("max_width"), fixed(teardrops.MaxWidth)),
		sexpr.L(sexpr.A("curved_edges"), yesNo(teardrops.CurvedEdges)),
		sexpr.L(sexpr.A("filter_ratio"), sexpr.F(teardrops.FilterRatio)),
		sexpr.L(sexpr.A("enabled"), yesNo(teardrops.Enabled)),
		sexpr.L(sexpr.A("allow_two_segments"), yesNo(teardrops.AllowTwoSegments)),
		sexpr.L(sexpr.A("prefer_zone_connections"), yesNo(teardrops.PreferZoneConnection)),
	)
}

func renderDrawing(drawing Drawing) sexpr.List {
	return renderGraphic("gr", Drawing(drawing))
}

func renderFootprintGraphic(graphic FootprintGraphic) sexpr.List {
	return renderGraphic("fp", Drawing(graphic))
}

func renderGraphic(prefix string, drawing Drawing) sexpr.List {
	nodes := []sexpr.Node{sexpr.A(prefix + "_" + drawingKind(drawing))}
	switch {
	case drawing.Line != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("start"), fixed(drawing.Line.Start.X), fixed(drawing.Line.Start.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Line.End.X), fixed(drawing.Line.End.Y)),
			renderStroke(drawing.Line.Width, drawing.StrokeType),
		)
	case drawing.Rect != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("start"), fixed(drawing.Rect.Start.X), fixed(drawing.Rect.Start.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Rect.End.X), fixed(drawing.Rect.End.Y)),
			renderStroke(drawing.Rect.Width, drawing.StrokeType),
			sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))),
		)
	case drawing.Circle != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("center"), fixed(drawing.Circle.Center.X), fixed(drawing.Circle.Center.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Circle.End.X), fixed(drawing.Circle.End.Y)),
			renderStroke(drawing.Circle.Width, drawing.StrokeType),
			sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))),
		)
	case drawing.Arc != nil:
		nodes = append(nodes,
			sexpr.L(sexpr.A("start"), fixed(drawing.Arc.Start.X), fixed(drawing.Arc.Start.Y)),
			sexpr.L(sexpr.A("mid"), fixed(drawing.Arc.Mid.X), fixed(drawing.Arc.Mid.Y)),
			sexpr.L(sexpr.A("end"), fixed(drawing.Arc.End.X), fixed(drawing.Arc.End.Y)),
			renderStroke(drawing.Arc.Width, drawing.StrokeType),
			sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))),
		)
	case drawing.Poly != nil:
		nodes = append(nodes, renderPoints(drawing.Poly.Points), renderStroke(drawing.Poly.Width, drawing.StrokeType), sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))))
	case drawing.Curve != nil:
		nodes = append(nodes, renderPoints(drawing.Curve.Points), renderStroke(drawing.Curve.Width, drawing.StrokeType))
		if strings.TrimSpace(drawing.Fill) != "" {
			nodes = append(nodes, sexpr.L(sexpr.A("fill"), sexpr.A(fillMode(drawing.Fill))))
		}
	case drawing.Text != nil:
		nodes = append(nodes,
			sexpr.S(drawing.Text.Text),
			renderAt(drawing.Text.Position, drawing.Text.Rotation),
		)
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("layer"), sexpr.S(string(drawing.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(drawing.UUID))),
	)
	if drawing.Text != nil {
		nodes = append(nodes, renderEffects(drawing.Text.Effects))
	}
	if drawing.NetCode > 0 && drawing.Poly != nil {
		nodes = append(nodes, sexpr.L(sexpr.A("net"), sexpr.I(int64(drawing.NetCode))))
	}
	return sexpr.L(nodes...)
}

func renderStroke(width kicadfiles.IU, strokeType string) sexpr.List {
	if strings.TrimSpace(strokeType) == "" {
		strokeType = "solid"
	}
	return sexpr.L(sexpr.A("stroke"), sexpr.L(sexpr.A("width"), fixed(width)), sexpr.L(sexpr.A("type"), sexpr.A(strokeType)))
}

func fillMode(fill string) string {
	if strings.TrimSpace(fill) == "" {
		return "none"
	}
	return fill
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultIU(value, fallback kicadfiles.IU) kicadfiles.IU {
	if value == 0 {
		return fallback
	}
	return value
}

func renderTrack(track Track, netNames map[int]string) sexpr.List {
	return sexpr.L(
		sexpr.A("segment"),
		sexpr.L(sexpr.A("start"), fixed(track.Start.X), fixed(track.Start.Y)),
		sexpr.L(sexpr.A("end"), fixed(track.End.X), fixed(track.End.Y)),
		sexpr.L(sexpr.A("width"), fixed(track.Width)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(track.Layer))),
		sexpr.L(sexpr.A("net"), sexpr.I(int64(track.NetCode))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(track.UUID))),
	)
}

func renderTrackArc(arc TrackArc, netNames map[int]string) sexpr.List {
	return sexpr.L(
		sexpr.A("arc"),
		sexpr.L(sexpr.A("start"), fixed(arc.Start.X), fixed(arc.Start.Y)),
		sexpr.L(sexpr.A("mid"), fixed(arc.Mid.X), fixed(arc.Mid.Y)),
		sexpr.L(sexpr.A("end"), fixed(arc.End.X), fixed(arc.End.Y)),
		sexpr.L(sexpr.A("width"), fixed(arc.Width)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(arc.Layer))),
		sexpr.L(sexpr.A("net"), sexpr.I(int64(arc.NetCode))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(arc.UUID))),
	)
}

func renderVia(via Via, netNames map[int]string) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("via"),
		sexpr.L(sexpr.A("at"), fixed(via.Position.X), fixed(via.Position.Y)),
		sexpr.L(sexpr.A("size"), fixed(via.Size)),
		sexpr.L(sexpr.A("drill"), fixed(via.Drill)),
		renderLayerList("layers", via.Layers),
	}
	if via.TentingFront || via.TentingBack {
		nodes = append(nodes, renderSidePair("tenting", via.TentingFront, via.TentingBack))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("net"), sexpr.I(int64(via.NetCode))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(via.UUID))),
	)
	return sexpr.L(nodes...)
}

func routedNetName(code int, explicit string, netNames map[int]string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	return netNames[code]
}

func renderZone(zone Zone, netNames map[int]string) sexpr.List {
	netName := netNames[zone.NetCode]
	if strings.TrimSpace(zone.NetName) != "" {
		netName = zone.NetName
	}
	nodes := []sexpr.Node{sexpr.A("zone")}
	if zone.Keepout == nil {
		nodes = append(nodes,
			sexpr.L(sexpr.A("net"), sexpr.I(int64(zone.NetCode))),
			sexpr.L(sexpr.A("net_name"), sexpr.S(netName)),
		)
	}
	nodes = append(nodes,
		renderLayerList("layers", zone.Layers),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(zone.UUID))),
	)
	if strings.TrimSpace(zone.Name) != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("name"), sexpr.S(zone.Name)))
	}
	nodes = append(nodes, sexpr.L(sexpr.A("hatch"), sexpr.A(defaultString(zone.HatchStyle, "edge")), fixed(defaultIU(zone.HatchPitch, kicadfiles.MM(0.5)))))
	if zone.Priority > 0 {
		nodes = append(nodes, sexpr.L(sexpr.A("priority"), sexpr.I(int64(zone.Priority))))
	}
	for _, attr := range zone.Attributes {
		nodes = append(nodes, renderZoneAttribute(attr))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("connect_pads"), sexpr.A(zoneConnectMode(zone)), sexpr.L(sexpr.A("clearance"), fixed(zone.Clearance))),
		sexpr.L(sexpr.A("min_thickness"), fixed(defaultIU(zone.MinThickness, kicadfiles.MM(0.25)))),
		sexpr.L(sexpr.A("filled_areas_thickness"), yesNo(zone.FilledAreasThickness)),
	)
	if zone.Keepout != nil {
		nodes = append(nodes, renderZoneKeepout(*zone.Keepout))
	}
	nodes = append(nodes, renderZoneFill(zone.Fill))
	for _, polygon := range zone.Polygons {
		nodes = append(nodes, sexpr.L(sexpr.A("polygon"), renderPoints(polygon)))
	}
	for _, polygon := range zone.FilledPolygons {
		nodes = append(nodes, sexpr.L(sexpr.A("filled_polygon"), sexpr.L(sexpr.A("layer"), sexpr.S(string(polygon.Layer))), renderPoints(polygon.Points)))
	}
	return sexpr.L(nodes...)
}

func renderZoneKeepout(keepout ZoneKeepout) sexpr.List {
	return sexpr.L(
		sexpr.A("keepout"),
		sexpr.L(sexpr.A("tracks"), sexpr.A(defaultString(keepout.Tracks, "allowed"))),
		sexpr.L(sexpr.A("vias"), sexpr.A(defaultString(keepout.Vias, "allowed"))),
		sexpr.L(sexpr.A("pads"), sexpr.A(defaultString(keepout.Pads, "allowed"))),
		sexpr.L(sexpr.A("copperpour"), sexpr.A(defaultString(keepout.CopperPour, "allowed"))),
		sexpr.L(sexpr.A("footprints"), sexpr.A(defaultString(keepout.Footprints, "allowed"))),
	)
}

func renderZoneAttribute(attr ZoneAttribute) sexpr.List {
	children := []sexpr.Node{sexpr.A(attr.Name)}
	keys := make([]string, 0, len(attr.Values))
	for key := range attr.Values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		children = append(children, sexpr.L(sexpr.A(key), sexpr.S(attr.Values[key])))
	}
	return sexpr.L(sexpr.A("attr"), sexpr.L(children...))
}

func zoneConnectMode(zone Zone) string {
	if strings.TrimSpace(zone.ConnectPadsMode) != "" {
		return zone.ConnectPadsMode
	}
	if zone.ConnectPads {
		return "yes"
	}
	return "no"
}

func renderZoneFill(fill ZoneFillSettings) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("fill")}
	if fill.Enabled {
		nodes = append(nodes, sexpr.A("yes"))
	}
	// KiCad 10.0.3 rejects "(fill no ...)" for unfilled generated zones; omit
	// the flag unless the zone is already filled.
	nodes = append(nodes,
		sexpr.L(sexpr.A("thermal_gap"), fixed(fill.ThermalGap)),
		sexpr.L(sexpr.A("thermal_bridge_width"), fixed(fill.ThermalBridgeWidth)),
		sexpr.L(sexpr.A("island_removal_mode"), sexpr.I(int64(fill.IslandRemovalMode))),
		sexpr.L(sexpr.A("island_area_min"), sexpr.F(fill.IslandAreaMin)),
	)
	return sexpr.L(nodes...)
}

func renderDimension(dimension Dimension) sexpr.List {
	return sexpr.L(
		sexpr.A("dimension"),
		sexpr.L(sexpr.A("type"), sexpr.A(dimension.Type)),
		sexpr.L(sexpr.A("layer"), sexpr.S(string(dimension.Layer))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(dimension.UUID))),
		renderPoints(dimension.Points),
		sexpr.L(sexpr.A("height"), fixed(dimension.Height)),
		sexpr.L(
			sexpr.A("gr_text"),
			sexpr.S(dimension.Text),
			renderAt(dimension.Position, dimension.Rotation),
			sexpr.L(sexpr.A("layer"), sexpr.S(string(dimension.Layer))),
			sexpr.L(sexpr.A("uuid"), sexpr.S(string(dimension.UUID))),
			renderEffects(dimension.Effects),
		),
	)
}

func renderPoints(points []kicadfiles.Point) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("pts")}
	for _, point := range points {
		nodes = append(nodes, sexpr.L(sexpr.A("xy"), fixed(point.X), fixed(point.Y)))
	}
	return sexpr.L(nodes...)
}

func renderAt(point kicadfiles.Point, rotation kicadfiles.Angle) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("at"), fixed(point.X), fixed(point.Y)}
	if rotation != 0 {
		nodes = append(nodes, sexpr.F(float64(rotation)))
	}
	return sexpr.L(nodes...)
}

func renderAtWithRotation(point kicadfiles.Point, rotation kicadfiles.Angle) sexpr.List {
	return sexpr.L(sexpr.A("at"), fixed(point.X), fixed(point.Y), sexpr.F(float64(rotation)))
}

func renderLayerList(name string, layers []kicadfiles.BoardLayer) sexpr.List {
	nodes := []sexpr.Node{sexpr.A(name)}
	for _, layer := range layers {
		nodes = append(nodes, sexpr.S(string(layer)))
	}
	return sexpr.L(nodes...)
}

func fixed(value kicadfiles.IU) sexpr.Fixed {
	formatted := kicadfiles.ToMMString(value)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(formatted, "0")
		formatted = strings.TrimSuffix(formatted, ".")
	}
	return sexpr.X(formatted)
}

func yesNo(value bool) sexpr.Atom {
	if value {
		return sexpr.A("yes")
	}
	return sexpr.A("no")
}

func sortedNets(nets []Net) []Net {
	ordered := slices.Clone(nets)
	if !hasNetZero(ordered) {
		ordered = append(ordered, Net{Code: 0, Name: ""})
	}
	slices.SortFunc(ordered, func(a, b Net) int { return cmp.Compare(a.Code, b.Code) })
	return ordered
}

func sortedFootprints(footprints []Footprint) []Footprint {
	ordered := slices.Clone(footprints)
	slices.SortFunc(ordered, func(a, b Footprint) int {
		return cmp.Compare(string(a.UUID), string(b.UUID))
	})
	return ordered
}

func routedNetSortCodes(footprints []Footprint, netNames map[int]string) map[string]int {
	codes := map[string]int{}
	next := 1
	for _, footprint := range footprints {
		for _, pad := range footprint.Pads {
			netName := routedNetName(pad.NetCode, pad.NetName, netNames)
			if strings.TrimSpace(netName) == "" {
				continue
			}
			if _, ok := codes[netName]; ok {
				continue
			}
			codes[netName] = next
			next++
		}
	}
	return codes
}

func sortedDrawings(drawings []Drawing, layers map[kicadfiles.BoardLayer]int) []Drawing {
	ordered := slices.Clone(drawings)
	slices.SortFunc(ordered, func(a, b Drawing) int {
		if byType := cmp.Compare(drawingTypeOrder(a), drawingTypeOrder(b)); byType != 0 {
			return byType
		}
		if byLayer := cmp.Compare(layerNumber(a.Layer, layers), layerNumber(b.Layer, layers)); byLayer != 0 {
			return byLayer
		}
		if drawingTypeOrder(a) == kicadTypePCBShape {
			if byShape := compareShape(a, b); byShape != 0 {
				return byShape
			}
		}
		return cmp.Compare(string(a.UUID), string(b.UUID))
	})
	return ordered
}

type routedItem struct {
	kind    int
	netCode int
	layer   kicadfiles.BoardLayer
	uuid    kicadfiles.UUID
	track   *Track
	arc     *TrackArc
	via     *Via
}

func (item routedItem) render(netNames map[int]string) sexpr.List {
	switch {
	case item.track != nil:
		return renderTrack(*item.track, netNames)
	case item.arc != nil:
		return renderTrackArc(*item.arc, netNames)
	case item.via != nil:
		return renderVia(*item.via, netNames)
	default:
		return nil
	}
}

func sortedRoutedItems(tracks []Track, arcs []TrackArc, vias []Via, layers map[kicadfiles.BoardLayer]int, netNames map[int]string, netSortCodes map[string]int) []routedItem {
	items := make([]routedItem, 0, len(tracks)+len(arcs)+len(vias))
	for i := range tracks {
		items = append(items, routedItem{
			kind:    kicadTypePCBTrace,
			netCode: routedItemNetSortCode(tracks[i].NetCode, tracks[i].NetName, netNames, netSortCodes),
			layer:   tracks[i].Layer,
			uuid:    tracks[i].UUID,
			track:   &tracks[i],
		})
	}
	for i := range arcs {
		items = append(items, routedItem{
			kind:    kicadTypePCBArc,
			netCode: routedItemNetSortCode(arcs[i].NetCode, arcs[i].NetName, netNames, netSortCodes),
			layer:   arcs[i].Layer,
			uuid:    arcs[i].UUID,
			arc:     &arcs[i],
		})
	}
	for i := range vias {
		items = append(items, routedItem{
			kind:    kicadTypePCBVia,
			netCode: routedItemNetSortCode(vias[i].NetCode, vias[i].NetName, netNames, netSortCodes),
			layer:   viaTopLayer(vias[i]),
			uuid:    vias[i].UUID,
			via:     &vias[i],
		})
	}
	slices.SortFunc(items, func(a, b routedItem) int {
		if byNet := cmp.Compare(a.netCode, b.netCode); byNet != 0 {
			return byNet
		}
		if byLayer := cmp.Compare(layerNumber(a.layer, layers), layerNumber(b.layer, layers)); byLayer != 0 {
			return byLayer
		}
		if byType := cmp.Compare(a.kind, b.kind); byType != 0 {
			return byType
		}
		return cmp.Compare(string(a.uuid), string(b.uuid))
	})
	return items
}

func routedItemNetSortCode(code int, explicit string, netNames map[int]string, netSortCodes map[string]int) int {
	name := routedNetName(code, explicit, netNames)
	if sortCode, ok := netSortCodes[name]; ok {
		return sortCode
	}
	return code
}

const (
	kicadTypePCBShape = 5
	kicadTypePCBText  = 9
	kicadTypePCBTrace = 13
	kicadTypePCBVia   = 14
	kicadTypePCBArc   = 15

	kicadShapeSegment = 0
	kicadShapeRect    = 1
	kicadShapeArc     = 2
	kicadShapeCircle  = 3
	kicadShapePoly    = 4
)

func drawingTypeOrder(drawing Drawing) int {
	if drawing.Text != nil {
		return kicadTypePCBText
	}
	return kicadTypePCBShape
}

func compareShape(a, b Drawing) int {
	aStart, aEnd := shapeStartEnd(a)
	bStart, bEnd := shapeStartEnd(b)
	if byStart := comparePoint(aStart, bStart); byStart != 0 {
		return byStart
	}
	if byEnd := comparePoint(aEnd, bEnd); byEnd != 0 {
		return byEnd
	}
	if byShape := cmp.Compare(shapeOrder(a), shapeOrder(b)); byShape != 0 {
		return byShape
	}
	if shapeOrder(a) == kicadShapeArc {
		if byMid := comparePoint(a.Arc.Mid, b.Arc.Mid); byMid != 0 {
			return byMid
		}
	}
	if shapeOrder(a) == kicadShapePoly {
		if byVertices := cmp.Compare(len(a.Poly.Points), len(b.Poly.Points)); byVertices != 0 {
			return byVertices
		}
		for i := range a.Poly.Points {
			if byPoint := comparePoint(a.Poly.Points[i], b.Poly.Points[i]); byPoint != 0 {
				return byPoint
			}
		}
	}
	if byWidth := cmp.Compare(shapeWidth(a), shapeWidth(b)); byWidth != 0 {
		return byWidth
	}
	return cmp.Compare(fillOrder(a.Fill), fillOrder(b.Fill))
}

func shapeStartEnd(drawing Drawing) (kicadfiles.Point, kicadfiles.Point) {
	switch {
	case drawing.Line != nil:
		return drawing.Line.Start, drawing.Line.End
	case drawing.Rect != nil:
		return drawing.Rect.Start, drawing.Rect.End
	case drawing.Arc != nil:
		return drawing.Arc.Start, drawing.Arc.End
	case drawing.Circle != nil:
		return drawing.Circle.Center, drawing.Circle.End
	case drawing.Poly != nil && len(drawing.Poly.Points) > 0:
		return drawing.Poly.Points[0], drawing.Poly.Points[len(drawing.Poly.Points)-1]
	default:
		return kicadfiles.Point{}, kicadfiles.Point{}
	}
}

func shapeOrder(drawing Drawing) int {
	switch {
	case drawing.Line != nil:
		return kicadShapeSegment
	case drawing.Rect != nil:
		return kicadShapeRect
	case drawing.Arc != nil:
		return kicadShapeArc
	case drawing.Circle != nil:
		return kicadShapeCircle
	case drawing.Poly != nil:
		return kicadShapePoly
	default:
		return kicadShapeSegment
	}
}

func shapeWidth(drawing Drawing) kicadfiles.IU {
	switch {
	case drawing.Line != nil:
		return drawing.Line.Width
	case drawing.Rect != nil:
		return drawing.Rect.Width
	case drawing.Arc != nil:
		return drawing.Arc.Width
	case drawing.Circle != nil:
		return drawing.Circle.Width
	case drawing.Poly != nil:
		return drawing.Poly.Width
	default:
		return 0
	}
}

func comparePoint(a, b kicadfiles.Point) int {
	if byX := cmp.Compare(a.X, b.X); byX != 0 {
		return byX
	}
	return cmp.Compare(a.Y, b.Y)
}

func fillOrder(fill string) int {
	switch fillMode(fill) {
	case "solid", "yes":
		return 2
	case "background":
		return 3
	case "none":
		return 1
	default:
		return 1
	}
}

func layerNumberMap(layers []LayerDefinition) map[kicadfiles.BoardLayer]int {
	numbers := make(map[kicadfiles.BoardLayer]int, len(layers))
	for _, layer := range layers {
		numbers[layer.Name] = layer.Number
	}
	return numbers
}

func layerNumber(layer kicadfiles.BoardLayer, layers map[kicadfiles.BoardLayer]int) int {
	if number, ok := layers[layer]; ok {
		return number
	}
	if strings.HasPrefix(string(layer), "In") && strings.HasSuffix(string(layer), ".Cu") {
		index, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(string(layer), "In"), ".Cu"))
		if err == nil {
			return index*2 + 2
		}
	}
	return int(^uint(0) >> 1)
}

func viaTopLayer(via Via) kicadfiles.BoardLayer {
	if len(via.Layers) == 0 {
		return ""
	}
	return via.Layers[0]
}

func netCodeSet(nets []Net) map[int]struct{} {
	codes := map[int]struct{}{0: struct{}{}}
	for _, net := range nets {
		codes[net.Code] = struct{}{}
	}
	return codes
}

func netNameMap(nets []Net) map[int]string {
	names := map[int]string{0: ""}
	for _, net := range nets {
		names[net.Code] = net.Name
	}
	return names
}

func hasNetZero(nets []Net) bool {
	for _, net := range nets {
		if net.Code == 0 {
			return true
		}
	}
	return false
}
