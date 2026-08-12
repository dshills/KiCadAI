package closedloopopensetcontract

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionEightBlindBaselinePublisherIsOutcomeNeutralAndFrozen(t *testing.T) {
	directory := v8ContractDirectory(t)
	repository := filepath.Clean(filepath.Join(directory, "..", ".."))
	manifestPath := filepath.Join("specs", "closed-loop-open-set-capability-expansion", "V8_BASELINE_PUBLISHER.sha256")
	v8VerifyManifest(t, repository, manifestPath)

	file, err := os.Open(filepath.Join(repository, manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	production := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		_, path, ok := strings.Cut(scanner.Text(), "  ")
		if !ok {
			t.Fatal("V8 baseline publisher manifest entry is invalid")
		}
		if strings.Contains(path, "capabilityfeedback/testdata") || strings.Contains(path, "closed_loop_open_set_v8_corpus") {
			t.Fatalf("V8 baseline publisher freeze consumed corpus or outcome data: %s", path)
		}
		if strings.HasPrefix(path, "internal/blindbaseline/v8_") && !strings.HasSuffix(path, "_test.go") {
			production++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if production != 4 {
		t.Fatalf("V8 baseline publisher production file count = %d, want 4", production)
	}
}
