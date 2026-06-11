package design

import (
	"fmt"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/library"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
)

type LibraryMapping struct {
	SymbolFootprints        []SymbolFootprintAssignment
	SymbolTables            []library.TableEntry
	FootprintTables         []library.TableEntry
	KnownSymbolLibraries    []string
	KnownFootprintLibraries []string
}

type SymbolFootprintAssignment struct {
	SymbolLibraryID    string
	ReferencePrefix    string
	FootprintLibraryID string
}

func ApplyLibraryMapping(design *Design, mapping LibraryMapping) error {
	if design == nil {
		return fmt.Errorf("design required")
	}
	assignments, err := normalizeSymbolFootprintAssignments(mapping.SymbolFootprints)
	if err != nil {
		return err
	}
	working := cloneDesignForMapping(*design)
	working.SymbolTables = mergeTableEntries(working.SymbolTables, mapping.SymbolTables)
	working.FootprintTables = mergeTableEntries(working.FootprintTables, mapping.FootprintTables)
	working.KnownSymbolLibraries = mergeNames(working.KnownSymbolLibraries, mapping.KnownSymbolLibraries)
	working.KnownFootprintLibraries = mergeNames(working.KnownFootprintLibraries, mapping.KnownFootprintLibraries)

	symbolsByReference := map[string]string{}
	for _, file := range schematicFiles(&working) {
		for i := range file.Symbols {
			symbol := &file.Symbols[i]
			if strings.HasPrefix(symbol.Reference, "#") {
				continue
			}
			assignment, ok := matchingAssignment(symbol, assignments)
			if !ok {
				continue
			}
			if err := assignSymbolFootprint(symbol, assignment.FootprintLibraryID); err != nil {
				return err
			}
			reference := referenceKey(symbol.Reference)
			if existing, ok := symbolsByReference[reference]; ok && existing != assignment.FootprintLibraryID {
				return fmt.Errorf("schematic reference %s maps to both %q and %q", strings.TrimSpace(symbol.Reference), existing, assignment.FootprintLibraryID)
			}
			symbolsByReference[reference] = assignment.FootprintLibraryID
		}
	}
	if working.PCB != nil {
		for i := range working.PCB.Footprints {
			footprint := &working.PCB.Footprints[i]
			assigned, ok := symbolsByReference[referenceKey(footprint.Reference)]
			if !ok {
				continue
			}
			if strings.TrimSpace(footprint.LibraryID) == "" {
				footprint.LibraryID = assigned
				continue
			}
			if !sameLibraryID(footprint.LibraryID, assigned) {
				return fmt.Errorf("pcb footprint %s library %q conflicts with mapped schematic footprint %q", footprint.Reference, footprint.LibraryID, assigned)
			}
		}
	}
	*design = working
	return nil
}

func normalizeSymbolFootprintAssignments(assignments []SymbolFootprintAssignment) ([]SymbolFootprintAssignment, error) {
	normalized := make([]SymbolFootprintAssignment, 0, len(assignments))
	for i, assignment := range assignments {
		assignment.SymbolLibraryID = strings.TrimSpace(assignment.SymbolLibraryID)
		assignment.ReferencePrefix = referenceKey(assignment.ReferencePrefix)
		assignment.FootprintLibraryID = strings.TrimSpace(assignment.FootprintLibraryID)
		if assignment.SymbolLibraryID == "" && assignment.ReferencePrefix == "" {
			return nil, fmt.Errorf("symbol footprint assignments[%d]: symbol library id or reference prefix required", i)
		}
		if assignment.SymbolLibraryID != "" {
			if _, err := libraryNickname(assignment.SymbolLibraryID); err != nil {
				return nil, fmt.Errorf("symbol footprint assignments[%d].symbol_library_id: %w", i, err)
			}
		}
		if assignment.FootprintLibraryID == "" {
			return nil, fmt.Errorf("symbol footprint assignments[%d].footprint_library_id: required", i)
		}
		if _, err := libraryNickname(assignment.FootprintLibraryID); err != nil {
			return nil, fmt.Errorf("symbol footprint assignments[%d].footprint_library_id: %w", i, err)
		}
		normalized = append(normalized, assignment)
	}
	return normalized, nil
}

