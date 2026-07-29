package schematic

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFablePhase3ImportedSchematicPresentationSurvivesRoundTrip(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_sch`,
		`  (version 20260306)`,
		`  (generator "eeschema")`,
		`  (generator_version "10.0.3")`,
		`  (uuid "11111111-1111-5111-8111-111111111111")`,
		`  (paper "User" 210 297)`,
		`  (title_block (title "Imported amplifier") (comment 1 "Preserve me"))`,
		`  (lib_symbols)`,
		`  (junction (at 10 10) (diameter 1.2) (color 1 2 3 4) (uuid "22222222-2222-5222-8222-222222222222"))`,
		`  (text "BIAS" (exclude_from_sim no) (at 20 20 45) (effects (font (size 2 1) (thickness 0.2)) (justify left)) (uuid "33333333-3333-5333-8333-333333333333"))`,
		`  (sheet`,
		`    (at 30 30) (size 20 10) (exclude_from_sim no) (in_bom no) (on_board no) (dnp yes)`,
		`    (uuid "44444444-4444-5444-8444-444444444444")`,
		`    (property "Sheetname" "Power" (at 30 27.46 0) (effects (font (size 1.27 1.27))))`,
		`    (property "Sheetfile" "power.kicad_sch" (at 30 42.54 0) (effects (font (size 1.27 1.27))))`,
		`  )`,
		`)`,
	}, "\n")
	source, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if source.Paper.Width != kicadfiles.MM(210) || source.TitleBlock.Title != "Imported amplifier" {
		t.Fatalf("schematic metadata not inventoried: %#v %#v", source.Paper, source.TitleBlock)
	}
	var output bytes.Buffer
	if err := Write(&output, source); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`(paper "User" 210 297)`,
		`(title_block (title "Imported amplifier") (comment 1 "Preserve me"))`,
		`(diameter 1.2)`,
		`(effects (font (size 2 1) (thickness 0.2)) (justify left))`,
		`(in_bom no)`,
		`(on_board no)`,
		`(dnp yes)`,
	} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("round-trip output missing %q:\n%s", fragment, output.String())
		}
	}
	staged, err := Read(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePreservedMutation(source, staged); err != nil {
		t.Fatal(err)
	}
}

func TestFablePhase3SchematicPreservationInventoryRejectsDuplicateSingleton(t *testing.T) {
	file, err := Read([]byte(`(kicad_sch (version 20260306) (generator "eeschema") (uuid "11111111-1111-5111-8111-111111111111") (paper A4) (paper A3))`))
	if err != nil {
		t.Fatal(err)
	}
	if !kicadfiles.HasUnsupportedPreservation(file.Preservation) {
		t.Fatalf("duplicate singleton was not classified unsupported: %#v", file.Preservation)
	}
}
