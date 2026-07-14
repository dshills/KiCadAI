package design

import (
	"fmt"
	"sort"
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
	RuleFiles               []TextArtifact
	WorksheetFiles          []TextArtifact
	AssetFiles              []TextArtifact
	ExpectedNets            []string
}

type TextArtifact struct {
	Path     string
	Contents []byte
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
	resistorLibrary := input.LibraryResistor
	if resistorLibrary == "" {
		resistorLibrary = "Device:R"
	}
	ledLibrary := input.LibraryLED
	if ledLibrary == "" {
		ledLibrary = "Device:LED"
	}
	projectFile := project.ProjectFile{
		Name:          input.Name,
		DesignID:      input.DesignID,
		FormatVersion: kicadfiles.KiCadFormatV20260306,
		Generator:     "eeschema",
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
		LibraryResistor: resistorLibrary,
		LibraryLED:      ledLibrary,
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
		if err := ApplyLibraryMapping(&design, LibraryMapping{SymbolFootprints: []SymbolFootprintAssignment{
			{SymbolLibraryID: resistorLibrary, ReferencePrefix: "R", FootprintLibraryID: "Resistor_SMD:R_0805_2012Metric"},
			{SymbolLibraryID: ledLibrary, ReferencePrefix: "D", FootprintLibraryID: "LED_SMD:LED_0805_2012Metric"},
		}}); err != nil {
			return Design{}, err
		}
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
		errs = append(errs, validateSheetFiles(design)...)
		errs = append(errs, validateDesignSchematicReferences(design)...)
		errs = append(errs, validateSymbolLibraryReferences(design)...)
	}
	if design.PCB != nil {
		if err := pcb.Validate(*design.PCB); err != nil {
			errs = append(errs, nestedErrors(err)...)
		}
		errs = append(errs, validateFootprintLibraryReferences(design)...)
		errs = append(errs, validateFootprintReferences(design)...)
		errs = append(errs, validateSchematicFootprintAssignments(design)...)
		errs = append(errs, validateExpectedNets(design)...)
	}
	errs = append(errs, validateUniqueUUIDs(design)...)
	if err := library.ValidateTableEntries("sym_lib_table", design.SymbolTables); err != nil {
		errs = append(errs, nestedErrors(err)...)
	}
	if err := library.ValidateTableEntries("fp_lib_table", design.FootprintTables); err != nil {
		errs = append(errs, nestedErrors(err)...)
	}
	errs = append(errs, validateArtifacts("rule_files", design.RuleFiles, ".kicad_dru")...)
	errs = append(errs, validateArtifacts("worksheet_files", design.WorksheetFiles, ".kicad_wks")...)
	errs = append(errs, validateArtifacts("asset_files", design.AssetFiles, "")...)
	return errs.Err()
}

func validateArtifacts(field string, artifacts []TextArtifact, requiredExt string) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	for i, artifact := range artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			errs = append(errs, designError(field+"["+strconv.Itoa(i)+"].path", "required"))
			continue
		}
		cleaned, err := normalizeGeneratedPath(artifact.Path)
		if err != nil {
			errs = append(errs, designError(field+"["+strconv.Itoa(i)+"].path", err.Error()))
			continue
		}
		if requiredExt != "" && !strings.HasSuffix(strings.ToLower(cleaned), strings.ToLower(requiredExt)) {
			errs = append(errs, designError(field+"["+strconv.Itoa(i)+"].path", "must use "+requiredExt+" extension"))
		}
	}
	return errs
}