func matchingAssignment(symbol *schematic.SchematicSymbol, assignments []SymbolFootprintAssignment) (SymbolFootprintAssignment, bool) {
	if symbol == nil {
		return SymbolFootprintAssignment{}, false
	}
	libraryID := strings.TrimSpace(symbol.LibraryID)
	reference := strings.TrimSpace(symbol.Reference)
	for _, assignment := range assignments {
		if assignment.SymbolLibraryID != "" && libraryID != assignment.SymbolLibraryID {
			continue
		}
		if assignment.ReferencePrefix != "" && !referenceMatchesNormalizedPrefix(reference, assignment.ReferencePrefix) {
			continue
		}
		return assignment, true
	}
	return SymbolFootprintAssignment{}, false
}

func assignSymbolFootprint(symbol *schematic.SchematicSymbol, footprintID string) error {
	footprintID = strings.TrimSpace(footprintID)
	updated := false
	for i := range symbol.Properties {
		if strings.EqualFold(strings.TrimSpace(symbol.Properties[i].Name), "Footprint") {
			if value := strings.TrimSpace(symbol.Properties[i].Value); value != "" && !sameLibraryID(value, footprintID) {
				return fmt.Errorf("schematic symbol %s footprint %q conflicts with mapped footprint %q", symbol.Reference, symbol.Properties[i].Value, footprintID)
			}
			symbol.Properties[i].Name = "Footprint"
			symbol.Properties[i].Value = footprintID
			updated = true
		}
	}
	for i := range symbol.Fields {
		if strings.EqualFold(strings.TrimSpace(symbol.Fields[i].Name), "Footprint") {
			if value := strings.TrimSpace(symbol.Fields[i].Value); value != "" && !sameLibraryID(value, footprintID) {
				return fmt.Errorf("schematic symbol %s footprint %q conflicts with mapped footprint %q", symbol.Reference, symbol.Fields[i].Value, footprintID)
			}
			symbol.Fields[i].Name = "Footprint"
			symbol.Fields[i].Value = footprintID
			updated = true
		}
	}
	if updated {
		return nil
	}
	symbol.Properties = append(symbol.Properties, schematic.Property{
		Name:     "Footprint",
		Value:    footprintID,
		Hidden:   true,
		Position: symbol.Position,
		Rotation: symbol.Rotation,
	})
	return nil
}

func cloneDesignForMapping(design Design) Design {
	clone := design
	if design.Schematic != nil {
		clone.Schematic = cloneSchematicFile(design.Schematic)
	}
	if len(design.SheetFiles) > 0 {
		clone.SheetFiles = make([]*schematic.SchematicFile, len(design.SheetFiles))
		for i, file := range design.SheetFiles {
			if file == nil {
				continue
			}
			clone.SheetFiles[i] = cloneSchematicFile(file)
		}
	}
	if design.PCB != nil {
		pcbClone := *design.PCB
		pcbClone.Footprints = clonePCBFootprints(design.PCB.Footprints)
		clone.PCB = &pcbClone
	}
	clone.SymbolTables = append([]library.TableEntry(nil), design.SymbolTables...)
	clone.FootprintTables = append([]library.TableEntry(nil), design.FootprintTables...)
	clone.KnownSymbolLibraries = append([]string(nil), design.KnownSymbolLibraries...)
	clone.KnownFootprintLibraries = append([]string(nil), design.KnownFootprintLibraries...)
	return clone
}

func clonePCBFootprints(footprints []pcb.Footprint) []pcb.Footprint {
	cloned := append([]pcb.Footprint(nil), footprints...)
	for i := range cloned {
		cloned[i] = clonePCBFootprint(cloned[i])
	}
	return cloned
}

