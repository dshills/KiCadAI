package inspect

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

func TestProjectSummarizesExpectedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "demo.kicad_pro"), "{}")
	writeFile(t, filepath.Join(root, "demo.kicad_sch"), `(kicad_sch (version 20260306) (generator "kicadai") (symbol (lib_id "Device:R")) (wire))`)
	writeFile(t, filepath.Join(root, "demo.kicad_pcb"), `(kicad_pcb (net 0 "") (layers (0 "F.Cu" signal)) (gr_rect (layer "Edge.Cuts")) (footprint "Device:R" (pad "1" smd rect (layers "F.Cu"))))`)

	summary, err := Project(root)
	if err != nil {
		t.Fatalf("Project returned error: %v", err)
	}
	if summary.Name != "demo" || len(summary.Files) != 3 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.Schematic == nil || summary.Schematic.SymbolCount != 1 || summary.Schematic.WireCount != 1 {
		t.Fatalf("unexpected schematic summary: %#v", summary.Schematic)
	}
	if summary.PCB == nil || summary.PCB.FootprintCount != 1 || summary.PCB.PadCount != 1 || !summary.PCB.HasBoardOutline {
		t.Fatalf("unexpected PCB summary: %#v", summary.PCB)
	}
}

func TestProjectReportsMissingFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "demo.kicad_pro"), "{}")

	summary, err := Project(root)
	if err != nil {
		t.Fatalf("Project returned error: %v", err)
	}
	if len(summary.Issues) != 2 {
		t.Fatalf("issues = %#v, want missing schematic and PCB", summary.Issues)
	}
	for _, issue := range summary.Issues {
		if issue.Code != reports.CodeMissingFile || issue.Severity != reports.SeverityWarning {
			t.Fatalf("unexpected issue: %#v", issue)
		}
	}
}

func TestProjectDiscoversProjectFileName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "container")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "actual_name.kicad_pro"), "{}")
	writeFile(t, filepath.Join(root, "actual_name.kicad_sch"), `(kicad_sch (version 20260306) (generator "kicadai"))`)
	writeFile(t, filepath.Join(root, "actual_name.kicad_pcb"), `(kicad_pcb (gr_line (layer "Edge.Cuts")))`)

	summary, err := Project(root)
	if err != nil {
		t.Fatalf("Project returned error: %v", err)
	}
	if summary.Name != "actual_name" {
		t.Fatalf("project name = %q, want actual_name", summary.Name)
	}
	if summary.Schematic == nil || summary.PCB == nil {
		t.Fatalf("expected discovered schematic and PCB, got %#v", summary)
	}
}

func TestProjectReportsManifestStatus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "demo.kicad_pro")
	writeFile(t, projectPath, "{}")
	writeFile(t, filepath.Join(root, "demo.kicad_sch"), `(kicad_sch)`)
	writeFile(t, filepath.Join(root, "demo.kicad_pcb"), `(kicad_pcb)`)
	if _, err := manifest.Write(root, manifest.Manifest{ProjectName: "demo", Artifacts: []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: projectPath}}}); err != nil {
		t.Fatal(err)
	}

	summary, err := Project(root)
	if err != nil {
		t.Fatalf("Project returned error: %v", err)
	}
	if !summary.Manifest.Present || summary.Manifest.Stale {
		t.Fatalf("unexpected manifest status: %#v", summary.Manifest)
	}
}

func TestPCBSummaryUsesReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeFile(t, path, `(kicad_pcb
  (net 0 "")
  (footprint "Test:One"
    (pad "1" smd roundrect (at 0 0) (layers "F.Cu" "F.Mask"))
  )
  (segment (start 0 0) (end 1 0) (layer "F.Cu"))
  (via (at 1 0))
  (zone (layer "F.Cu"))
  (future_widget)
)`)

	summary, err := PCB(path)
	if err != nil {
		t.Fatalf("PCB returned error: %v", err)
	}
	if summary.FootprintCount != 1 || summary.PadCount != 1 || summary.TrackCount != 1 || summary.ViaCount != 1 || summary.ZoneCount != 1 {
		t.Fatalf("unexpected PCB counts: %#v", summary)
	}
	if len(summary.Unsupported) != 1 || summary.Unsupported[0].Kind != "future_widget" {
		t.Fatalf("unexpected unsupported nodes: %#v", summary.Unsupported)
	}
	if len(summary.Issues) != 1 || summary.Issues[0].Code != reports.CodeMissingBoardOutline {
		t.Fatalf("expected missing board outline warning, got %#v", summary.Issues)
	}
}

func TestSchematicSummaryUsesReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	writeFile(t, path, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (symbol (lib_id "Device:R"))
  (wire)
  (label "OUT")
  (global_label "VCC")
  (junction)
  (no_connect)
  (sheet (name "Child"))
)`)

	summary, err := Schematic(path)
	if err != nil {
		t.Fatalf("Schematic returned error: %v", err)
	}
	if summary.FormatVersion != "20260306" || summary.Generator != "kicadai" {
		t.Fatalf("metadata = version %q generator %q", summary.FormatVersion, summary.Generator)
	}
	if summary.SymbolCount != 1 || summary.WireCount != 1 || summary.LabelCount != 2 || summary.JunctionCount != 1 || summary.NoConnectCount != 1 || summary.SheetCount != 1 {
		t.Fatalf("unexpected schematic counts: %#v", summary)
	}
	if summary.InspectionDepth != "structured" || len(summary.Issues) != 0 {
		t.Fatalf("expected structured summary, got %#v", summary)
	}
}

func TestSchematicMetadataIgnoresCommentsAndStrings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	writeFile(t, path, `(kicad_sch
  ; (version 1)
  (version 20260306)
  (generator "KiCad \"writer\"")
  (symbol (property "note" "(wire) (generator ignored)"))
  (wire)
)`)

	summary, err := Schematic(path)
	if err != nil {
		t.Fatalf("Schematic returned error: %v", err)
	}
	if summary.FormatVersion != "20260306" || summary.Generator != `KiCad "writer"` {
		t.Fatalf("metadata = version %q generator %q", summary.FormatVersion, summary.Generator)
	}
	if summary.SymbolCount != 1 || summary.WireCount != 1 {
		t.Fatalf("unexpected counts: %#v", summary.ObjectCounts)
	}
}

func TestSchematicMetadataTreatsSemicolonAsAtomDelimiter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	writeFile(t, path, `(kicad_sch
  (version 20260306; comment without leading whitespace
  )
  (generator kicadai; comment without leading whitespace
  )
)`)

	summary, err := Schematic(path)
	if err != nil {
		t.Fatalf("Schematic returned error: %v", err)
	}
	if summary.FormatVersion != "20260306" || summary.Generator != "kicadai" {
		t.Fatalf("metadata = version %q generator %q", summary.FormatVersion, summary.Generator)
	}
}

func TestSchematicMetadataSkipsCommentsBeforeScalar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	writeFile(t, path, `(kicad_sch
  (version ; comment before value
    20260306)
  (generator ; comment before value
    "kicadai")
)`)

	summary, err := Schematic(path)
	if err != nil {
		t.Fatalf("Schematic returned error: %v", err)
	}
	if summary.FormatVersion != "20260306" || summary.Generator != "kicadai" {
		t.Fatalf("metadata = version %q generator %q", summary.FormatVersion, summary.Generator)
	}
}

func TestSchematicSummaryReportsUnsupportedAndTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many.kicad_sch")
	var builder strings.Builder
	builder.WriteString(`(kicad_sch (version 20260306) (generator "kicadai")`)
	for i := 0; i < inspectSampleLimit+1; i++ {
		builder.WriteString(` (symbol (lib_id "Device:R") (property "Reference" "R`)
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString(`"))`)
	}
	builder.WriteString(` (rule_area (uuid "22222222-2222-5222-8222-222222222222"))`)
	builder.WriteString(`)`)
	writeFile(t, path, builder.String())

	summary, err := Schematic(path)
	if err != nil {
		t.Fatalf("Schematic returned error: %v", err)
	}
	if !summary.Truncated || len(summary.Symbols) != inspectSampleLimit {
		t.Fatalf("expected truncated symbols, got %#v", summary)
	}
	if len(summary.Unsupported) != 1 || summary.Unsupported[0].Kind != "rule_area" {
		t.Fatalf("expected unsupported rule_area, got %#v", summary.Unsupported)
	}
}

func TestPCBSummaryUsesReaderSamplesAndTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many.kicad_pcb")
	var builder strings.Builder
	builder.WriteString(`(kicad_pcb (version 20260206) (generator "pcbnew") (layers (0 "F.Cu" signal) (25 "Edge.Cuts" user)) (setup)`)
	for i := 0; i < inspectSampleLimit+1; i++ {
		builder.WriteString(` (footprint "Test:One" (property "Reference" "J`)
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString(`") (pad "1" smd rect (at 0 0) (layers "F.Cu") (net "NET`)
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString(`")))`)
	}
	builder.WriteString(` (gr_line (start 0 0) (end 1 0) (layer "Edge.Cuts"))`)
	builder.WriteString(`)`)
	writeFile(t, path, builder.String())

	summary, err := PCB(path)
	if err != nil {
		t.Fatalf("PCB returned error: %v", err)
	}
	if !summary.Truncated || len(summary.Footprints) != inspectSampleLimit || len(summary.Nets) != inspectSampleLimit {
		t.Fatalf("expected truncated PCB samples, got %#v", summary)
	}
	if !summary.HasBoardOutline {
		t.Fatalf("expected board outline: %#v", summary)
	}
}

func TestPCBSummarySamplesZoneNets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zone.kicad_pcb")
	writeFile(t, path, strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (layers (0 "F.Cu" signal) (25 "Edge.Cuts" user))`,
		`  (setup)`,
		`  (zone (net_name "GND") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n"))
	summary, err := PCB(path)
	if err != nil {
		t.Fatalf("PCB returned error: %v", err)
	}
	if len(summary.Nets) != 1 || summary.Nets[0] != "GND" {
		t.Fatalf("expected zone net sample, got %#v", summary.Nets)
	}
}

func TestSchematicScanHandlesLeadingWhitespaceAndNestedParens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	writeFile(t, path, `(kicad_sch
  ( version 20260306)
  ( generator "kicadai")
  ( symbol (lib_id "Device:R"))
  ((ignored_nested))
)`)

	summary, err := Schematic(path)
	if err != nil {
		t.Fatalf("Schematic returned error: %v", err)
	}
	if summary.FormatVersion != "20260306" || summary.Generator != "kicadai" {
		t.Fatalf("metadata = version %q generator %q", summary.FormatVersion, summary.Generator)
	}
	if summary.SymbolCount != 1 {
		t.Fatalf("symbol count = %d, want 1", summary.SymbolCount)
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