func validateFootprintReferences(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if design.Schematic == nil || design.PCB == nil {
		return errs
	}
	symbolsByUnit := map[string]*schematic.SchematicSymbol{}
	symbolRefs := map[string]struct{}{}
	symbolPathsByRef := map[string]map[string]struct{}{}
	symbolReferenceByRef := map[string]string{}
	symbolFieldsByRef := map[string]string{}
	addSymbols := func(prefix string, file *schematic.SchematicFile) {
		if file == nil {
			return
		}
		for i := range file.Symbols {
			symbol := &file.Symbols[i]
			if !symbolRequiresPCBFootprint(symbol) {
				continue
			}
			key := referenceUnitKey(symbol.Reference, symbol.Unit)
			if _, exists := symbolsByUnit[key]; exists {
				errs = append(errs, designError(prefix+"["+strconv.Itoa(i)+"].reference", "duplicate schematic reference "+strings.TrimSpace(symbol.Reference)))
				continue
			}
			symbolsByUnit[key] = symbol
			refKey := referenceKey(symbol.Reference)
			if prior, exists := symbolReferenceByRef[refKey]; exists && prior != strings.TrimSpace(symbol.Reference) {
				errs = append(errs, designError(prefix+"["+strconv.Itoa(i)+"].reference", "multi-unit reference casing must match "+prior))
			} else {
				symbolReferenceByRef[refKey] = strings.TrimSpace(symbol.Reference)
			}
			symbolRefs[refKey] = struct{}{}
			if symbolPathsByRef[refKey] == nil {
				symbolPathsByRef[refKey] = map[string]struct{}{}
			}
			symbolPathsByRef[refKey][symbol.Path] = struct{}{}
			symbolFieldsByRef[key] = prefix + "[" + strconv.Itoa(i) + "].reference"
		}
	}
	addSymbols("schematic.symbols", design.Schematic)
	for fileIndex, sheetFile := range design.SheetFiles {
		addSymbols("sheet_files["+strconv.Itoa(fileIndex)+"].symbols", sheetFile)
	}
	footprintsByRef := map[string][]int{}
	firstFootprintIndexByRef := map[string]int{}
	for i := range design.PCB.Footprints {
		footprint := &design.PCB.Footprints[i]
		key := referenceKey(footprint.Reference)
		if len(footprintsByRef[key]) > 0 {
			errs = append(errs, designError("pcb.footprints["+strconv.Itoa(i)+"].reference", "duplicate of pcb.footprints["+strconv.Itoa(firstFootprintIndexByRef[key])+"].reference"))
		} else {
			firstFootprintIndexByRef[key] = i
		}
		footprintsByRef[key] = append(footprintsByRef[key], i)
	}
	for i, footprint := range design.PCB.Footprints {
		if _, ok := symbolRefs[referenceKey(footprint.Reference)]; !ok {
			errs = append(errs, designError("pcb.footprints["+strconv.Itoa(i)+"].reference", "missing schematic symbol"))
		}
	}
	missingFootprintRefs := map[string]struct{}{}
	validatedReferences := map[string]struct{}{}
	keys := make([]string, 0, len(symbolsByUnit))
	for key := range symbolsByUnit {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		symbol := symbolsByUnit[key]
		if symbol.OnBoard != nil && !*symbol.OnBoard {
			continue
		}
		refKey := referenceKey(symbol.Reference)
		footprintIndexes, ok := footprintsByRef[refKey]
		if !ok {
			if _, reported := missingFootprintRefs[refKey]; !reported {
				errs = append(errs, designError(symbolFieldsByRef[key], "missing PCB footprint for schematic reference "+symbol.Reference))
				missingFootprintRefs[refKey] = struct{}{}
			}
			continue
		}
		if _, validated := validatedReferences[refKey]; validated {
			continue
		}
		validatedReferences[refKey] = struct{}{}
		validPaths := symbolPathsByRef[refKey]
		for _, footprintIndex := range footprintIndexes {
			footprint := &design.PCB.Footprints[footprintIndex]
			_, matchesUnitPath := validPaths[footprint.Path]
			if !matchesUnitPath && !isKiCadPCBPath(footprint.Path) {
				errs = append(errs, designError("pcb.footprints["+strconv.Itoa(footprintIndex)+"].path", "must match schematic symbol path for "+symbol.Reference))
			}
		}
	}
	return errs
}

