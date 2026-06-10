package pcb

import "kicadai/internal/kicadfiles"

type LEDIndicatorInput struct {
	Name     string
	DesignID kicadfiles.UUID
	Seed     string
}

func LEDIndicatorPCB(input LEDIndicatorInput) (PCBFile, error) {
	if input.Name == "" {
		input.Name = "led_indicator"
	}
	if input.Seed == "" {
		input.Seed = input.Name
	}
	generator, err := kicadfiles.NewDeterministicIDGenerator(input.DesignID, input.Seed)
	if err != nil {
		return PCBFile{}, err
	}

	nets := NewNetRegistry()
	vcc := nets.EnsureNet("VCC").Code
	ledOut := nets.EnsureNet("LED_OUT").Code
	gnd := nets.EnsureNet("GND").Code
	resistor := twoPadFootprint(generator, "r1", "Resistor_SMD:R_0805_2012Metric", "R1", "1k", kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(20)}, vcc, ledOut)
	led := twoPadFootprint(generator, "d1", "LED_SMD:LED_0805_2012Metric", "D1", "LED", kicadfiles.Point{X: kicadfiles.MM(45), Y: kicadfiles.MM(20)}, ledOut, gnd)
	embeddedFonts := false

	return PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "pcbnew",
		GeneratorVersion: "10.0",
		General:          DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           DefaultTwoLayerStack(),
		Setup:            DefaultSetup(),
		Nets:             nets.Nets(),
		Footprints:       []Footprint{led, resistor},
		Drawings:         rectangleOutline(generator, kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}, kicadfiles.Point{X: kicadfiles.MM(60), Y: kicadfiles.MM(30)}),
		Tracks: []Track{
			track(generator, "vcc-r1", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}, kicadfiles.Point{X: kicadfiles.MM(23.9), Y: kicadfiles.MM(20)}, vcc),
			track(generator, "r1-d1", kicadfiles.Point{X: kicadfiles.MM(26.1), Y: kicadfiles.MM(20)}, kicadfiles.Point{X: kicadfiles.MM(43.9), Y: kicadfiles.MM(20)}, ledOut),
			track(generator, "d1-gnd", kicadfiles.Point{X: kicadfiles.MM(46.1), Y: kicadfiles.MM(20)}, kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}, gnd),
		},
		Vias: []Via{
			via(generator, "vcc", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}, vcc),
			via(generator, "gnd", kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}, gnd),
		},
		TitleBlock:           kicadfiles.TitleBlock{Title: "LED Indicator"},
		EmbeddedFonts:        &embeddedFonts,
		RequireClosedOutline: true,
	}, nil
}

func twoPadFootprint(generator kicadfiles.IDGenerator, key, libraryID, reference, value string, position kicadfiles.Point, leftNet, rightNet int) Footprint {
	path := "root.component." + key
	uuid := generator.New("root.pcb.footprint."+key, path)
	pcbPath := "/" + string(generator.New("root.pcb.footprint."+key+".path", path))
	embeddedFonts := false
	duplicatePadNumbersAreJumpers := false
	return Footprint{
		UUID:                          uuid,
		Path:                          pcbPath,
		LibraryID:                     libraryID,
		Reference:                     reference,
		Value:                         value,
		Position:                      position,
		Layer:                         kicadfiles.LayerFCu,
		Properties:                    footprintProperties(generator, key, reference, value),
		Attributes:                    []string{"smd"},
		EmbeddedFonts:                 &embeddedFonts,
		DuplicatePadNumbersAreJumpers: &duplicatePadNumbersAreJumpers,
		Pads: []Pad{
			{Name: "1", UUID: generator.New("root.pcb.footprint."+key+".pad", "1"), NetCode: leftNet, Shape: "roundrect", Position: kicadfiles.Point{X: kicadfiles.MM(-1.1), Y: 0}, Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1.45)}, Layers: smdPadLayers()},
			{Name: "2", UUID: generator.New("root.pcb.footprint."+key+".pad", "2"), NetCode: rightNet, Shape: "roundrect", Position: kicadfiles.Point{X: kicadfiles.MM(1.1), Y: 0}, Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1.45)}, Layers: smdPadLayers()},
		},
	}
}

func footprintProperties(generator kicadfiles.IDGenerator, key, reference, value string) []FootprintProperty {
	return []FootprintProperty{
		footprintProperty(generator, key, "Reference", reference, kicadfiles.Point{X: 0, Y: kicadfiles.MM(-1.5)}, kicadfiles.LayerFSilkS, false),
		footprintProperty(generator, key, "Value", value, kicadfiles.Point{X: 0, Y: kicadfiles.MM(1.5)}, kicadfiles.LayerFSilkS, false),
		footprintProperty(generator, key, "Datasheet", "", kicadfiles.Point{X: 0, Y: 0}, kicadfiles.LayerFFab, true),
		footprintProperty(generator, key, "Description", "", kicadfiles.Point{X: 0, Y: 0}, kicadfiles.LayerFFab, true),
	}
}

func footprintProperty(generator kicadfiles.IDGenerator, key, name, value string, position kicadfiles.Point, layer kicadfiles.BoardLayer, hide bool) FootprintProperty {
	return FootprintProperty{
		UUID:     generator.New("root.pcb.footprint."+key+".property", name),
		Name:     name,
		Value:    value,
		Position: position,
		Layer:    layer,
		Hide:     hide,
		Unlocked: true,
		Effects: TextEffects{
			FontSize:          kicadfiles.Point{X: kicadfiles.MM(1.27), Y: kicadfiles.MM(1.27)},
			OmitFontThickness: hide,
		},
	}
}

func smdPadLayers() []kicadfiles.BoardLayer {
	return []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask, kicadfiles.LayerFPaste}
}

func rectangleOutline(generator kicadfiles.IDGenerator, topLeft, bottomRight kicadfiles.Point) []Drawing {
	topRight := kicadfiles.Point{X: bottomRight.X, Y: topLeft.Y}
	bottomLeft := kicadfiles.Point{X: topLeft.X, Y: bottomRight.Y}
	return []Drawing{
		outlineLine(generator, "top", topLeft, topRight),
		outlineLine(generator, "right", topRight, bottomRight),
		outlineLine(generator, "bottom", bottomRight, bottomLeft),
		outlineLine(generator, "left", bottomLeft, topLeft),
	}
}

func outlineLine(generator kicadfiles.IDGenerator, key string, start, end kicadfiles.Point) Drawing {
	return Drawing{
		UUID:  generator.New("root.pcb.outline." + key),
		Layer: kicadfiles.LayerEdge,
		Kind:  "line",
		Line:  &LineDrawing{Start: start, End: end, Width: kicadfiles.MM(0.1)},
	}
}

func track(generator kicadfiles.IDGenerator, key string, start, end kicadfiles.Point, netCode int) Track {
	return Track{
		UUID:    generator.New("root.pcb.track." + key),
		Start:   start,
		End:     end,
		Width:   kicadfiles.MM(0.25),
		Layer:   kicadfiles.LayerFCu,
		NetCode: netCode,
	}
}

func via(generator kicadfiles.IDGenerator, key string, position kicadfiles.Point, netCode int) Via {
	return Via{
		UUID:     generator.New("root.pcb.via." + key),
		Position: position,
		Size:     kicadfiles.MM(0.8),
		Drill:    kicadfiles.MM(0.4),
		NetCode:  netCode,
		Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
	}
}
