package project

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestWriteMinimalProject(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, minimalProject())
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	for _, want := range []string{
		"\"board\":",
		"\"boards\": []",
		"\"component_class_settings\": {}",
		"\"cvpcb\": {}",
		"\"erc\": {}",
		"\"libraries\": {}",
		"\"meta\": {\n    \"version\": 1",
		"\"net_settings\":",
		"\"pcbnew\": {}",
		"\"schematic\": {}",
		"\"sheets\": []",
		"\"text_variables\": {}",
		"\"time_domain_parameters\": {}",
		"\"name\": \"Default\"",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
	for _, unwanted := range []string{
		"\"design_id\"",
		"\"generator\"",
		"\"page_settings\"",
	} {
		if strings.Contains(buf.String(), unwanted) {
			t.Fatalf("output includes KiCad-removed key %s:\n%s", unwanted, buf.String())
		}
	}
}

func TestWriteProjectWithTextVariablesUsesStableOrdering(t *testing.T) {
	project := minimalProject()
	project.TextVariables = map[string]string{
		"ZETA":  "last",
		"ALPHA": "first",
	}

	var buf bytes.Buffer
	if err := Write(&buf, project); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if !strings.Contains(buf.String(), "\"ALPHA\": \"first\",\n    \"ZETA\": \"last\"") {
		t.Fatalf("text variables are not stably ordered:\n%s", buf.String())
	}
}

func TestWriteProjectWithNetClasses(t *testing.T) {
	project := minimalProject()
	project.NetClasses[0].Clearance = kicadfiles.MM(0.2)
	project.NetClasses[0].TrackWidth = kicadfiles.MM(0.25)
	project.NetClasses[0].ViaDiameter = kicadfiles.MM(0.8)
	project.NetClasses[0].ViaDrill = kicadfiles.MM(0.4)

	var buf bytes.Buffer
	if err := Write(&buf, project); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	for _, want := range []string{
		"\"name\": \"Default\"",
		"\"clearance\": 0.2",
		"\"track_width\": 0.25",
		"\"via_diameter\": 0.8",
		"\"via_drill\": 0.4",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
}

func TestWriteProjectWithSheets(t *testing.T) {
	project := minimalProject()
	project.Sheets = []Sheet{{UUID: "root-uuid", Name: "Root"}, {UUID: "child-uuid", Name: "Power"}}

	var buf bytes.Buffer
	if err := Write(&buf, project); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	for _, want := range []string{
		"\"sheets\": [",
		"\"root-uuid\"",
		"\"Root\"",
		"\"child-uuid\"",
		"\"Power\"",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
}

func TestWriteTrimsValidatedNames(t *testing.T) {
	project := minimalProject()
	project.PageSettings.Paper.Name = " A4 "
	project.NetClasses[0].Name = " Default "

	var buf bytes.Buffer
	if err := Write(&buf, project); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if strings.Contains(buf.String(), " Default ") || strings.Contains(buf.String(), " A4 ") {
		t.Fatalf("output contains untrimmed names:\n%s", buf.String())
	}
}

func TestValidateRejectsEmptyProjectName(t *testing.T) {
	project := minimalProject()
	project.Name = ""

	err := Validate(project)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "project.name: required") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidPathSeparator(t *testing.T) {
	project := minimalProject()
	project.Name = "bad/name"

	err := Validate(project)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported filename characters") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsReservedWindowsName(t *testing.T) {
	project := minimalProject()
	project.Name = "CON"

	err := Validate(project)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "reserved Windows") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateNetClass(t *testing.T) {
	project := minimalProject()
	project.NetClasses = []NetClass{{Name: "Default"}, {Name: "Default"}}

	err := Validate(project)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate Default") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsMissingDefaultNetClass(t *testing.T) {
	project := minimalProject()
	project.NetClasses = []NetClass{{Name: "Signal"}}

	err := Validate(project)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Default net class required") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidTextVariableKey(t *testing.T) {
	project := minimalProject()
	project.TextVariables = map[string]string{"bad key": "value"}

	err := Validate(project)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidDimensions(t *testing.T) {
	project := minimalProject()
	project.PageSettings.Paper.Width = -1
	project.NetClasses[0].TrackWidth = 0
	project.NetClasses[0].ViaDrill = project.NetClasses[0].ViaDiameter

	err := Validate(project)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"page_settings.width", "track_width", "via_drill"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %s: %v", want, err)
		}
	}
}

func TestWriteValidatesBeforeWriting(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, ProjectFile{})
	var validationErrors kicadfiles.ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("error = %v, want ValidationErrors", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Write emitted output despite validation error: %q", buf.String())
	}
}

func minimalProject() ProjectFile {
	return ProjectFile{
		Name:          "led_indicator",
		DesignID:      kicadfiles.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		FormatVersion: kicadfiles.KiCadFormatV20230121,
		Generator:     "kicadai",
		PageSettings:  PageSettings{Paper: kicadfiles.Paper{Name: "A4"}},
		NetClasses: []NetClass{{
			Name:        "Default",
			Clearance:   kicadfiles.MM(0.2),
			TrackWidth:  kicadfiles.MM(0.25),
			ViaDiameter: kicadfiles.MM(0.8),
			ViaDrill:    kicadfiles.MM(0.4),
		}},
	}
}
