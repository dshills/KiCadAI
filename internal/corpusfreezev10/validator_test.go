package corpusfreezev10

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestPolicyAndPacket(t *testing.T) {
	historyPath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V9_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHistoricalBoundary(history); err != nil {
		t.Fatal(err)
	}
	policy := PolicyForHistory(history.Base.SourceSHA256)
	if policy.Version != 10 || len(policy.AuthorSlots) != 6 || policy.CasesPerAuthor != 8 ||
		len(policy.Domains) != 6 || len(policy.CircuitRoles) != 6 || len(policy.SafetyImpacts) != 4 ||
		policy.PacketSetSHA256 != PacketSetSHA256 || policy.HistoricalCommitmentsSHA256 != history.Base.SourceSHA256 {
		t.Fatalf("unexpected V10 policy: %+v", policy)
	}
	root := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "v10-authoring-packet")
	packet, err := LoadPacket(root, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Assignments) != 6 || len(packet.Binding.AuthorPacketSHA256) != 6 || len(packet.Binding.AssignmentSHA256) != 6 {
		t.Fatalf("unexpected packet shape: %+v", packet.Binding)
	}
	if err := validateFrozenAssignmentPreflight(packet.Assignments, policy); err != nil {
		t.Fatal(err)
	}
}

func TestV10FrozenAssignmentPreflightRejectsLostHighSafetyCoverage(t *testing.T) {
	historyPath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V9_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	policy := PolicyForHistory(history.Base.SourceSHA256)
	root := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "v10-authoring-packet")
	packet, err := LoadPacket(root, policy)
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := decodeAssignment(packet.Assignments["author_1"])
	if err != nil {
		t.Fatal(err)
	}
	for index := range assignment.Entries {
		if assignment.Entries[index].SafetyImpact == "safety_relevant" || assignment.Entries[index].SafetyImpact == "safety_critical" {
			assignment.Entries[index].SafetyImpact = "non_safety"
		}
	}
	data, err := json.Marshal(assignment)
	if err != nil {
		t.Fatal(err)
	}
	packet.Assignments["author_1"] = data
	if err := validateFrozenAssignmentPreflight(packet.Assignments, policy); err == nil || !strings.Contains(err.Error(), "V10_ASSIGNMENT_PREFLIGHT") {
		t.Fatalf("lost high-safety coverage error = %v", err)
	}
}

func TestV10StrictShapes(t *testing.T) {
	assignment := []byte(`{"schema":"kicadai.closed-loop-open-set-author-assignment.v10","version":10,"author_slot":"author_1","entries":[{"id":"x","role":"discovery","domain":"analog_signal_path","circuit_role":"source_bias","safety_impact":"non_safety","primary_class":"static","required_primary_analysis":"dc_sweep","output_multiplicity":"single","require_off_nominal":false,"source_id":"s","requirement_file":"discovery/request_001.json"}]}`)
	if _, err := decodeAssignment(assignment); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(assignment, []byte(`,"primary_class":"static"`), nil, 1),
		bytes.Replace(assignment, []byte(`,"require_off_nominal":false`), nil, 1),
		bytes.Replace(assignment, []byte(`"circuit_role":"source_bias"`), []byte(`"circuit_role":"source_bias","unknown":true`), 1),
	} {
		if _, err := decodeAssignment(invalid); err == nil {
			t.Fatal("invalid assignment shape was accepted")
		}
	}

	templatePath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "v10-authoring-packet", "AUTHORSHIP_TEMPLATE.json")
	template, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	var authorship Authorship
	if err := decodeStrict(template, &authorship); err != nil {
		t.Fatal(err)
	}
	if !authorship.Attestations.allTrue() {
		t.Fatal("frozen V10 template attestations are incomplete")
	}
	old := bytes.Replace(template, []byte(`"no_obligation_anchor_gap_exposure_or_causal_path_authorship"`), []byte(`"no_obligation_anchor_or_causal_path_authorship"`), 1)
	if _, err := decodeAuthorship(old); err == nil {
		t.Fatal("legacy authorship attestation was accepted as V10")
	}
}

func TestBehaviorSignatureCanonicalizesNegativeZero(t *testing.T) {
	positive, negative := 0.0, -0.0
	if got, want := floatPointer(&negative), floatPointer(&positive); got != want || got != "0" {
		t.Fatalf("negative zero = %q, positive zero = %q", got, want)
	}
	if got, want := floatValue(negative), floatValue(positive); got != want || got != "0" {
		t.Fatalf("negative scalar zero = %q, positive scalar zero = %q", got, want)
	}
}

func TestSemanticHashesRejectDanglingOperatingCase(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "internal", "opentopologysynthesis", "testdata", "architecture_generalization_corpus", "regulated_low_voltage_output.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("fixture has %d contract issues", len(issues))
	}
	requirement.Requirements.BehavioralRequirements[0].OperatingCases[0] = "missing_case"
	if _, _, err := semanticHashes(requirement); err == nil {
		t.Fatal("dangling operating-case reference was normalized")
	}
}

func TestV10AnalysisClassMatchesFrozenAssignments(t *testing.T) {
	for analysis, want := range map[string]string{
		"dc_operating_point": "static", "dc_sweep": "static", "thermal": "static", "electrothermal": "static",
		"ac_sweep": "dynamic", "distortion": "dynamic", "noise": "dynamic", "stability": "dynamic", "startup": "dynamic", "transient": "dynamic",
	} {
		if got := analysisClass(analysis); got != want {
			t.Fatalf("analysisClass(%q) = %q, want %q", analysis, got, want)
		}
	}
}

func TestV10ReportFailsClosed(t *testing.T) {
	if _, err := (Report{}).MarshalJSONStable(); err == nil {
		t.Fatal("empty report was accepted")
	}
}

func TestV10PacketPathTraversalRejectsEmptyAndDotSegments(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"/absolute", "nested//file", "nested/./file", "nested/../file", `nested\file`} {
		if validRelativePath(relative) {
			t.Fatalf("validRelativePath(%q) = true", relative)
		}
		if _, err := readRegularFileUnder(root, relative); err == nil {
			t.Fatalf("readRegularFileUnder accepted %q", relative)
		}
	}
}

func TestProhibitedScanExcludesOnlyProtocolSchemaValue(t *testing.T) {
	pattern, err := prohibitedPattern([]string{"topology", "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	requirement := ots.Requirement{Schema: "kicadai.open-topology-requirement.v1", Project: ots.Project{Name: "bounded_behavior", Description: "A neutral behavior study."}}
	if containsProhibitedRequirement(requirement, []string{"v10_case_", "v10_source_"}, pattern) {
		t.Fatal("protocol-owned schema or key triggered prohibited-value scan")
	}
	for name, mutate := range map[string]func(*ots.Requirement){
		"implementation term": func(value *ots.Requirement) { value.Project.Description = "Use a fixture." },
		"manifest identity":   func(value *ots.Requirement) { value.Project.Name = "v10_case_001" },
	} {
		candidate := requirement
		mutate(&candidate)
		if !containsProhibitedRequirement(candidate, []string{"v10_case_", "v10_source_"}, pattern) {
			t.Fatalf("%s did not fail closed", name)
		}
	}
}
