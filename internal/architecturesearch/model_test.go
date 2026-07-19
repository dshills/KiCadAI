package architecturesearch

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestFrozenOpenSetCorpusDecodesWithProductionContract(t *testing.T) {
	root := frozenCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Fixtures []struct {
			ID   string `json:"id"`
			File string `json:"file"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Fixtures) != 5 {
		t.Fatalf("fixture count = %d, want 5", len(manifest.Fixtures))
	}
	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			requirement, issues := DecodeStrict(bytes.NewReader(contents))
			if len(issues) != 0 {
				t.Fatalf("decode issues = %#v", issues)
			}
			if requirement.Project.Name != fixture.ID {
				t.Fatalf("project name = %q, want %q", requirement.Project.Name, fixture.ID)
			}
			if hash, err := CanonicalHash(requirement); err != nil || len(hash) != 64 {
				t.Fatalf("canonical hash = %q, %v", hash, err)
			}
		})
	}
}

func TestDecodeStrictNormalizesOrderAndProducesStableHash(t *testing.T) {
	firstInput := validRequirement()
	secondInput := cloneRequirement(firstInput)
	secondInput.Project.Name = " Synthetic_Threshold "
	secondInput.Requirements.Domains[0].ID = " VCC "
	secondInput.Requirements.Ports[0].Domain = " VCC "
	secondInput.Requirements.Ports[0].ID = " POWER "
	secondInput.Requirements.Objectives[0].Bindings[0].Port = " POWER "
	secondInput.Requirements.Objectives[0].Constraints[0].Unit = "v"
	slices.Reverse(secondInput.Requirements.Domains)
	slices.Reverse(secondInput.Requirements.Ports)
	slices.Reverse(secondInput.Requirements.Objectives[0].Bindings)
	slices.Reverse(secondInput.Requirements.Objectives[0].Constraints)

	first := decodeRequirement(t, firstInput)
	second := decodeRequirement(t, secondInput)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("normalized requirements differ\nfirst=%#v\nsecond=%#v", first, second)
	}
	firstJSON, err := CanonicalJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := CanonicalJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical JSON differs\n%s\n%s", firstJSON, secondJSON)
	}
	firstHash, err := CanonicalHash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := CanonicalHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("canonical hashes differ: %s != %s", firstHash, secondHash)
	}

	changed := cloneRequirement(first)
	for index := range changed.Requirements.Objectives[0].Constraints {
		if changed.Requirements.Objectives[0].Constraints[index].Name == "threshold_voltage" {
			changed.Requirements.Objectives[0].Constraints[index].Value = json.RawMessage(`1.6`)
		}
	}
	changedHash, err := CanonicalHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == firstHash {
		t.Fatal("meaningful electrical requirement change did not change canonical hash")
	}
}

func TestNormalizeIsIdempotentAndDoesNotMutateCaller(t *testing.T) {
	requirement := validRequirement()
	slices.Reverse(requirement.Requirements.Domains)
	slices.Reverse(requirement.Requirements.Ports)
	original, err := json.Marshal(requirement)
	if err != nil {
		t.Fatal(err)
	}
	first := Normalize(requirement)
	second := Normalize(first)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("normalization is not idempotent\nfirst=%#v\nsecond=%#v", first, second)
	}
	after, err := json.Marshal(requirement)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Fatal("Normalize mutated its caller")
	}
}

func TestDecodeStrictRejectsUnknownTrailingAndOversizedInput(t *testing.T) {
	encoded, err := json.Marshal(validRequirement())
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(encoded, []byte(`"version":1`), []byte(`"version":1,"unknown":true`), 1)
	if _, issues := DecodeStrict(bytes.NewReader(unknown)); len(issues) != 1 || issues[0].Code != CodeSchemaInvalid || !strings.Contains(issues[0].Message, "unknown field") {
		t.Fatalf("unknown-field issues = %#v", issues)
	}
	trailing := append(append([]byte(nil), encoded...), []byte(` {}`)...)
	if _, issues := DecodeStrict(bytes.NewReader(trailing)); len(issues) != 1 || issues[0].Code != CodeSchemaInvalid || !strings.Contains(issues[0].Message, "trailing") {
		t.Fatalf("trailing issues = %#v", issues)
	}
	if _, issues := DecodeStrict(bytes.NewReader(bytes.Repeat([]byte(" "), MaxRequirementBytes+1))); len(issues) != 1 || issues[0].Code != CodeLimitExceeded {
		t.Fatalf("oversized issues = %#v", issues)
	}
}

func TestValidationFailsClosedForInvalidRequirements(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Requirement)
		code reports.Code
		path string
	}{
		{
			name: "duplicate normalized domain",
			edit: func(requirement *Requirement) {
				requirement.Requirements.Domains = append(requirement.Requirements.Domains, Domain{ID: "VCC", Kind: "supply", NominalVoltageV: 5, Source: "external"})
			},
			code: CodeIdentityDuplicate,
			path: ".id",
		},
		{
			name: "unknown port domain",
			edit: func(requirement *Requirement) {
				requirement.Requirements.Ports[0].Domain = "missing"
			},
			code: CodeBindingUnresolved,
			path: ".domain",
		},
		{
			name: "contradictory domain range",
			edit: func(requirement *Requirement) {
				requirement.Requirements.Domains[0].MinVoltageV = floatPointer(6)
				requirement.Requirements.Domains[0].MaxVoltageV = floatPointer(4)
			},
			code: CodeDomainInvalid,
			path: "requirements.domains",
		},
		{
			name: "ambiguous binding union",
			edit: func(requirement *Requirement) {
				requirement.Requirements.Objectives[0].Bindings[0].Participant = "controller"
				requirement.Requirements.Objectives[0].Bindings[0].ParticipantPort = "bus"
			},
			code: CodeBindingUnresolved,
			path: ".bindings",
		},
		{
			name: "descending range",
			edit: func(requirement *Requirement) {
				constraint := &requirement.Requirements.Objectives[0].Constraints[0]
				constraint.Relation = "range"
				constraint.Value = json.RawMessage(`[2,1]`)
				constraint.TolerancePercent = nil
			},
			code: CodeConstraintInvalid,
			path: ".value",
		},
		{
			name: "false required constraint",
			edit: func(requirement *Requirement) {
				requirement.Requirements.Objectives[0].Constraints = append(requirement.Requirements.Objectives[0].Constraints, Constraint{Name: "safe_start", Relation: "required", Value: json.RawMessage(`false`)})
			},
			code: CodeConstraintInvalid,
			path: ".value",
		},
		{
			name: "acceptance weakened",
			edit: func(requirement *Requirement) {
				requirement.Acceptance.RequireStrictDRC = false
			},
			code: CodeAcceptanceInvalid,
			path: "acceptance.require_strict_drc",
		},
		{
			name: "component budget above policy",
			edit: func(requirement *Requirement) {
				requirement.Requirements.Constraints.MaxComponents = MaxComponents + 1
			},
			code: CodeLimitExceeded,
			path: "max_components",
		},
		{
			name: "non finite voltage",
			edit: func(requirement *Requirement) {
				requirement.Requirements.Domains[0].NominalVoltageV = math.Inf(1)
			},
			code: CodeDomainInvalid,
			path: "nominal_voltage_v",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement := validRequirement()
			test.edit(&requirement)
			issues := Validate(Normalize(requirement))
			if !containsIssue(issues, test.code, test.path) {
				t.Fatalf("issues = %#v, want code=%s path containing %q", issues, test.code, test.path)
			}
			if _, err := CanonicalHash(requirement); err == nil {
				t.Fatal("CanonicalHash accepted invalid requirement")
			}
		})
	}
}

func validRequirement() Requirement {
	return Requirement{
		Schema:  SchemaID,
		Version: Version,
		Project: Project{Name: "synthetic_threshold", Title: "Synthetic threshold", Description: "Synthetic requirement independent of the frozen corpus."},
		Requirements: Requirements{
			Domains: []Domain{
				{ID: "vcc", Kind: "supply", MinVoltageV: floatPointer(4.75), NominalVoltageV: 5, MaxVoltageV: floatPointer(5.25), MaxCurrentA: floatPointer(0.02), Source: "external"},
				{ID: "ground", Kind: "reference", NominalVoltageV: 0, Source: "external"},
			},
			Ports: []Port{
				{ID: "power", Kind: "power", Direction: "sink", Domain: "vcc", Electrical: &Electrical{MaxCurrentA: floatPointer(0.02)}},
				{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "ground"},
				{ID: "sense", Kind: "analog_voltage", Direction: "sink", Domain: "vcc", Electrical: &Electrical{MinVoltageV: floatPointer(0), MaxVoltageV: floatPointer(3.3)}},
				{ID: "alert", Kind: "digital_logic", Direction: "source", Domain: "vcc", Electrical: &Electrical{MinVoltageV: floatPointer(0), MaxVoltageV: floatPointer(5), DefaultState: "inactive"}},
			},
			Objectives: []Objective{{
				ID: "detect", Capability: "threshold_detection",
				Bindings: []Binding{{Role: "power", Port: "power"}, {Role: "reference", Port: "ground"}, {Role: "sense", Port: "sense"}, {Role: "output", Port: "alert"}},
				Constraints: []Constraint{
					{Name: "threshold_voltage", Relation: "target", Value: json.RawMessage(`1.5`), Unit: "V", TolerancePercent: floatPointer(2)},
					{Name: "inactive_at_power_up", Relation: "required", Value: json.RawMessage(`true`)},
				},
			}},
			Constraints: BoardLimits{MaxComponents: 10, MaxWidthMM: 30, MaxHeightMM: 20},
		},
		Acceptance: Acceptance{
			RequireERC: true, RequireStrictDRC: true, RequireCompleteRouting: true,
			RequireConnectivity: true, RequireWriterCorrectness: true,
			RequireRoundTripZeroDiff: true, RequireDeterministicReplay: true,
		},
	}
}

func decodeRequirement(t *testing.T, requirement Requirement) Requirement {
	t.Helper()
	encoded, err := json.Marshal(requirement)
	if err != nil {
		t.Fatal(err)
	}
	decoded, issues := DecodeStrict(bytes.NewReader(encoded))
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	return decoded
}

func containsIssue(issues []reports.Issue, code reports.Code, path string) bool {
	for _, issue := range issues {
		if issue.Code == code && strings.Contains(issue.Path, path) {
			return true
		}
	}
	return false
}

func floatPointer(value float64) *float64 {
	return &value
}

func frozenCorpusRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture search tests")
	}
	return filepath.Join(filepath.Dir(sourcePath), "..", "circuitgraph", "testdata", "open_set_composition_corpus")
}
