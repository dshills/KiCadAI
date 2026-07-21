package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/aiprovider"
	"kicadai/internal/architecturesearch"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestInstalledBehavioralCapabilitiesIgnoreCatalogLoadTimestamp(t *testing.T) {
	catalogDir, err := filepath.Abs(filepath.Join("..", "..", "data", "components"))
	if err != nil {
		t.Fatal(err)
	}
	firstCatalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{CatalogDir: catalogDir})
	if err != nil {
		t.Fatal(err)
	}
	secondCatalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{CatalogDir: catalogDir})
	if err != nil {
		t.Fatal(err)
	}
	if firstCatalog.GeneratedAt == nil || secondCatalog.GeneratedAt == nil || firstCatalog.GeneratedAt.Equal(*secondCatalog.GeneratedAt) {
		t.Fatal("test requires distinct volatile catalog load timestamps")
	}
	firstRegistry, firstIssues := architecturesearch.NewCatalogRegistry(firstCatalog)
	secondRegistry, secondIssues := architecturesearch.NewCatalogRegistry(secondCatalog)
	if reports.HasBlockingIssue(firstIssues) || reports.HasBlockingIssue(secondIssues) {
		t.Fatalf("registry issues: first=%#v second=%#v", firstIssues, secondIssues)
	}
	first, err := installedBehavioralCapabilities(firstCatalog, firstRegistry)
	if err != nil {
		t.Fatal(err)
	}
	second, err := installedBehavioralCapabilities(secondCatalog, secondRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("capability snapshots changed with volatile load time:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestRunBehavioralIntentCompileRecordedClarificationPersistsEvidence(t *testing.T) {
	proposal := ambiguousBehavioralProposal()
	record := writeBehavioralRecordedResponse(t, proposal)
	catalogDir, err := filepath.Abs(filepath.Join("..", "..", "data", "components"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "compiled")
	opts := cliOptions{
		intentText: "Design a battery-powered sensor interface.", aiProvider: "recorded", aiProviderRecord: record,
		aiProfile: aiprovider.BehavioralIntentProfileID, maxAIAttempts: 1, catalogDir: catalogDir,
		output: output, outputFormat: "json", jsonOutput: true,
	}
	var stdout bytes.Buffer
	if err := runBehavioralIntentCompile(context.Background(), opts, &stdout); err != nil {
		t.Fatalf("intent compile: %v\n%s", err, stdout.String())
	}
	var response struct {
		OK   bool `json:"ok"`
		Data struct {
			Compilation behavioralintent.Result `json:"compilation"`
			Attempts    []aiAttemptEvidence     `json:"attempts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.Compilation.Status != behavioralintent.StatusNeedsClarification || response.Data.Compilation.Requirement != nil || len(response.Data.Attempts) != 1 {
		t.Fatalf("compile response = %#v", response)
	}
	for _, name := range []string{"behavioral-source.txt", "behavioral-capabilities.json", "behavioral-proposal.json", "behavioral-compilation.json", "behavioral-attempts.json", "behavioral-follow-up-template.json"} {
		if _, err := os.Stat(filepath.Join(output, ".kicadai", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestRunBehavioralIntentCompileRecordedFollowUpUsesBoundPriorArtifacts(t *testing.T) {
	prompt := "Design a battery-powered sensor interface."
	firstRecord := writeBehavioralRecordedResponse(t, ambiguousBehavioralProposal())
	catalogDir, err := filepath.Abs(filepath.Join("..", "..", "data", "components"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "compiled")
	firstOpts := cliOptions{
		intentText: prompt, aiProvider: "recorded", aiProviderRecord: firstRecord,
		aiProfile: aiprovider.BehavioralIntentProfileID, maxAIAttempts: 1, catalogDir: catalogDir,
		output: output, outputFormat: "json", jsonOutput: true,
	}
	var firstStdout bytes.Buffer
	if err := runBehavioralIntentCompile(context.Background(), firstOpts, &firstStdout); err != nil {
		t.Fatalf("initial intent compile: %v\n%s", err, firstStdout.String())
	}
	proposalFile, err := os.Open(filepath.Join(output, ".kicadai", "behavioral-proposal.json"))
	if err != nil {
		t.Fatal(err)
	}
	priorProposal, proposalIssues := behavioralintent.DecodeProposalStrict(proposalFile)
	if err := proposalFile.Close(); err != nil {
		t.Fatal(err)
	}
	if reports.HasBlockingIssue(proposalIssues) {
		t.Fatalf("prior proposal issues = %#v", proposalIssues)
	}
	var priorCompilation behavioralintent.Result
	if err := decodeBehavioralArtifact(filepath.Join(output, ".kicadai", "behavioral-compilation.json"), &priorCompilation); err != nil {
		t.Fatal(err)
	}
	followUp, err := behavioralintent.BindFollowUp(priorProposal, priorCompilation, []behavioralintent.ClarificationAnswer{{
		ClarificationID: "input_voltage", UncertaintyIDs: []string{"battery_voltage"}, Answer: "Support 3.0 V through 4.2 V.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	followUpData, err := json.Marshal(followUp)
	if err != nil {
		t.Fatal(err)
	}
	followUpPath := filepath.Join(t.TempDir(), "follow-up.json")
	if err := os.WriteFile(followUpPath, followUpData, 0o644); err != nil {
		t.Fatal(err)
	}
	unsupported := behavioralintent.Proposal{
		Version:        behavioralintent.ProposalVersion,
		Coverage:       []behavioralintent.CoverageRecord{{StatementID: "statement_001", Disposition: behavioralintent.DispositionCapabilityGap, Rationale: "trusted battery-life verification is unavailable", References: []behavioralintent.Reference{{Kind: "capability_gap", ID: "battery_life_verification"}, {Kind: "uncertainty", ID: "battery_life_model"}}}},
		Uncertainties:  []behavioralintent.Uncertainty{{ID: "battery_life_model", Path: "requirements.behavioral_requirements", Kind: "model_coverage", Description: "trusted battery aging behavior is unavailable", Resolution: behavioralintent.ResolutionCapabilityGap, ResolvedBy: "battery_life_verification"}},
		Clarifications: []behavioralintent.Clarification{},
		CapabilityGaps: []behavioralintent.CapabilityGap{{ID: "battery_life_verification", Capability: "battery_life_verification", Path: "requirements.behavioral_requirements", Reason: "installed trusted analyses do not cover battery aging", RequiredEvidence: []string{"reviewed battery aging model"}}},
	}
	secondRecord := writeBehavioralRecordedResponse(t, unsupported)
	secondOpts := firstOpts
	secondOpts.aiProviderRecord = secondRecord
	secondOpts.behavioralFollowUp = followUpPath
	secondOpts.overwrite = true
	var secondStdout bytes.Buffer
	if err := runBehavioralIntentCompile(context.Background(), secondOpts, &secondStdout); err != nil {
		t.Fatalf("follow-up intent compile: %v\n%s", err, secondStdout.String())
	}
	var response struct {
		OK   bool                          `json:"ok"`
		Data behavioralIntentCompileOutput `json:"data"`
	}
	if err := json.Unmarshal(secondStdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.FollowUp == nil || response.Data.Compilation.Status != behavioralintent.StatusUnsupported || response.Data.Compilation.Requirement != nil {
		t.Fatalf("follow-up response = %#v", response)
	}
	if _, err := os.Stat(filepath.Join(output, ".kicadai", "behavioral-follow-up.json")); err != nil {
		t.Fatalf("missing follow-up evidence: %v", err)
	}
}

func TestGenerateValidatedBehavioralIntentRetriesInvalidButNotClarification(t *testing.T) {
	valid, err := json.Marshal(ambiguousBehavioralProposal())
	if err != nil {
		t.Fatal(err)
	}
	provider := &sequenceAIProvider{results: []aiprovider.GenerateResult{
		{Provider: "sequence", IntentJSON: json.RawMessage(`{"version":1}`)},
		{Provider: "sequence", IntentJSON: valid},
	}}
	result, _, compilation, attempts, err := generateValidatedBehavioralIntent(context.Background(), provider, aiprovider.BehavioralIntentProfile(`{"capabilities":true}`), "Design a battery-powered sensor interface.", testBehavioralCapabilitySHA256, 2)
	if err != nil || result.Provider != "sequence" || compilation.Status != behavioralintent.StatusNeedsClarification || len(attempts) != 2 || len(attempts[1].Diagnostics) == 0 {
		t.Fatalf("result=%#v compilation=%#v attempts=%#v err=%v", result, compilation, attempts, err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("provider calls = %d", len(provider.requests))
	}
}

func TestGenerateValidatedBehavioralIntentReadyV3(t *testing.T) {
	proposal := readyBehavioralProposal(t)
	data, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	provider := &sequenceAIProvider{results: []aiprovider.GenerateResult{{Provider: "sequence", IntentJSON: data}}}
	_, _, compilation, attempts, err := generateValidatedBehavioralIntent(context.Background(), provider, aiprovider.BehavioralIntentProfile(`{"capabilities":true}`), readyBehavioralPrompt, testBehavioralCapabilitySHA256, 2)
	if err != nil || compilation.Status != behavioralintent.StatusReady || compilation.Requirement == nil || len(attempts) != 1 || reports.HasBlockingIssue(compilation.Issues) {
		t.Fatalf("compilation=%#v attempts=%#v err=%v", compilation, attempts, err)
	}
}

func TestRunBehavioralIntentCompileRecordedReadyPersistsSearchEvidence(t *testing.T) {
	record := writeBehavioralRecordedResponse(t, readyBehavioralProposal(t))
	catalogDir, err := filepath.Abs(filepath.Join("..", "..", "data", "components"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "compiled")
	opts := cliOptions{
		intentText: readyBehavioralPrompt, aiProvider: "recorded", aiProviderRecord: record,
		aiProfile: aiprovider.BehavioralIntentProfileID, maxAIAttempts: 1, catalogDir: catalogDir,
		output: output, outputFormat: "json", jsonOutput: true,
	}
	var stdout bytes.Buffer
	if err := runBehavioralIntentCompile(context.Background(), opts, &stdout); err != nil {
		t.Fatalf("intent compile: %v\n%s", err, stdout.String())
	}
	var response struct {
		OK   bool                          `json:"ok"`
		Data behavioralIntentCompileOutput `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.Search == nil || response.Data.Search.Status != architecturesearch.SearchSelected || response.Data.ClosedLoop == nil || response.Data.ClosedLoop.Status != "pass" || response.Data.Compilation.Status != behavioralintent.StatusReady || response.Data.Compilation.Requirement == nil {
		closedLoop, _ := json.MarshalIndent(response.Data.ClosedLoop, "", "  ")
		t.Fatalf("compile response = %#v\nclosed loop = %s", response, closedLoop)
	}
	if _, err := os.Stat(filepath.Join(output, ".kicadai", "behavioral-architecture-search.json")); err != nil {
		t.Fatalf("missing architecture search evidence: %v", err)
	}
	for _, name := range []string{"behavioral-closed-loop.json", "behavioral-design-request.json"} {
		if _, err := os.Stat(filepath.Join(output, ".kicadai", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	replayOutput := filepath.Join(t.TempDir(), "replay")
	replayOpts := opts
	replayOpts.output = replayOutput
	var replayStdout bytes.Buffer
	if err := runBehavioralIntentCompile(context.Background(), replayOpts, &replayStdout); err != nil {
		t.Fatalf("replay intent compile: %v\n%s", err, replayStdout.String())
	}
	for _, name := range []string{
		"behavioral-source.txt", "behavioral-capabilities.json", "behavioral-proposal.json", "behavioral-compilation.json",
		"behavioral-attempts.json", "behavioral-architecture-search.json", "behavioral-closed-loop.json", "behavioral-design-request.json",
	} {
		first, err := os.ReadFile(filepath.Join(output, ".kicadai", name))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(replayOutput, ".kicadai", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("recorded replay artifact %s differs", name)
		}
	}
}

const readyBehavioralPrompt = "Filter a broadband input between 18 and 22 kHz and amplify it by 9.5 to 10.5. Protect the output and keep phase margin above 50 degrees. Settle within 200 us across the 10.8 to 13.2 V supply and declared load range."

func readyBehavioralProposal(t *testing.T) behavioralintent.Proposal {
	t.Helper()
	return behavioralintent.Proposal{
		Version:     behavioralintent.ProposalVersion,
		Requirement: behavioralRequirementForCLI(t),
		Coverage: []behavioralintent.CoverageRecord{
			{StatementID: "statement_001", Disposition: behavioralintent.DispositionCompiled, Rationale: "interfaces, filter, and gain requirements", References: []behavioralintent.Reference{{Kind: "requirement", ID: "analog_12v"}, {Kind: "requirement", ID: "ground"}, {Kind: "requirement", ID: "power"}, {Kind: "requirement", ID: "input"}, {Kind: "requirement", ID: "output"}, {Kind: "requirement", ID: "filter"}, {Kind: "requirement", ID: "amplify"}, {Kind: "requirement", ID: "cutoff"}, {Kind: "requirement", ID: "passband_gain"}}},
			{StatementID: "statement_002", Disposition: behavioralintent.DispositionCompiled, Rationale: "protection and stability requirements", References: []behavioralintent.Reference{{Kind: "requirement", ID: "protect"}, {Kind: "requirement", ID: "stability"}}},
			{StatementID: "statement_003", Disposition: behavioralintent.DispositionCompiled, Rationale: "settling and operating corners", References: []behavioralintent.Reference{{Kind: "requirement", ID: "settling"}, {Kind: "requirement", ID: "rated_load"}, {Kind: "uncertainty", ID: "supply_range"}}},
		},
		Uncertainties:  []behavioralintent.Uncertainty{{ID: "supply_range", Path: "requirements.operating_cases.rated_load", Kind: "operating_corner", Description: "supply and load are explicitly bounded", Resolution: behavioralintent.ResolutionBounded}},
		Clarifications: []behavioralintent.Clarification{}, CapabilityGaps: []behavioralintent.CapabilityGap{},
	}
}

const testBehavioralCapabilitySHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func ambiguousBehavioralProposal() behavioralintent.Proposal {
	return behavioralintent.Proposal{
		Version:        behavioralintent.ProposalVersion,
		Coverage:       []behavioralintent.CoverageRecord{{StatementID: "statement_001", Disposition: behavioralintent.DispositionClarification, Rationale: "battery voltage is ambiguous", References: []behavioralintent.Reference{{Kind: "clarification", ID: "input_voltage"}, {Kind: "uncertainty", ID: "battery_voltage"}}}},
		Uncertainties:  []behavioralintent.Uncertainty{{ID: "battery_voltage", Path: "requirements.domains.input", Kind: "supply_range", Description: "battery chemistry and voltage range are not stated", Resolution: behavioralintent.ResolutionClarification, ResolvedBy: "input_voltage"}},
		Clarifications: []behavioralintent.Clarification{{ID: "input_voltage", Path: "requirements.domains.input", Question: "What battery chemistry or minimum and maximum input voltage must be supported?", WhyNeeded: "ratings and operating corners require a bounded input", UncertaintyIDs: []string{"battery_voltage"}}},
		CapabilityGaps: []behavioralintent.CapabilityGap{},
	}
}

func writeBehavioralRecordedResponse(t *testing.T, proposal behavioralintent.Proposal) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"schema": aiprovider.EnvelopeSchemaV1, "intent": proposal})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "recorded.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func behavioralRequirementForCLI(t *testing.T) *architecturesearch.Requirement {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "architecturesearch", "testdata", "simulation_grounded_closed_loop_corpus", "active_filter_amplifier.json"))
	if err != nil {
		t.Fatal(err)
	}
	requirement, issues := architecturesearch.DecodeStrict(bytes.NewReader(data))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("requirement issues = %#v", issues)
	}
	return &requirement
}
