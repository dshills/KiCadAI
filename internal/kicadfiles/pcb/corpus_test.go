package pcb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanCorpusCountsPCBObjects(t *testing.T) {
	root := t.TempDir()
	board := []byte(`(kicad_pcb
  (footprint "Test:One")
  (segment (start 0 0) (end 1 0)) (via (at 1 0))
  (gr_text "(not_an_object)" (at 1 1))
  ; (comment_object)
  (via (at 1 0))
  (zone (net 1))
)`)
	if err := os.WriteFile(filepath.Join(root, "board.kicad_pcb"), board, 0o644); err != nil {
		t.Fatalf("write board: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignore.kicad_sch"), board, 0o644); err != nil {
		t.Fatalf("write schematic: %v", err)
	}
	footprint := []byte(`(footprint "Test:Mod" (pad "1" smd rect (layers "F.Cu")))`)
	if err := os.WriteFile(filepath.Join(root, "one.kicad_mod"), footprint, 0o644); err != nil {
		t.Fatalf("write footprint: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "two.KICAD_MOD"), footprint, 0o644); err != nil {
		t.Fatalf("write uppercase footprint: %v", err)
	}

	report, err := ScanCorpus(root)
	if err != nil {
		t.Fatalf("ScanCorpus returned error: %v", err)
	}
	if report.Files != 3 {
		t.Fatalf("Files = %d, want 3", report.Files)
	}
	for object, want := range map[string]int{"kicad_pcb": 1, "footprint": 3, "pad": 2, "segment": 1, "via": 2, "zone": 1, "gr_text": 1, "not_an_object": 0, "comment_object": 0} {
		if got := report.ObjectCount[object]; got != want {
			t.Errorf("ObjectCount[%s] = %d, want %d", object, got, want)
		}
	}
}

func TestScanCorpusReportsPCBCompatibilityDimensions(t *testing.T) {
	root := t.TempDir()
	board := []byte(`(kicad_pcb
  (version 20260206)
  (layers
    (0 "F.Cu" signal)
    (2 "B.Cu" signal)
  )
  (footprint "Test:One"
    (layer "F.Cu")
    (fp_text reference "R1" (at 0 0))
    (pad "1" smd roundrect (at 0 0) (size 1 1) (layers "F.Cu" "F.Mask"))
    (pad "2" thru_hole circle (at 2 0) (size 1 1) (drill 0.5) (layers "*.Cu" "*.Mask"))
    (teardrops (best_length_ratio 0.5))
  )
  (zone (net 1) (layer "F.Cu")
    (polygon (pts (xy 0 0) (xy 1 0) (xy 1 1)))
    (filled_polygon (layer "F.Cu") (pts (xy 0 0) (xy 1 0) (xy 1 1)))
  )
  (future_widget (layer "User.\"Quoted\""))
  (future_note (layer "User\nEscaped"))
  (future_after_child (child yes) trailing_scalar "quoted_scalar" *.Cu 123.456 aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa)
  (future_comment_scalar (child yes) comment_scalar; ignored_scalar
  )
)`)
	if err := os.WriteFile(filepath.Join(root, "board.kicad_pcb"), board, 0o644); err != nil {
		t.Fatalf("write board: %v", err)
	}

	report, err := ScanCorpus(root)
	if err != nil {
		t.Fatalf("ScanCorpus returned error: %v", err)
	}
	assertCorpusCount(t, report.TopLevelObjects, "version", 1)
	assertCorpusCount(t, report.TopLevelObjects, "footprint", 1)
	assertCorpusCount(t, report.FootprintChildTypes, "pad", 2)
	assertCorpusCount(t, report.FootprintChildTypes, "teardrops", 1)
	assertCorpusCount(t, report.PadTypes, "smd", 1)
	assertCorpusCount(t, report.PadTypes, "thru_hole", 1)
	assertCorpusCount(t, report.PadShapes, "roundrect", 1)
	assertCorpusCount(t, report.PadShapes, "circle", 1)
	assertCorpusCount(t, report.LayerUsage, "F.Cu", 5)
	assertCorpusCount(t, report.LayerUsage, "F.Mask", 1)
	assertCorpusCount(t, report.LayerUsage, "*.Cu", 1)
	assertCorpusCount(t, report.ZoneLayers, "F.Cu", 2)
	assertCorpusCount(t, report.PreservationOnly, "teardrops", 1)
	assertCorpusCount(t, report.LayerUsage, `User."Quoted"`, 1)
	assertCorpusCount(t, report.LayerUsage, "User\nEscaped", 1)
	assertCorpusCount(t, report.ScalarCount, "trailing_scalar", 1)
	assertCorpusCount(t, report.ScalarCount, "quoted_scalar", 1)
	assertCorpusCount(t, report.ScalarCount, "*.Cu", 2)
	assertCorpusCount(t, report.ScalarCount, "123.456", 0)
	assertCorpusCount(t, report.ScalarCount, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", 0)
	assertCorpusCount(t, report.ScalarCount, "comment_scalar", 1)
	assertCorpusCount(t, report.ScalarCount, "ignored_scalar", 0)
	assertCorpusCount(t, report.UnsupportedObjects, "future_widget", 1)
	assertCorpusCount(t, report.UnsupportedObjects, "future_after_child", 1)
}

func TestScanCorpusContinuesAfterFileError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "good.kicad_pcb"), []byte(`(kicad_pcb (segment (start 0 0) (end 1 0)))`), 0o644); err != nil {
		t.Fatalf("write good board: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.kicad_pcb"), []byte(`(kicad_pcb (segment`), 0o644); err != nil {
		t.Fatalf("write bad board: %v", err)
	}

	report, err := ScanCorpus(root)
	if err == nil {
		t.Fatal("expected error")
	}
	if report.Files != 1 {
		t.Fatalf("Files = %d, want 1", report.Files)
	}
	assertCorpusCount(t, report.ObjectCount, "segment", 1)
}

func TestScanCorpusRejectsUnterminatedQuotedString(t *testing.T) {
	_, err := scanPCBFile(strings.NewReader(`(kicad_pcb (gr_text "unterminated)`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestScanCorpusRejectsUnbalancedSExpression(t *testing.T) {
	_, err := scanPCBFile(strings.NewReader(`(kicad_pcb (segment (start 0 0) (end 1 0))`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestScanCorpusExternalKiCadDemos(t *testing.T) {
	if os.Getenv("KICADAI_RUN_KICAD_DEMO_CORPUS") != "1" {
		t.Skip("set KICADAI_RUN_KICAD_DEMO_CORPUS=1 to scan the KiCad demo corpus")
	}
	root := os.Getenv("KICADAI_KICAD_DEMO_CORPUS")
	if root == "" {
		t.Skip("set KICADAI_KICAD_DEMO_CORPUS to the KiCad demo corpus directory")
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("KiCad demo corpus not available at %s: %v", root, err)
	}

	report, err := ScanCorpus(root)
	if err != nil {
		t.Fatalf("ScanCorpus returned error: %v", err)
	}
	if report.Files == 0 {
		t.Fatalf("Files = 0, want at least one KiCad PCB in %s", root)
	}
	for _, object := range []string{"kicad_pcb", "footprint", "pad"} {
		if report.ObjectCount[object] == 0 {
			t.Fatalf("ObjectCount[%s] = 0 in %s", object, root)
		}
	}
}

func assertCorpusCount(t *testing.T, counts map[string]int, key string, want int) {
	t.Helper()
	if got := counts[key]; got != want {
		t.Fatalf("count[%s] = %d, want %d", key, got, want)
	}
}
