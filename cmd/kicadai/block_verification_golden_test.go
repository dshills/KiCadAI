package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/blocks"
)

var updateBlockVerificationGoldens = flag.Bool("update-block-verification-goldens", false, "update block verification CLI golden files")

func TestRunBlockVerificationGoldens(t *testing.T) {
	goldenDir := filepath.FromSlash("testdata/golden/block_verification")
	cases := []struct {
		name      string
		args      []string
		beforeRun func(t *testing.T) []string
		wantErr   bool
	}{
		{
			name: "builtins_summary",
			args: []string{"--json", "--builtins", "block", "verify"},
		},
		{
			name: "led_case_pass",
			args: []string{"--json", "--case", "__CASE__", "block", "verify"},
			beforeRun: func(t *testing.T) []string {
				data, err := fs.ReadFile(blocks.BuiltinVerificationFS(), "led_indicator_default/manifest.json")
				if err != nil {
					t.Fatalf("read embedded manifest: %v", err)
				}
				path := filepath.Join(t.TempDir(), "manifest.json")
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatalf("write embedded manifest copy: %v", err)
				}
				return []string{"__CASE__", path}
			},
		},
		{
			name: "blocked_case",
			args: []string{"--json", "--case", "__CASE__", "block", "verify"},
			beforeRun: func(t *testing.T) []string {
				path := filepath.Join(t.TempDir(), "blocked.json")
				data := strings.ReplaceAll(blockedBlockVerificationManifest, "__SYMBOL__", "Device:C")
				if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
					t.Fatalf("write blocked manifest: %v", err)
				}
				return []string{"__CASE__", path}
			},
			wantErr: true,
		},
		{
			name: "skipped_erc_drc",
			args: []string{"--json", "--case", "__CASE__", "--output", "__OUT__", "block", "verify"},
			beforeRun: func(t *testing.T) []string {
				t.Setenv("KICADAI_KICAD_CLI", filepath.Join(t.TempDir(), "missing-kicad-cli"))
				dir := t.TempDir()
				path := filepath.Join(dir, "skipped.json")
				if err := os.WriteFile(path, []byte(optionalERCDRCBlockVerificationManifest), 0o644); err != nil {
					t.Fatalf("write skipped manifest: %v", err)
				}
				return []string{"__CASE__", path, "__OUT__", filepath.Join(dir, "out")}
			},
		},
		{
			name: "kicad_corpus_optional_skip",
			args: []string{"--json", "--case", "__CASE__", "--kicad-corpus", "--output", "__OUT__", "block", "verify"},
			beforeRun: func(t *testing.T) []string {
				t.Setenv("KICADAI_KICAD_CLI", filepath.Join(t.TempDir(), "missing-kicad-cli"))
				dir := t.TempDir()
				path := filepath.Join(dir, "corpus.json")
				if err := os.WriteFile(path, []byte(optionalKiCadCorpusBlockVerificationManifest), 0o644); err != nil {
					t.Fatalf("write corpus manifest: %v", err)
				}
				return []string{"__CASE__", path, "__OUT__", filepath.Join(dir, "out")}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := slices.Clone(tc.args)
			if tc.beforeRun != nil {
				replacements := tc.beforeRun(t)
				for i := 0; i+1 < len(replacements); i += 2 {
					args = replaceArgs(args, replacements[i], replacements[i+1])
				}
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := run(args, &stdout, &stderr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, stdout=%s", stdout.String())
				}
			} else if err != nil {
				t.Fatalf("run err = %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
			}
			got := normalizeBlockVerificationGolden(t, stdout.Bytes())
			path := filepath.Join(goldenDir, tc.name+".json")
			if *updateBlockVerificationGoldens {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir golden dir: %v", err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v", path, err)
			}
			if string(got) != string(want) {
				t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, want)
			}
		})
	}
}

func replaceArgs(args []string, old string, new string) []string {
	for index, arg := range args {
		if arg == old {
			args[index] = new
		}
	}
	return args
}