func clonePCBFootprint(footprint pcb.Footprint) pcb.Footprint {
	footprint.Attributes = append([]string(nil), footprint.Attributes...)
	footprint.Properties = append([]pcb.FootprintProperty(nil), footprint.Properties...)
	for i := range footprint.Properties {
		footprint.Properties[i].Effects.Justify = append([]string(nil), footprint.Properties[i].Effects.Justify...)
	}
	footprint.MetadataProperties = append([]pcb.FootprintMetadataProperty(nil), footprint.MetadataProperties...)
	footprint.Units = append([]pcb.FootprintUnit(nil), footprint.Units...)
	for i := range footprint.Units {
		footprint.Units[i].Pins = append([]string(nil), footprint.Units[i].Pins...)
	}
	footprint.NetTiePadGroups = append([]string(nil), footprint.NetTiePadGroups...)
	footprint.Texts = append([]pcb.FootprintText(nil), footprint.Texts...)
	footprint.Pads = append([]pcb.Pad(nil), footprint.Pads...)
	for i := range footprint.Pads {
		footprint.Pads[i].Layers = append([]kicadfiles.BoardLayer(nil), footprint.Pads[i].Layers...)
		footprint.Pads[i].RemoveUnusedLayers = clonePtr(footprint.Pads[i].RemoveUnusedLayers)
		footprint.Pads[i].ThermalBridgeAngle = clonePtr(footprint.Pads[i].ThermalBridgeAngle)
		footprint.Pads[i].Teardrops = clonePtr(footprint.Pads[i].Teardrops)
	}
	footprint.Graphics = append([]pcb.FootprintGraphic(nil), footprint.Graphics...)
	for i := range footprint.Graphics {
		footprint.Graphics[i] = cloneFootprintGraphic(footprint.Graphics[i])
	}
	footprint.Models = append([]pcb.Model3D(nil), footprint.Models...)
	footprint.EmbeddedFonts = clonePtr(footprint.EmbeddedFonts)
	footprint.DuplicatePadNumbersAreJumpers = clonePtr(footprint.DuplicatePadNumbersAreJumpers)
	return footprint
}

func cloneFootprintGraphic(graphic pcb.FootprintGraphic) pcb.FootprintGraphic {
	return pcb.FootprintGraphic(cloneDrawing(pcb.Drawing(graphic)))
}

func cloneDrawing(drawing pcb.Drawing) pcb.Drawing {
	drawing.Line = clonePtr(drawing.Line)
	drawing.Rect = clonePtr(drawing.Rect)
	drawing.Circle = clonePtr(drawing.Circle)
	drawing.Arc = clonePtr(drawing.Arc)
	drawing.Poly = clonePolylineDrawing(drawing.Poly)
	drawing.Curve = clonePolylineDrawing(drawing.Curve)
	drawing.Text = clonePtr(drawing.Text)
	if drawing.Text != nil {
		drawing.Text.Effects.Justify = append([]string(nil), drawing.Text.Effects.Justify...)
	}
	return drawing
}

func clonePolylineDrawing(drawing *pcb.PolylineDrawing) *pcb.PolylineDrawing {
	if drawing == nil {
		return nil
	}
	cloned := *drawing
	cloned.Points = append([]kicadfiles.Point(nil), drawing.Points...)
	return &cloned
}

func clonePtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSchematicFile(file *schematic.SchematicFile) *schematic.SchematicFile {
	clone := *file
	clone.LibSymbols = append([]schematic.EmbeddedSymbol(nil), file.LibSymbols...)
	for i := range clone.LibSymbols {
		clone.LibSymbols[i].Body = cloneSexprList(clone.LibSymbols[i].Body)
	}
	clone.Symbols = append([]schematic.SchematicSymbol(nil), file.Symbols...)
	for i := range clone.Symbols {
		clone.Symbols[i].Properties = append([]schematic.Property(nil), clone.Symbols[i].Properties...)
		clone.Symbols[i].Fields = append([]schematic.Field(nil), clone.Symbols[i].Fields...)
		clone.Symbols[i].Pins = append([]schematic.SymbolPin(nil), clone.Symbols[i].Pins...)
		clone.Symbols[i].PinAnchors = append([]kicadfiles.Point(nil), clone.Symbols[i].PinAnchors...)
		clone.Symbols[i].Instances = append([]schematic.SymbolInstance(nil), clone.Symbols[i].Instances...)
	}
	clone.Wires = append([]schematic.Wire(nil), file.Wires...)
	for i := range clone.Wires {
		clone.Wires[i].Points = append([]kicadfiles.Point(nil), clone.Wires[i].Points...)
	}
	clone.NoConnects = append([]schematic.NoConnect(nil), file.NoConnects...)
	clone.Labels = append([]schematic.Label(nil), file.Labels...)
	for i := range clone.Labels {
		clone.Labels[i].Fields = append([]schematic.Field(nil), clone.Labels[i].Fields...)
	}
	clone.Junctions = append([]schematic.Junction(nil), file.Junctions...)
	clone.Buses = append([]schematic.Bus(nil), file.Buses...)
	for i := range clone.Buses {
		clone.Buses[i].Points = append([]kicadfiles.Point(nil), clone.Buses[i].Points...)
	}
	clone.Polylines = append([]schematic.Polyline(nil), file.Polylines...)
	for i := range clone.Polylines {
		clone.Polylines[i].Points = append([]kicadfiles.Point(nil), clone.Polylines[i].Points...)
	}
	clone.BusEntries = append([]schematic.BusEntry(nil), file.BusEntries...)
	clone.Texts = append([]schematic.Text(nil), file.Texts...)
	clone.Sheets = append([]schematic.Sheet(nil), file.Sheets...)
	for i := range clone.Sheets {
		clone.Sheets[i].Properties = append([]schematic.Property(nil), clone.Sheets[i].Properties...)
		clone.Sheets[i].Pins = append([]schematic.SheetPin(nil), clone.Sheets[i].Pins...)
		clone.Sheets[i].Instances = append([]schematic.SheetInstance(nil), clone.Sheets[i].Instances...)
	}
	clone.RawItems = append([]schematic.RawSchematicItem(nil), file.RawItems...)
	clone.Instances = append([]schematic.SymbolInstance(nil), file.Instances...)
	clone.SheetInstances = append([]schematic.SheetInstance(nil), file.SheetInstances...)
	return &clone
}

