package capabilityexpansion

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/capabilitygate"
)

func TestPlansThreeUnsupportedDomainsDeterministically(t *testing.T) {
	tests := []struct {
		domain string
		gap    capabilitygate.Gap
		kind   NeedKind
	}{
		{
			domain: "analog",
			gap: capabilitygate.Gap{Code: "ARCHITECTURE_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementArchitecture,
				ID: "precision_buffering", Stage: "architecture_selection", Reason: "no provider"},
			kind: NeedArchitecture,
		},
		{
			domain: "power",
			gap: capabilitygate.Gap{Code: "MODEL_PROVENANCE_MISSING", Kind: capabilitygate.RequirementModel,
				ID: "switching_loss_model", Stage: "simulation", Reason: "no reviewed dynamic model"},
			kind: NeedModel,
		},
		{
			domain: "digital",
			gap: capabilitygate.Gap{Code: "ROUTE_LAYER_TRANSITION_UNSUPPORTED", Kind: capabilitygate.RequirementPhysical,
				ID: "differential_pair_escape", Stage: "routing", Reason: "no endpoint transition policy"},
			kind: NeedRouting,
		},
	}
	for _, test := range tests {
		t.Run(test.domain, func(t *testing.T) {
			assessment := unsupportedAssessment(t, test.domain, []capabilitygate.Gap{test.gap})
			first, err := Plan(assessment)
			if err != nil {
				t.Fatal(err)
			}
			second, err := Plan(assessment)
			if err != nil {
				t.Fatal(err)
			}
			firstJSON, _ := MarshalJSONStable(first)
			secondJSON, _ := MarshalJSONStable(second)
			if string(firstJSON) != string(secondJSON) {
				t.Fatalf("plan replay differs\n%s\n%s", firstJSON, secondJSON)
			}
			pointerJSON, err := MarshalJSONStable(&first)
			if err != nil {
				t.Fatal(err)
			}
			if string(firstJSON) != string(pointerJSON) {
				t.Fatalf("pointer serialization differs\n%s\n%s", firstJSON, pointerJSON)
			}
			if len(first.Needs) != 1 || first.Needs[0].Kind != test.kind ||
				!slices.Contains(first.Domains, test.domain) {
				t.Fatalf("plan = %#v", first)
			}
		})
	}
}

func TestPlanRejectsUnboundedGapSet(t *testing.T) {
	gaps := make([]capabilitygate.Gap, MaxExpansionNeeds+1)
	for index := range gaps {
		gaps[index] = capabilitygate.Gap{
			Code: "ARCHITECTURE_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementArchitecture,
			ID: fmt.Sprintf("capability_%03d", index), Stage: "architecture_selection", Reason: "no provider",
		}
	}
	assessment := unsupportedAssessment(t, "stress", gaps)
	if _, err := Plan(assessment); err == nil {
		t.Fatalf("Plan() accepted more than %d gaps", MaxExpansionNeeds)
	}
}

func TestCanonicalSortedIDsRejectsEmptyNonCanonicalAndDuplicateDomains(t *testing.T) {
	for _, domains := range [][]string{
		nil,
		{"Analog"},
		{"analog", "analog"},
		{"power", "analog"},
		{"***"},
	} {
		if canonicalSortedIDs(domains) {
			t.Fatalf("canonicalSortedIDs(%q) = true", domains)
		}
	}
	if !canonicalSortedIDs([]string{"analog", "digital", "power"}) {
		t.Fatal("canonical sorted domains were rejected")
	}
}

