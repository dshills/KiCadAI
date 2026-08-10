package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

const (
	v4ImpactRegistryHash  = "64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377"
	v4SynthesisPolicyHash = "4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4"
)

type v4GapTransitionPolicy struct {
	Schema                                  string   `json:"schema"`
	Version                                 string   `json:"version"`
	SelectedClusterIdentityFields           []string `json:"selected_cluster_identity_fields"`
	GapIdentityFields                       []string `json:"gap_identity_fields"`
	RequiredEvidenceNormalization           string   `json:"required_evidence_normalization"`
	IdentityEncoding                        string   `json:"identity_encoding"`
	Relation                                string   `json:"relation"`
	AllowNewFinalGaps                       bool     `json:"allow_new_final_gaps"`
	RequireExactCaseSet                     bool     `json:"require_exact_case_set"`
	RequireUniqueCaseIDs                    bool     `json:"require_unique_case_ids"`
	BaselinePassMustRemainPass              bool     `json:"baseline_pass_must_remain_pass"`
	BaselineUnsafeMustNotBecomePass         bool     `json:"baseline_unsafe_must_not_become_pass"`
	FinalPassRequiresExternalPromotionGates bool     `json:"final_pass_requires_external_promotion_gates"`
}

type v4SelectedCluster struct {
	Stage      string
	Scope      capabilityfeedback.GapScope
	Capability string
	Code       string
}