func validateSchematicFootprintAssignments(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if design.Schematic == nil || design.PCB == nil {
		return errs
	}
	footprintsByRef := pcbFootprintsByReference(design.PCB)
	checkFile := func(prefix string, file *schematic.SchematicFile) {
		if file == nil {
			return
		}
		for i := range file.Symbols {
			symbol := &file.Symbols[i]
			if !symbolRequiresPCBFootprint(symbol) {
				continue
			}
			assigned, assignedOK := schematicFootprintProperty(symbol)
			assigned = strings.TrimSpace(assigned)
			footprints, footprintsOK := footprintsByRef[referenceKey(symbol.Reference)]
			if !footprintsOK {
				continue
			}
			if !assignedOK || assigned == "" {
				errs = append(errs, designError(prefix+"["+strconv.Itoa(i)+"].properties.Footprint", "required for PCB footprint "+strings.TrimSpace(footprints[0].LibraryID)))
				continue
			}
			for _, footprint := range footprints {
				footprintLibraryID := strings.TrimSpace(footprint.LibraryID)
				if !sameLibraryID(footprintLibraryID, assigned) {
					errs = append(errs, designError(prefix+"["+strconv.Itoa(i)+"].properties.Footprint", "symbol "+strings.TrimSpace(symbol.Reference)+" must match PCB footprint library "+footprintLibraryID))
				}
			}
		}
	}
	checkFile("schematic.symbols", design.Schematic)
	for fileIndex, sheetFile := range design.SheetFiles {
		checkFile("sheet_files["+strconv.Itoa(fileIndex)+"].symbols", sheetFile)
	}
	return errs
}

func symbolRequiresPCBFootprint(symbol *schematic.SchematicSymbol) bool {
	if symbol == nil {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(symbol.Reference), "#") {
		return false
	}
	return symbol.OnBoard == nil || *symbol.OnBoard
}

func isKiCadPCBPath(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "/") {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(value, "/"), "/")
	for _, segment := range segments {
		if !kicadfiles.UUID(segment).Valid() {
			return false
		}
	}
	return true
}

func validateDesignSchematicReferences(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	seen := map[string]string{}
	for _, file := range designSchematicFiles(design) {
		if file.schematic == nil {
			continue
		}
		for i, symbol := range file.schematic.Symbols {
			reference := strings.TrimSpace(symbol.Reference)
			if strings.HasPrefix(reference, "#") {
				continue
			}
			key := referenceUnitKey(reference, symbol.Unit)
			location := file.prefix + ".symbols[" + strconv.Itoa(i) + "].reference"
			if prior, ok := seen[key]; ok {
				errs = append(errs, designError(location, "duplicate schematic reference "+reference+" also used by "+prior))
				continue
			}
			seen[key] = location
		}
	}
	return errs
}

func validateSymbolLibraryReferences(design Design) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	tables := tableNicknames(design.SymbolTables)
	known := nameSet(design.KnownSymbolLibraries)
	for _, file := range designSchematicFiles(design) {
		embedded := map[string]struct{}{}
		for _, symbol := range file.schematic.LibSymbols {
			embedded[symbol.LibraryID] = struct{}{}
		}
		for i, symbol := range file.schematic.Symbols {
			if _, ok := embedded[symbol.LibraryID]; ok {
				continue
			}
			nickname, err := libraryNickname(symbol.LibraryID)
			if err != nil {
				errs = append(errs, designError(file.prefix+".symbols["+strconv.Itoa(i)+"].library_id", err.Error()))
				continue
			}
			if _, ok := tables[nickname]; ok {
				continue
			}
			if _, ok := known[nickname]; ok {
				continue
			}
			errs = append(errs, designError(file.prefix+".symbols["+strconv.Itoa(i)+"].library_id", "unresolved library "+nickname))
		}
	}
	return errs
}

type designSchematicFile struct {
	prefix    string
	schematic *schematic.SchematicFile
}

func designSchematicFiles(design Design) []designSchematicFile {
	files := make([]designSchematicFile, 0, 1+len(design.SheetFiles))
	if design.Schematic != nil {
		files = append(files, designSchematicFile{prefix: "schematic", schematic: design.Schematic})
	}
	for i, file := range design.SheetFiles {
		if file != nil {
			files = append(files, designSchematicFile{prefix: "sheet_files[" + strconv.Itoa(i) + "]", schematic: file})
		}
	}
	return files
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
		addSchematicUUIDs(design.Schematic, "schematic", add)
	}
	for fileIndex, sheetFile := range design.SheetFiles {
		if sheetFile == nil {
			errs = append(errs, designError("sheet_files["+strconv.Itoa(fileIndex)+"]", "nil"))
			continue
		}
		addSchematicUUIDs(sheetFile, "sheet_files["+strconv.Itoa(fileIndex)+"]", add)
	}
	if design.PCB != nil {
		addPCBUUIDs(design.PCB, add)
	}
	return errs
}

type uuidAddFunc func(id kicadfiles.UUID, location uuidLocation)