func TestCandidateSourceBoundsAndCanonicalSort(t *testing.T) {
	assessment := unsupportedAssessment(t, "analog", []capabilitygate.Gap{{
		Code: "ARCHITECTURE_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementArchitecture,
		ID: "bounded_source_test", Stage: "architecture_selection", Reason: "no provider",
	}})
	plan, err := Plan(assessment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ingestSources(plan, make([]SourceInput, MaxCandidateSources+1)); err == nil {
		t.Fatalf("ingestSources() accepted more than %d sources", MaxCandidateSources)
	}

	content := make([]byte, maxSourceBytes)
	digest := sourceContentSHA256(content)
	inputs := make([]SourceInput, MaxCandidateSourceBytes/maxSourceBytes+1)
	for index := range inputs {
		inputs[index] = SourceInput{
			ID: fmt.Sprintf("source_%02d", index), Kind: SourceEngineeringReference,
			Publisher: "Independent Publisher",
			Locator:   fmt.Sprintf("https://example.invalid/source/%02d", index),
			Claims:    []string{plan.Needs[0].ID}, Content: content, ExpectedSHA256: digest,
		}
	}
	if _, err := ingestSources(plan, inputs); err == nil {
		t.Fatalf("ingestSources() accepted more than %d aggregate bytes", MaxCandidateSourceBytes)
	}

	candidate := CandidateRegistry{Artifacts: []CapabilityArtifact{
		{ID: "Zulu", Payload: json.RawMessage(`{"id":"zulu"}`)},
		{ID: "alpha", Payload: json.RawMessage(`{"id":"alpha"}`)},
	}}
	if err := normalizeCandidate(&candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Artifacts[0].ID != "alpha" || candidate.Artifacts[1].ID != "zulu" {
		t.Fatalf("canonical artifact order = %#v", candidate.Artifacts)
	}
}

func TestEvidenceIngestionFailsClosed(t *testing.T) {
	assessment := unsupportedAssessment(t, "analog", []capabilitygate.Gap{{
		Code: "ARCHITECTURE_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementArchitecture,
		ID: "precision_buffering", Stage: "architecture_selection", Reason: "no provider",
	}})
	plan, err := Plan(assessment)
	if err != nil {
		t.Fatal(err)
	}
	provider, artifact := providerAndArtifact(t, plan.Needs[0], "precision_buffer_provider")
	tests := []struct {
		name   string
		mutate func([]SourceInput) []SourceInput
	}{
		{
			name: "fabricated_digest",
			mutate: func(inputs []SourceInput) []SourceInput {
				inputs[0].ExpectedSHA256 = sourceContentSHA256([]byte("different"))
				return inputs
			},
		},
		{
			name: "irrelevant_claim",
			mutate: func(inputs []SourceInput) []SourceInput {
				inputs[0].Claims = []string{"architecture:unrelated"}
				return inputs
			},
		},
		{
			name: "conflicting_identity",
			mutate: func(inputs []SourceInput) []SourceInput {
				conflict := inputs[0]
				conflict.Content = []byte("conflicting source")
				conflict.ExpectedSHA256 = sourceContentSHA256(conflict.Content)
				return append(inputs, conflict)
			},
		},
		{
			name: "missing_required_kind",
			mutate: func(inputs []SourceInput) []SourceInput {
				return inputs[:1]
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildCandidate(plan, test.mutate(sourceInputs(plan.Needs[0])), []CapabilityArtifact{artifact}, []DeclarativeProviderRecord{provider}, nil, nil); err == nil {
				t.Fatal("expected source evidence rejection")
			}
		})
	}
	if assessment.Classification != capabilitygate.ClassificationUnsupported ||
		assessment.FabricationReadyEligible || len(assessment.Gaps) != 1 {
		t.Fatalf("failed evidence mutated original unsupported assessment: %#v", assessment)
	}
}

func TestComponentAndModelEvidenceRemainQuarantinedUntilPromotion(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		gap    capabilitygate.Gap
		kind   NeedKind
	}{
		{
			name: "component", domain: "power",
			gap: capabilitygate.Gap{
				Code: "COMPONENT_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementComponent,
				ID: "high_voltage_current_sensor", Stage: "component_selection", Reason: "no verified catalog record",
			},
			kind: NeedComponent,
		},
		{
			name: "model", domain: "power",
			gap: capabilitygate.Gap{
				Code: "MODEL_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementModel,
				ID: "switching_loss_model", Stage: "simulation", Reason: "no provenance-backed model",
			},
			kind: NeedModel,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment := unsupportedAssessment(t, test.domain, []capabilitygate.Gap{test.gap})
			plan, err := Plan(assessment)
			if err != nil {
				t.Fatal(err)
			}
			need := plan.Needs[0]
			if need.Kind != test.kind {
				t.Fatalf("need kind = %q, want %q", need.Kind, test.kind)
			}
			sources := genericSourceInputs(need)
			evidenceIDs := make([]string, 0, len(sources))
			for _, source := range sources {
				evidenceIDs = append(evidenceIDs, source.ID)
			}
			payload := json.RawMessage(`{"capability":"` + need.CapabilityID + `","kind":"` + string(need.Kind) + `"}`)
			artifact := CapabilityArtifact{
				ID: need.CapabilityID + "_candidate", NeedID: need.ID, Kind: need.Kind,
				ArtifactType: need.RequiredArtifact,
				SHA256:       sourceContentSHA256(payload),
				EvidenceIDs:  evidenceIDs,
				Payload:      payload,
			}
			candidate, err := BuildCandidate(
				plan, sources, []CapabilityArtifact{artifact}, nil, nil,
				[]string{"independent installed-KiCad evidence pending"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if candidate.Status != StatusExperimental {
				t.Fatalf("candidate status = %q, want experimental", candidate.Status)
			}
			bundle, err := BuildBundle(candidate, nil, candidate.Risks)
			if err != nil {
				t.Fatal(err)
			}
			if bundle.Status != StatusExperimental {
				t.Fatalf("incomplete bundle status = %q, want experimental", bundle.Status)
			}
			approval := PromotionApproval{
				Schema: ApprovalSchema, BundleHash: bundle.Hash, Decision: "approve",
				Reviewer: "reviewer", ReviewRef: "review://quarantine-check",
				ReviewSHA256: sourceContentSHA256([]byte("review")),
			}
			if _, err := Promote(EmptySupportedRegistry(), bundle, approval, true); err == nil {
				t.Fatal("experimental package unexpectedly promoted")
			}
		})
	}
}

func TestTwoCapabilitiesPromoteAndServeFreshHeldOutSearches(t *testing.T) {
	registry := EmptySupportedRegistry()
	for _, capability := range []string{"precision_buffering", "low_side_current_sensing"} {
		assessment := unsupportedAssessment(t, "analog", []capabilitygate.Gap{{
			Code: "ARCHITECTURE_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementArchitecture,
			ID: capability, Stage: "architecture_selection", Reason: "no registered provider",
		}})
		plan, err := Plan(assessment)
		if err != nil {
			t.Fatal(err)
		}
		provider, artifact := providerAndArtifact(t, plan.Needs[0], capability+"_provider")
		candidate, err := BuildCandidate(
			plan, sourceInputs(plan.Needs[0]),
			[]CapabilityArtifact{artifact}, []DeclarativeProviderRecord{provider},
			[]string{"catalog function bindings are reviewed separately"},
			[]string{"fresh physical promotion remains mandatory for changed payloads"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if candidate.Status != StatusExperimental {
			t.Fatalf("candidate status = %q", candidate.Status)
		}
		if _, err := Promote(registry, PromotionBundle{}, PromotionApproval{}, true); err == nil {
			t.Fatal("invalid bundle unexpectedly promoted")
		}
		results := passingResults(candidate.Cases)
		bundle, err := BuildBundle(candidate, results, nil)
		if err != nil {
			t.Fatal(err)
		}
		if bundle.Status != StatusReviewReady {
			t.Fatalf("bundle status = %q", bundle.Status)
		}
		approval := PromotionApproval{
			Schema: ApprovalSchema, BundleHash: bundle.Hash, Decision: "approve",
			Reviewer: "independent-reviewer", ReviewRef: "review://" + capability,
			ReviewSHA256: sourceContentSHA256([]byte("approved " + capability)),
		}
		if _, err := Promote(registry, bundle, approval, false); err == nil {
			t.Fatal("promotion without execute unexpectedly succeeded")
		}
		registry, err = Promote(registry, bundle, approval, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(registry.Capabilities) != 2 || registry.Status != StatusSupported {
		t.Fatalf("supported registry = %#v", registry)
	}
	providers, err := Providers(registry)
	if err != nil {
		t.Fatal(err)
	}
	searchRegistry, issues := architecturesearch.NewRegistry(providers...)
	if len(issues) != 0 {
		t.Fatalf("provider registry issues = %#v", issues)
	}
	for _, capability := range []string{"precision_buffering", "low_side_current_sensing"} {
		requirement := heldOutRequirement(capability)
		result := architecturesearch.Search(context.Background(), requirement, searchRegistry, architecturesearch.SearchOptions{CatalogHash: sourceContentSHA256([]byte("catalog"))})
		if result.Status != architecturesearch.SearchSelected || result.Selected == nil ||
			len(result.Selected.Selections) != 1 ||
			result.Selected.Selections[0].Capability != capability {
			t.Fatalf("%s fresh held-out search = %#v", capability, result)
		}
	}
	firstJSON, err := MarshalJSONStable(registry)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalJSONStable(registry)
	if err != nil || string(firstJSON) != string(secondJSON) {
		t.Fatal("supported registry replay is not byte-identical")
	}
}

func unsupportedAssessment(t *testing.T, domain string, gaps []capabilitygate.Gap) capabilitygate.Assessment {
	t.Helper()
	evidenceDigest, err := capabilitygate.Digest("domain:" + domain)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := capabilitygate.Assess(capabilitygate.Input{
		Stage: "capability_assessment",
		Requirements: []capabilitygate.Requirement{{
			Kind: capabilitygate.RequirementDomain, ID: domain, EvidenceIDs: []string{"domain"},
		}},
		Evidence: []capabilitygate.Evidence{{
			ID: "domain", Kind: "normalized_requirement", Status: capabilitygate.EvidenceVerified,
			Source: "requirement://domain", Digest: evidenceDigest,
		}},
		Gaps: gaps,
	})
	if err != nil {
		t.Fatal(err)
	}
	return assessment
}

func sourceInputs(need ExpansionNeed) []SourceInput {
	reference := []byte("reviewed engineering reference for " + need.CapabilityID)
	verification := []byte("independent verification procedure for " + need.CapabilityID)
	return []SourceInput{
		{
			ID: "engineering_reference", Kind: SourceEngineeringReference,
			Publisher: "Example Engineering Standards Body",
			Locator:   "https://example.invalid/reference/" + need.CapabilityID,
			Claims:    []string{need.ID}, ExpectedSHA256: sourceContentSHA256(reference), Content: reference,
		},
		{
			ID: "verification_record", Kind: SourceVerification,
			Publisher: "Independent Verification Lab",
			Locator:   "https://example.invalid/verification/" + need.CapabilityID,
			Claims:    []string{need.ID}, ExpectedSHA256: sourceContentSHA256(verification), Content: verification,
		},
	}
}

func genericSourceInputs(need ExpansionNeed) []SourceInput {
	inputs := make([]SourceInput, 0, len(need.RequiredSourceKinds))
	for _, kind := range need.RequiredSourceKinds {
		id := canonicalID(string(kind)) + "_source"
		content := []byte("reviewed " + string(kind) + " evidence for " + need.CapabilityID)
		input := SourceInput{
			ID: id, Kind: kind, Publisher: "Independent Engineering Publisher",
			Locator: "https://example.invalid/evidence/" + id,
			Claims:  []string{need.ID}, ExpectedSHA256: sourceContentSHA256(content),
			Content: content,
		}
		if kind == SourceModel {
			input.License = "SPDX:MIT"
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func providerAndArtifact(t *testing.T, need ExpansionNeed, providerID string) (DeclarativeProviderRecord, CapabilityArtifact) {
	t.Helper()
	realization, err := architecturesearch.MarshalFragmentRealization(architecturesearch.FragmentRealization{
		Schema: architecturesearch.FragmentRealizationSchema, Capability: need.CapabilityID,
		Instances: []architecturesearch.RealizationInstance{{
			ID: "amplifier", CatalogID: "opamp.ti.lmv321.sot23_5",
			Usage:             "signal_conditioning",
			RequiredFunctions: []string{"GND", "IN", "OUT", "VCC"},
		}},
		PortBindings: []architecturesearch.RealizationPortBinding{
			{Role: "power", Instance: "amplifier", Function: "VCC"},
			{Role: "reference", Instance: "amplifier", Function: "GND"},
			{Role: "sense", Instance: "amplifier", Function: "IN"},
			{Role: "output", Instance: "amplifier", Function: "OUT"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := DeclarativeProviderRecord{
		ID: providerID, Revision: "1.0.0", Capability: need.CapabilityID,
		NeedID: need.ID, EvidenceIDs: []string{"engineering_reference", "verification_record"},
		Expansion: architecturesearch.ProviderExpansion{
			ID:           "reviewed_realization",
			OfferedPorts: reviewedAnalogPorts(),
			Components: []architecturesearch.SelectedComponent{{
				InstanceID: "amplifier", CatalogID: "opamp.ti.lmv321.sot23_5",
				Evidence: architecturesearch.EvidenceVerified,
			}},
			Metrics: architecturesearch.ExpansionMetrics{},
			Evidence: architecturesearch.ContractEvidence{
				Confidence: architecturesearch.EvidenceVerified,
				Sources:    []string{"engineering_reference", "verification_record"},
			},
			Payload: realization,
		},
	}
	providerPayload, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	providerPayload, err = canonicalArtifactPayload(providerPayload)
	if err != nil {
		t.Fatal(err)
	}
	artifact := CapabilityArtifact{
		ID: providerID + "_artifact", NeedID: need.ID, Kind: need.Kind,
		ArtifactType: need.RequiredArtifact, SHA256: sourceContentSHA256(providerPayload),
		EvidenceIDs: []string{"engineering_reference", "verification_record"},
		Payload:     providerPayload,
	}
	return provider, artifact
}

func reviewedAnalogPorts() []architecturesearch.RoleContract {
	evidence := architecturesearch.ContractEvidence{
		Confidence: architecturesearch.EvidenceVerified,
		Sources:    []string{"engineering_reference", "verification_record"},
	}
	return []architecturesearch.RoleContract{
		{
			Role: "power",
			Contract: architecturesearch.PortContract{
				Kind: "power", Direction: "sink", Domain: "vcc",
				Voltage: architecturesearch.NumericRange{
					Minimum: float64Pointer(4.75), Maximum: float64Pointer(5.25),
				},
				CurrentDemandA: float64Pointer(0.02), Evidence: evidence,
			},
		},
		{
			Role: "reference",
			Contract: architecturesearch.PortContract{
				Kind: "reference", Direction: "bidirectional", Domain: "ground",
				Voltage: architecturesearch.NumericRange{
					Minimum: float64Pointer(0), Maximum: float64Pointer(0),
				},
				Evidence: evidence,
			},
		},
		{
			Role: "sense",
			Contract: architecturesearch.PortContract{
				Kind: "analog_voltage", Direction: "sink", Domain: "vcc",
				Voltage: architecturesearch.NumericRange{
					Minimum: float64Pointer(0), Maximum: float64Pointer(3.3),
				},
				Evidence: evidence,
			},
		},
		{
			Role: "output",
			Contract: architecturesearch.PortContract{
				Kind: "analog_voltage", Direction: "source", Domain: "vcc",
				Voltage: architecturesearch.NumericRange{
					Minimum: float64Pointer(0), Maximum: float64Pointer(5),
				},
				CurrentCapacityA: float64Pointer(0.02), Evidence: evidence,
			},
		},
	}
}

func passingResults(cases []GeneratedCase) []GateResult {
	var results []GateResult
	for _, testCase := range cases {
		for _, gate := range testCase.RequiredGates {
			content := []byte(testCase.ID + ":" + gate + ":passed")
			results = append(results, GateResult{
				CaseID: testCase.ID, Gate: gate, Passed: true,
				EvidencePath:   "evidence/" + canonicalID(testCase.ID+"_"+gate) + ".json",
				EvidenceSHA256: sourceContentSHA256(content),
			})
		}
	}
	return results
}

func heldOutRequirement(capability string) architecturesearch.Requirement {
	minV, maxV, current := 4.75, 5.25, 0.02
	return architecturesearch.Requirement{
		Schema: architecturesearch.SchemaID, Version: architecturesearch.Version,
		Project: architecturesearch.Project{
			Name: "fresh_held_out", Title: "Fresh held-out request",
			Description: "Identity-neutral request created after capability promotion.",
		},
		Requirements: architecturesearch.Requirements{
			Domains: []architecturesearch.Domain{
				{ID: "vcc", Kind: "supply", MinVoltageV: &minV, NominalVoltageV: 5, MaxVoltageV: &maxV, MaxCurrentA: &current, Source: "external"},
				{ID: "ground", Kind: "reference", NominalVoltageV: 0, Source: "external"},
			},
			Ports: []architecturesearch.Port{
				{ID: "power", Kind: "power", Direction: "sink", Domain: "vcc", Electrical: &architecturesearch.Electrical{MaxCurrentA: &current}},
				{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "ground"},
				{ID: "sense", Kind: "analog_voltage", Direction: "sink", Domain: "vcc", Electrical: &architecturesearch.Electrical{MinVoltageV: float64Pointer(0), MaxVoltageV: float64Pointer(3.3)}},
				{ID: "output", Kind: "analog_voltage", Direction: "source", Domain: "vcc", Electrical: &architecturesearch.Electrical{MinVoltageV: float64Pointer(0), MaxVoltageV: float64Pointer(5)}},
			},
			Objectives: []architecturesearch.Objective{{
				ID: "implement", Capability: capability,
				Bindings: []architecturesearch.Binding{
					{Role: "power", Port: "power"}, {Role: "reference", Port: "ground"},
					{Role: "sense", Port: "sense"}, {Role: "output", Port: "output"},
				},
				Constraints: []architecturesearch.Constraint{{
					Name: "bounded_behavior", Relation: "required", Value: json.RawMessage(`true`),
				}},
			}},
			Constraints: architecturesearch.BoardLimits{MaxComponents: 8, MaxWidthMM: 30, MaxHeightMM: 20},
		},
		Acceptance: architecturesearch.Acceptance{
			RequireERC: true, RequireStrictDRC: true, RequireCompleteRouting: true,
			RequireConnectivity: true, RequireWriterCorrectness: true,
			RequireRoundTripZeroDiff: true, RequireDeterministicReplay: true,
		},
	}
}

func float64Pointer(value float64) *float64 { return &value }
