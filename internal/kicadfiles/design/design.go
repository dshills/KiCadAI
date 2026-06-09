package design

import (
	"fmt"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/library"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/kicadfiles/schematic"
)

type Design struct {
	Name                    string
	Project                 project.ProjectFile
	Schematic               *schematic.SchematicFile
	SheetFiles              []*schematic.SchematicFile
	PCB                     *pcb.PCBFile
	SymbolTables            []library.TableEntry
	FootprintTables         []library.TableEntry
	KnownSymbolLibraries    []string
	KnownFootprintLibraries []string
	ExpectedNets            []string
}

type LEDIndicatorInput struct {
	Name            string
	DesignID        kicadfiles.UUID
	Seed            string
	IncludePCB      bool
	LibraryVCC      string
	LibraryGND      string
	LibraryResistor string
	LibraryLED      string
}

func LEDIndicatorDesign(input LEDIndicatorInput) (Design, error) {
	if input.Name == "" {
		input.Name = "led_indicator"
	}
	if input.Seed == "" {
		input.Seed = input.Name
	}
	projectFile := project.ProjectFile{
		Name:          input.Name,
		DesignID:      input.DesignID,
		FormatVersion: kicadfiles.KiCadFormatV20230121,
		Generator:     "kicadai",
		PageSettings:  project.PageSettings{Paper: kicadfiles.Paper{Name: "A4"}},
		NetClasses: []project.NetClass{{
			Name:        "Default",
			Clearance:   kicadfiles.MM(0.2),
			TrackWidth:  kicadfiles.MM(0.25),
			ViaDiameter: kicadfiles.MM(0.8),
			ViaDrill:    kicadfiles.MM(0.4),
		}},
	}
	schematicFile, err := schematic.LEDIndicatorSchematic(schematic.LEDIndicatorInput{
		Name:            input.Name,
		DesignID:        input.DesignID,
		Seed:            input.Seed,
		LibraryVCC:      input.LibraryVCC,
		LibraryGND:      input.LibraryGND,
		LibraryResistor: input.LibraryResistor,
		LibraryLED:      input.LibraryLED,
	})
	if err != nil {
		return Design{}, err
	}
	design := Design{
		Name:         input.Name,
		Project:      projectFile,
		Schematic:    &schematicFile,
		ExpectedNets: []string{"VCC", "LED_OUT", "GND"},
	}
	if input.IncludePCB {
		pcbFile, err := pcb.LEDIndicatorPCB(pcb.LEDIndicatorInput{
			Name:     input.Name,
			DesignID: input.DesignID,
			Seed:     input.Seed,
		})
		if err != nil {
			return Design{}, err
		}
		design.PCB = &pcbFile
	}
	if err := Validate(design); err != nil {
		return Design{}, err
	}
	return design, nil
}

func Validate(design Design) error {
	var errs kicadfiles.ValidationErrors
	name := strings.TrimSpace(design.Name)
	if name == "" {
		errs = append(errs, designError("name", "required"))
	}
	if strings.TrimSpace(design.Project.Name) != name {
		errs = append(errs, designError("project.name", "must match design name"))
	}
	if err := project.Validate(design.Project); err != nil {
		errs = append(errs, nestedErrors(err)...)
	}
	if design.Schematic != nil {
		if design.Schematic.Filename != "" && design.Schematic.Filename != name+".kicad_sch" {
			errs = append(errs, designError("schematic.filename", "must match design name"))
		}
		if err := schematic.Validate(*design.Schematic); err != nil {
			errs = append(errs, nestedErrors(err)...)
		}
		errs = append(errs, validateSchematicReferences(design.Schematic)...)
		errs = append(errs, validateSymbolLibraryReferences(design)...)
		errs = append(errs, validateSheetFiles(design)...)
	}
	if design.PCB != nil {
		if err := pcb.Validate(*design.PCB); err != nil {
			errs = append(errs, nestedErrors(err)...)
		}
		errs = append(errs, validateFootprintLibraryReferences(design)...)
		errs = append(errs, validateFootprintReferences(design)...)
		errs = append(errs, validateExpectedNets(design)...)
	}
	errs = append(errs, validateUniqueUUIDs(design)...)
	if err := library.ValidateTableEntries("sym_lib_table", design.SymbolTables); err != nil {
		errs = append(errs, nestedErrors(err)...)
	}
	if err := library.ValidateTableEntries("fp_lib_table", design.FootprintTables); err != nil {
		errs = append(errs, nestedErrors(err)...)
	}
	return errs.Err()
}

