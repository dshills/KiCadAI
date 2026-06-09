package pcb

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestWriteMinimalPCB(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, minimalPCB())
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	want := strings.Join([]string{
		"(kicad_pcb",
		"  (version 20230121)",
		"  (generator \"kicadai\")",
		"  (general)",
		"  (paper \"A4\")",
		"  (layers",
		"    (0 \"F.Cu\" signal)",
		"    (31 \"B.Cu\" signal)",
		"    (36 \"B.SilkS\" user)",
		"    (37 \"F.SilkS\" user)",
		"    (44 \"Edge.Cuts\" user)",
		"  )",
		"  (setup",
		"    (stackup",
		"      (thickness 1.6)",
		"    )",
		"    (solder_mask_min_width 0.0)",
		"    (pad_to_mask_clearance 0.0)",
		"  )",
		"  (net 0 \"\")",
		")",
		"",
	}, "\n")
	if got := buf.String(); got != want {
		t.Fatalf("Write =\n%s\nwant =\n%s", got, want)
	}
}

func TestValidateRejectsMissingGenerator(t *testing.T) {
	board := minimalPCB()
	board.Generator = " "

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pcb.generator") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateNetCode(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}, {Code: 1, Name: "B"}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[1].code") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateNetName(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: "A"}, {Code: 2, Name: "A"}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[1].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsEmptyNonZeroNetName(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 1, Name: " "}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[0].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsNamedNetZero(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 0, Name: "GND"}}

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nets[0].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidLayer(t *testing.T) {
	board := minimalPCB()
	board.Layers[0].Name = "Nope.Cu"

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "layers[0].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidSetup(t *testing.T) {
	tests := []struct {
		name  string
		board PCBFile
		want  string
	}{
		{
			name: "zero thickness",
			board: func() PCBFile {
				board := minimalPCB()
				board.Setup.Stackup.Thickness = 0
				return board
			}(),
			want: "setup.stackup.thickness",
		},
		{
			name: "negative solder mask min width",
			board: func() PCBFile {
				board := minimalPCB()
				board.Setup.SolderMaskMinWidth = -1
				return board
			}(),
			want: "setup.solder_mask_min_width",
		},
		{
			name: "negative pad to mask clearance",
			board: func() PCBFile {
				board := minimalPCB()
				board.Setup.PadToMaskClearance = -1
				return board
			}(),
			want: "setup.pad_to_mask_clearance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.board)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateRejectsDuplicateLayerName(t *testing.T) {
	board := minimalPCB()
	board.Layers[1].Name = board.Layers[0].Name

	err := Validate(board)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "layers[1].name") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteRendersTitleBlock(t *testing.T) {
	board := minimalPCB()
	board.TitleBlock = kicadfiles.TitleBlock{
		Title:    "LED Board",
		Date:     "2026-06-09",
		Revision: "A",
		Company:  "KiCadAI",
		Comments: []string{"generated"},
	}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"(title_block",
		"(title \"LED Board\")",
		"(date \"2026-06-09\")",
		"(rev \"A\")",
		"(company \"KiCadAI\")",
		"(comment 1 \"generated\")",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteSortsNetsByCode(t *testing.T) {
	board := minimalPCB()
	board.Nets = []Net{{Code: 2, Name: "LED_OUT"}, {Code: 1, Name: "GND"}}

	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	output := buf.String()
	first := strings.Index(output, "(net 0 \"\")")
	second := strings.Index(output, "(net 1 \"GND\")")
	third := strings.Index(output, "(net 2 \"LED_OUT\")")
	if !(first < second && second < third) {
		t.Fatalf("nets not ordered by code:\n%s", output)
	}
}

func TestWriteValidatesBeforeRendering(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, PCBFile{})
	var validationErrors kicadfiles.ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("error = %v, want ValidationErrors", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Write emitted output despite validation error: %q", buf.String())
	}
}

func minimalPCB() PCBFile {
	return PCBFile{
		Version:   kicadfiles.KiCadFormatV20230121,
		Generator: "kicadai",
		Paper:     kicadfiles.Paper{Name: "A4"},
		Layers:    DefaultTwoLayerStack(),
		Setup: PCBSetup{
			Stackup: PCBStackup{Thickness: kicadfiles.MM(1.6)},
		},
	}
}
