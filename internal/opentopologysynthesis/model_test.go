package opentopologysynthesis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFrozenCorpusDecodesWithProductionRequirement(t *testing.T) {
	var manifest frozenManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(frozenCorpusRoot(), "manifest.json")), &manifest)
	for _, entry := range manifest.Cases {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			data := mustRead(t, filepath.Join(frozenCorpusRoot(), entry.RequirementFile))
			requirement, issues := DecodeStrict(bytes.NewReader(data))
			if len(issues) != 0 {
				t.Fatalf("decode issues: %#v", issues)
			}
			if requirement.Schema != RequirementSchema ||
				requirement.Version != RequirementVersion ||
				requirement.Project.Name != entry.ID {
				t.Fatalf("requirement identity = %#v", requirement.Project)
			}
			first, err := CanonicalHash(requirement)
			if err != nil {
				t.Fatal(err)
			}
			reordered := cloneRequirement(requirement)
			reverseRequirement(&reordered)
			second, err := CanonicalHash(reordered)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatalf("canonical hash changed under order permutation: %s != %s", first, second)
			}
		})
	}
}

func TestDecodeStrictRejectsImplementationFieldsAndMalformedDocuments(t *testing.T) {
	data := mustRead(t, filepath.Join(frozenCorpusRoot(), "powered_lowpass.json"))
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	requirements := document["requirements"].(map[string]any)
	requirements["objectives"] = []any{map[string]any{"capability": "frequency_filter"}}
	withObjective, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := DecodeStrict(bytes.NewReader(withObjective)); len(issues) != 1 ||
		issues[0].Code != CodeRequirementInvalid ||
		!strings.Contains(issues[0].Message, "unknown field") {
		t.Fatalf("objective-field issues = %#v", issues)
	}

	withTrailing := append(append([]byte(nil), data...), []byte("\n{}")...)
	if _, issues := DecodeStrict(bytes.NewReader(withTrailing)); len(issues) != 1 ||
		issues[0].Code != CodeRequirementInvalid ||
		!strings.Contains(issues[0].Message, "trailing") {
		t.Fatalf("trailing issues = %#v", issues)
	}

	oversized := bytes.Repeat([]byte{' '}, MaxRequirementBytes+1)
	if _, issues := DecodeStrict(bytes.NewReader(oversized)); len(issues) != 1 ||
		issues[0].Code != CodeRequirementInvalid ||
		!strings.Contains(issues[0].Message, "exceeds") {
		t.Fatalf("oversized issues = %#v", issues)
	}
}

