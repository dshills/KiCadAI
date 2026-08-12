package capabilityfeedback

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	closedLoopV7ImplementationSealSchema    = "kicadai.closed-loop-open-set-reviewed-implementation.v7"
	closedLoopV7ImplementationSealUpdateEnv = "UPDATE_CLOSED_LOOP_V7_IMPLEMENTATION_SEAL"
	closedLoopV7ImplementationCommit        = "4c6a5733ebd29797c1d8c8b657f8a12d3e16184f"
	closedLoopV7ImplementationReview        = "prism_reviewed_no_actionable_findings"
)

type closedLoopV7ImplementationSeal struct {
	Schema                string                             `json:"schema"`
	Version               int                                `json:"version"`
	SelectionSHA256       string                             `json:"selection_sha256"`
	GenericPlanSHA256     string                             `json:"generic_plan_sha256"`
	ImplementationCommit  string                             `json:"implementation_commit"`
	Review                string                             `json:"review"`
	CapabilityBindings    []closedLoopV7CapabilityBinding    `json:"capability_bindings"`
	ProductionArtifacts   []closedLoopArtifactEvidence       `json:"production_artifacts"`
	VerificationArtifacts []closedLoopArtifactEvidence       `json:"verification_artifacts"`
	HistoricalProjections []closedLoopV7HistoricalProjection `json:"historical_projections"`
	PrerequisiteConsumers []closedLoopV7PrerequisiteConsumer `json:"prerequisite_consumers"`
	Hash                  string                             `json:"hash"`
}

type closedLoopV7CapabilityBinding struct {
	Capability    string   `json:"capability"`
	MemberCode    string   `json:"member_code"`
	ArtifactPaths []string `json:"artifact_paths"`
}

