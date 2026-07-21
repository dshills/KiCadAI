package behavioralintent

import (
	"bytes"
	"os"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
)

const testCapabilitySHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestCompileReadyRequirementIsDeterministicAndEnforcesFullAcceptance(t *testing.T) {
	prompt := "Filter a broadband input between 18 and 22 kHz and amplify it by 9.5 to 10.5. Protect the output and keep phase margin above 50 degrees. Settle within 200 us across the 10.8 to 13.2 V supply and declared load range."
	proposal := Proposal{
		Version:     ProposalVersion,
		Requirement: validBehavioralRequirement(t),
		Coverage: []CoverageRecord{
			{StatementID: "statement_001", Disposition: DispositionCompiled, Rationale: "declares the external interfaces, supply domain, filter and amplification objectives, and bounded AC behavior", References: []Reference{{Kind: "requirement", ID: "analog_12v"}, {Kind: "requirement", ID: "ground"}, {Kind: "requirement", ID: "power"}, {Kind: "requirement", ID: "input"}, {Kind: "requirement", ID: "output"}, {Kind: "requirement", ID: "filter"}, {Kind: "requirement", ID: "amplify"}, {Kind: "requirement", ID: "cutoff"}, {Kind: "requirement", ID: "passband_gain"}}},
			{StatementID: "statement_002", Disposition: DispositionCompiled, Rationale: "declares output protection and the stability safety limit", References: []Reference{{Kind: "requirement", ID: "protect"}, {Kind: "requirement", ID: "stability"}}},
			{StatementID: "statement_003", Disposition: DispositionCompiled, Rationale: "declares transient settling and the bounded operating corner", References: []Reference{{Kind: "requirement", ID: "settling"}, {Kind: "requirement", ID: "rated_load"}, {Kind: "uncertainty", ID: "supply_range"}}},
		},
		Uncertainties:  []Uncertainty{{ID: "supply_range", Path: "requirements.operating_cases.rated_load", Kind: "operating_corner", Description: "supply variation is bounded by the declared rated-load case", Resolution: ResolutionBounded}},
		Clarifications: []Clarification{},
		CapabilityGaps: []CapabilityGap{},
	}
	proposal.Requirement.Acceptance = architecturesearch.Acceptance{}
	first := Compile(prompt, proposal, testCapabilitySHA256)
	if first.Status != StatusReady || first.Requirement == nil || reports.HasBlockingIssue(first.Issues) {
		t.Fatalf("ready compilation = %#v", first)
	}
	acceptance := first.Requirement.Acceptance
	if !acceptance.RequireERC || !acceptance.RequireStrictDRC || !acceptance.RequireCompleteRouting || !acceptance.RequireConnectivity || !acceptance.RequireWriterCorrectness || !acceptance.RequireRoundTripZeroDiff || !acceptance.RequireDeterministicReplay || !acceptance.RequireSimulation || !acceptance.RequireAllCorners || !acceptance.RequireModelProvenance || !acceptance.RequireClosedLoopEvidence {
		t.Fatalf("mandatory acceptance was not enforced: %#v", acceptance)
	}

	reversed := proposal
	reversed.Coverage = slices.Clone(proposal.Coverage)
	reversed.Coverage[0].References = slices.Clone(proposal.Coverage[0].References)
	slices.Reverse(reversed.Coverage[0].References)
	second := Compile(prompt, reversed, testCapabilitySHA256)
	if second.Status != first.Status || second.Source.SHA256 != first.Source.SHA256 || !slices.Equal(second.Coverage[0].References, first.Coverage[0].References) {
		t.Fatalf("normalized compilation changed: first=%#v second=%#v", first, second)
	}
}

func TestCompileAmbiguousPromptRequestsOneTargetedClarification(t *testing.T) {
	result := Compile("Design a battery-powered sensor interface.", Proposal{
		Version: ProposalVersion,
		Coverage: []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionClarification, Rationale: "the requested battery supply is not electrically bounded", References: []Reference{
			{Kind: "clarification", ID: "input_voltage"},
			{Kind: "uncertainty", ID: "battery_voltage"},
		}}},
		Uncertainties:  []Uncertainty{{ID: "battery_voltage", Path: "requirements.domains.input", Kind: "supply_range", Description: "battery chemistry and input-voltage range are not stated", Resolution: ResolutionClarification, ResolvedBy: "input_voltage"}},
		Clarifications: []Clarification{{ID: "input_voltage", Path: "requirements.domains.input", Question: "What battery chemistry or minimum and maximum input voltage must the circuit support?", WhyNeeded: "component ratings and operating corners cannot be bounded without the input range", UncertaintyIDs: []string{"battery_voltage"}}},
		CapabilityGaps: []CapabilityGap{},
	}, testCapabilitySHA256)
	if result.Status != StatusNeedsClarification || result.Requirement != nil || len(result.Clarifications) != 1 || reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("clarification compilation = %#v", result)
	}
}

