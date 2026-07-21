package behavioralintent

import (
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestFollowUpBindsAnswersToExactPriorEvidence(t *testing.T) {
	prompt := "Design a battery-powered sensor interface."
	priorProposal := ambiguousProposalForCapabilityTest()
	prior := Compile(prompt, priorProposal, testCapabilitySHA256)
	followUp, err := BindFollowUp(priorProposal, prior, []ClarificationAnswer{{
		ClarificationID: "input_voltage", UncertaintyIDs: []string{"battery_voltage"}, Answer: "Support 3.0 V through 4.2 V.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	normalized, issues := ValidateFollowUp(prompt, priorProposal, prior, followUp, testCapabilitySHA256)
	if reports.HasBlockingIssue(issues) || normalized.Answers[0].Answer != "Support 3.0 V through 4.2 V." {
		t.Fatalf("normalized=%#v issues=%#v", normalized, issues)
	}

	capabilities := testInstalledCapabilities(t)
	baseJSON, err := BuildProviderContext(prompt, capabilities)
	if err != nil {
		t.Fatal(err)
	}
	var base ProviderContext
	if err := json.Unmarshal([]byte(baseJSON), &base); err != nil {
		t.Fatal(err)
	}
	contextPrior := Compile(prompt, priorProposal, base.CapabilitySHA256)
	contextFollowUp, err := BindFollowUp(priorProposal, contextPrior, followUp.Answers)
	if err != nil {
		t.Fatal(err)
	}
	contextJSON, contextIssues := BuildFollowUpProviderContext(prompt, capabilities, priorProposal, contextPrior, contextFollowUp)
	if reports.HasBlockingIssue(contextIssues) {
		t.Fatalf("context issues = %#v", contextIssues)
	}
	var context ProviderContext
	if err := json.Unmarshal([]byte(contextJSON), &context); err != nil {
		t.Fatal(err)
	}
	if context.FollowUp == nil || context.Source.SHA256 != contextPrior.Source.SHA256 || context.FollowUp.Input.Answers[0].ClarificationID != "input_voltage" || strings.Contains(context.Source.Statements[0].Text, "3.0 V") {
		t.Fatalf("follow-up context = %#v", context)
	}
}

func TestFollowUpRejectsUnrelatedMissingInventedAndCircularReferences(t *testing.T) {
	prompt := "Design a battery-powered sensor interface."
	priorProposal := ambiguousProposalForCapabilityTest()
	prior := Compile(prompt, priorProposal, testCapabilitySHA256)
	valid, err := BindFollowUp(priorProposal, prior, []ClarificationAnswer{{ClarificationID: "input_voltage", UncertaintyIDs: []string{"battery_voltage"}, Answer: "3.0 V to 4.2 V"}})
	if err != nil {
		t.Fatal(err)
	}

	mutations := []func(*FollowUp){
		func(value *FollowUp) { value.SourceSHA256 = strings.Repeat("0", 64) },
		func(value *FollowUp) { value.Answers[0].ClarificationID = "invented_question" },
		func(value *FollowUp) { value.Answers[0].UncertaintyIDs = nil },
		func(value *FollowUp) { value.Answers = append(value.Answers, value.Answers[0]) },
	}
	for index, mutate := range mutations {
		encoded, err := json.Marshal(valid)
		if err != nil {
			t.Fatal(err)
		}
		var mutation FollowUp
		if err := json.Unmarshal(encoded, &mutation); err != nil {
			t.Fatal(err)
		}
		mutate(&mutation)
		_, issues := ValidateFollowUp(prompt, priorProposal, prior, mutation, testCapabilitySHA256)
		if !hasCode(issues, CodeFollowUpInvalid) {
			t.Fatalf("mutation %d did not fail closed: %#v", index, issues)
		}
	}

	circular := CompileFollowUp(prompt, priorProposal, prior, valid, priorProposal, testCapabilitySHA256)
	if circular.Status != StatusInvalid || circular.Requirement != nil || !hasCode(circular.Issues, CodeFollowUpInvalid) {
		t.Fatalf("circular follow-up = %#v", circular)
	}
}

func TestDecodeFollowUpStrictRejectsUnknownFields(t *testing.T) {
	_, issues := DecodeFollowUpStrict(strings.NewReader(`{"schema":"kicadai.behavioral-intent-follow-up.v1","version":1,"source_sha256":"x","capability_sha256":"x","prior_proposal_sha256":"x","prior_compilation_sha256":"x","answers":[],"invented":true}`))
	if !hasCode(issues, CodeFollowUpInvalid) {
		t.Fatalf("strict decode issues = %#v", issues)
	}
}