func validateFootprintReferences(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if design.Schematic == nil || design.PCB == nil {
		return errs
	}
	symbolsByRef := schematicSymbolsByReference(design.Schematic)
	footprintsByRef := map[string]*pcb.Footprint{}
	for i := range design.PCB.Footprints {
		footprint := &design.PCB.Footprints[i]
		if _, ok := footprintsByRef[footprint.Reference]; ok {
			errs = append(errs, designError("pcb.footprints["+strconv.Itoa(i)+"].reference", "duplicate"))
		}
		footprintsByRef[footprint.Reference] = footprint
	}
	for i, footprint := range design.PCB.Footprints {
		if _, ok := symbolsByRef[footprint.Reference]; !ok {
			errs = append(errs, designError("pcb.footprints["+strconv.Itoa(i)+"].reference", "missing schematic symbol"))
		}
	}
	for ref, symbol := range symbolsByRef {
		footprint, ok := footprintsByRef[ref]
		if !ok {
			errs = append(errs, designError("pcb.footprints", "missing footprint for schematic reference "+ref))
			continue
		}
		if symbol.Path != footprint.Path {
			errs = append(errs, designError("pcb.footprints."+ref+".path", "must match schematic symbol path"))
		}
	}
	return errs
}

func validateSchematicReferences(schematicFile *schematic.SchematicFile) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	seen := map[string]struct{}{}
	for i, symbol := range schematicFile.Symbols {
		if strings.HasPrefix(symbol.Reference, "#") {
			continue
		}
		if _, ok := seen[symbol.Reference]; ok {
			errs = append(errs, designError("schematic.symbols["+strconv.Itoa(i)+"].reference", "duplicate"))
		}
		seen[symbol.Reference] = struct{}{}
	}
	return errs
}

func validateSymbolLibraryReferences(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if design.Schematic == nil {
		return errs
	}
	embedded := map[string]struct{}{}
	for _, symbol := range design.Schematic.LibSymbols {
		embedded[symbol.LibraryID] = struct{}{}
	}
	tables := tableNicknames(design.SymbolTables)
	known := nameSet(design.KnownSymbolLibraries)
	for i, symbol := range design.Schematic.Symbols {
		if _, ok := embedded[symbol.LibraryID]; ok {
			continue
		}
		nickname, err := libraryNickname(symbol.LibraryID)
		if err != nil {
			errs = append(errs, designError("schematic.symbols["+strconv.Itoa(i)+"].library_id", err.Error()))
			continue
		}
		if _, ok := tables[nickname]; ok {
			continue
		}
		if _, ok := known[nickname]; ok {
			continue
		}
		errs = append(errs, designError("schematic.symbols["+strconv.Itoa(i)+"].library_id", "unresolved library "+nickname))
	}
	return errs
}

func validateFootprintLibraryReferences(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if design.PCB == nil {
		return errs
	}
	tables := tableNicknames(design.FootprintTables)
	known := nameSet(design.KnownFootprintLibraries)
	for i, footprint := range design.PCB.Footprints {
		if len(footprint.Pads) > 0 || len(footprint.Graphics) > 0 {
			continue
		}
		nickname, err := libraryNickname(footprint.LibraryID)
		if err != nil {
			errs = append(errs, designError("pcb.footprints["+strconv.Itoa(i)+"].library_id", err.Error()))
			continue
		}
		if _, ok := tables[nickname]; ok {
			continue
		}
		if _, ok := known[nickname]; ok {
			continue
		}
		errs = append(errs, designError("pcb.footprints["+strconv.Itoa(i)+"].library_id", "unresolved library "+nickname))
	}
	return errs
}

func libraryNickname(libID string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(libID), ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("must be library_nickname:item_name")
	}
	nickname := strings.TrimSpace(parts[0])
	item := strings.TrimSpace(parts[1])
	if nickname == "" || item == "" {
		return "", fmt.Errorf("must be library_nickname:item_name")
	}
	return nickname, nil
}

func tableNicknames(entries []library.TableEntry) map[string]struct{} {
	names := map[string]struct{}{}
	for _, entry := range entries {
		names[strings.TrimSpace(entry.Name)] = struct{}{}
	}
	return names
}

