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

func TestGroundedCommandSyntheticCorpus(t *testing.T) {
	testGroundedCommandSyntheticCorpus(t, "partitioned-v7")
}

func TestGroundedFullCommandSyntheticCorpus(t *testing.T) {
	testGroundedCommandSyntheticCorpus(t, "partitioned-full-v7")
}

func groundedCommandMode(protocol string) (string, string, func(string) (boardfamily.ReferencedJournalAudit, error)) {
	if protocol == "semantic-boundaries-v10" {
		return protocol, "--inspect-semantic-boundary-journal", boardfamily.InspectSemanticBoundaryJournal
	}
	if protocol == "source-addressed-v9" {
		return protocol, "--inspect-source-addressed-journal", boardfamily.InspectSourceAddressedJournal
	}
	if protocol == "source-eligible-v8" {
		return protocol, "--inspect-source-eligible-journal", boardfamily.InspectSourceEligibleJournal
	}
	if protocol == "partitioned-full-v7" {
		return "partitioned-full-v7", "--inspect-partitioned-full-journal", boardfamily.InspectGroundedFullJournal
	}
	return "partitioned-v7", "--inspect-partitioned-journal", boardfamily.InspectGroundedJournal
}

func testGroundedCommandSyntheticCorpus(t *testing.T, mode string) {
	t.Helper()
	protocol, inspectFlag, inspect := groundedCommandMode(mode)
	full := protocol != "partitioned-v7"
	version, profile := boardfamily.GroundedEvidenceVersion, boardfamily.GroundedFullAccountingProfile
	fixture, decode := groundedCorpusFixture, boardfamily.DecodeGroundedEvidenceIntent
	if protocol == "source-eligible-v8" {
		version, profile = boardfamily.SourceEligibleVersion, boardfamily.SourceEligibleAccountingProfile
		fixture, decode = eligibleCorpusFixture, boardfamily.DecodeSourceEligibleEvidenceIntent
	}
	if protocol == "source-addressed-v9" {
		version, profile = boardfamily.SourceAddressedVersion, boardfamily.SourceAddressedAccountingProfile
		fixture, decode = addressedCorpusFixture, boardfamily.DecodeSourceAddressedEvidenceIntent
	}
	if protocol == "semantic-boundaries-v10" {
		version, profile = boardfamily.SemanticBoundaryVersion, boardfamily.SemanticBoundaryAccountingProfile
		fixture, decode = boundaryCorpusFixture, boardfamily.DecodeSemanticBoundaryIntent
	}
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
			raw := fixture(t, c.ID, c.Prompt)
			before := bytes.Clone(raw)
			decision, err := decode(c.Prompt, raw)
			if err != nil || decision.Disposition != c.Disposition {
				t.Fatal("incorrect synthetic admission", decision, err)
			}
			if c.Disposition == "supported" && (decision.Configuration == nil || decision.Configuration.Family != c.Family || decision.Configuration.Profile != c.Profile) {
				t.Fatal("configuration changed")
			}
			root := t.TempDir()
			out, journal, ledger, budget := filepath.Join(root, "out"), filepath.Join(root, "journal"), filepath.Join(root, "ledger.json"), filepath.Join(root, "budget.json")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-partitioned-" + c.ID, MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			p, counts := evidenceCommandPipeline(t, raw, "", "", protocol)
			if _, err := runIndexedCommand(t, p, "--intent-protocol", protocol, "--prompt", c.Prompt, "--live-budget", budget, "--ledger", ledger, "--evidence-journal", journal, "--output", out); err != nil {
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
			a, err := inspect(journal)
			if err != nil || !reflect.DeepEqual(a.Selection.Decision, decision) || a.Selection.AdmissionVersion != version || len(a.FilesSHA256) != 8 || len(a.Ledger.Entries) != 1 {
				t.Fatal("journal mismatch", err)
			}
			if full && (a.Selection.Model != boardfamily.GroundedFullModel || a.Ledger.Version != 3 || a.Ledger.AccountingProfile != profile || a.Ledger.Entries[0].EstimatedMicroUSD != 1800) {
				t.Fatal("full-model identity/accounting differs")
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
			for _, inspect := range []func(string) (boardfamily.ReferencedJournalAudit, error){boardfamily.InspectDirectJournal, boardfamily.InspectConnectionJournal, boardfamily.InspectOwnedJournal, boardfamily.InspectReferencedJournal} {
				if _, err := inspect(journal); err == nil {
					t.Fatal("cross-version journal accepted")
				}
			}
			// Inspector uses the real CLI dispatch and must never call the provider.
			if _, err := runIndexedCommand(t, p, inspectFlag, journal); err != nil || counts.requests != 1 {
				t.Fatal("inspection called provider", err)
			}
		})
	}
}

func TestGroundedCommandFailureGates(t *testing.T) {
	testGroundedCommandFailureGates(t, "partitioned-v7")
}

func TestGroundedFullCommandFailureGates(t *testing.T) {
	testGroundedCommandFailureGates(t, "partitioned-full-v7")
}

