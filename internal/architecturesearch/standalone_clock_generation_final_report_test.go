package architecturesearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneClockGenerationFinalReportClosesFrozenAndHeldOutCorpora(t *testing.T) {
	root := filepath.Join("..", "..", "specs", "standalone-clock-generation")
	body, err := os.ReadFile(filepath.Join(root, "FINAL_REPORT.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Schema string `json:"schema"`
		Cases  []struct {
			ID                 string `json:"id"`
			Status             string `json:"status"`
			ArchitectureFamily string `json:"architecture_family"`
			CornerEvaluations  int    `json:"corner_evaluations"`
			InstalledKiCad     string `json:"installed_kicad"`
			Replay             string `json:"replay"`
		} `json:"cases"`
		HeldOut struct {
			Cases   int `json:"cases"`
			Passed  int `json:"passed"`
			Blocked int `json:"blocked"`
		} `json:"held_out_benchmark"`
		Aggregate struct {
			Cases                int `json:"cases"`
			Passed               int `json:"passed"`
			Blocked              int `json:"blocked"`
			ArchitectureFamilies int `json:"architecture_families"`
		} `json:"aggregate"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != "kicadai.standalone-clock-generation-final.v1" ||
		report.Aggregate.Cases != 2 || report.Aggregate.Passed != 2 ||
		report.Aggregate.Blocked != 0 || report.Aggregate.ArchitectureFamilies != 2 {
		t.Fatalf("final aggregate = %#v", report.Aggregate)
	}
	families := map[string]bool{}
	for _, result := range report.Cases {
		if result.ID == "" || result.Status != "pass" || result.CornerEvaluations != 8 ||
			result.InstalledKiCad != "pass" || result.Replay != "byte_identical" {
			t.Fatalf("incomplete final result = %#v", result)
		}
		families[result.ArchitectureFamily] = true
	}
	if len(families) != 2 {
		t.Fatalf("architecture families = %#v", families)
	}
	if report.HeldOut.Cases != 12 || report.HeldOut.Passed != 12 || report.HeldOut.Blocked != 0 {
		t.Fatalf("held-out benchmark = %#v", report.HeldOut)
	}
	sum := sha256.Sum256(body)
	checksum, err := os.ReadFile(filepath.Join(root, "FINAL_REPORT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	want := hex.EncodeToString(sum[:]) + "  FINAL_REPORT.json"
	if strings.TrimSpace(string(checksum)) != want {
		t.Fatalf("final checksum = %q, want %q", strings.TrimSpace(string(checksum)), want)
	}
}