type closedLoopV7HistoricalProjection struct {
	Round        int    `json:"round"`
	Path         string `json:"path"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
	Capability   string `json:"capability"`
}

type closedLoopV7PrerequisiteConsumer struct {
	Path          string   `json:"path"`
	StaticTests   []string `json:"static_tests"`
	RuntimeTests  []string `json:"runtime_tests"`
	ProductionUse bool     `json:"production_use"`
}

func TestClosedLoopV7ReviewedImplementationSealIsFrozen(t *testing.T) {
	path := filepath.Join(closedLoopSpecDirectory(t), "V7_REVIEWED_IMPLEMENTATION.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loadClosedLoopV7CurrentImplementationSeal(t)
}

func TestUpdateClosedLoopV7ReviewedImplementationSeal(t *testing.T) {
	if os.Getenv(closedLoopV7ImplementationSealUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V7_IMPLEMENTATION_SEAL=1 to seal the reviewed V7 implementation")
	}
	specRoot := closedLoopSpecDirectory(t)
	jsonPath := filepath.Join(specRoot, "V7_REVIEWED_IMPLEMENTATION.json")
	checksumPath := filepath.Join(specRoot, "V7_REVIEWED_IMPLEMENTATION.sha256")
	for _, path := range []string{jsonPath, checksumPath} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("V7 reviewed implementation seal already exists at %s; refusing overwrite", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat V7 reviewed implementation seal %s: %v", path, err)
		}
	}
	moduleRoot := closedLoopModuleRoot(t)
	artifactEvidence := func(paths []string) []closedLoopArtifactEvidence {
		artifacts := make([]closedLoopArtifactEvidence, 0, len(paths))
		for _, path := range paths {
			artifacts = append(artifacts, closedLoopArtifactEvidence{
				Path: path, SHA256: corpusHash(mustCorpusRead(t, filepath.Join(moduleRoot, filepath.FromSlash(path)))),
			})
		}
		return artifacts
	}
	production := artifactEvidence(closedLoopV7ProductionArtifactPaths())
	verification := artifactEvidence(closedLoopV7VerificationArtifactPaths())
	selection := loadClosedLoopV7FrozenSelection(t)
	plan := loadClosedLoopV7FrozenGenericPlan(t)
	seal := closedLoopV7ImplementationSeal{
		Schema: closedLoopV7ImplementationSealSchema, Version: closedLoopV7BaselineVersion,
		SelectionSHA256: selection.Hash, GenericPlanSHA256: plan.Hash,
		ImplementationCommit: closedLoopV7ImplementationCommit, Review: closedLoopV7ImplementationReview,
		CapabilityBindings:  closedLoopV7ImplementationCapabilityBindings(),
		ProductionArtifacts: production, VerificationArtifacts: verification,
		HistoricalProjections: closedLoopV7HistoricalProjections(t, production, verification),
		PrerequisiteConsumers: closedLoopV7PrerequisiteConsumers(),
	}
	var err error
	seal.Hash, err = hashClosedLoopV7ImplementationSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	data := corpusJSON(t, seal)
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	checksum := []byte(fmt.Sprintf("%s  %s\n", corpusHash(data), filepath.Base(jsonPath)))
	if err := os.WriteFile(checksumPath, checksum, 0o644); err != nil {
		t.Fatal(err)
	}
	loadClosedLoopV7CurrentImplementationSeal(t)
}

func loadClosedLoopV7CurrentImplementationSeal(t *testing.T) closedLoopV7ImplementationSeal {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V7_REVIEWED_IMPLEMENTATION.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V7_REVIEWED_IMPLEMENTATION.sha256"), filepath.Base(path), data)
	var seal closedLoopV7ImplementationSeal
	decodeCorpusStrict(t, data, &seal)
	selection := loadClosedLoopV7FrozenSelection(t)
	plan := loadClosedLoopV7FrozenGenericPlan(t)
	if seal.Schema != closedLoopV7ImplementationSealSchema || seal.Version != closedLoopV7BaselineVersion {
		t.Fatalf("V7 reviewed implementation schema/version = %q/%d", seal.Schema, seal.Version)
	}
	if seal.SelectionSHA256 != selection.Hash || seal.GenericPlanSHA256 != plan.Hash {
		t.Fatal("V7 reviewed implementation selection or generic-plan binding is invalid")
	}
	if seal.ImplementationCommit != closedLoopV7ImplementationCommit || seal.Review != closedLoopV7ImplementationReview {
		t.Fatal("V7 reviewed implementation commit or review binding is invalid")
	}
	if !slices.Equal(seal.ProductionArtifacts, currentClosedLoopV7ArtifactEvidence(t, closedLoopV7ProductionArtifactPaths())) ||
		!slices.Equal(seal.VerificationArtifacts, currentClosedLoopV7ArtifactEvidence(t, closedLoopV7VerificationArtifactPaths())) {
		t.Fatal("V7 reviewed implementation artifacts drifted")
	}
	if !slices.EqualFunc(seal.CapabilityBindings, closedLoopV7ImplementationCapabilityBindings(), func(left, right closedLoopV7CapabilityBinding) bool {
		return left.Capability == right.Capability && left.MemberCode == right.MemberCode && slices.Equal(left.ArtifactPaths, right.ArtifactPaths)
	}) {
		t.Fatal("V7 reviewed implementation capability bindings are invalid")
	}
	wantProjections := closedLoopV7HistoricalProjections(t, seal.ProductionArtifacts, seal.VerificationArtifacts)
	if !slices.Equal(seal.HistoricalProjections, wantProjections) ||
		!slices.EqualFunc(seal.PrerequisiteConsumers, closedLoopV7PrerequisiteConsumers(), func(left, right closedLoopV7PrerequisiteConsumer) bool {
			return left.Path == right.Path && slices.Equal(left.StaticTests, right.StaticTests) &&
				slices.Equal(left.RuntimeTests, right.RuntimeTests) && left.ProductionUse == right.ProductionUse
		}) {
		t.Fatal("V7 reviewed implementation prerequisite evidence is invalid")
	}
	if want, err := hashClosedLoopV7ImplementationSeal(seal); err != nil || want != seal.Hash {
		t.Fatal("V7 reviewed implementation seal hash is invalid")
	}
	return seal
}

func loadClosedLoopV7FrozenGenericPlan(t *testing.T) closedLoopV7GenericPlan {
	t.Helper()
	data := mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "generic_plan.json"))
	var plan closedLoopV7GenericPlan
	decodeCorpusStrict(t, data, &plan)
	if want, err := hashClosedLoopV7GenericPlan(plan); err != nil || want != plan.Hash || plan.Hash != closedLoopV7GenericPlanHash {
		t.Fatal("V7 generic implementation plan hash is invalid")
	}
	return plan
}

func currentClosedLoopV7ArtifactEvidence(t *testing.T, paths []string) []closedLoopArtifactEvidence {
	t.Helper()
	root := closedLoopModuleRoot(t)
	result := make([]closedLoopArtifactEvidence, 0, len(paths))
	for _, path := range paths {
		result = append(result, closedLoopArtifactEvidence{Path: path, SHA256: corpusHash(mustCorpusRead(t, filepath.Join(root, filepath.FromSlash(path))))})
	}
	return result
}

func closedLoopV7ProductionArtifactPaths() []string {
	return []string{
		"internal/opentopologysynthesis/causal_repair.go",
		"internal/opentopologysynthesis/causal_topology_repair.go",
		"internal/opentopologysynthesis/multi_obligation_composition.go",
		"internal/opentopologysynthesis/multi_output_composition.go",
		"internal/opentopologysynthesis/realizability.go",
		"internal/opentopologysynthesis/search.go",
		"internal/simmodel/mna_measurements.go",
		"internal/simmodel/mna_registry.go",
		"internal/simmodel/mna_solver.go",
	}
}

func closedLoopV7VerificationArtifactPaths() []string {
	return []string{
		"internal/capabilityfeedback/v5_reviewed_implementation_test.go",
		"internal/capabilityfeedback/v6_reviewed_implementation_test.go",
		"internal/opentopologysynthesis/causal_topology_repair_test.go",
		"internal/opentopologysynthesis/dc_input_impedance_simulation_test.go",
		"internal/opentopologysynthesis/multi_obligation_composition_test.go",
		"internal/opentopologysynthesis/realizability_test.go",
		"internal/simmodel/mna_dc_input_impedance_test.go",
	}
}

func closedLoopV7ImplementationCapabilityBindings() []closedLoopV7CapabilityBinding {
	return []closedLoopV7CapabilityBinding{
		{Capability: "causal_topology_repair", MemberCode: "OPEN_TOPOLOGY_REPAIR_EXHAUSTED", ArtifactPaths: []string{
			"internal/opentopologysynthesis/causal_repair.go", "internal/opentopologysynthesis/causal_topology_repair.go",
		}},
		{Capability: "dc_operating_point_solver", MemberCode: "SIMULATION_INVALID", ArtifactPaths: []string{
			"internal/simmodel/mna_measurements.go", "internal/simmodel/mna_registry.go", "internal/simmodel/mna_solver.go",
		}},
		{Capability: "multi_obligation_composition", MemberCode: "OPEN_TOPOLOGY_MULTI_CONTROL_COMPOSITION_REQUIRED", ArtifactPaths: []string{
			"internal/opentopologysynthesis/multi_obligation_composition.go", "internal/opentopologysynthesis/multi_output_composition.go",
			"internal/opentopologysynthesis/realizability.go", "internal/opentopologysynthesis/search.go",
		}},
	}
}

func closedLoopV7HistoricalProjections(t *testing.T, production, verification []closedLoopArtifactEvidence) []closedLoopV7HistoricalProjection {
	t.Helper()
	after := map[string]string{}
	for _, artifact := range append(append([]closedLoopArtifactEvidence(nil), production...), verification...) {
		after[artifact.Path] = artifact.SHA256
	}
	v5 := loadClosedLoopV5HistoricalImplementationSeal(t)
	v6 := loadClosedLoopV6HistoricalImplementationSeal(t)
	before := func(artifacts []closedLoopArtifactEvidence, path string) string {
		for _, artifact := range artifacts {
			if artifact.Path == path {
				return artifact.SHA256
			}
		}
		t.Fatalf("historical implementation seal lacks projected path %s", path)
		return ""
	}
	afterHash := func(path string) string {
		hash, found := after[path]
		if !found || !closedLoopV7ValidHash(hash) {
			t.Fatalf("V7 implementation artifacts lack projected path %s", path)
		}
		return hash
	}
	v6Artifacts := append(append([]closedLoopArtifactEvidence(nil), v6.ProductionArtifacts...), v6.VerificationArtifacts...)
	return []closedLoopV7HistoricalProjection{
		{Round: 5, Path: "internal/simmodel/mna_registry.go", BeforeSHA256: before(v5.Artifacts, "internal/simmodel/mna_registry.go"), AfterSHA256: afterHash("internal/simmodel/mna_registry.go"), Capability: "dc_operating_point_solver"},
		{Round: 6, Path: "internal/opentopologysynthesis/multi_output_composition.go", BeforeSHA256: before(v6Artifacts, "internal/opentopologysynthesis/multi_output_composition.go"), AfterSHA256: afterHash("internal/opentopologysynthesis/multi_output_composition.go"), Capability: "multi_obligation_composition"},
		{Round: 6, Path: "internal/opentopologysynthesis/realizability.go", BeforeSHA256: before(v6Artifacts, "internal/opentopologysynthesis/realizability.go"), AfterSHA256: afterHash("internal/opentopologysynthesis/realizability.go"), Capability: "multi_obligation_composition"},
		{Round: 6, Path: "internal/opentopologysynthesis/realizability_test.go", BeforeSHA256: before(v6Artifacts, "internal/opentopologysynthesis/realizability_test.go"), AfterSHA256: afterHash("internal/opentopologysynthesis/realizability_test.go"), Capability: "multi_obligation_composition"},
		{Round: 6, Path: "internal/opentopologysynthesis/search.go", BeforeSHA256: before(v6Artifacts, "internal/opentopologysynthesis/search.go"), AfterSHA256: afterHash("internal/opentopologysynthesis/search.go"), Capability: "multi_obligation_composition"},
	}
}

func closedLoopV7PrerequisiteConsumers() []closedLoopV7PrerequisiteConsumer {
	return []closedLoopV7PrerequisiteConsumer{
		{Path: "internal/capabilityfeedback/v5_reviewed_implementation_test.go", StaticTests: []string{"TestClosedLoopV5ReviewedImplementationSealIsFrozen"}, RuntimeTests: []string{"TestClosedLoopV5PublicAdmissionArtifactsAreFrozen", "TestClosedLoopV5ReviewedImplementationSealIsFrozen"}, ProductionUse: false},
		{Path: "internal/capabilityfeedback/v6_reviewed_implementation_test.go", StaticTests: []string{"TestClosedLoopV6PublicAdmissionArtifactsAreFrozen", "TestClosedLoopV6ReviewedImplementationSealIsFrozen", "TestUpdateClosedLoopV6ReviewedImplementationSeal"}, RuntimeTests: []string{"TestClosedLoopV6PublicAdmissionArtifactsAreFrozen", "TestClosedLoopV6ReviewedImplementationSealIsFrozen"}, ProductionUse: false},
	}
}

func hashClosedLoopV7ImplementationSeal(value closedLoopV7ImplementationSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}
