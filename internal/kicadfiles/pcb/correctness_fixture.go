package pcb

import "kicadai/internal/kicadfiles"

type CorrectnessFixtureInput struct {
	Name     string
	DesignID kicadfiles.UUID
	Seed     string
}

func CorrectnessFixturePCB(input CorrectnessFixtureInput) (PCBFile, error) {
	if input.Name == "" {
		input.Name = "pcb_object_correctness"
	}
	if input.Seed == "" {
		input.Seed = input.Name
	}
	generator, err := kicadfiles.NewDeterministicIDGenerator(input.DesignID, input.Seed)
	if err != nil {
		return PCBFile{}, err
	}

	nets := NewNetRegistry()
	gnd := nets.EnsureNet("GND").Code
	vcc := nets.EnsureNet("VCC").Code
	signal := nets.EnsureNet("SIGNAL").Code

	resistor := correctnessSMD(generator, "r1", "Resistor_SMD:R_0805_2012Metric", "R1", "1k", kicadfiles.Point{X: kicadfiles.MM(28), Y: kicadfiles.MM(24)}, vcc, signal)
	led := correctnessSMD(generator, "d1", "LED_SMD:LED_0805_2012Metric", "D1", "LED", kicadfiles.Point{X: kicadfiles.MM(46), Y: kicadfiles.MM(24)}, signal, gnd)
	connector := correctnessConnector(generator, "j1", kicadfiles.Point{X: kicadfiles.MM(16), Y: kicadfiles.MM(24)}, vcc, gnd)

	return PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "pcbnew",
		GeneratorVersion: "10.0",
		General:          DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           DefaultTwoLayerStack(),
		Setup:            DefaultSetup(),
		Nets:             nets.Nets(),
		Footprints:       []Footprint{connector, led, resistor},
		Drawings: append(
			rectangleOutline(generator, kicadfiles.Point{X: kicadfiles.MM(8), Y: kicadfiles.MM(12)}, kicadfiles.Point{X: kicadfiles.MM(62), Y: kicadfiles.MM(36)}),
			Drawing{
				UUID:  generator.New("root.pcb.text", "label"),
				Layer: kicadfiles.LayerFSilkS,
				Text: &TextDrawing{
					Text:     "PCB object correctness",
					Position: kicadfiles.Point{X: kicadfiles.MM(35), Y: kicadfiles.MM(16)},
					Effects:  TextEffects{FontSize: kicadfiles.Point{X: kicadfiles.MM(1.2), Y: kicadfiles.MM(1.2)}, FontThickness: kicadfiles.MM(0.15)},
				},
			},
		),
		Tracks: []Track{
			trackOnLayer(generator, "j1-vcc", kicadfiles.Point{X: kicadfiles.MM(16), Y: kicadfiles.MM(22.73)}, kicadfiles.Point{X: kicadfiles.MM(26.9), Y: kicadfiles.MM(24)}, kicadfiles.LayerFCu, vcc),
			trackOnLayer(generator, "r1-signal", kicadfiles.Point{X: kicadfiles.MM(29.1), Y: kicadfiles.MM(24)}, kicadfiles.Point{X: kicadfiles.MM(36), Y: kicadfiles.MM(24)}, kicadfiles.LayerFCu, signal),
			trackOnLayer(generator, "back-gnd", kicadfiles.Point{X: kicadfiles.MM(47.1), Y: kicadfiles.MM(24)}, kicadfiles.Point{X: kicadfiles.MM(16), Y: kicadfiles.MM(25.27)}, kicadfiles.LayerBCu, gnd),
		},
		TrackArcs: []TrackArc{{
			UUID:    generator.New("root.pcb.arc", "signal"),
			Start:   kicadfiles.Point{X: kicadfiles.MM(36), Y: kicadfiles.MM(24)},
			Mid:     kicadfiles.Point{X: kicadfiles.MM(39), Y: kicadfiles.MM(21)},
			End:     kicadfiles.Point{X: kicadfiles.MM(44.9), Y: kicadfiles.MM(24)},
			Width:   kicadfiles.MM(0.25),
			Layer:   kicadfiles.LayerFCu,
			NetCode: signal,
		}},
		Vias: []Via{
			via(generator, "signal-test", kicadfiles.Point{X: kicadfiles.MM(36), Y: kicadfiles.MM(24)}, signal),
			via(generator, "gnd-return", kicadfiles.Point{X: kicadfiles.MM(47.1), Y: kicadfiles.MM(24)}, gnd),
		},
		Zones: []Zone{{
			UUID:                 generator.New("root.pcb.zone", "gnd"),
			NetCode:              gnd,
			NetName:              "GND",
			Name:                 "GND pour",
			Layers:               []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
			HatchStyle:           "edge",
			HatchPitch:           kicadfiles.MM(0.5),
			ConnectPads:          true,
			Clearance:            kicadfiles.MM(0.2),
			MinThickness:         kicadfiles.MM(0.25),
			FilledAreasThickness: false,
			Fill:                 ZoneFillSettings{ThermalGap: kicadfiles.MM(0.5), ThermalBridgeWidth: kicadfiles.MM(0.5), IslandRemovalMode: zoneIslandRemovalNever},
			Polygons: [][]kicadfiles.Point{{
				{X: kicadfiles.MM(10), Y: kicadfiles.MM(14)},
				{X: kicadfiles.MM(60), Y: kicadfiles.MM(14)},
				{X: kicadfiles.MM(60), Y: kicadfiles.MM(34)},
				{X: kicadfiles.MM(10), Y: kicadfiles.MM(34)},
			}},
		}},
		TitleBlock:           kicadfiles.TitleBlock{Title: "PCB Object Correctness"},
		EmbeddedFonts:        boolPtr(false),
		RequireClosedOutline: true,
	}, nil
}

