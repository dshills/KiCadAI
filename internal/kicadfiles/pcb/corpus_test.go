package pcb

import (
	"os"
	"path/filepath"
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

	report, err := ScanCorpus(root)
	if err != nil {
		t.Fatalf("ScanCorpus returned error: %v", err)
	}
	if report.Files != 1 {
		t.Fatalf("Files = %d, want 1", report.Files)
	}
	for object, want := range map[string]int{"kicad_pcb": 1, "footprint": 1, "segment": 1, "via": 2, "zone": 1, "gr_text": 1, "not_an_object": 0, "comment_object": 0} {
		if got := report.ObjectCount[object]; got != want {
			t.Errorf("ObjectCount[%s] = %d, want %d", object, got, want)
		}
	}
}
