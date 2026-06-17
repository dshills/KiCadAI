package writercorrectness

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSchematicsSkipsWithoutSchematic(t *testing.T) {
	_, checks := CheckSchematics(Target{})
	if len(checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(checks))
	}
	if checks[0].Status != CheckSkipped || checks[1].Status != CheckSkipped {
		t.Fatalf("statuses = %#v", checks)
	}
}

func TestCheckSchematicsReportsDuplicateReferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	writeFile(t, path, schematicWithBody(`
  (symbol (lib_id "Device:R") (at 10 10 0)
    (property "Reference" "R1" (at 10 10 0))
    (property "Value" "1k" (at 10 12 0))
    (property "Footprint" "Resistor_SMD:R_0603" (at 10 14 0) hide)
  )
  (symbol (lib_id "Device:R") (at 20 10 0)
    (property "Reference" "R1" (at 20 10 0))
    (property "Value" "2k" (at 20 12 0))
    (property "Footprint" "Resistor_SMD:R_0603" (at 20 14 0) hide)
  )
`))

	_, checks := CheckSchematics(Target{SchematicFiles: []string{path}})
	assertCheckIssueContains(t, checks, "duplicate schematic reference")
}

func TestCheckSchematicsReportsMissingFootprint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	writeFile(t, path, schematicWithBody(`
  (symbol (lib_id "Device:R") (at 10 10 0)
    (property "Reference" "R1" (at 10 10 0))
    (property "Value" "1k" (at 10 12 0))
  )
`))

	snapshot, checks := CheckSchematics(Target{SchematicFiles: []string{path}})
	if len(snapshot.MissingFootprints) != 1 || snapshot.MissingFootprints[0] != "R1" {
		t.Fatalf("missing footprints = %#v", snapshot.MissingFootprints)
	}
	assertCheckIssueContains(t, checks, "no footprint assignment")
}

func TestCheckSchematicsIgnoresPowerFootprint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	writeFile(t, path, schematicWithBody(`
  (symbol (lib_id "power:GND") (at 10 10 0)
    (property "Reference" "#PWR01" (at 10 10 0))
    (property "Value" "GND" (at 10 12 0))
  )
`))

	_, checks := CheckSchematics(Target{SchematicFiles: []string{path}})
	for _, check := range checks {
		if check.Status == CheckFail {
			t.Fatalf("power symbol should not fail footprint check: %#v", checks)
		}
	}
}

func TestCheckSchematicsWarnsForUnattachedLabel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	writeFile(t, path, schematicWithBody(`
  (wire (pts (xy 0 0) (xy 10 0)))
  (label "NET_A" (at 20 0 0))
`))

	_, checks := CheckSchematics(Target{SchematicFiles: []string{path}})
	assertCheckIssueContains(t, checks, "not attached")
}

func TestCheckSchematicsAllowsLabelOnWireSegment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	writeFile(t, path, schematicWithBody(`
  (wire (pts (xy 0 0) (xy 10 0)))
  (label "NET_A" (at 5 0 0))
`))

	_, checks := CheckSchematics(Target{SchematicFiles: []string{path}})
	for _, check := range checks {
		for _, issue := range check.Issues {
			if strings.Contains(issue.Message, "not attached") {
				t.Fatalf("label on segment reported as unattached: %#v", checks)
			}
		}
	}
}

func TestCheckSchematicsMatchesSheetPinToHierarchicalLabel(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root.kicad_sch")
	child := filepath.Join(dir, "child.kicad_sch")
	writeFile(t, root, schematicWithBody(`
  (sheet (at 10 10) (size 20 20)
    (property "Sheetname" "Child" (at 10 10 0))
    (property "Sheetfile" "child.kicad_sch" (at 10 15 0))
    (pin "SIG" input (at 10 20 0))
  )
`))
	writeFile(t, child, schematicWithBody(`
  (hierarchical_label "SIG" (at 5 5 0))
`))

	_, checks := CheckSchematics(Target{SchematicFiles: []string{root, child}})
	for _, check := range checks {
		if check.Status == CheckFail {
			t.Fatalf("hierarchical label match should pass: %#v", checks)
		}
	}
}

func TestCheckSchematicsReportsParseFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.kicad_sch")
	writeFile(t, path, `(not_a_schematic)`)

	_, checks := CheckSchematics(Target{SchematicFiles: []string{path}})
	assertCheckIssueContains(t, checks, "expected kicad_sch root")
}

func assertCheckIssueContains(t *testing.T, checks []CheckResult, want string) {
	t.Helper()
	for _, check := range checks {
		for _, issue := range check.Issues {
			if strings.Contains(issue.Message, want) {
				return
			}
		}
	}
	t.Fatalf("missing issue containing %q in %#v", want, checks)
}

func schematicWithBody(body string) string {
	return `(kicad_sch
  (version 20260306)
  (generator "kicadai-test")
  (uuid "00000000-0000-0000-0000-000000000001")
  (paper "A4")
` + body + `
)`
}
