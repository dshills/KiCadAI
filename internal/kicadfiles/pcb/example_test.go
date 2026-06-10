package pcb_test

import (
	"bytes"
	"fmt"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/pcb"
)

func ExampleWrite() {
	nets := pcb.NewNetRegistry()
	gnd := nets.EnsureNet("GND").Code

	board := pcb.PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "pcbnew",
		GeneratorVersion: "10.0",
		General:          pcb.DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           pcb.DefaultTwoLayerStack(),
		Setup:            pcb.DefaultSetup(),
		Nets:             nets.Nets(),
		Drawings: []pcb.Drawing{{
			UUID:  kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
			Layer: kicadfiles.LayerEdge,
			Rect: &pcb.RectDrawing{
				Start: kicadfiles.Point{X: kicadfiles.MM(0), Y: kicadfiles.MM(0)},
				End:   kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
				Width: kicadfiles.MM(0.1),
			},
		}},
		Zones: []pcb.Zone{{
			UUID:    kicadfiles.UUID("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
			NetCode: gnd,
			Layers:  []kicadfiles.BoardLayer{kicadfiles.LayerFCu},
			Polygons: [][]kicadfiles.Point{{
				{X: kicadfiles.MM(1), Y: kicadfiles.MM(1)},
				{X: kicadfiles.MM(19), Y: kicadfiles.MM(1)},
				{X: kicadfiles.MM(19), Y: kicadfiles.MM(9)},
				{X: kicadfiles.MM(1), Y: kicadfiles.MM(9)},
			}},
		}},
		RequireClosedOutline: true,
	}

	var out bytes.Buffer
	if err := pcb.Write(&out, board); err != nil {
		panic(err)
	}
	fmt.Println(bytes.Contains(out.Bytes(), []byte(`(kicad_pcb`)))
	fmt.Println(bytes.Contains(out.Bytes(), []byte(`(zone`)))
	// Output:
	// true
	// true
}
