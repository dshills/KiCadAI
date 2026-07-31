package opentopologysynthesis

import (
	"bytes"
	"encoding/json"
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

func TestPolicyAndReportContractsAreDeterministic(t *testing.T) {
	policy := DefaultPolicy()
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
