package main

import (
	"bytes"
	"encoding/json"
	"kicadai/internal/boardfamily"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedFlagsRequireExplicitSeparateEvidence(t *testing.T) {
	for _, mode := range []string{"unknown", "no-journal", "no-budget", "no-ledger", "config", "typed-journal", "list", "export-journal", "overlap"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			out, ledger, budget, journal := filepath.Join(dir, "out"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "budget.json"), filepath.Join(dir, "evidence")
			if err := os.WriteFile(budget, []byte(`{"goal":"offline-flags","max_requests":1,"max_micro_usd":50000}`), 0600); err != nil {
				t.Fatal(err)
			}
			protocol := "owned-v4"
			switch mode {
			case "unknown":
				protocol = "unknown"
			case "no-journal":
				journal = ""
			case "no-budget":
				budget = ""
			case "no-ledger":
				ledger = ""
			case "typed-journal":
				protocol = "typed-v2"
			case "overlap":
				journal = filepath.Join(out, "evidence")
			}
			args := []string{"--intent-protocol", protocol, "--output", out, "--ledger", ledger, "--live-budget", budget, "--evidence-journal", journal, "--prompt", "Please use BMP280."}
			switch mode {
			case "config":
				args = append(args, "--config", "never-open.json")
			case "list":
				args = append(args, "--list-families")
			case "export-journal":
				args = append(args, "--export-live-contract", filepath.Join(dir, "contract.json"))
			}
			pipeline, counts := ownedCommandPipeline(t, []byte(`{}`), "", "")
			stdout, err := runIndexedCommand(t, pipeline, args...)
			if err == nil || counts.requests != 0 || counts.generate != 0 || counts.validate != 0 || len(stdout) != 0 {
				t.Fatal("invalid experimental flags performed work", err, *counts)
			}
			if _, err := os.Lstat(out); !os.IsNotExist(err) {
				t.Fatal("invalid flags wrote output")
			}
		})
	}
}

func TestOwnedContractExportRequiresExactPrompt(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	prompt := "Use BMP280 with 2.2k pull-ups and 100 pF capacitance."
	for _, mode := range []string{"prompt", "prompt-file", "missing", "both", "ledger", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "contract.json")
			file := filepath.Join(dir, "prompt.txt")
			if err := os.WriteFile(file, []byte(prompt), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--intent-protocol", "owned-v4", "--export-live-contract", output}
			switch mode {
			case "prompt":
				args = append(args, "--prompt", prompt)
			case "prompt-file":
				args = append(args, "--prompt-file", file)
			case "both":
				args = append(args, "--prompt", prompt, "--prompt-file", file)
			case "ledger":
				args = append(args, "--prompt", prompt, "--ledger", filepath.Join(dir, "ledger.json"))
			case "invalid":
				args = append(args, "--prompt", " ")
			}
			p, counts := ownedCommandPipeline(t, []byte("{}"), "", "")
			stdout, err := runIndexedCommand(t, p, args...)
			valid := mode == "prompt" || mode == "prompt-file"
			if (err == nil) != valid || counts.requests != 0 || counts.generate != 0 || counts.validate != 0 || len(stdout) != 0 {
				t.Fatal("contract export crossed a generation/transport boundary", err, *counts)
			}
			if !valid {
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("invalid export wrote a contract")
				}
				return
			}
			b, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			want, err := boardfamily.OwnedEvidenceContract(prompt)
			if err != nil {
				t.Fatal(err)
			}
			canonical, _ := json.Marshal(got)
			expected, _ := json.Marshal(want)
			var normalized map[string]any
			if err := json.Unmarshal(expected, &normalized); err != nil {
				t.Fatal(err)
			}
			expected, _ = json.Marshal(normalized)
			if !bytes.Equal(canonical, expected) || got["schema"] == nil || got["source"] == nil {
				t.Fatal("export is not the exact non-secret request contract")
			}
		})
	}
}

func TestOwnedCommandFailureGates(t *testing.T) {
	for _, mode := range []string{"invalid-extraction", "malformed-output", "refusal", "missing-model", "broken-stream", "generation-failure", "validation-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			prompt := "Please use BMP280 with standard profile."
			// Hand-authored local facts for this single-clause request, never history.
			raw := []byte(`{"version":"4-owned-evidence-experimental","facts":[{"kind":"sensor","value":"BMP280","state":"required","evidence":["c0"]},{"kind":"profile","value":"standard","state":"required","evidence":["c0"]}]}`)
			if mode == "invalid-extraction" {
				raw = []byte(`{"version":"wrong","facts":[]}`)
			}
			if mode == "malformed-output" {
				raw = []byte(`{"broken":`)
			}
			dir := t.TempDir()
			out, journal, ledger, budget := filepath.Join(dir, "out"), filepath.Join(dir, "journal"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "budget.json")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-owned-command-failure", MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			p, counts := ownedCommandPipeline(t, raw, mode, "")
			_, err := runIndexedCommand(t, p, "--intent-protocol", "owned-v4", "--prompt", prompt, "--output", out, "--evidence-journal", journal, "--ledger", ledger, "--live-budget", budget)
			if err == nil || counts.requests != 1 {
				t.Fatal("failed command incorrectly succeeded or retried", err, *counts)
			}
			expectedGenerate, expectedValidate := 0, 0
			if mode == "generation-failure" || mode == "validation-failure" {
				expectedGenerate = 1
			}
			if mode == "validation-failure" {
				expectedValidate = 1
			}
			if counts.generate != expectedGenerate || counts.validate != expectedValidate {
				t.Fatal("failed selection reached native work", *counts)
			}
			if _, err := os.Stat(filepath.Join(journal, "selection", "selection.json")); err != nil {
				t.Fatal("failure lost journal", err)
			}
		})
	}
}
