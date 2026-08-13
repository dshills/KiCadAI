package corpusfreezev9

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
	historyPath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "V8_HISTORICAL_COMMITMENTS.json")
	history, err := LoadHistoricalCommitments(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	policy := PolicyForHistory(history.Base.SourceSHA256)
	if policy.Version != 9 || len(policy.AuthorSlots) != 6 || policy.CasesPerAuthor != 8 ||
		len(policy.Domains) != 6 || len(policy.CircuitRoles) != 6 || len(policy.SafetyImpacts) != 4 ||
		policy.PacketSetSHA256 != PacketSetSHA256 || policy.HistoricalCommitmentsSHA256 != history.Base.SourceSHA256 {
		t.Fatalf("unexpected V9 policy: %+v", policy)
	}
	root := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "v9-authoring-packet")
	packet, err := LoadPacket(root, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Assignments) != 6 || len(packet.Binding.AuthorPacketSHA256) != 6 || len(packet.Binding.AssignmentSHA256) != 6 {
		t.Fatalf("unexpected packet shape: %+v", packet.Binding)
	}
}

func TestV9StrictShapes(t *testing.T) {
	assignment := []byte(`{"schema":"kicadai.closed-loop-open-set-author-assignment.v9","version":9,"author_slot":"author_1","entries":[{"id":"x","role":"discovery","domain":"analog_signal_path","circuit_role":"source_bias","safety_impact":"non_safety","primary_class":"static","required_primary_analysis":"dc_sweep","output_multiplicity":"single","require_off_nominal":false,"source_id":"s","requirement_file":"discovery/request_001.json"}]}`)
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

	templatePath := filepath.Join(repositoryRoot(t), "specs", "closed-loop-open-set-capability-expansion", "v9-authoring-packet", "AUTHORSHIP_TEMPLATE.json")
	template, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	var authorship Authorship
	if err := decodeStrict(template, &authorship); err != nil {
		t.Fatal(err)
	}
	if !authorship.Attestations.allTrue() {
		t.Fatal("frozen V9 template attestations are incomplete")
	}
	old := bytes.Replace(template, []byte(`"no_obligation_anchor_gap_exposure_or_causal_path_authorship"`), []byte(`"no_obligation_anchor_or_causal_path_authorship"`), 1)
	if _, err := decodeAuthorship(old); err == nil {
		t.Fatal("legacy authorship attestation was accepted as V9")
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

func TestV9AnalysisClassMatchesFrozenAssignments(t *testing.T) {
	for analysis, want := range map[string]string{
		"dc_operating_point": "static", "dc_sweep": "static", "thermal": "static", "electrothermal": "static",
		"ac_sweep": "dynamic", "distortion": "dynamic", "noise": "dynamic", "stability": "dynamic", "startup": "dynamic", "transient": "dynamic",
	} {
		if got := analysisClass(analysis); got != want {
			t.Fatalf("analysisClass(%q) = %q, want %q", analysis, got, want)
		}
	}
}

func TestV9ReportFailsClosed(t *testing.T) {
	if _, err := (Report{}).MarshalJSONStable(); err == nil {
		t.Fatal("empty report was accepted")
	}
}

func TestProhibitedScanExcludesOnlyProtocolSchemaValue(t *testing.T) {
	pattern, err := prohibitedPattern([]string{"topology", "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	valid := []byte(`{"schema":"kicadai.open-topology-requirement.v1","project":{"name":"bounded_behavior","description":"A neutral behavior study."},"acceptance":{"require_topology_search":true}}`)
	if containsProhibited(valid, []string{"v9_case_", "v9_source_"}, pattern) {
		t.Fatal("protocol-owned schema or key triggered prohibited-value scan")
	}
	if containsProhibited([]byte(`["neutral behavior"]`), []string{"v9_case_", "v9_source_"}, pattern) {
		t.Fatal("neutral non-object JSON triggered prohibited-value scan")
	}
	for name, data := range map[string][]byte{
		"implementation term": []byte(`{"schema":"kicadai.open-topology-requirement.v1","project":{"description":"Use a fixture."}}`),
		"manifest identity":   []byte(`{"schema":"kicadai.open-topology-requirement.v1","project":{"name":"v9_case_001"}}`),
		"prohibited key":      []byte(`{"schema":"kicadai.open-topology-requirement.v1","fixture":true}`),
		"non-object term":     []byte(`["fixture"]`),
		"invalid JSON":        []byte(`{"schema":`),
	} {
		if !containsProhibited(data, []string{"v9_case_", "v9_source_"}, pattern) {
			t.Fatalf("%s did not fail closed", name)
		}
	}
}
