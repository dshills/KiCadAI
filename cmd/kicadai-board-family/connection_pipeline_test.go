package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/boardfamily"
)

// Only existing hand-authored synthetic fixtures are adapted. Captured model
// responses and all frozen corpus prompts/results are never read as fixtures
// to repair, migrate, or relabel as a new live success.
func connectionCorpusFixture(t testing.TB, id, prompt string) []byte {
	t.Helper()
	_, original := ownedCorpusFixture(t, id, prompt)
	var wire struct{ Facts []map[string]any }
	if err := json.Unmarshal(original, &wire); err != nil {
		t.Fatal(err)
	}
	for _, fact := range wire.Facts {
		if fact["kind"] == "feature" && fact["value"] == "wireless_operation" {
			fact["kind"], fact["value"] = "connection", "wireless"
		}
	}
	if id == "useful-01" || id == "useful-05" {
		clause := "c0"
		if id == "useful-01" {
			clause = "c1"
		}
		wire.Facts = append(wire.Facts, map[string]any{"kind": "connection", "value": "wired", "state": "required", "evidence": []string{clause}})
	}
	raw, err := json.Marshal(map[string]any{"version": boardfamily.ConnectionEvidenceVersion, "facts": wire.Facts})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestConnectionCommandSyntheticCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Cases []struct {
			ID, Prompt, Family, Profile string
			Disposition                 string `json:"expected_disposition"`
		}
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Cases) != 14 {
		t.Fatal("original corpus changed")
	}
	for _, c := range spec.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw := connectionCorpusFixture(t, c.ID, c.Prompt)
			before := bytes.Clone(raw)
			decision, err := boardfamily.DecodeConnectionEvidenceIntent(c.Prompt, raw)
			if err != nil || decision.Disposition != c.Disposition {
				t.Fatal("incorrect synthetic admission", decision, err)
			}
			if c.Disposition == "supported" && (decision.Configuration == nil || decision.Configuration.Family != c.Family || decision.Configuration.Profile != c.Profile) {
				t.Fatal("configuration changed")
			}
			root := t.TempDir()
			out, journal, ledger, budget := filepath.Join(root, "out"), filepath.Join(root, "journal"), filepath.Join(root, "ledger.json"), filepath.Join(root, "budget.json")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-connection-" + c.ID, MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			p, counts := connectionCommandPipeline(t, raw, "", "")
			if _, err := runIndexedCommand(t, p, "--intent-protocol", "connection-v5", "--prompt", c.Prompt, "--live-budget", budget, "--ledger", ledger, "--evidence-journal", journal, "--output", out); err != nil {
				t.Fatal(err)
			}
			want := 0
			if c.Disposition == "supported" {
				want = 1
				compareOwnedGeneratedFiles(t, out, c.Family, c.Profile)
			}
			if counts.requests != 1 || counts.generate != want || counts.validate != want {
				t.Fatal("incorrect execution boundary", counts)
			}
			a, err := boardfamily.InspectConnectionJournal(journal)
			if err != nil || !reflect.DeepEqual(a.Selection.Decision, decision) || a.Selection.AdmissionVersion != boardfamily.ConnectionEvidenceVersion || len(a.FilesSHA256) != 8 || len(a.Ledger.Entries) != 1 {
				t.Fatal("journal mismatch", err)
			}
			// The JSON selection is indented on disk; the original transport
			// stream is retained separately and replayed by the inspector.
			var compact bytes.Buffer
			if err := json.Compact(&compact, a.Selection.RawIntent); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(compact.Bytes(), raw) || !bytes.Equal(raw, before) {
				t.Fatal("extracted facts or caller bytes were changed")
			}
			for _, inspect := range []func(string) (boardfamily.ReferencedJournalAudit, error){boardfamily.InspectOwnedJournal, boardfamily.InspectReferencedJournal} {
				if _, err := inspect(journal); err == nil {
					t.Fatal("cross-version journal accepted")
				}
			}
			// Inspector uses the real CLI dispatch and must never call the provider.
			if _, err := runIndexedCommand(t, p, "--inspect-connection-journal", journal); err != nil || counts.requests != 1 {
				t.Fatal("inspection called provider", err)
			}
		})
	}
}

