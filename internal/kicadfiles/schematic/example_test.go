package schematic

import (
	"bytes"
	"fmt"

	"kicadai/internal/kicadfiles"
)

func ExampleWrite() {
	start := kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}
	end := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}
	schematic := SchematicFile{
		Version:          kicadfiles.KiCadFormatV20260306,
		Generator:        "eeschema",
		GeneratorVersion: "10.0",
		UUID:             kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Wires: []Wire{
			NewWire(kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), start, end),
		},
		Labels: []Label{
			NewLabel(kicadfiles.UUID("33333333-3333-4333-8333-333333333333"), "LED_OUT", LabelLocal, end),
		},
		SheetInstances: []SheetInstance{{Path: "/", Page: "1"}},
	}
	if err := Validate(schematic); err != nil {
		panic(err)
	}

	var out bytes.Buffer
	if err := Write(&out, schematic); err != nil {
		panic(err)
	}
	fmt.Println(bytes.Contains(out.Bytes(), []byte("(wire")))
	// Output: true
}
