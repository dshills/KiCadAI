package genericcausaltopologyrepair

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Historical source and seals authenticate generation zero independently of
// the current maintenance seal. Replacing a live hash must never relabel the
// already published evaluation as evidence for changed code.
func TestV21GenerationZeroSourceIsPreserved(t *testing.T) {
	manifests := map[string]string{
		"V21_EVALUATOR.sha256": "400b3ab32100b356d3db373c3111ce04ee678785fcc8598d93e20100c532320b",
		"V21_CONTRACT.sha256":  "c0f46c533cd17590867d0af16d6f11480f224d16b0b79711667f16764aa8c39a",
	}
	for name, want := range manifests {
		path := filepath.Join("generation-zero-source", name+".snapshot")
		if actual := v21FileSHA256(t, path); actual != want {
			t.Fatalf("historical %s changed: got %s want %s", name, actual, want)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) < 67 || line[64:66] != "  " {
				t.Fatalf("invalid historical manifest line %q", line)
			}
			original := filepath.FromSlash(line[66:])
			archived := filepath.Join("generation-zero-source", filepath.Base(original)+".snapshot")
			source := original
			if _, err := os.Stat(archived); err == nil {
				source = archived
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if actual := v21FileSHA256(t, source); actual != line[:64] {
				t.Fatalf("generation-zero source drift: %s = %s want %s", original, actual, line[:64])
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v21_generation_zero", "report.json"))
	if err != nil || !strings.Contains(string(data), manifests["V21_EVALUATOR.sha256"]) {
		t.Fatalf("published report must retain its original evaluator attribution: %v", err)
	}
}