func correctnessSMD(generator kicadfiles.IDGenerator, key, libraryID, reference, value string, position kicadfiles.Point, leftNet, rightNet int) Footprint {
	footprint := twoPadFootprint(generator, key, libraryID, reference, value, position, leftNet, rightNet)
	footprint.Graphics = footprintBodyGraphics(generator, key, false)
	for index := range footprint.Pads {
		footprint.Pads[index].PinFunction = footprint.Pads[index].Name
		footprint.Pads[index].PinType = "passive"
	}
	return footprint
}

func correctnessConnector(generator kicadfiles.IDGenerator, key string, position kicadfiles.Point, vccNet, gndNet int) Footprint {
	footprint := Footprint{
		UUID:                          generator.New("root.pcb.footprint."+key, "connector"),
		Path:                          "/" + string(generator.New("root.pcb.footprint."+key+".path", "connector")),
		LibraryID:                     "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical",
		Reference:                     "J1",
		Value:                         "PWR",
		Position:                      position,
		Layer:                         kicadfiles.LayerFCu,
		Properties:                    footprintProperties(generator, key, "J1", "PWR"),
		Attributes:                    []string{"through_hole"},
		Graphics:                      footprintBodyGraphics(generator, key, true),
		EmbeddedFonts:                 boolPtr(false),
		DuplicatePadNumbersAreJumpers: boolPtr(false),
		Pads: []Pad{
			{Name: "1", UUID: generator.New("root.pcb.footprint."+key+".pad", "1"), Type: "thru_hole", Shape: "circle", Position: kicadfiles.Point{X: 0, Y: kicadfiles.MM(-1.27)}, Size: kicadfiles.Point{X: kicadfiles.MM(1.7), Y: kicadfiles.MM(1.7)}, Drill: kicadfiles.MM(1), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}, NetCode: vccNet, PinFunction: "VCC", PinType: "power_in"},
			{Name: "2", UUID: generator.New("root.pcb.footprint."+key+".pad", "2"), Type: "thru_hole", Shape: "circle", Position: kicadfiles.Point{X: 0, Y: kicadfiles.MM(1.27)}, Size: kicadfiles.Point{X: kicadfiles.MM(1.7), Y: kicadfiles.MM(1.7)}, Drill: kicadfiles.MM(1), Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllCu, kicadfiles.LayerAllMask}, NetCode: gndNet, PinFunction: "GND", PinType: "power_in"},
		},
	}
	return footprint
}

func boolPtr(value bool) *bool {
	return &value
}

func footprintBodyGraphics(generator kicadfiles.IDGenerator, key string, connector bool) []FootprintGraphic {
	width := kicadfiles.MM(4.2)
	height := kicadfiles.MM(3)
	if connector {
		width = kicadfiles.MM(4)
		height = kicadfiles.MM(6)
	}
	return []FootprintGraphic{
		{UUID: generator.New("root.pcb.footprint."+key+".graphic", "silk"), Layer: kicadfiles.LayerFSilkS, Rect: &RectDrawing{Start: kicadfiles.Point{X: -width / 2, Y: -height / 2}, End: kicadfiles.Point{X: width / 2, Y: height / 2}, Width: kicadfiles.MM(0.12)}},
		{UUID: generator.New("root.pcb.footprint."+key+".graphic", "fab"), Layer: kicadfiles.LayerFFab, Rect: &RectDrawing{Start: kicadfiles.Point{X: -width / 2, Y: -height / 2}, End: kicadfiles.Point{X: width / 2, Y: height / 2}, Width: kicadfiles.MM(0.1)}},
		{UUID: generator.New("root.pcb.footprint."+key+".graphic", "courtyard"), Layer: kicadfiles.LayerFCrtYd, Rect: &RectDrawing{Start: kicadfiles.Point{X: -width/2 - kicadfiles.MM(0.25), Y: -height/2 - kicadfiles.MM(0.25)}, End: kicadfiles.Point{X: width/2 + kicadfiles.MM(0.25), Y: height/2 + kicadfiles.MM(0.25)}, Width: kicadfiles.MM(0.05)}},
	}
}