func normalizeBlockVerificationGolden(t *testing.T, data []byte) []byte {
	t.Helper()
	// Keep these goldens focused on the stable agent-facing verification
	// contract. Full block outputs include verbose operation payloads that are
	// covered by lower-level block tests and make CLI snapshots brittle.
	type snapshotIssue struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	}
	type snapshotStage struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	type snapshotCorpusCase struct {
		CaseID         string   `json:"case_id"`
		BlockID        string   `json:"block_id"`
		Tier           string   `json:"tier,omitempty"`
		Readiness      string   `json:"readiness,omitempty"`
		Status         string   `json:"status"`
		ExpectedStatus string   `json:"expected_status,omitempty"`
		ExpectedIssues []string `json:"expected_issues,omitempty"`
		Notes          string   `json:"notes,omitempty"`
	}
	type snapshotResult struct {
		CaseID        string              `json:"case_id"`
		BlockID       string              `json:"block_id"`
		EvidenceLevel string              `json:"evidence_level"`
		Status        string              `json:"status"`
		KiCadCorpus   *snapshotCorpusCase `json:"kicad_corpus,omitempty"`
		Stages        []snapshotStage     `json:"stages"`
		Issues        []snapshotIssue     `json:"issues,omitempty"`
	}
	type snapshotCorpusSummary struct {
		Enabled        bool                 `json:"enabled"`
		SelectedCount  int                  `json:"selected_count"`
		TotalCount     int                  `json:"total_count"`
		CaseIDs        []string             `json:"case_ids,omitempty"`
		CountsByStatus map[string]int       `json:"counts_by_status,omitempty"`
		CountsByTier   map[string]int       `json:"counts_by_tier,omitempty"`
		CountsByBlock  map[string]int       `json:"counts_by_block,omitempty"`
		Results        []snapshotCorpusCase `json:"results,omitempty"`
	}
	type snapshotReport struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Count       int                    `json:"count"`
			KiCadCorpus *snapshotCorpusSummary `json:"kicad_corpus,omitempty"`
			Results     []snapshotResult       `json:"results"`
		} `json:"data"`
		Issues []snapshotIssue `json:"issues,omitempty"`
	}
	var root struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Count       int                    `json:"count"`
			KiCadCorpus *snapshotCorpusSummary `json:"kicad_corpus"`
			Results     []struct {
				CaseID        string              `json:"case_id"`
				BlockID       string              `json:"block_id"`
				EvidenceLevel string              `json:"evidence_level"`
				Status        string              `json:"status"`
				KiCadCorpus   *snapshotCorpusCase `json:"kicad_corpus"`
				Stages        []snapshotStage     `json:"stages"`
				Issues        []snapshotIssue     `json:"issues"`
			} `json:"results"`
		} `json:"data"`
		Issues []snapshotIssue `json:"issues"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("decode report: %v\n%s", err, data)
	}
	normalized := snapshotReport{OK: root.OK, Command: root.Command}
	normalized.Data.Count = root.Data.Count
	normalized.Data.KiCadCorpus = root.Data.KiCadCorpus
	for _, result := range root.Data.Results {
		normalized.Data.Results = append(normalized.Data.Results, snapshotResult{
			CaseID:        result.CaseID,
			BlockID:       result.BlockID,
			EvidenceLevel: result.EvidenceLevel,
			Status:        result.Status,
			KiCadCorpus:   result.KiCadCorpus,
			Stages:        result.Stages,
			Issues:        result.Issues,
		})
	}
	normalized.Issues = root.Issues
	out, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		t.Fatalf("marshal normalized report: %v", err)
	}
	return append(out, '\n')
}

const blockedBlockVerificationManifest = `{
  "id": "led_indicator_blocked_golden",
  "block_id": "led_indicator",
  "request": {"instance_id": "status"},
  "expected": {
    "evidence_level": "schematic_verified",
    "components": [
      {"role": "resistor", "ref_prefix": "R", "symbol_id": "__SYMBOL__"},
      {"role": "led", "ref_prefix": "D", "symbol_id": "Device:LED"}
    ]
  }
}`

const optionalERCDRCBlockVerificationManifest = `{
  "id": "led_indicator_skipped_erc_drc",
  "block_id": "led_indicator",
  "request": {"instance_id": "status"},
  "expected": {
    "evidence_level": "schematic_verified",
    "erc_drc": {"allowed_codes": ["OPTIONAL_ONLY"]},
    "components": [
      {"role": "resistor", "ref_prefix": "R", "symbol_id": "Device:R"},
      {"role": "led", "ref_prefix": "D", "symbol_id": "Device:LED"}
    ]
  }
}`

const optionalKiCadCorpusBlockVerificationManifest = `{
  "id": "led_indicator_corpus_optional",
  "block_id": "led_indicator",
  "request": {"instance_id": "status"},
  "expected": {
    "evidence_level": "schematic_verified",
    "kicad_corpus": {
      "include": true,
      "tier": "smoke",
      "readiness": "candidate",
      "expected_status": "skip",
      "allowed_codes": ["OPTIONAL_ONLY"]
    },
    "components": [
      {"role": "resistor", "ref_prefix": "R", "symbol_id": "Device:R"},
      {"role": "led", "ref_prefix": "D", "symbol_id": "Device:LED"}
    ]
  }
}`
