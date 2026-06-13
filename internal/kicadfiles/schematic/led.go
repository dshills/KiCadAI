package schematic

import (
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

func LEDIndicatorSchematic(input LEDIndicatorInput) (SchematicFile, error) {
	if input.Name == "" {
		input.Name = "led_indicator"
	}
	if input.Seed == "" {
		input.Seed = input.Name
	}
	if input.LibraryVCC == "" {
		input.LibraryVCC = "power:VCC"
	}
	if input.LibraryGND == "" {
		input.LibraryGND = "power:GND"
	}
	if input.LibraryResistor == "" {
		input.LibraryResistor = "Device:R"
	}
	if input.LibraryLED == "" {
		input.LibraryLED = "Device:LED"
	}
	generator, err := kicadfiles.NewDeterministicIDGenerator(input.DesignID, input.Seed)
	if err != nil {
		return SchematicFile{}, err
	}

	powerVCC := symbol(generator, "vcc", input.LibraryVCC, "#PWR01", "VCC", kicadfiles.Point{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(25.4)}, "1")
	resistor := symbol(generator, "r1", input.LibraryResistor, "R1", "1k", kicadfiles.Point{X: kicadfiles.MM(50.8), Y: kicadfiles.MM(25.4)}, "2", "1")
	led := symbol(generator, "d1", input.LibraryLED, "D1", "LED", kicadfiles.Point{X: kicadfiles.MM(76.2), Y: kicadfiles.MM(25.4)}, "1", "2")
	powerGND := symbol(generator, "gnd", input.LibraryGND, "#PWR02", "GND", kicadfiles.Point{X: kicadfiles.MM(101.6), Y: kicadfiles.MM(25.4)}, "1")
	vccPin := offsetX(powerVCC.Position, kicadfiles.MM(5.08))
	resistorLeft := offsetX(resistor.Position, -kicadfiles.MM(5.08))
	resistorRight := offsetX(resistor.Position, kicadfiles.MM(5.08))
	ledLeft := offsetX(led.Position, -kicadfiles.MM(5.08))
	ledRight := offsetX(led.Position, kicadfiles.MM(5.08))
	gndPin := offsetX(powerGND.Position, -kicadfiles.MM(5.08))

	return SchematicFile{
		Filename:         input.Name + ".kicad_sch",
		Version:          kicadfiles.KiCadFormatV20260306,
		Generator:        "eeschema",
		GeneratorVersion: "10.0",
		UUID:             generator.New("root.schematic"),
		Paper:            kicadfiles.Paper{Name: "A4"},
		TitleBlock: kicadfiles.TitleBlock{
			Title: "LED Indicator",
		},
		LibSymbols: []EmbeddedSymbol{
			{LibraryID: input.LibraryLED, Body: twoPinSymbolBody(input.LibraryLED, "LED", "passive")},
			{LibraryID: input.LibraryResistor, Body: twoPinSymbolBody(input.LibraryResistor, "R", "passive")},
			{LibraryID: input.LibraryGND, Body: powerSymbolBody(input.LibraryGND, "GND", "power_in", -5.08)},
			{LibraryID: input.LibraryVCC, Body: powerSymbolBody(input.LibraryVCC, "VCC", "power_out", 5.08)},
		},
		Symbols: []SchematicSymbol{powerVCC, resistor, led, powerGND},
		Wires: []Wire{
			wire(generator, "vcc-r1", vccPin, resistorLeft),
			wire(generator, "r1-d1", resistorRight, ledLeft),
			wire(generator, "d1-gnd", ledRight, gndPin),
		},
		Labels: []Label{
			{
				UUID:     generator.New("root.schematic.label.led_out"),
				Text:     "LED_OUT",
				Kind:     LabelLocal,
				Position: kicadfiles.Point{X: kicadfiles.MM(63.5), Y: kicadfiles.MM(20.32)},
			},
		},
		Junctions: []Junction{
			{UUID: generator.New("root.schematic.junction.r1_led"), Position: led.Position},
		},
		Instances: []SymbolInstance{
			{Path: powerVCC.Path, Reference: powerVCC.Reference, Unit: 1, Value: powerVCC.Value},
			{Path: resistor.Path, Reference: resistor.Reference, Unit: 1, Value: resistor.Value},
			{Path: led.Path, Reference: led.Reference, Unit: 1, Value: led.Value},
			{Path: powerGND.Path, Reference: powerGND.Reference, Unit: 1, Value: powerGND.Value},
		},
	}, nil
}

func symbol(generator kicadfiles.IDGenerator, key, libraryID, reference, value string, position kicadfiles.Point, pinNumbers ...string) SchematicSymbol {
	path := "root.component." + key
	pins := make([]SymbolPin, 0, len(pinNumbers))
	for _, number := range pinNumbers {
		pins = append(pins, SymbolPin{
			Number: number,
			UUID:   generator.New(path + ".pin." + number),
		})
	}
	return SchematicSymbol{
		UUID:      generator.New(path),
		Path:      path,
		LibraryID: libraryID,
		Reference: reference,
		Value:     value,
		Position:  position,
		Pins:      pins,
	}
}

func wire(generator kicadfiles.IDGenerator, key string, start, end kicadfiles.Point) Wire {
	return Wire{
		UUID:   generator.New("root.schematic.wire." + key),
		Points: []kicadfiles.Point{start, end},
	}
}

func offsetX(point kicadfiles.Point, offset kicadfiles.IU) kicadfiles.Point {
	return kicadfiles.Point{X: point.X + offset, Y: point.Y}
}

func twoPinSymbolBody(libraryID, bodyName, pinType string) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(libraryID)}
	nodes = append(nodes, embeddedSymbolDefaults()...)
	nodes = append(nodes,
		sexpr.L(
			sexpr.A("symbol"),
			sexpr.S(bodyName+"_1_1"),
			embeddedPin(pinType, -5.08, 0, "~", "1"),
			embeddedPin(pinType, 5.08, 180, "~", "2"),
		),
		// KiCad 10.0.3 writes embedded_fonts inside each embedded lib symbol.
		sexpr.L(sexpr.A("embedded_fonts"), sexpr.A("no")),
	)
	return sexpr.L(nodes...)
}

