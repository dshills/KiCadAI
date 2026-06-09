package design

import (
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/kicadfiles/schematic"
)

type Design struct {
	Name         string
	Project      project.ProjectFile
	Schematic    *schematic.SchematicFile
	PCB          *pcb.PCBFile
	ExpectedNets []string
}

type LEDIndicatorInput struct {
	Name       string
	DesignID   kicadfiles.UUID
	Seed       string
	IncludePCB bool
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
		Name:     input.Name,
		DesignID: input.DesignID,
		Seed:     input.Seed,
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
	}
	if design.PCB != nil {
		if err := pcb.Validate(*design.PCB); err != nil {
			errs = append(errs, nestedErrors(err)...)
		}
		errs = append(errs, validateFootprintReferences(design)...)
		errs = append(errs, validateExpectedNets(design)...)
	}
	errs = append(errs, validateUniqueUUIDs(design)...)
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