func addSchematicUUIDs(schematicFile *schematic.SchematicFile, prefix string, add uuidAddFunc) {
	add(schematicFile.UUID, uuidLocation{field: prefix + ".uuid"})
	for i, symbol := range schematicFile.Symbols {
		add(symbol.UUID, uuidLocation{collection: prefix + ".symbols", index: i, field: "uuid"})
		pinCollection := prefix + ".symbols[" + strconv.Itoa(i) + "].pins"
		for pinIndex, pin := range symbol.Pins {
			add(pin.UUID, uuidLocation{collection: pinCollection, index: pinIndex, field: "uuid"})
		}
	}
	for i, wire := range schematicFile.Wires {
		add(wire.UUID, uuidLocation{collection: prefix + ".wires", index: i, field: "uuid"})
	}
	for i, noConnect := range schematicFile.NoConnects {
		add(noConnect.UUID, uuidLocation{collection: prefix + ".no_connects", index: i, field: "uuid"})
	}
	for i, label := range schematicFile.Labels {
		add(label.UUID, uuidLocation{collection: prefix + ".labels", index: i, field: "uuid"})
	}
	for i, junction := range schematicFile.Junctions {
		add(junction.UUID, uuidLocation{collection: prefix + ".junctions", index: i, field: "uuid"})
	}
	for i, bus := range schematicFile.Buses {
		add(bus.UUID, uuidLocation{collection: prefix + ".buses", index: i, field: "uuid"})
	}
	for i, polyline := range schematicFile.Polylines {
		add(polyline.UUID, uuidLocation{collection: prefix + ".polylines", index: i, field: "uuid"})
	}
	for i, entry := range schematicFile.BusEntries {
		add(entry.UUID, uuidLocation{collection: prefix + ".bus_entries", index: i, field: "uuid"})
	}
	for i, text := range schematicFile.Texts {
		add(text.UUID, uuidLocation{collection: prefix + ".texts", index: i, field: "uuid"})
	}
	for i, sheet := range schematicFile.Sheets {
		add(sheet.UUID, uuidLocation{collection: prefix + ".sheets", index: i, field: "uuid"})
		pinCollection := prefix + ".sheets[" + strconv.Itoa(i) + "].pins"
		for pinIndex, pin := range sheet.Pins {
			add(pin.UUID, uuidLocation{collection: pinCollection, index: pinIndex, field: "uuid"})
		}
	}
	for i, raw := range schematicFile.RawItems {
		add(raw.UUID, uuidLocation{collection: prefix + ".raw_items", index: i, field: "uuid"})
	}
}

func addPCBUUIDs(board *pcb.PCBFile, add uuidAddFunc) {
	for i, footprint := range board.Footprints {
		add(footprint.UUID, uuidLocation{collection: "pcb.footprints", index: i, field: "uuid"})
		footprintCollection := "pcb.footprints[" + strconv.Itoa(i) + "]"
		for propertyIndex, property := range footprint.Properties {
			add(property.UUID, uuidLocation{collection: footprintCollection + ".properties", index: propertyIndex, field: "uuid"})
		}
		for textIndex, text := range footprint.Texts {
			add(text.UUID, uuidLocation{collection: footprintCollection + ".texts", index: textIndex, field: "uuid"})
		}
		for padIndex, pad := range footprint.Pads {
			add(pad.UUID, uuidLocation{collection: footprintCollection + ".pads", index: padIndex, field: "uuid"})
		}
		for graphicIndex, graphic := range footprint.Graphics {
			add(graphic.UUID, uuidLocation{collection: footprintCollection + ".graphics", index: graphicIndex, field: "uuid"})
		}
	}
	for i, drawing := range board.Drawings {
		add(drawing.UUID, uuidLocation{collection: "pcb.drawings", index: i, field: "uuid"})
	}
	for i, track := range board.Tracks {
		add(track.UUID, uuidLocation{collection: "pcb.tracks", index: i, field: "uuid"})
	}
	for i, via := range board.Vias {
		add(via.UUID, uuidLocation{collection: "pcb.vias", index: i, field: "uuid"})
	}
	for i, zone := range board.Zones {
		add(zone.UUID, uuidLocation{collection: "pcb.zones", index: i, field: "uuid"})
	}
	for i, dimension := range board.Dimensions {
		add(dimension.UUID, uuidLocation{collection: "pcb.dimensions", index: i, field: "uuid"})
	}
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