func cloneSexprList(list sexpr.List) sexpr.List {
	clone := append(sexpr.List(nil), list...)
	for i, node := range clone {
		if nested, ok := node.(sexpr.List); ok {
			clone[i] = cloneSexprList(nested)
		}
	}
	return clone
}

func mergeTableEntries(existing, additions []library.TableEntry) []library.TableEntry {
	merged := make([]library.TableEntry, 0, len(existing)+len(additions))
	seen := map[string]struct{}{}
	for _, entry := range existing {
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Name != "" {
			seen[strings.ToLower(entry.Name)] = struct{}{}
		}
		merged = append(merged, entry)
	}
	for _, entry := range additions {
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Name == "" {
			continue
		}
		key := strings.ToLower(entry.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, entry)
	}
	return merged
}

func mergeNames(existing, additions []string) []string {
	merged := make([]string, 0, len(existing)+len(additions))
	seen := map[string]struct{}{}
	for _, name := range existing {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			seen[strings.ToLower(trimmed)] = struct{}{}
			merged = append(merged, trimmed)
		}
	}
	for _, name := range additions {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, trimmed)
	}
	return merged
}

func schematicFiles(design *Design) []*schematic.SchematicFile {
	var files []*schematic.SchematicFile
	if design.Schematic != nil {
		files = append(files, design.Schematic)
	}
	for _, file := range design.SheetFiles {
		if file != nil {
			files = append(files, file)
		}
	}
	return files
}

func schematicFootprintProperty(symbol *schematic.SchematicSymbol) (string, bool) {
	return symbolPropertyValue(symbol, "Footprint")
}

func symbolPropertyValue(symbol *schematic.SchematicSymbol, name string) (string, bool) {
	if symbol == nil {
		return "", false
	}
	for _, property := range symbol.Properties {
		if strings.EqualFold(strings.TrimSpace(property.Name), name) {
			return property.Value, true
		}
	}
	for _, field := range symbol.Fields {
		if strings.EqualFold(strings.TrimSpace(field.Name), name) {
			return field.Value, true
		}
	}
	return "", false
}

func sameLibraryID(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func referenceKey(reference string) string {
	return strings.ToUpper(strings.TrimSpace(reference))
}

func referenceMatchesNormalizedPrefix(reference, prefix string) bool {
	reference = referenceKey(reference)
	if prefix == "" || !strings.HasPrefix(reference, prefix) {
		return false
	}
	remaining := reference[len(prefix):]
	return remaining == "" || remaining[0] >= '0' && remaining[0] <= '9'
}

func pcbFootprintsByReference(board *pcb.PCBFile) map[string][]*pcb.Footprint {
	footprints := map[string][]*pcb.Footprint{}
	if board == nil {
		return footprints
	}
	for i := range board.Footprints {
		footprint := &board.Footprints[i]
		reference := referenceKey(footprint.Reference)
		if reference != "" {
			footprints[reference] = append(footprints[reference], footprint)
		}
	}
	return footprints
}
