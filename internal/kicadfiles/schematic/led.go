package schematic

import (
	"kicadai/internal/kicadfiles"
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

	schematic := SchematicFile{
		Filename:         input.Name + ".kicad_sch",
		Version:          kicadfiles.KiCadFormatV20260306,
		Generator:        "eeschema",
		GeneratorVersion: "10.0",
		UUID:             generator.New("root.schematic"),
		Paper:            kicadfiles.Paper{Name: "A4"},
		TitleBlock: kicadfiles.TitleBlock{
			Title: "LED Indicator",
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
	}
	EnsureEmbeddedTwoPinSymbol(&schematic, input.LibraryLED, "LED", "passive")
	EnsureEmbeddedTwoPinSymbol(&schematic, input.LibraryResistor, "R", "passive")
	EnsureEmbeddedPowerSymbol(&schematic, input.LibraryGND, "GND", "power_in", -5.08)
	EnsureEmbeddedPowerSymbol(&schematic, input.LibraryVCC, "VCC", "power_in", 5.08)
	return schematic, nil
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