func TestVersionFourContractIsFrozen(t *testing.T) {
	directory := v4ContractDirectory(t)
	manifest, err := os.Open(filepath.Join(directory, "V4_CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || seen[fields[1]] || filepath.Base(fields[1]) != fields[1] {
			t.Fatalf("invalid V4 contract entry %q", scanner.Text())
		}
		if got := v4FileSHA256(t, filepath.Join(directory, fields[1])); got != fields[0] {
			t.Fatalf("%s hash = %s, want frozen %s", fields[1], got, fields[0])
		}
		seen[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"V4_SPEC_ADDENDUM.md", "V4_GAP_TRANSITION_PROTOCOL.md", "V4_GAP_TRANSITION_POLICY.json",
		"V4_CORPUS_RULES.md", "V4_BASELINE_PROTOCOL.md", "V4_PLAN.md", "V4_IMPACT_REGISTRY.json",
		"V4_SYNTHESIS_POLICY.json", "V4_IMPLEMENTATION.sha256", "v4_contract_test.go",
	} {
		if !seen[required] {
			t.Fatalf("V4 frozen contract omits %s", required)
		}
	}
}

func TestVersionFourImplementationHashesAreFrozen(t *testing.T) {
	directory := v4ContractDirectory(t)
	repository := filepath.Clean(filepath.Join(directory, "..", ".."))
	manifest, err := os.Open(filepath.Join(directory, "V4_IMPLEMENTATION.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	want := map[string]bool{
		"internal/opentopologysynthesis/realizability.go":                     false,
		"internal/capabilityfeedback/observe.go":                              false,
		"internal/capabilityfeedback/evaluate.go":                             false,
		"specs/behavioral-contract-feasibility-realizability/CONTRACT.sha256": false,
	}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || filepath.IsAbs(fields[1]) || strings.HasPrefix(filepath.Clean(fields[1]), "..") {
			t.Fatalf("invalid V4 implementation entry %q", scanner.Text())
		}
		seen, exists := want[fields[1]]
		if !exists || seen {
			t.Fatalf("unexpected or duplicate V4 implementation entry %q", fields[1])
		}
		if got := v4FileSHA256(t, filepath.Join(repository, filepath.Clean(fields[1]))); got != fields[0] {
			t.Fatalf("%s hash = %s, want frozen %s", fields[1], got, fields[0])
		}
		want[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for path, seen := range want {
		if !seen {
			t.Fatalf("V4 implementation manifest omits %s", path)
		}
	}
}

func TestVersionFourPoliciesAreExact(t *testing.T) {
	directory := v4ContractDirectory(t)
	var registry capabilityevaluation.ImpactRegistry
	v4DecodeStrictFile(t, filepath.Join(directory, "V4_IMPACT_REGISTRY.json"), &registry)
	report, err := capabilityfeedback.EvaluateRealizabilityAware(capabilityfeedback.RoleHeldOut, nil, registry)
	if err != nil {
		t.Fatalf("V4 impact registry: %v", err)
	}
	if report.PolicyVersion != capabilityfeedback.RealizabilityPolicyVersion || report.ImpactRegistryHash != v4ImpactRegistryHash {
		t.Fatalf("V4 evaluator/registry = %q/%q", report.PolicyVersion, report.ImpactRegistryHash)
	}

	var synthesis opentopologysynthesis.Policy
	v4DecodeStrictFile(t, filepath.Join(directory, "V4_SYNTHESIS_POLICY.json"), &synthesis)
	hash, err := opentopologysynthesis.PolicyHash(synthesis)
	if err != nil || hash != v4SynthesisPolicyHash {
		t.Fatalf("V4 synthesis policy hash = %q err=%v", hash, err)
	}

	var transition v4GapTransitionPolicy
	v4DecodeStrictFile(t, filepath.Join(directory, "V4_GAP_TRANSITION_POLICY.json"), &transition)
	wantTransition := v4GapTransitionPolicy{
		Schema: "closed-loop-gap-transition-policy", Version: "closed-loop-gap-transition-v1",
		SelectedClusterIdentityFields: []string{"stage", "scope", "capability", "code"},
		GapIdentityFields:             []string{"stage", "scope", "capability", "code", "required_evidence"},
		RequiredEvidenceNormalization: "sorted_unique_bytewise", IdentityEncoding: "decimal_utf8_byte_length_delimited",
		Relation: "baseline_nonselected_subset_of_final", AllowNewFinalGaps: true, RequireExactCaseSet: true,
		RequireUniqueCaseIDs: true, BaselinePassMustRemainPass: true, BaselineUnsafeMustNotBecomePass: true,
		FinalPassRequiresExternalPromotionGates: true,
	}
	if !reflect.DeepEqual(transition, wantTransition) {
		t.Fatalf("unexpected V4 gap-transition policy: %+v", transition)
	}
}

func TestVersionFourGapTransitionAdversarialBoundaries(t *testing.T) {
	selected := v4SelectedCluster{Stage: "topology", Scope: capabilityfeedback.ScopeTopology, Capability: "selected", Code: "blocked"}
	selectedGap := v4Gap("topology", capabilityfeedback.ScopeTopology, "selected", "blocked", "netlist")
	other := v4Gap("simulation", capabilityfeedback.ScopeSimulation, "model", "missing", "dc", "ac")
	newGap := v4Gap("physical", capabilityfeedback.ScopePhysical, "routing", "incomplete", "drc")

	tests := []struct {
		name   string
		before []capabilityfeedback.Gap
		after  []capabilityfeedback.Gap
		want   bool
	}{
		{name: "equal sets", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{other}, want: true},
		{name: "strict final superset", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{other, newGap}, want: true},
		{name: "duplicates and order normalize", before: []capabilityfeedback.Gap{v4Gap("simulation", capabilityfeedback.ScopeSimulation, "model", "missing", "ac", "dc", "dc")}, after: []capabilityfeedback.Gap{other, other}, want: true},
		{name: "exact selected removal", before: []capabilityfeedback.Gap{selectedGap, other}, after: []capabilityfeedback.Gap{other}, want: true},
		{name: "same capability different stage is retained", before: []capabilityfeedback.Gap{v4Gap("simulation", capabilityfeedback.ScopeTopology, "selected", "blocked", "netlist")}, after: nil, want: false},
		{name: "same capability different scope is retained", before: []capabilityfeedback.Gap{v4Gap("topology", capabilityfeedback.ScopeSimulation, "selected", "blocked", "netlist")}, after: nil, want: false},
		{name: "same capability different code is retained", before: []capabilityfeedback.Gap{v4Gap("topology", capabilityfeedback.ScopeTopology, "selected", "other", "netlist")}, after: nil, want: false},
		{name: "unrelated removal", before: []capabilityfeedback.Gap{other}, after: nil, want: false},
		{name: "stage rename", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{v4Gap("verification", other.Scope, other.Capability, other.Code, "ac", "dc")}, want: false},
		{name: "scope reclassification", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{v4Gap(other.Stage, capabilityfeedback.ScopeVerification, other.Capability, other.Code, "ac", "dc")}, want: false},
		{name: "capability rename", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{v4Gap(other.Stage, other.Scope, "different", other.Code, "ac", "dc")}, want: false},
		{name: "code rename", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{v4Gap(other.Stage, other.Scope, other.Capability, "different", "ac", "dc")}, want: false},
		{name: "required evidence addition", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{v4Gap(other.Stage, other.Scope, other.Capability, other.Code, "ac", "dc", "transient")}, want: false},
		{name: "required evidence removal", before: []capabilityfeedback.Gap{other}, after: []capabilityfeedback.Gap{v4Gap(other.Stage, other.Scope, other.Capability, other.Code, "dc")}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := v4GapsPreserved(test.before, test.after, selected); got != test.want {
				t.Fatalf("v4GapsPreserved() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestVersionFourGapTransitionRequiresExactUniqueCaseSet(t *testing.T) {
	selected := v4SelectedCluster{Stage: "topology", Scope: capabilityfeedback.ScopeTopology, Capability: "selected", Code: "blocked"}
	base := capabilityfeedback.CaseEvidence{Case: capabilityfeedback.CaseMeta{ID: "case_a"}, Outcome: capabilityfeedback.OutcomeUnsupported}
	pass := base
	pass.Outcome = capabilityfeedback.OutcomePass
	unsafe := base
	unsafe.Outcome = capabilityfeedback.OutcomeUnsafe

	if !v4CasesPreserved([]capabilityfeedback.CaseEvidence{base}, []capabilityfeedback.CaseEvidence{base}, selected) {
		t.Fatal("matching unique case set should pass")
	}
	if v4CasesPreserved([]capabilityfeedback.CaseEvidence{base}, nil, selected) {
		t.Fatal("missing final case should fail")
	}
	if v4CasesPreserved([]capabilityfeedback.CaseEvidence{base, base}, []capabilityfeedback.CaseEvidence{base}, selected) {
		t.Fatal("duplicate baseline case should fail")
	}
	if v4CasesPreserved([]capabilityfeedback.CaseEvidence{pass}, []capabilityfeedback.CaseEvidence{base}, selected) {
		t.Fatal("baseline pass regression should fail")
	}
	if v4CasesPreserved([]capabilityfeedback.CaseEvidence{unsafe}, []capabilityfeedback.CaseEvidence{pass}, selected) {
		t.Fatal("baseline unsafe to pass should fail")
	}
}

func v4Gap(stage string, scope capabilityfeedback.GapScope, capability, code string, required ...string) capabilityfeedback.Gap {
	return capabilityfeedback.Gap{Stage: stage, Scope: scope, Capability: capability, Code: code, RequiredEvidence: required}
}

func v4CasesPreserved(before, after []capabilityfeedback.CaseEvidence, selected v4SelectedCluster) bool {
	beforeByID, ok := v4UniqueCases(before)
	if !ok {
		return false
	}
	afterByID, ok := v4UniqueCases(after)
	if !ok || len(beforeByID) != len(afterByID) {
		return false
	}
	for id, current := range beforeByID {
		next, exists := afterByID[id]
		if !exists || current.Outcome == capabilityfeedback.OutcomePass && next.Outcome != capabilityfeedback.OutcomePass ||
			current.Outcome == capabilityfeedback.OutcomeUnsafe && next.Outcome == capabilityfeedback.OutcomePass {
			return false
		}
		if next.Outcome != capabilityfeedback.OutcomePass && !v4GapsPreserved(current.Gaps, next.Gaps, selected) {
			return false
		}
	}
	return true
}

func v4UniqueCases(cases []capabilityfeedback.CaseEvidence) (map[string]capabilityfeedback.CaseEvidence, bool) {
	result := make(map[string]capabilityfeedback.CaseEvidence, len(cases))
	for _, current := range cases {
		if current.Case.ID == "" {
			return nil, false
		}
		if _, exists := result[current.Case.ID]; exists {
			return nil, false
		}
		result[current.Case.ID] = current
	}
	return result, true
}

func v4GapsPreserved(before, after []capabilityfeedback.Gap, selected v4SelectedCluster) bool {
	afterSet := map[string]bool{}
	for _, gap := range after {
		afterSet[v4GapIdentity(gap)] = true
	}
	for _, gap := range before {
		if v4MatchesSelected(gap, selected) {
			continue
		}
		if !afterSet[v4GapIdentity(gap)] {
			return false
		}
	}
	return true
}

func v4MatchesSelected(gap capabilityfeedback.Gap, selected v4SelectedCluster) bool {
	return gap.Stage == selected.Stage && gap.Scope == selected.Scope && gap.Capability == selected.Capability && gap.Code == selected.Code
}

func v4GapIdentity(gap capabilityfeedback.Gap) string {
	required := slices.Clone(gap.RequiredEvidence)
	slices.Sort(required)
	required = slices.Compact(required)
	values := make([]string, 0, 4+len(required))
	values = append(values, gap.Stage, string(gap.Scope), gap.Capability, gap.Code)
	values = append(values, required...)
	var encoded strings.Builder
	capacity := 0
	for _, value := range values {
		capacity += len(strconv.Itoa(len(value))) + 1 + len(value)
	}
	encoded.Grow(capacity)
	for _, value := range values {
		encoded.WriteString(strconv.Itoa(len(value)))
		encoded.WriteByte(':')
		encoded.WriteString(value)
	}
	return encoded.String()
}

func v4ContractDirectory(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate V4 contract test source")
	}
	return filepath.Dir(sourceFile)
}

func v4DecodeStrictFile(t *testing.T, path string, target any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("V4 contract JSON contains trailing data: %v", err)
	}
}

func v4FileSHA256(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
