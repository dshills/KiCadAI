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
	generator, err := kicadfiles.NewDeterministicIDGenerator(input.DesignID, input.Seed)
	if err != nil {
		return SchematicFile{}, err
	}

	powerVCC := symbol(generator, "vcc", "power:VCC", "#PWR01", "VCC", kicadfiles.Point{X: kicadfiles.MM(25.4), Y: kicadfiles.MM(25.4)})
	resistor := symbol(generator, "r1", "Device:R", "R1", "1k", kicadfiles.Point{X: kicadfiles.MM(50.8), Y: kicadfiles.MM(25.4)})
	led := symbol(generator, "d1", "Device:LED", "D1", "LED", kicadfiles.Point{X: kicadfiles.MM(76.2), Y: kicadfiles.MM(25.4)})
	powerGND := symbol(generator, "gnd", "power:GND", "#PWR02", "GND", kicadfiles.Point{X: kicadfiles.MM(101.6), Y: kicadfiles.MM(25.4)})
	vccPin := offsetX(powerVCC.Position, kicadfiles.MM(5.08))
	resistorLeft := offsetX(resistor.Position, -kicadfiles.MM(5.08))
	resistorRight := offsetX(resistor.Position, kicadfiles.MM(5.08))
	ledLeft := offsetX(led.Position, -kicadfiles.MM(5.08))
	ledRight := offsetX(led.Position, kicadfiles.MM(5.08))
	gndPin := offsetX(powerGND.Position, -kicadfiles.MM(5.08))

	return SchematicFile{
		Filename:  input.Name + ".kicad_sch",
		Version:   kicadfiles.KiCadFormatV20230121,
		Generator: "kicadai",
		UUID:      generator.New("root.schematic"),
		Paper:     kicadfiles.Paper{Name: "A4"},
		TitleBlock: kicadfiles.TitleBlock{
			Title: "LED Indicator",
		},
		LibSymbols: []EmbeddedSymbol{
			{LibraryID: "power:VCC", Body: powerSymbolBody("power:VCC", "power_out", 5.08)},
			{LibraryID: "power:GND", Body: powerSymbolBody("power:GND", "power_in", -5.08)},
			{LibraryID: "Device:R", Body: twoPinSymbolBody("Device:R", "passive")},
			{LibraryID: "Device:LED", Body: twoPinSymbolBody("Device:LED", "passive")},
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

func symbol(generator kicadfiles.IDGenerator, key, libraryID, reference, value string, position kicadfiles.Point) SchematicSymbol {
	path := "root.component." + key
	return SchematicSymbol{
		UUID:      generator.New(path),
		Path:      path,
		LibraryID: libraryID,
		Reference: reference,
		Value:     value,
		Position:  position,
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

func twoPinSymbolBody(libraryID, pinType string) sexpr.List {
	return sexpr.L(
		sexpr.A("symbol"),
		sexpr.S(libraryID),
		sexpr.L(sexpr.A("pin"), sexpr.A(pinType), sexpr.A("line"),
			sexpr.L(sexpr.A("at"), sexpr.X("-5.08"), sexpr.X("0.0"), sexpr.I(0)),
			sexpr.L(sexpr.A("length"), sexpr.X("2.54")),
			sexpr.L(sexpr.A("name"), sexpr.S("~")),
			sexpr.L(sexpr.A("number"), sexpr.S("1")),
		),
		sexpr.L(sexpr.A("pin"), sexpr.A(pinType), sexpr.A("line"),
			sexpr.L(sexpr.A("at"), sexpr.X("5.08"), sexpr.X("0.0"), sexpr.I(180)),
			sexpr.L(sexpr.A("length"), sexpr.X("2.54")),
			sexpr.L(sexpr.A("name"), sexpr.S("~")),
			sexpr.L(sexpr.A("number"), sexpr.S("2")),
		),
	)
}

func powerSymbolBody(libraryID, pinType string, pinX float64) sexpr.List {
	return sexpr.L(
		sexpr.A("symbol"),
		sexpr.S(libraryID),
		sexpr.L(sexpr.A("pin"), sexpr.A(pinType), sexpr.A("line"),
			sexpr.L(sexpr.A("at"), sexpr.F(pinX), sexpr.X("0.0"), sexpr.I(180)),
			sexpr.L(sexpr.A("length"), sexpr.X("2.54")),
			sexpr.L(sexpr.A("name"), sexpr.S(libraryID)),
			sexpr.L(sexpr.A("number"), sexpr.S("1")),
		),
	)
}