func TestCompileUnsupportedPromptProducesStableCapabilityGap(t *testing.T) {
	proposal := Proposal{
		Version: ProposalVersion,
		Coverage: []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionCapabilityGap, Rationale: "the requested behavior depends on unavailable trusted RF verification", References: []Reference{
			{Kind: "capability_gap", ID: "rf_power_verification"},
			{Kind: "uncertainty", ID: "rf_model_evidence"},
		}}},
		Uncertainties:  []Uncertainty{{ID: "rf_model_evidence", Path: "requirements.behavioral_requirements", Kind: "model_coverage", Description: "trusted RF nonlinear and electromagnetic models are unavailable", Resolution: ResolutionCapabilityGap, ResolvedBy: "rf_power_verification"}},
		Clarifications: []Clarification{},
		CapabilityGaps: []CapabilityGap{{ID: "rf_power_verification", Capability: "rf_power_amplifier", Path: "requirements.objectives", Reason: "the autonomous lane has no trusted RF power or electromagnetic verification", RequiredEvidence: []string{"reviewed nonlinear RF models", "registered electromagnetic analysis"}}},
	}
	first := Compile("Design a 2.4 GHz RF power amplifier.", proposal, testCapabilitySHA256)
	second := Compile("Design a 2.4 GHz RF power amplifier.", proposal, testCapabilitySHA256)
	if first.Status != StatusUnsupported || first.Requirement != nil || reports.HasBlockingIssue(first.Issues) {
		t.Fatalf("unsupported compilation = %#v", first)
	}
	if len(first.CapabilityGaps) != 1 || first.CapabilityGaps[0].ID != second.CapabilityGaps[0].ID {
		t.Fatalf("capability gap is not stable: first=%#v second=%#v", first.CapabilityGaps, second.CapabilityGaps)
	}
}

func TestCompileFailsClosedOnMissingCoverageAndDuplicateClarificationPath(t *testing.T) {
	result := Compile("Need gain. Need filtering.", Proposal{Version: ProposalVersion, Coverage: []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionContext, Rationale: "incorrectly treated as context"}}}, testCapabilitySHA256)
	if result.Status != StatusInvalid || result.Requirement != nil || !hasCode(result.Issues, CodeSourceCoverageInvalid) {
		t.Fatalf("missing coverage did not fail closed: %#v", result)
	}

	result = Compile("Need an analog front end.", Proposal{
		Version:       ProposalVersion,
		Coverage:      []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionClarification, Rationale: "input limits are ambiguous", References: []Reference{{Kind: "clarification", ID: "first"}, {Kind: "uncertainty", ID: "one"}}}},
		Uncertainties: []Uncertainty{{ID: "one", Path: "requirements.domains.input", Kind: "voltage", Description: "missing", Resolution: ResolutionClarification, ResolvedBy: "first"}},
		Clarifications: []Clarification{
			{ID: "first", Path: "requirements.domains.input", Question: "Voltage?", WhyNeeded: "required", UncertaintyIDs: []string{"one"}},
			{ID: "second", Path: "requirements.domains.input", Question: "Range?", WhyNeeded: "required", UncertaintyIDs: []string{"one"}},
		},
	}, testCapabilitySHA256)
	if result.Status != StatusInvalid || !hasCode(result.Issues, CodeClarificationInvalid) {
		t.Fatalf("non-minimal clarification did not fail closed: %#v", result)
	}
}

func TestCompileFailsClosedWithoutValidatedCapabilitySnapshot(t *testing.T) {
	result := Compile("Design a battery-powered sensor interface.", ambiguousProposalForCapabilityTest(), "")
	if result.Status != StatusInvalid || !hasCode(result.Issues, CodeCapabilityContextInvalid) {
		t.Fatalf("missing capability snapshot did not fail closed: %#v", result)
	}
}

