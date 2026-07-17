package kicadfiles

import (
	"errors"
	"strings"
	"testing"
)

func TestUnitConversion(t *testing.T) {
	tests := []struct {
		name  string
		value IU
		want  string
	}{
		{name: "zero", value: 0, want: "0.0"},
		{name: "one mm", value: MM(1), want: "1.0"},
		{name: "half mm", value: MM(0.5), want: "0.5"},
		{name: "negative", value: MM(-1.25), want: "-1.25"},
		{name: "nanometer precision", value: IU(1_234_567), want: "1.234567"},
		{name: "minimum int64", value: IU(-9_223_372_036_854_775_808), want: "-9223372036854.775808"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ToMMString(test.value); got != test.want {
				t.Fatalf("ToMMString(%d) = %q, want %q", test.value, got, test.want)
			}
		})
	}

	if got := MM(1.2345674); got != IU(1_234_567) {
		t.Fatalf("MM rounded to %d", got)
	}
	if got := Mil(100); got != IU(2_540_000) {
		t.Fatalf("Mil rounded to %d", got)
	}
}

func TestDeterministicIDGenerator(t *testing.T) {
	generator, err := NewDeterministicIDGenerator(UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), "seed")
	if err != nil {
		t.Fatalf("NewDeterministicIDGenerator returned error: %v", err)
	}

	first := generator.New("root.component.r1", "pad.1")
	second := generator.New("root.component.r1", "pad.1")
	other := generator.New("root.component.r1", "pad.2")

	if first != second {
		t.Fatalf("deterministic IDs differ: %s != %s", first, second)
	}
	if first == other {
		t.Fatalf("different logical paths produced same UUID %s", first)
	}
	if !first.Valid() {
		t.Fatalf("generated UUID is invalid: %s", first)
	}
	if got := string(first)[14]; got != '5' {
		t.Fatalf("generated UUID version = %q, want 5 in %s", got, first)
	}
	if got := string(first)[19]; !strings.ContainsRune("89ab", rune(got)) {
		t.Fatalf("generated UUID variant = %q, want RFC 4122 variant in %s", got, first)
	}
}

func TestDeterministicIDGeneratorNormalizesUnicode(t *testing.T) {
	generator, err := NewDeterministicIDGenerator(UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), "seed")
	if err != nil {
		t.Fatalf("NewDeterministicIDGenerator returned error: %v", err)
	}

	precomposed := generator.New("root.component.café")
	decomposed := generator.New("root.component.cafe\u0301")

	if precomposed != decomposed {
		t.Fatalf("normalized IDs differ: %s != %s", precomposed, decomposed)
	}
}

func TestDeterministicIDGeneratorSeparatesPathComponents(t *testing.T) {
	generator, err := NewDeterministicIDGenerator(UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), "seed")
	if err != nil {
		t.Fatalf("NewDeterministicIDGenerator returned error: %v", err)
	}

	first := generator.New("a:b", "c")
	second := generator.New("a", "b:c")

	if first == second {
		t.Fatalf("component delimiter collision produced %s", first)
	}
}

func TestRejectsInvalidDesignID(t *testing.T) {
	_, err := NewDeterministicIDGenerator(UUID("not-a-uuid"), "seed")
	if !errors.Is(err, ErrInvalidUUID) {
		t.Fatalf("error = %v, want ErrInvalidUUID", err)
	}
}

func TestUUIDValidation(t *testing.T) {
	if !UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8").Valid() {
		t.Fatal("valid UUID rejected")
	}
	if UUID("6ba7b8109dad11d180b400c04fd430c8").Valid() {
		t.Fatal("UUID without dashes accepted")
	}
}

func TestBoardLayerValidation(t *testing.T) {
	valid := []BoardLayer{
		LayerFCu, LayerBCu, BoardLayer("In1.Cu"), BoardLayer("In30.Cu"),
		LayerFAdhes, LayerBAdhes, LayerFPaste, LayerBPaste,
		LayerFSilkS, LayerBSilkS, LayerFMask, LayerBMask,
		LayerFCrtYd, LayerBCrtYd, LayerFFab, LayerBFab,
		LayerEdge, LayerMargin, LayerDwgs, LayerCmts, LayerEco1, LayerEco2,
		LayerUserDwgs, LayerUserCmts, LayerAllCu, LayerAllMask, LayerAll,
	}
	for _, layer := range valid {
		if !IsValidBoardLayer(layer) {
			t.Fatalf("valid layer rejected: %s", layer)
		}
	}
	invalid := []BoardLayer{BoardLayer("Inner.Cu"), BoardLayer("In0.Cu"), BoardLayer("In31.Cu")}
	for _, layer := range invalid {
		if IsValidBoardLayer(layer) {
			t.Fatalf("invalid layer accepted: %s", layer)
		}
	}
}

func TestCanonicalCopperLayer(t *testing.T) {
	tests := map[string]BoardLayer{
		"f.cu":    LayerFCu,
		" B.CU ":  LayerBCu,
		"IN1.CU":  BoardLayer("In1.Cu"),
		"in30.cu": BoardLayer("In30.Cu"),
		"In31.Cu": BoardLayer("In31.Cu"),
	}
	for input, want := range tests {
		if got := CanonicalCopperLayer(input); got != want {
			t.Errorf("CanonicalCopperLayer(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidationErrors(t *testing.T) {
	errs := ValidationErrors{
		{File: "board.kicad_pcb", Section: "footprint", Field: "reference", Message: "required"},
		{Field: "generator", Message: "required"},
	}

	if errs.Err() == nil {
		t.Fatal("Err returned nil for non-empty errors")
	}
	want := "board.kicad_pcb.footprint.reference: required; generator: required"
	if got := errs.Error(); got != want {
		t.Fatalf("Error = %q, want %q", got, want)
	}
	if got := (ValidationErrors{}).Err(); got != nil {
		t.Fatalf("empty Err = %v, want nil", got)
	}
}
