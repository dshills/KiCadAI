package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/boardfamily"
)

type groundedReviewCase struct {
	ID, Prompt, Family, Profile string
	Disposition                 string `json:"expected_disposition"`
}

func groundedReviewCases(t testing.TB) []groundedReviewCase {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []groundedReviewCase }
	if err := json.Unmarshal(b, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("frozen corpus unavailable or changed", err)
	}
	return corpus.Cases
}

// A new explicit local directory retains synthetic review artifacts. Normal
// tests use disposable directories. Existing records are never overwritten.
func groundedReviewDirectory(t testing.TB, name string) string {
	t.Helper()
	root := os.Getenv("KICADAI_PARTITIONED_REVIEW_ROOT")
	if root == "" {
		return t.TempDir()
	}
	if !filepath.IsAbs(root) {
		t.Fatal("review root must be absolute")
	}
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal("review directory must be new", err)
	}
	return dir
}

func TestGroundedOfflineReviewFixtures(t *testing.T) {
	root := groundedReviewDirectory(t, "fixtures")
	for _, c := range groundedReviewCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := groundedCorpusFixture(t, c.ID, c.Prompt)
			contract, err := boardfamily.GroundedEvidenceContract(c.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, raw, "", "  "); err != nil {
				t.Fatal(err)
			}
			fixture := map[string]any{"id": c.ID, "prompt": c.Prompt, "expected_disposition": c.Disposition,
				"synthetic": true, "raw_text": string(raw), "compact_bytes": len(raw), "pretty_bytes": pretty.Len(), "contract": contract}
			if err := save(filepath.Join(root, c.ID+".json"), fixture); err != nil {
				t.Fatal(err)
			}
			t.Logf("synthetic visible JSON only: compact %d bytes, indented %d bytes; not an API token count", len(raw), pretty.Len())
		})
	}
}

func TestGroundedCandidateCommandNative(t *testing.T) {
	testPartitionedCandidateCommandNative(t, "partitioned-v7")
}

func testPartitionedCandidateCommandNative(t *testing.T, protocol string) {
	t.Helper()
	cli := os.Getenv("KICADAI_OFFLINE_NATIVE_CLI")
	if testing.Short() || cli == "" {
		t.Skip("requires explicit offline KiCad CLI; never a live model test")
	}
	name := "native"
	fixture, inspect := groundedCorpusFixture, boardfamily.InspectGroundedJournal
	if protocol == "source-eligible-v8" {
		name = "source-eligible-native"
		fixture, inspect = eligibleCorpusFixture, boardfamily.InspectSourceEligibleJournal
	}
	if protocol == "source-addressed-v9" {
		name = "source-addressed-native"
		fixture, inspect = addressedCorpusFixture, boardfamily.InspectSourceAddressedJournal
	}
	if protocol == "semantic-boundaries-v10" {
		name = "semantic-boundaries-native"
		fixture, inspect = boundaryCorpusFixture, boardfamily.InspectSemanticBoundaryJournal
	}
	if protocol == "requirement-coverage-v11" {
		name = "requirement-coverage-native"
		fixture, inspect = coverageCorpusFixture, boardfamily.InspectCoverageJournal
	}
	root := groundedReviewDirectory(t, name)
	useful := 0
	for _, c := range groundedReviewCases(t) {
		if c.Disposition != "supported" {
			continue
		}
		useful++
		t.Run(c.ID, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			dir := filepath.Join(root, c.ID)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			output, ledger, budget, journal := filepath.Join(dir, "output"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "budget.json"), filepath.Join(dir, "journal")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-partitioned-native-" + c.ID, MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			pipeline, counts := evidenceCommandPipeline(t, fixture(t, c.ID, c.Prompt), "", cli, protocol)
			stdout, err := runIndexedCommand(t, pipeline, "--intent-protocol", protocol, "--prompt", c.Prompt, "--output", output, "--ledger", ledger, "--live-budget", budget, "--evidence-journal", journal, "--kicad-cli", cli)
			if err != nil || counts.requests != 1 || counts.generate != 1 || counts.validate != 1 {
				t.Fatalf("offline native command failed: %v counts=%+v stdout=%s", err, counts, stdout)
			}
			read := func(path string) boardfamily.Validation {
				t.Helper()
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var v boardfamily.Validation
				if err := json.Unmarshal(b, &v); err != nil {
					t.Fatal(err)
				}
				return v
			}
			v := read(filepath.Join(output, "validation.json"))
			sensor := "bmp280"
			if c.Family == boardfamily.FamilySHT31 {
				sensor = "sht31"
			}
			reviewed := read(filepath.Join("..", "..", "examples", "board-family-v2", sensor+"-"+c.Profile, "validation.json"))
			if !v.Passed || v.KiCadVersion != "10.0.3" || len(v.Checks) != 14 || !reflect.DeepEqual(v.NativeSHA256, reviewed.NativeSHA256) {
				t.Fatal("native qualification or reviewed bytes differ", v)
			}
			for i, check := range v.Checks {
				if !check.Passed || check.Error != "" || check.Name != reviewed.Checks[i].Name {
					t.Fatal("required gate missing or failed", check)
				}
			}
			audit, err := inspect(journal)
			if err != nil || audit.Selection.Decision.Configuration == nil || audit.Selection.Decision.Configuration.Family != c.Family || audit.Selection.Decision.Configuration.Profile != c.Profile || !strings.Contains(audit.Selection.ResponseID, "offline") {
				t.Fatal("native handoff lost synthetic provenance", err)
			}
			t.Logf("SYNTHETIC extraction -> real %s/%s native bundle: 14 gates pass; no live model, no manual repair, no physical qualification", c.Family, c.Profile)
		})
	}
	if useful != 5 {
		t.Fatal("native fixture scope changed", useful)
	}
}