func TestCompileCoalescesClarificationsAndReverseCoversBlockers(t *testing.T) {
	proposal := ambiguousProposalForCapabilityTest()
	proposal.Uncertainties = append(proposal.Uncertainties, Uncertainty{
		ID: "battery_current", Path: "requirements.domains.input", Kind: "supply_current",
		Description: "available current is not stated", Resolution: ResolutionClarification, ResolvedBy: "input_voltage",
	})
	result := Compile("Design a battery-powered sensor interface.", proposal, testCapabilitySHA256)
	if result.Status != StatusInvalid || !hasCode(result.Issues, CodeClarificationInvalid) {
		t.Fatalf("uncoalesced clarification = %#v", result)
	}

	proposal = ambiguousProposalForCapabilityTest()
	proposal.CapabilityGaps = []CapabilityGap{{ID: "sensor_model", Capability: "sensor_model", Path: "requirements.participants.sensor", Reason: "trusted model missing", RequiredEvidence: []string{"reviewed model"}}}
	result = Compile("Design a battery-powered sensor interface.", proposal, testCapabilitySHA256)
	if result.Status != StatusInvalid || !hasCode(result.Issues, CodeProposalInvalid) || !hasCode(result.Issues, CodeSourceCoverageInvalid) {
		t.Fatalf("mixed or unreferenced blocker = %#v", result)
	}
}

func TestCompileRejectsAdversarialBlockerReferenceMutations(t *testing.T) {
	prompt := "Design a battery-powered sensor interface."
	tests := []struct {
		name   string
		mutate func(*Proposal)
		code   reports.Code
	}{
		{name: "unrelated clarification path", code: CodeClarificationInvalid, mutate: func(proposal *Proposal) {
			proposal.Clarifications[0].Path = "requirements.ports.input"
		}},
		{name: "invented clarification reference", code: CodeSourceCoverageInvalid, mutate: func(proposal *Proposal) {
			proposal.Coverage[0].References[0].ID = "invented_question"
		}},
		{name: "missing uncertainty reference", code: CodeSourceCoverageInvalid, mutate: func(proposal *Proposal) {
			proposal.Coverage[0].References = proposal.Coverage[0].References[:1]
		}},
		{name: "cross namespace identity", code: CodeProposalInvalid, mutate: func(proposal *Proposal) {
			proposal.Uncertainties[0].ID = proposal.Clarifications[0].ID
			proposal.Uncertainties[0].ResolvedBy = proposal.Clarifications[0].ID
			proposal.Clarifications[0].UncertaintyIDs = []string{proposal.Clarifications[0].ID}
			proposal.Coverage[0].References[1].ID = proposal.Clarifications[0].ID
		}},
		{name: "invented gap owner", code: CodeUncertaintyInvalid, mutate: func(proposal *Proposal) {
			proposal.Clarifications = nil
			proposal.Uncertainties[0].Resolution = ResolutionCapabilityGap
			proposal.Uncertainties[0].ResolvedBy = "invented_gap"
			proposal.Coverage[0].Disposition = DispositionCapabilityGap
			proposal.Coverage[0].References = []Reference{{Kind: "uncertainty", ID: "battery_voltage"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal := ambiguousProposalForCapabilityTest()
			test.mutate(&proposal)
			result := Compile(prompt, proposal, testCapabilitySHA256)
			if result.Status != StatusInvalid || result.Requirement != nil || !hasCode(result.Issues, test.code) {
				t.Fatalf("mutation result = %#v", result)
			}
		})
	}
}

func ambiguousProposalForCapabilityTest() Proposal {
	return Proposal{
		Version:        ProposalVersion,
		Coverage:       []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionClarification, Rationale: "input range is missing", References: []Reference{{Kind: "clarification", ID: "input_voltage"}, {Kind: "uncertainty", ID: "battery_voltage"}}}},
		Uncertainties:  []Uncertainty{{ID: "battery_voltage", Path: "requirements.domains.input", Kind: "supply_range", Description: "battery voltage is not stated", Resolution: ResolutionClarification, ResolvedBy: "input_voltage"}},
		Clarifications: []Clarification{{ID: "input_voltage", Path: "requirements.domains.input", Question: "What input range is required?", WhyNeeded: "ratings require a bound", UncertaintyIDs: []string{"battery_voltage"}}},
		CapabilityGaps: []CapabilityGap{},
	}
}

func validBehavioralRequirement(t *testing.T) *architecturesearch.Requirement {
	t.Helper()
	data, err := os.ReadFile("../architecturesearch/testdata/simulation_grounded_closed_loop_corpus/active_filter_amplifier.json")
	if err != nil {
		t.Fatal(err)
	}
	requirement, issues := architecturesearch.DecodeStrict(bytes.NewReader(data))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("fixture requirement issues = %#v", issues)
	}
	return &requirement
}

func hasCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
