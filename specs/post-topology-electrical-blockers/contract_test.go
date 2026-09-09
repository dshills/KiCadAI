package posttopologyelectrical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestDiagnosticPopulationAndBoundary(t *testing.T) {
	data, err := os.ReadFile("DIAGNOSTIC_RUN.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Schema      string   `json:"schema"`
		Base        string   `json:"base_commit"`
		Boundary    string   `json:"boundary"`
		Diagnostic  bool     `json:"diagnostic_only"`
		Cases       []string `json:"cases"`
		Runs        int      `json:"runs_per_case"`
		Timeout     string   `json:"timeout"`
		Policy      string   `json:"policy"`
		Go          string   `json:"go_version"`
		Platform    string   `json:"platform"`
		Baseline    string   `json:"baseline_file_sha256"`
		Assessment  string   `json:"assessment_file_sha256"`
		Corpus      string   `json:"corpus_manifest_sha256"`
		HeldOut     bool     `json:"held_out_access"`
		Corrections bool     `json:"synthesis_corrections"`
		Physical    bool     `json:"physical_promotion"`
		Repeat      bool     `json:"repeat_after_outcome"`
		Match       bool     `json:"required_historical_replay_match"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Schema != "kicadai.post-topology-diagnostic-run.v1" || config.Base != "abe617d182369352df8fe0e16f9071a539881b21" || config.Boundary != "unchanged_v21" || !config.Diagnostic || config.Runs != 1 || config.Timeout != "3h" || config.Policy != "unchanged_DefaultPolicy" || config.Go != "go1.26.8" || config.Platform != "darwin/arm64" || config.HeldOut || config.Corrections || config.Physical || config.Repeat || !config.Match {
		t.Fatal("diagnostic boundary differs")
	}
	root := filepath.Join("..", "..")
	assessmentPath := filepath.Join(root, "specs/generic-causal-topology-repair/maintenance-evaluation-1/ASSESSMENT.json")
	if fileHash(t, assessmentPath) != config.Assessment || fileHash(t, filepath.Join(root, "internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json")) != config.Baseline || fileHash(t, filepath.Join(root, "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus/manifest.json")) != config.Corpus {
		t.Fatal("diagnostic input binding differs")
	}
	data, err = os.ReadFile(assessmentPath)
	if err != nil {
		t.Fatal(err)
	}
	var assessment struct {
		Cases []string `json:"qualifying_cases"`
	}
	if err := json.Unmarshal(data, &assessment); err != nil {
		t.Fatal(err)
	}
	if len(config.Cases) != 4 || !reflect.DeepEqual(config.Cases, assessment.Cases) {
		t.Fatal("diagnostic population is not the four frozen qualifying public cases")
	}
}

func TestDiagnosticSourceFreeze(t *testing.T) {
	data, err := os.ReadFile("DIAGNOSTIC.sha256")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if len(line) < 67 || line[64:66] != "  " || seen[line[66:]] {
			t.Fatal("invalid diagnostic checksum record")
		}
		path := line[66:]
		seen[path] = true
		if fileHash(t, path) != line[:64] {
			t.Fatalf("diagnostic source differs: %s", path)
		}
	}
	if len(seen) != 10 {
		t.Fatalf("diagnostic source inventory size %d", len(seen))
	}
}
