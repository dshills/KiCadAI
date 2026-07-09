package designapi

import (
	"path/filepath"
	"testing"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
)

func TestBuilderWritesGeneratedSchematicHierarchy(t *testing.T) {
	builder, err := New(Options{
		Name:     "hierarchy_demo",
		DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:     "hierarchy_demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []struct {
		ref string
		x   float64
	}{
		{ref: "R1", x: 30},
		{ref: "R2", x: 300},
	} {
		if _, err := builder.AddSymbol(SymbolOptions{
			Reference: symbol.ref,
			Role:      "resistor",
			Value:     "10k",
			LibraryID: "Device:R",
			Position:  kicadfiles.Point{X: kicadfiles.MM(symbol.x), Y: kicadfiles.MM(50)},
			Pins: []PinSpec{
				{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
				{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := builder.Connect(Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, "LONG_NET"); err != nil {
		t.Fatal(err)
	}
	if err := builder.SetSchematicHierarchy(SchematicHierarchy{
		Sheets: []SchematicSheet{
			{ID: "left", Name: "Left", Filename: "sch/left.kicad_sch", References: []string{"R1"}},
			{ID: "right", Name: "Right", Filename: "sch/right.kicad_sch", References: []string{"R2"}},
		},
		CrossSheetNets: []SchematicCrossSheetNet{{
			Name:      "LONG_NET",
			Endpoints: []Endpoint{{Reference: "R1", Pin: "2"}, {Reference: "R2", Pin: "1"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "hierarchy_demo")
	if _, err := builder.WriteSchematicProject(root, kicaddesign.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	read, err := kicaddesign.ReadProjectDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if read.Schematic == nil || len(read.Schematic.Sheets) != 2 {
		t.Fatalf("root sheets = %#v", read.Schematic)
	}
	if len(read.SheetFiles) != 2 {
		t.Fatalf("child sheets = %#v", read.SheetFiles)
	}
	for _, child := range read.SheetFiles {
		if len(child.Symbols) != 1 {
			t.Fatalf("child %s symbols = %#v", child.Filename, child.Symbols)
		}
		globalLabels := 0
		for _, label := range child.Labels {
			if label.Text == "LONG_NET" && label.Kind == schematic.LabelGlobal {
				globalLabels++
			}
		}
		if globalLabels != 1 {
			t.Fatalf("child %s labels = %#v", child.Filename, child.Labels)
		}
	}
}
