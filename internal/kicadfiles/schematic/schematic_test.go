package schematic

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestWriteMinimalSchematic(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, minimalSchematic())
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	want := strings.Join([]string{
		"(kicad_sch",
		"  (version 20230121)",
		"  (generator \"kicadai\")",
		"  (uuid \"6ba7b810-9dad-11d1-80b4-00c04fd430c8\")",
		"  (paper \"A4\")",
		")",
		"",
	}, "\n")
	if got := buf.String(); got != want {
		t.Fatalf("Write =\n%s\nwant =\n%s", got, want)
	}
}

func TestWriteTitleBlockEscapesStrings(t *testing.T) {
	schematic := minimalSchematic()
	schematic.TitleBlock = kicadfiles.TitleBlock{
		Title:    "LED \"Demo\"",
		Date:     "2026-06-09",
		Revision: "A",
		Company:  "KiCadAI",
		Comments: []string{"line\nbreak"},
	}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	for _, want := range []string{
		"(title \"LED \\\"Demo\\\"\")",
		"(comment 1 \"line\\nbreak\")",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
}

func TestValidateRejectsMissingUUID(t *testing.T) {
	schematic := minimalSchematic()
	schematic.UUID = ""

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.uuid") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsMissingGenerator(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Generator = " "

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.generator") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteValidatesBeforeRendering(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, SchematicFile{})
	var validationErrors kicadfiles.ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("error = %v, want ValidationErrors", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Write emitted output despite validation error: %q", buf.String())
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	var first bytes.Buffer
	var second bytes.Buffer
	schematic := minimalSchematic()

	if err := Write(&first, schematic); err != nil {
		t.Fatalf("first Write returned error: %v", err)
	}
	if err := Write(&second, schematic); err != nil {
		t.Fatalf("second Write returned error: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("outputs differ:\n%s\n---\n%s", first.String(), second.String())
	}
}

func minimalSchematic() SchematicFile {
	return SchematicFile{
		Version:   kicadfiles.KiCadFormatV20230121,
		Generator: "kicadai",
		UUID:      kicadfiles.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		Paper:     kicadfiles.Paper{Name: "A4"},
	}
}
