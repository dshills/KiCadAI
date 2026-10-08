package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/boardfamily"
)

func TestSourceEligibleCommandSyntheticCorpus(t *testing.T) {
	testGroundedCommandSyntheticCorpus(t, "source-eligible-v8")
}

func TestSourceEligibleCommandFailureGates(t *testing.T) {
	testGroundedCommandFailureGates(t, "source-eligible-v8")
}

func TestSourceEligibleFlagsAndContract(t *testing.T) {
	testGroundedFlagsAndContract(t, "source-eligible-v8")
}

func TestSourceEligibleCommandNative(t *testing.T) {
	testPartitionedCandidateCommandNative(t, "source-eligible-v8")
}

func TestSourceEligibleInventedFeatureCannotReachGeneration(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
	prompt := "Use BMP280 standard with a wired connection."
	raw := []byte(`{"version":"8-source-eligible-evidence-experimental","requirements":[{"kind":"sensor","value":"BMP280","state":"requested","evidence":["c0"]},{"kind":"profile","value":"standard","state":"requested","evidence":["c0"]},{"kind":"feature","value":"delivered_firmware","state":"must_not_occur","anchor":"c0","evidence":["c0"]}],"quantities":{}}`)
	root := t.TempDir()
	out, journal, ledger, budget := filepath.Join(root, "out"), filepath.Join(root, "journal"), filepath.Join(root, "ledger.json"), filepath.Join(root, "budget.json")
	if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-eligible-invention", MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
		t.Fatal(err)
	}
	p, counts := evidenceCommandPipeline(t, raw, "", "", "source-eligible-v8")
	if _, err := runIndexedCommand(t, p, "--intent-protocol", "source-eligible-v8", "--prompt", prompt, "--live-budget", budget, "--ledger", ledger, "--evidence-journal", journal, "--output", out); err == nil {
		t.Fatal("invented feature admitted")
	}
	if counts.requests != 1 || counts.generate != 0 || counts.validate != 0 {
		t.Fatal("invalid extraction reached native work", counts)
	}
	a, err := boardfamily.InspectSourceEligibleJournal(journal)
	if err != nil || a.Outcome != "invalid_extraction" || a.Selection.Decision.Configuration != nil {
		t.Fatal("failure did not retain auditable evidence", err)
	}
	var selection boardfamily.ReferencedSelection
	b, err := os.ReadFile(filepath.Join(out, "selection.json"))
	if err != nil || json.Unmarshal(b, &selection) != nil || selection.Outcome != "invalid_extraction" {
		t.Fatal("failure output missing", err)
	}
}
