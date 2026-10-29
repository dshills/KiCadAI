package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"kicadai/internal/aiprovider"
	"kicadai/internal/atomicfile"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/schematicassistant"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchematicAssistantCLIOfflineAndResumable(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "wizard")
	var stdout, prompts bytes.Buffer
	opts := cliOptions{commandArgs: []string{"new"}, output: out}
	err := runSchematicAssistant(context.Background(), opts, strings.NewReader("my_divider\ndivider\n5\n"), &stdout, &prompts)
	if err == nil {
		t.Fatal("EOF authorized generation")
	}
	if _, err = os.Stat(filepath.Join(out, "project")); !os.IsNotExist(err) {
		t.Fatal("unconfirmed project written")
	}
	opts.commandArgs = []string{"resume"}
	if err = runSchematicAssistant(context.Background(), opts, strings.NewReader("10k\n0\n10k\nyes\n"), &stdout, &prompts); err != nil {
		t.Fatal(err)
	}
	if err = schematicassistant.Verify(filepath.Join(out, "project")); err != nil {
		t.Fatal(err)
	}
	opts.commandArgs = []string{"generate"}
	opts.requestPath = filepath.Join(out, "session.json")
	opts.output = filepath.Join(root, "regenerated")
	if err = runSchematicAssistant(context.Background(), opts, strings.NewReader(""), &stdout, &prompts); err != nil {
		t.Fatal(err)
	}
}

func TestSchematicAssistantResumeLocksBeforeReadingSession(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "session.json"), []byte("{not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	lock, err := atomicfile.AcquireLock(filepath.Join(root, ".assistant-session.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Release(); err != nil {
			t.Error(err)
		}
	}()
	var output bytes.Buffer
	err = runSchematicAssistant(context.Background(), cliOptions{commandArgs: []string{"resume"}, output: root}, strings.NewReader(""), &output, &output)
	if !errors.Is(err, atomicfile.ErrLockHeld) {
		t.Fatalf("expected lock error before malformed-session read, got %v", err)
	}
}
func TestSchematicAssistantCLICommandsAndSafety(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"schematic", "list"}, &output, &output); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"new", "extra"}, {"example", "unknown"}, {"generate"}, {"bogus"}} {
		opts := cliOptions{commandArgs: args, output: filepath.Join(t.TempDir(), "out")}
		if err := runSchematicAssistant(context.Background(), opts, strings.NewReader(""), &output, &output); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	opts := cliOptions{commandArgs: []string{"example", "divider"}, output: filepath.Join(t.TempDir(), "out"), overwrite: true}
	if err := runSchematicAssistant(context.Background(), opts, strings.NewReader(""), &output, &output); err == nil {
		t.Fatal("accepted overwrite")
	}
}
func clarificationWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "prior")
	catalog, err := filepath.Abs("../../data/components")
	if err != nil {
		t.Fatal(err)
	}
	opts := cliOptions{intentText: "Design a battery-powered sensor interface.", aiProvider: "recorded", aiProviderRecord: writeBehavioralRecordedResponse(t, ambiguousBehavioralProposal()), aiProfile: aiprovider.BehavioralIntentProfileID, maxAIAttempts: 1, catalogDir: catalog, output: root, jsonOutput: true}
	var stdout bytes.Buffer
	if err = runBehavioralIntentCompile(context.Background(), opts, &stdout); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestIntentClarifyCollectsBoundAnswersWithoutMutatingPrior(t *testing.T) {
	prior := clarificationWorkspace(t)
	before, err := os.ReadFile(filepath.Join(prior, ".kicadai", "behavioral-compilation.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "answers")
	opts := cliOptions{commandArgs: []string{"clarify"}, target: prior, output: out}
	var stdout, prompts bytes.Buffer
	if err = runIntentClarify(context.Background(), opts, strings.NewReader("Support 3.0 V through 4.2 V.\n"), &stdout, &prompts); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "answers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var followUp behavioralintent.FollowUp
	if err = json.Unmarshal(raw, &followUp); err != nil {
		t.Fatal(err)
	}
	if len(followUp.Answers) != 1 || followUp.Answers[0].Answer != "Support 3.0 V through 4.2 V." || followUp.PriorCompilationSHA256 == "" {
		t.Fatalf("%+v", followUp)
	}
	after, _ := os.ReadFile(filepath.Join(prior, ".kicadai", "behavioral-compilation.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("prior compilation overwritten")
	}
	if !strings.Contains(stdout.String(), "\"api_requests\": 0") {
		t.Fatal(stdout.String())
	}
}
func TestIntentClarifyRejectsStaleEvidenceAndCancel(t *testing.T) {
	prior := clarificationWorkspace(t)
	for _, answer := range []string{"", "cancel\n"} {
		opts := cliOptions{commandArgs: []string{"clarify"}, target: prior, output: filepath.Join(t.TempDir(), "out")}
		var stdout bytes.Buffer
		if err := runIntentClarify(context.Background(), opts, strings.NewReader(answer), &stdout, &stdout); err == nil {
			t.Fatal("accepted empty/cancelled answer")
		}
		if _, err := os.Stat(opts.output); !os.IsNotExist(err) {
			t.Fatal("cancel wrote workspace")
		}
	}
	if err := os.WriteFile(filepath.Join(prior, ".kicadai", "behavioral-source.txt"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := cliOptions{commandArgs: []string{"clarify"}, target: prior, output: filepath.Join(t.TempDir(), "out")}
	var stdout bytes.Buffer
	if err := runIntentClarify(context.Background(), opts, strings.NewReader("3.3 V\n"), &stdout, &stdout); err == nil {
		t.Fatal("accepted stale evidence")
	}
}

func TestIntentClarifyRecordedResumeRemainsFailClosed(t *testing.T) {
	prior := clarificationWorkspace(t)
	unsupported := behavioralintent.Proposal{
		Version:        behavioralintent.ProposalVersion,
		Coverage:       []behavioralintent.CoverageRecord{{StatementID: "statement_001", Disposition: behavioralintent.DispositionCapabilityGap, Rationale: "battery aging cannot be verified", References: []behavioralintent.Reference{{Kind: "capability_gap", ID: "battery_life_verification"}, {Kind: "uncertainty", ID: "battery_life_model"}}}},
		Uncertainties:  []behavioralintent.Uncertainty{{ID: "battery_life_model", Path: "requirements.behavioral_requirements", Kind: "model_coverage", Description: "battery aging is unavailable", Resolution: behavioralintent.ResolutionCapabilityGap, ResolvedBy: "battery_life_verification"}},
		Clarifications: []behavioralintent.Clarification{},
		CapabilityGaps: []behavioralintent.CapabilityGap{{ID: "battery_life_verification", Capability: "battery_life_verification", Path: "requirements.behavioral_requirements", Reason: "no reviewed battery aging model", RequiredEvidence: []string{"reviewed model"}}},
	}
	opts := cliOptions{commandArgs: []string{"clarify"}, target: prior, output: filepath.Join(t.TempDir(), "answered"), aiProvider: "recorded", aiProviderRecord: writeBehavioralRecordedResponse(t, unsupported)}
	var output, prompts bytes.Buffer
	if err := runIntentClarify(context.Background(), opts, strings.NewReader("Support 3.0 V through 4.2 V.\n"), &output, &prompts); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data behavioralIntentCompileOutput `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.FollowUp == nil || response.Data.Compilation.Status != behavioralintent.StatusUnsupported || response.Data.Compilation.Requirement != nil {
		t.Fatalf("invalid resume result: %s", output.String())
	}
}
