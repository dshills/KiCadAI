package roundtrip

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarizePCBCountsTopLevelSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeTestFile(t, path, `(kicad_pcb
  (version 20260206)
  (generator "pcbnew")
  (layers
    (0 "F.Cu" signal)
  )
  (net 0 "")
  (net 1 "GND")
  (footprint "Device:R")
  (gr_line)
  (segment)
  (via)
  (zone)
)
`)

	summary, err := SummarizePCB(path)
	if err != nil {
		t.Fatalf("SummarizePCB returned error: %v", err)
	}
	for key, want := range map[string]int{
		"version":         1,
		"generator":       1,
		"layers":          1,
		"net":             2,
		"footprint":       1,
		"board_graphics":  1,
		"tracks_and_vias": 2,
		"zone":            1,
	} {
		if got := summary.Sections[key]; got != want {
			t.Fatalf("Sections[%q] = %d, want %d", key, got, want)
		}
	}
}

func TestSummaryStringIsDeterministic(t *testing.T) {
	summary := Summary{Sections: map[string]int{"zone": 1, "net": 2}}

	if got := summary.String(); got != "net=2\nzone=1\n" {
		t.Fatalf("String = %q", got)
	}
}

func TestSummarizePCBSkipsStrings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeTestFile(t, path, `(kicad_pcb
  (gr_text "(segment)")
)
`)

	summary, err := SummarizePCB(path)
	if err != nil {
		t.Fatalf("SummarizePCB returned error: %v", err)
	}
	if strings.Contains(summary.String(), "segment") {
		t.Fatalf("summary counted string contents:\n%s", summary.String())
	}
}

func TestSummarizePCBSkipsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeTestFile(t, path, `(kicad_pcb
  ; (segment)
  (net 0 "")
)
`)

	summary, err := SummarizePCB(path)
	if err != nil {
		t.Fatalf("SummarizePCB returned error: %v", err)
	}
	if summary.Sections["tracks_and_vias"] != 0 {
		t.Fatalf("summary counted comment contents:\n%s", summary.String())
	}
}

func TestSummarizePCBHandlesCommentAfterAtom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeTestFile(t, path, `(kicad_pcb
  (segment;comment
  )
)
`)

	summary, err := SummarizePCB(path)
	if err != nil {
		t.Fatalf("SummarizePCB returned error: %v", err)
	}
	if summary.Sections["tracks_and_vias"] != 1 {
		t.Fatalf("tracks_and_vias = %d", summary.Sections["tracks_and_vias"])
	}
}

func TestSummarizePCBHandlesCommentBeforeAtom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeTestFile(t, path, `(kicad_pcb
  (;comment
   net 0 "")
)
`)

	summary, err := SummarizePCB(path)
	if err != nil {
		t.Fatalf("SummarizePCB returned error: %v", err)
	}
	if summary.Sections["net"] != 1 {
		t.Fatalf("net = %d", summary.Sections["net"])
	}
}

func TestSummarizePCBRejectsUnterminatedString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeTestFile(t, path, "(kicad_pcb\n  (gr_text \"unterminated)\n")

	if _, err := SummarizePCB(path); err == nil {
		t.Fatal("expected unterminated string error")
	}
}