func TestValidationFailsClosedAndReturnsStableSortedIssues(t *testing.T) {
	data := mustRead(t, filepath.Join(frozenCorpusRoot(), "sensor_conditioner.json"))
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("valid decode issues: %#v", issues)
	}
	requirement.Acceptance.RequireStrictDRC = false
	requirement.Requirements.Ports[0].Domain = "missing"
	requirement.Requirements.BehavioralRequirements[0].Metric = "unknown_metric"
	requirement.Requirements.BehavioralRequirements[0].OperatingCases = []string{"missing"}

	first := Validate(requirement)
	second := Validate(requirement)
	if len(first) != 4 {
		t.Fatalf("issue count = %d, want 4: %#v", len(first), first)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("validation output is nondeterministic:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestPublicRequirementContractUpperBoundsAreAccepted(t *testing.T) {
	// These literals intentionally remain independent of the production
	// constants so the committed public contract cannot drift silently.
	const (
		publicMaxConditions = 32
		publicMaxEvents     = 32
		publicMaxComponents = 64
	)
	if MaxConditionsPerCase != publicMaxConditions || MaxTotalEvents != publicMaxEvents || MaxComponents != publicMaxComponents {
		t.Fatalf("production/public maxima = conditions %d/%d, events %d/%d, components %d/%d",
			MaxConditionsPerCase, publicMaxConditions, MaxTotalEvents, publicMaxEvents, MaxComponents, publicMaxComponents)
	}

	data := mustRead(t, filepath.Join(frozenCorpusRoot(), "sensor_conditioner.json"))
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("valid decode issues: %#v", issues)
	}
	if len(requirement.Requirements.OperatingCases) == 0 || len(requirement.Requirements.Ports) == 0 || len(requirement.Requirements.Domains) == 0 {
		t.Fatal("test fixture must contain at least one operating case, port, and domain")
	}
	if len(allowedConditionAxes) == 0 {
		t.Fatal("public contract must expose at least one condition axis")
	}
	conditionsPerTarget := len(allowedConditionAxes)
	requiredTargets := (publicMaxConditions + conditionsPerTarget - 1) / conditionsPerTarget
	targets := make([]string, 0, len(requirement.Requirements.Domains)+len(requirement.Requirements.Ports))
	seenTargets := map[string]bool{}
	addTarget := func(id string) bool {
		if seenTargets[id] {
			return false
		}
		targets = append(targets, id)
		seenTargets[id] = true
		return true
	}
	for _, domain := range requirement.Requirements.Domains {
		addTarget(domain.ID)
	}
	for _, port := range requirement.Requirements.Ports {
		addTarget(port.ID)
	}
	for index := 0; len(targets) < requiredTargets && len(requirement.Requirements.Domains) < MaxDomains; index++ {
		id := fmt.Sprintf("boundary_domain_%02d", index)
		if addTarget(id) {
			requirement.Requirements.Domains = append(requirement.Requirements.Domains, Domain{
				ID:     id,
				Kind:   "supply",
				Source: "external",
			})
		}
	}
	for index := 0; len(targets) < requiredTargets && len(requirement.Requirements.Ports) < MaxPorts; index++ {
		id := fmt.Sprintf("boundary_port_%02d", index)
		if addTarget(id) {
			requirement.Requirements.Ports = append(requirement.Requirements.Ports, Port{
				ID:        id,
				Kind:      "analog_voltage",
				Direction: "sink",
				Domain:    requirement.Requirements.Domains[0].ID,
			})
		}
	}
	if len(targets) < requiredTargets {
		t.Fatalf("public limits expose %d unique condition targets, want %d", len(targets), requiredTargets)
	}
	conditions := make([]OperatingCondition, 0, publicMaxConditions)
	for index := 0; index < publicMaxConditions; index++ {
		conditions = append(conditions, OperatingCondition{
			Axis:   allowedConditionAxes[index%conditionsPerTarget],
			Target: targets[index/conditionsPerTarget],
			Min:    0,
			Max:    1,
			Unit:   "V",
		})
	}
	for index := range requirement.Requirements.OperatingCases {
		requirement.Requirements.OperatingCases[index].Events = nil
	}
	events := make([]OperatingEvent, 0, publicMaxEvents)
	for index := 0; index < publicMaxEvents; index++ {
		events = append(events, OperatingEvent{
			ID:           fmt.Sprintf("boundary_event_%02d", index),
			Kind:         "input_step",
			Target:       requirement.Requirements.Ports[0].ID,
			TriggerTimeS: float64(index),
			Initial:      0,
			Applied:      1,
			Unit:         "V",
		})
	}
	requirement.Requirements.OperatingCases[0].Conditions = conditions
	requirement.Requirements.OperatingCases[0].Events = events
	// Open-topology requirements declare a component-count constraint; concrete
	// components do not exist until synthesis constructs a candidate graph.
	requirement.Requirements.Constraints.MaxComponents = publicMaxComponents

	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("public contract upper bounds rejected: %#v", issues)
	}
	deepCopy := func(t *testing.T, source Requirement) Requirement {
		t.Helper()
		data, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		var candidate Requirement
		if err := json.Unmarshal(data, &candidate); err != nil {
			t.Fatal(err)
		}
		return candidate
	}

	assertRejected := func(t *testing.T, candidate Requirement, path string) {
		t.Helper()
		for _, issue := range Validate(candidate) {
			if issue.Path == path {
				return
			}
		}
		t.Fatalf("missing rejection at %q", path)
	}

	t.Run("conditions", func(t *testing.T) {
		candidate := deepCopy(t, requirement)
		candidate.Requirements.OperatingCases[0].Conditions = append(
			candidate.Requirements.OperatingCases[0].Conditions,
			candidate.Requirements.OperatingCases[0].Conditions[0],
		)
		assertRejected(t, candidate, "requirements.operating_cases[0].conditions")
	})
	t.Run("events", func(t *testing.T) {
		candidate := deepCopy(t, requirement)
		candidate.Requirements.OperatingCases[0].Events = append(
			candidate.Requirements.OperatingCases[0].Events,
			OperatingEvent{
				ID:           "boundary_event_over_limit",
				Kind:         "input_step",
				Target:       candidate.Requirements.Ports[0].ID,
				TriggerTimeS: 1,
				Initial:      0,
				Applied:      1,
				Unit:         "V",
			},
		)
		assertRejected(t, candidate, "requirements.operating_cases")
	})
	t.Run("components", func(t *testing.T) {
		candidate := deepCopy(t, requirement)
		candidate.Requirements.Constraints.MaxComponents++
		assertRejected(t, candidate, "requirements.constraints.max_components")
	})
}

func TestPolicyAndReportContractsAreDeterministic(t *testing.T) {
	policy := DefaultPolicy()
	if policy.MaxCandidateSimulations < 50_000 || policy.MaxCornerEvaluations < 8_192 {
		t.Fatalf("default policy does not cover established multi-stage qualification bounds: %#v", policy)
	}
	first, err := PolicyHash(policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PolicyHash(policy)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("policy hashes = %q and %q", first, second)
	}

	report := Report{
		Schema:          ReportSchema,
		Version:         ReportVersion,
		PolicyVersion:   PolicyVersion,
		PolicyHash:      first,
		RequirementHash: strings.Repeat("a", 64),
		Policy:          policy,
		Status:          StatusUnsupported,
		StopReason:      StopPrimitiveUnavailable,
		Diagnostics: []Diagnostic{{
			Code:       CodePrimitiveUnavailable,
			Path:       "primitive_inventory",
			Message:    "no compatible primitive is available",
			Suggestion: "onboard reviewed primitive evidence",
		}},
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("report round trip changed:\n%s\n%s", encoded, reencoded)
	}
}

func reverseRequirement(requirement *Requirement) {
	slices.Reverse(requirement.Requirements.Domains)
	slices.Reverse(requirement.Requirements.Ports)
	slices.Reverse(requirement.Requirements.OperatingCases)
	slices.Reverse(requirement.Requirements.BehavioralRequirements)
	for index := range requirement.Requirements.OperatingCases {
		slices.Reverse(requirement.Requirements.OperatingCases[index].Conditions)
		slices.Reverse(requirement.Requirements.OperatingCases[index].Events)
	}
	for index := range requirement.Requirements.BehavioralRequirements {
		slices.Reverse(requirement.Requirements.BehavioralRequirements[index].OperatingCases)
	}
}

func TestCorpusPathIsPackageLocal(t *testing.T) {
	if _, err := os.Stat(frozenCorpusRoot()); err != nil {
		t.Fatal(err)
	}
}
