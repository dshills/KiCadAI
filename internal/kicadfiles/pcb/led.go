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

	nets := []Net{
		{Code: 1, Name: "VCC"},
		{Code: 2, Name: "LED_OUT"},
		{Code: 3, Name: "GND"},
	}
	resistor := twoPadFootprint(generator, "r1", "Resistor_SMD:R_0805_2012Metric", "R1", "1k", kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(20)}, 1, 2)
	led := twoPadFootprint(generator, "d1", "LED_SMD:LED_0805_2012Metric", "D1", "LED", kicadfiles.Point{X: kicadfiles.MM(45), Y: kicadfiles.MM(20)}, 2, 3)

	return PCBFile{
		Version:    kicadfiles.KiCadFormatV20230121,
		Generator:  "kicadai",
		Paper:      kicadfiles.Paper{Name: "A4"},
		Layers:     DefaultTwoLayerStack(),
		Setup:      PCBSetup{Stackup: PCBStackup{Thickness: kicadfiles.MM(1.6)}},
		Nets:       nets,
		Footprints: []Footprint{led, resistor},
		Drawings:   rectangleOutline(generator, kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}, kicadfiles.Point{X: kicadfiles.MM(60), Y: kicadfiles.MM(30)}),
		Tracks: []Track{
			track(generator, "vcc-r1", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}, kicadfiles.Point{X: kicadfiles.MM(23.9), Y: kicadfiles.MM(20)}, 1),
			track(generator, "r1-d1", kicadfiles.Point{X: kicadfiles.MM(26.1), Y: kicadfiles.MM(20)}, kicadfiles.Point{X: kicadfiles.MM(43.9), Y: kicadfiles.MM(20)}, 2),
			track(generator, "d1-gnd", kicadfiles.Point{X: kicadfiles.MM(46.1), Y: kicadfiles.MM(20)}, kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}, 3),
		},
		Vias: []Via{
			via(generator, "vcc", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}, 1),
			via(generator, "gnd", kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(20)}, 3),
		},
		TitleBlock:           kicadfiles.TitleBlock{Title: "LED Indicator"},
		RequireClosedOutline: true,
	}, nil
}

func twoPadFootprint(generator kicadfiles.IDGenerator, key, libraryID, reference, value string, position kicadfiles.Point, leftNet, rightNet int) Footprint {
	path := "root.component." + key
	return Footprint{
		UUID:      generator.New(path),
		Path:      path,
		LibraryID: libraryID,
		Reference: reference,
		Value:     value,
		Position:  position,
		Layer:     kicadfiles.LayerFCu,
		Texts: []FootprintText{
			{Kind: "reference", Text: reference, Position: kicadfiles.Point{X: 0, Y: kicadfiles.MM(-1.5)}, Layer: kicadfiles.LayerFSilkS},
			{Kind: "value", Text: value, Position: kicadfiles.Point{X: 0, Y: kicadfiles.MM(1.5)}, Layer: kicadfiles.LayerFSilkS},
		},
		Pads: []Pad{
			{Name: "1", NetCode: leftNet, Shape: "roundrect", Position: kicadfiles.Point{X: kicadfiles.MM(-1.1), Y: 0}, Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1.45)}, Layers: smdPadLayers()},
			{Name: "2", NetCode: rightNet, Shape: "roundrect", Position: kicadfiles.Point{X: kicadfiles.MM(1.1), Y: 0}, Size: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(1.45)}, Layers: smdPadLayers()},
		},
	}
}

func smdPadLayers() []kicadfiles.BoardLayer {
	return []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFPaste, kicadfiles.LayerFMask}
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