func TestConnectionCommandFailureGates(t *testing.T) {
	for _, mode := range []string{"invalid-extraction", "malformed-output", "refusal", "server-error", "missing-model", "broken-stream", "generation-failure", "validation-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			prompt := "Hello! Could you put together a wired pressure monitor for my desk? Your standard profile and reviewed default operating limits are fine. Thank you."
			raw := connectionCorpusFixture(t, "useful-01", prompt)
			if mode == "invalid-extraction" {
				raw = []byte(`{"version":"wrong","facts":[]}`)
			}
			if mode == "malformed-output" {
				raw = []byte(`{"broken":`)
			}
			root := t.TempDir()
			out, journal, ledger, budget := filepath.Join(root, "out"), filepath.Join(root, "journal"), filepath.Join(root, "ledger.json"), filepath.Join(root, "budget.json")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-connection-failure", MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			p, counts := connectionCommandPipeline(t, raw, mode, "")
			_, err := runIndexedCommand(t, p, "--intent-protocol", "connection-v5", "--prompt", prompt, "--live-budget", budget, "--ledger", ledger, "--evidence-journal", journal, "--output", out)
			if err == nil || counts.requests != 1 {
				t.Fatal("failure succeeded or retried", err, counts)
			}
			gen, val := 0, 0
			if mode == "generation-failure" || mode == "validation-failure" {
				gen = 1
			}
			if mode == "validation-failure" {
				val = 1
			}
			if counts.generate != gen || counts.validate != val {
				t.Fatal("failure crossed native boundary", counts)
			}
			if _, err := os.Stat(filepath.Join(journal, "selection", "selection.json")); err != nil {
				t.Fatal("failure lost evidence", err)
			}
		})
	}
}

func TestConnectionFlagsAndContract(t *testing.T) {
	for _, mode := range []string{"export", "export-file", "missing-prompt", "both-prompts", "no-journal", "no-budget", "no-ledger", "overlap", "mixed-inspect", "inspect-generation", "config", "nil-selector"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "")
			root := t.TempDir()
			out, journal, ledger, budget := filepath.Join(root, "out"), filepath.Join(root, "journal"), filepath.Join(root, "ledger.json"), filepath.Join(root, "budget.json")
			prompt := "Use SHT31 standard with a wired connection."
			p, counts := connectionCommandPipeline(t, []byte(`{}`), "", "")
			args := []string{"--intent-protocol", "connection-v5", "--prompt", prompt, "--output", out, "--ledger", ledger, "--live-budget", budget, "--evidence-journal", journal}
			switch mode {
			case "export", "export-file", "missing-prompt", "both-prompts":
				args = []string{"--intent-protocol", "connection-v5", "--export-live-contract", out}
				if mode == "export" || mode == "both-prompts" {
					args = append(args, "--prompt", prompt)
				}
				if mode == "export-file" || mode == "both-prompts" {
					file := filepath.Join(root, "prompt.txt")
					if err := os.WriteFile(file, []byte(prompt), 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--prompt-file", file)
				}
			case "no-journal":
				args[11] = ""
			case "no-budget":
				args[9] = ""
			case "no-ledger":
				args[7] = ""
			case "overlap":
				args[11] = filepath.Join(out, "journal")
			case "mixed-inspect":
				args = []string{"--inspect-connection-journal", journal, "--inspect-owned-journal", journal}
			case "inspect-generation":
				args = append(args, "--inspect-connection-journal", journal)
			case "config":
				args = append(args, "--config", "never-read.json")
			case "nil-selector":
				p.interpretConnection = nil
			}
			stdout, err := runIndexedCommand(t, p, args...)
			valid := mode == "export" || mode == "export-file"
			if (err == nil) != valid || counts.requests != 0 || counts.generate != 0 || counts.validate != 0 || len(stdout) != 0 {
				t.Fatal("invalid flags or export crossed boundary", err, counts)
			}
			if !valid {
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("invalid mode wrote output")
				}
				return
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			want, err := boardfamily.ConnectionEvidenceContract(prompt)
			if err != nil {
				t.Fatal(err)
			}
			var got any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(got)
			expected, _ := json.Marshal(want)
			var normalized any
			if err := json.Unmarshal(expected, &normalized); err != nil {
				t.Fatal(err)
			}
			expected, _ = json.Marshal(normalized)
			if !bytes.Equal(a, expected) {
				t.Fatal("export differs from exact request contract")
			}
		})
	}
}