func nameSet(values []string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func schematicSymbolsByReference(schematicFile *schematic.SchematicFile) map[string]*schematic.SchematicSymbol {
	symbolsByRef := map[string]*schematic.SchematicSymbol{}
	for i := range schematicFile.Symbols {
		symbol := &schematicFile.Symbols[i]
		if strings.HasPrefix(symbol.Reference, "#") {
			continue
		}
		symbolsByRef[symbol.Reference] = symbol
	}
	return symbolsByRef
}

func validateExpectedNets(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if design.PCB == nil {
		return errs
	}
	pcbNets := map[string]struct{}{}
	for _, net := range design.PCB.Nets {
		pcbNets[net.Name] = struct{}{}
	}
	for _, expected := range design.ExpectedNets {
		if _, ok := pcbNets[expected]; !ok {
			errs = append(errs, designError("pcb.nets", "missing expected net "+expected))
		}
	}
	return errs
}

func validateUniqueUUIDs(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	seen := map[kicadfiles.UUID]uuidLocation{}
	add := func(id kicadfiles.UUID, location uuidLocation) {
		if id == "" {
			errs = append(errs, designError(location.String(), "missing UUID"))
			return
		}
		if prior, ok := seen[id]; ok {
			errs = append(errs, designError(location.String(), "duplicate UUID also used by "+prior.String()))
			return
		}
		seen[id] = location
	}
	add(design.Project.DesignID, uuidLocation{field: "project.design_id"})
	if design.Schematic != nil {
		add(design.Schematic.UUID, uuidLocation{field: "schematic.uuid"})
		for i, symbol := range design.Schematic.Symbols {
			add(symbol.UUID, uuidLocation{collection: "schematic.symbols", index: i, field: "uuid"})
		}
		for i, wire := range design.Schematic.Wires {
			add(wire.UUID, uuidLocation{collection: "schematic.wires", index: i, field: "uuid"})
		}
		for i, label := range design.Schematic.Labels {
			add(label.UUID, uuidLocation{collection: "schematic.labels", index: i, field: "uuid"})
		}
		for i, junction := range design.Schematic.Junctions {
			add(junction.UUID, uuidLocation{collection: "schematic.junctions", index: i, field: "uuid"})
		}
		for i, sheet := range design.Schematic.Sheets {
			add(sheet.UUID, uuidLocation{collection: "schematic.sheets", index: i, field: "uuid"})
		}
	}
	for fileIndex, sheetFile := range design.SheetFiles {
		if sheetFile == nil {
			errs = append(errs, designError("sheet_files["+strconv.Itoa(fileIndex)+"]", "nil"))
			continue
		}
		add(sheetFile.UUID, uuidLocation{collection: "sheet_files", index: fileIndex, field: "uuid"})
		for i, symbol := range sheetFile.Symbols {
			add(symbol.UUID, uuidLocation{collection: "sheet_files[" + strconv.Itoa(fileIndex) + "].symbols", index: i, field: "uuid"})
		}
		for i, wire := range sheetFile.Wires {
			add(wire.UUID, uuidLocation{collection: "sheet_files[" + strconv.Itoa(fileIndex) + "].wires", index: i, field: "uuid"})
		}
		for i, label := range sheetFile.Labels {
			add(label.UUID, uuidLocation{collection: "sheet_files[" + strconv.Itoa(fileIndex) + "].labels", index: i, field: "uuid"})
		}
		for i, junction := range sheetFile.Junctions {
			add(junction.UUID, uuidLocation{collection: "sheet_files[" + strconv.Itoa(fileIndex) + "].junctions", index: i, field: "uuid"})
		}
		for i, sheet := range sheetFile.Sheets {
			add(sheet.UUID, uuidLocation{collection: "sheet_files[" + strconv.Itoa(fileIndex) + "].sheets", index: i, field: "uuid"})
		}
	}
	if design.PCB != nil {
		for i, footprint := range design.PCB.Footprints {
			add(footprint.UUID, uuidLocation{collection: "pcb.footprints", index: i, field: "uuid"})
		}
		for i, drawing := range design.PCB.Drawings {
			add(drawing.UUID, uuidLocation{collection: "pcb.drawings", index: i, field: "uuid"})
		}
		for i, track := range design.PCB.Tracks {
			add(track.UUID, uuidLocation{collection: "pcb.tracks", index: i, field: "uuid"})
		}
		for i, via := range design.PCB.Vias {
			add(via.UUID, uuidLocation{collection: "pcb.vias", index: i, field: "uuid"})
		}
		for i, zone := range design.PCB.Zones {
			add(zone.UUID, uuidLocation{collection: "pcb.zones", index: i, field: "uuid"})
		}
		for i, dimension := range design.PCB.Dimensions {
			add(dimension.UUID, uuidLocation{collection: "pcb.dimensions", index: i, field: "uuid"})
		}
	}
	return errs
}

func validateSheetFiles(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	children := map[string]*schematic.SchematicFile{}
	for i, sheetFile := range design.SheetFiles {
		if sheetFile == nil {
			errs = append(errs, designError("sheet_files["+strconv.Itoa(i)+"]", "nil"))
			continue
		}
		filename := strings.TrimSpace(sheetFile.Filename)
		if filename == "" {
			errs = append(errs, designError("sheet_files["+strconv.Itoa(i)+"].filename", "required"))
			continue
		}
		if _, ok := children[filename]; ok {
			errs = append(errs, designError("sheet_files["+strconv.Itoa(i)+"].filename", "duplicate "+filename))
		}
		children[filename] = sheetFile
		if err := schematic.Validate(*sheetFile); err != nil {
			errs = append(errs, nestedErrors(err)...)
		}
	}
	refs := map[string]struct{}{}
	collect := func(prefix string, sheets []schematic.Sheet) {
		for i, sheet := range sheets {
			filename := strings.TrimSpace(sheet.Filename)
			if filename == "" {
				continue
			}
			refs[filename] = struct{}{}
			if _, ok := children[filename]; !ok {
				errs = append(errs, designError(prefix+"["+strconv.Itoa(i)+"].filename", "missing child schematic "+filename))
			}
		}
	}
	if design.Schematic != nil {
		collect("schematic.sheets", design.Schematic.Sheets)
	}
	for fileIndex, sheetFile := range design.SheetFiles {
		if sheetFile != nil {
			collect("sheet_files["+strconv.Itoa(fileIndex)+"].sheets", sheetFile.Sheets)
		}
	}
	for filename := range children {
		if _, ok := refs[filename]; !ok {
			errs = append(errs, designError("sheet_files."+filename, "unreferenced child schematic"))
		}
	}
	errs = append(errs, validateSheetCycles(design, children)...)
	return errs
}

func validateSheetCycles(design Design, children map[string]*schematic.SchematicFile) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	const root = "<root>"
	graph := map[string][]string{root: sheetFilenames(design.Schematic)}
	for filename, sheetFile := range children {
		graph[filename] = sheetFilenames(sheetFile)
	}
	visiting := map[string]struct{}{}
	visited := map[string]struct{}{}
	var visit func(string, []string)
	visit = func(node string, stack []string) {
		if _, ok := visited[node]; ok {
			return
		}
		if _, ok := visiting[node]; ok {
			errs = append(errs, designError("sheet_files", "circular sheet reference "+strings.Join(append(stack, node), " -> ")))
			return
		}
		visiting[node] = struct{}{}
		for _, next := range graph[node] {
			if _, ok := graph[next]; ok {
				visit(next, append(stack, node))
			}
		}
		delete(visiting, node)
		visited[node] = struct{}{}
	}
	visit(root, nil)
	return errs
}

func sheetFilenames(file *schematic.SchematicFile) []string {
	if file == nil {
		return nil
	}
	names := make([]string, 0, len(file.Sheets))
	for _, sheet := range file.Sheets {
		name := strings.TrimSpace(sheet.Filename)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

type uuidLocation struct {
	collection string
	index      int
	field      string
}

func (location uuidLocation) String() string {
	if location.collection == "" {
		return location.field
	}
	return location.collection + "[" + strconv.Itoa(location.index) + "]." + location.field
}

func nestedErrors(err error) kicadfiles.ValidationErrors {
	if validationErrors, ok := err.(kicadfiles.ValidationErrors); ok {
		return validationErrors
	}
	return kicadfiles.ValidationErrors{{Section: "design", Field: "dependency", Message: err.Error()}}
}

func designError(field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{Section: "design", Field: field, Message: message}
}