func testGroundedCommandFailureGates(t *testing.T, protocolMode string) {
	t.Helper()
	protocol, _, inspect := groundedCommandMode(protocolMode)
	for _, mode := range []string{"invalid-extraction", "malformed-output", "refusal", "server-error", "missing-model", "broken-stream", "token-limit", "generation-failure", "validation-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			prompt := "Hello! Could you put together a wired pressure monitor for my desk? Your standard profile and reviewed default operating limits are fine. Thank you."
			raw := groundedCorpusFixture(t, "useful-01", prompt)
			if protocol == "source-eligible-v8" {
				raw = eligibleCorpusFixture(t, "useful-01", prompt)
			}
			if protocol == "source-addressed-v9" {
				raw = addressedCorpusFixture(t, "useful-01", prompt)
			}
			if protocol == "semantic-boundaries-v10" {
				raw = boundaryCorpusFixture(t, "useful-01", prompt)
			}
			if mode == "invalid-extraction" {
				raw = []byte(`{"version":"wrong","facts":[]}`)
			}
			if mode == "malformed-output" {
				raw = []byte(`{"broken":`)
			}
			root := t.TempDir()
			out, journal, ledger, budget := filepath.Join(root, "out"), filepath.Join(root, "journal"), filepath.Join(root, "ledger.json"), filepath.Join(root, "budget.json")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-partitioned-failure", MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			p, counts := evidenceCommandPipeline(t, raw, mode, "", protocol)
			_, err := runIndexedCommand(t, p, "--intent-protocol", protocol, "--prompt", prompt, "--live-budget", budget, "--ledger", ledger, "--evidence-journal", journal, "--output", out)
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
			if mode == "token-limit" {
				// A failed/incomplete journal is retained but must NOT be
				// certified by the completed-response replay inspector.
				if _, err := inspect(journal); err == nil {
					t.Fatal("incomplete response authenticated as complete")
				}
				read := func(path string, value any) {
					t.Helper()
					b, err := os.ReadFile(path)
					if err != nil || json.Unmarshal(b, value) != nil {
						t.Fatal("cannot read retained failure evidence", err)
					}
				}
				var selection boardfamily.ReferencedSelection
				var recorded boardfamily.Ledger
				read(filepath.Join(journal, "selection", "selection.json"), &selection)
				read(ledger, &recorded)
				if selection.Outcome != "provider_incomplete" || selection.Decision.Configuration != nil || selection.Usage.OutputTokens != 1600 || selection.ProviderEvidence == nil || !selection.ProviderEvidence.EOFObserved || len(recorded.Entries) != 1 || recorded.Entries[0].OutputTokens != 1600 || recorded.Entries[0].ResponseID != selection.ResponseID || recorded.Entries[0].EstimatedMicroUSD == 0 {
					t.Fatal("token-limit failure lost evidence or accounting")
				}
			}
		})
	}
}

func TestGroundedFlagsAndContract(t *testing.T) {
	testGroundedFlagsAndContract(t, "partitioned-v7")
}

func TestGroundedFullFlagsAndContract(t *testing.T) {
	testGroundedFlagsAndContract(t, "partitioned-full-v7")
}

func testGroundedFlagsAndContract(t *testing.T, protocolMode string) {
	t.Helper()
	protocol, inspectFlag, _ := groundedCommandMode(protocolMode)
	for _, mode := range []string{"export", "export-file", "missing-prompt", "both-prompts", "no-journal", "no-budget", "no-ledger", "overlap", "mixed-inspect", "inspect-generation", "config", "nil-selector"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "")
			root := t.TempDir()
			out, journal, ledger, budget := filepath.Join(root, "out"), filepath.Join(root, "journal"), filepath.Join(root, "ledger.json"), filepath.Join(root, "budget.json")
			prompt := "Use SHT31 standard with a wired connection."
			p, counts := evidenceCommandPipeline(t, []byte(`{}`), "", "", protocol)
			args := []string{"--intent-protocol", protocol, "--prompt", prompt, "--output", out, "--ledger", ledger, "--live-budget", budget, "--evidence-journal", journal}
			switch mode {
			case "export", "export-file", "missing-prompt", "both-prompts":
				args = []string{"--intent-protocol", protocol, "--export-live-contract", out}
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
				args = []string{inspectFlag, journal, "--inspect-owned-journal", journal}
			case "inspect-generation":
				args = append(args, inspectFlag, journal)
			case "config":
				args = append(args, "--config", "never-read.json")
			case "nil-selector":
				if protocol == "semantic-boundaries-v10" {
					p.interpretBoundary = nil
				} else if protocol == "source-addressed-v9" {
					p.interpretAddressed = nil
				} else if protocol == "source-eligible-v8" {
					p.interpretEligible = nil
				} else if protocol == "partitioned-full-v7" {
					p.interpretGroundedFull = nil
				} else {
					p.interpretGrounded = nil
				}
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
			export := boardfamily.GroundedEvidenceContract
			if protocol == "partitioned-full-v7" {
				export = boardfamily.GroundedFullEvidenceContract
			}
			if protocol == "source-eligible-v8" {
				export = boardfamily.SourceEligibleEvidenceContract
			}
			if protocol == "source-addressed-v9" {
				export = boardfamily.SourceAddressedEvidenceContract
			}
			if protocol == "semantic-boundaries-v10" {
				export = boardfamily.SemanticBoundaryEvidenceContract
			}
			want, err := export(prompt)
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