func powerSymbolBody(libraryID, bodyName, pinType string, pinX float64) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(libraryID)}
	nodes = append(nodes, embeddedSymbolDefaults()...)
	nodes = append(nodes,
		sexpr.L(
			sexpr.A("symbol"),
			sexpr.S(bodyName+"_1_1"),
			// Match KiCad's save output for these generated power symbols.
			embeddedPin(pinType, pinX, 180, libraryID, "1"),
		),
		// KiCad 10.0.3 writes embedded_fonts inside each embedded lib symbol.
		sexpr.L(sexpr.A("embedded_fonts"), sexpr.A("no")),
	)
	return sexpr.L(nodes...)
}

func embeddedSymbolDefaults() []sexpr.Node {
	return []sexpr.Node{
		sexpr.L(sexpr.A("exclude_from_sim"), sexpr.A("no")),
		sexpr.L(sexpr.A("in_bom"), sexpr.A("yes")),
		sexpr.L(sexpr.A("on_board"), sexpr.A("yes")),
		sexpr.L(sexpr.A("in_pos_files"), sexpr.A("yes")),
		sexpr.L(sexpr.A("duplicate_pin_numbers_are_jumpers"), sexpr.A("no")),
		embeddedLibraryProperty("Reference", "", false),
		embeddedLibraryProperty("Value", "", false),
		embeddedLibraryProperty("Footprint", "", true),
		embeddedLibraryProperty("Datasheet", "", true),
		embeddedLibraryProperty("Description", "", true),
	}
}

func embeddedLibraryProperty(name, value string, hidden bool) sexpr.List {
	return sexpr.L(
		sexpr.A("property"),
		sexpr.S(name),
		sexpr.S(value),
		sexpr.L(sexpr.A("at"), sexpr.I(0), sexpr.I(0), sexpr.I(0)),
		sexpr.L(sexpr.A("show_name"), sexpr.A("no")),
		sexpr.L(sexpr.A("do_not_autoplace"), sexpr.A("no")),
		sexpr.OmitIf(!hidden, sexpr.L(sexpr.A("hide"), sexpr.A("yes"))),
		renderEffects(false),
	)
}

func embeddedPin(pinType string, pinX float64, rotation int64, name string, number string) sexpr.List {
	return sexpr.L(
		sexpr.A("pin"),
		sexpr.A(pinType),
		sexpr.A("line"),
		sexpr.L(sexpr.A("at"), sexpr.F(pinX), sexpr.I(0), sexpr.I(rotation)),
		sexpr.L(sexpr.A("length"), sexpr.X("2.54")),
		sexpr.L(sexpr.A("name"), sexpr.S(name), renderEffects(false)),
		sexpr.L(sexpr.A("number"), sexpr.S(number), renderEffects(false)),
	)
}
