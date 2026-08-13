package capabilityfeedback

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

const (
	closedLoopV8ImplementationSealSchema = "kicadai.closed-loop-open-set-reviewed-implementation.v8"
	closedLoopV8ImplementationSealFile   = "V8_REVIEWED_IMPLEMENTATION.json"
	closedLoopV8ImplementationCommit     = "e8cda322d450ea97c8feed97e08afd415ffee06a"
	closedLoopV8ImplementationReview     = "prism_reviewed_no_high_or_medium_findings"
	closedLoopV8ImplementationFileCount  = 11
	closedLoopV8PrerequisiteCount        = 3
)

type closedLoopV8ImplementationSeal struct {
	Schema                  string                           `json:"schema"`
	Version                 int                              `json:"version"`
	SelectionSHA256         string                           `json:"selection_sha256"`
	EffectPlanSHA256        string                           `json:"effect_plan_sha256"`
	ImplementationCommit    string                           `json:"implementation_commit"`
	Review                  string                           `json:"review"`
	DirectMemberKeys        []string                         `json:"direct_member_keys"`
	ClosureMemberKeys       []string                         `json:"closure_member_keys"`
	RequiredEvidence        []string                         `json:"required_evidence"`
	Artifacts               []closedLoopV8ImplementationFile `json:"artifacts"`
	PrerequisiteConsumers   []closedLoopV8Prerequisite       `json:"prerequisite_consumers"`
	ReverseCallGraph        []string                         `json:"reverse_call_graph"`
	FocusedRuntimeConsumers []string                         `json:"focused_runtime_consumers"`
	Hash                    string                           `json:"hash"`
}

type closedLoopV8ImplementationFile struct {
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	MemberKey    string `json:"member_key"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
}

type closedLoopV8Prerequisite struct {
	Path   string   `json:"path"`
	Reason string   `json:"reason"`
	Tests  []string `json:"tests"`
}

func TestClosedLoopV8ReviewedImplementationSealIsFrozen(t *testing.T) {
	path := filepath.Join(closedLoopSpecDirectory(t), closedLoopV8ImplementationSealFile)
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V8_REVIEWED_IMPLEMENTATION.sha256"), filepath.Base(path), data)
	var seal closedLoopV8ImplementationSeal
	decodeCorpusStrict(t, data, &seal)
	var selection closedLoopV8SelectionDecision
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8SelectionRoot, "selection.json")), &selection)
	var plan closedLoopV8EffectPlanDocument
	planSource := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV8EffectPlanFile))
	decodeCorpusStrict(t, planSource, &plan)
	if seal.Schema != closedLoopV8ImplementationSealSchema || seal.Version != 8 {
		t.Fatalf("V8 reviewed implementation schema/version = %q/%d", seal.Schema, seal.Version)
	}
	if seal.SelectionSHA256 != selection.Hash || seal.EffectPlanSHA256 != corpusHash(planSource) {
		t.Fatal("V8 reviewed implementation selection or effect-plan binding is invalid")
	}
	if seal.ImplementationCommit != closedLoopV8ImplementationCommit || seal.Review != closedLoopV8ImplementationReview {
		t.Fatal("V8 reviewed implementation commit or Prism-review binding is invalid")
	}
	closureKeys := make([]string, len(plan.ClosureMembers))
	for index := range plan.ClosureMembers {
		closureKeys[index] = plan.ClosureMembers[index].Key
	}
	if !slices.Equal(seal.DirectMemberKeys, plan.DirectMemberKeys) || !slices.Equal(seal.ClosureMemberKeys, closureKeys) ||
		!slices.Equal(seal.RequiredEvidence, plan.RequiredEvidence) ||
		!slices.Equal(seal.ReverseCallGraph, plan.StaticEvidence.ReverseCallGraph) ||
		!slices.Equal(seal.FocusedRuntimeConsumers, plan.StaticEvidence.FocusedNonCorpusRuntimeConsumers) {
		t.Fatal("V8 reviewed implementation differs from the frozen effect plan")
	}
	assertClosedLoopV8ImplementationFiles(t, seal, plan)
	if len(seal.PrerequisiteConsumers) != closedLoopV8PrerequisiteCount {
		t.Fatal("V8 reviewed implementation prerequisite-consumer evidence is incomplete")
	}
	for _, prerequisite := range seal.PrerequisiteConsumers {
		if prerequisite.Path == "" || prerequisite.Reason == "" || len(prerequisite.Tests) == 0 {
			t.Fatal("V8 reviewed implementation prerequisite-consumer evidence is invalid")
		}
	}
	if want, err := hashClosedLoopV8ImplementationSeal(seal); err != nil || want != seal.Hash {
		t.Fatalf("V8 reviewed implementation self-hash = %q, want %q: %v", seal.Hash, want, err)
	}
}

func assertClosedLoopV8ImplementationFiles(t *testing.T, seal closedLoopV8ImplementationSeal, plan closedLoopV8EffectPlanDocument) {
	t.Helper()
	if len(seal.DirectMemberKeys) != 1 || len(seal.Artifacts) != closedLoopV8ImplementationFileCount {
		t.Fatal("V8 reviewed implementation artifact or member cardinality is invalid")
	}
	root := closedLoopModuleRoot(t)
	artifacts := make(map[string]closedLoopV8ImplementationFile, len(seal.Artifacts))
	changed := make([]string, 0, len(seal.Artifacts))
	previous := ""
	for _, artifact := range seal.Artifacts {
		if artifact.Path <= previous || artifacts[artifact.Path].Path != "" ||
			(artifact.Kind != "production" && artifact.Kind != "verification") ||
			artifact.MemberKey != seal.DirectMemberKeys[0] ||
			(artifact.BeforeSHA256 != "absent" && !closedLoopV8ValidHash(artifact.BeforeSHA256)) ||
			!closedLoopV8ValidHash(artifact.AfterSHA256) {
			t.Fatalf("V8 reviewed implementation artifact is invalid: %#v", artifact)
		}
		previous = artifact.Path
		artifacts[artifact.Path] = artifact
		if got := corpusHash(mustCorpusRead(t, filepath.Join(root, filepath.FromSlash(artifact.Path)))); got != artifact.AfterSHA256 {
			t.Fatalf("V8 reviewed implementation artifact %s = %s, want %s", artifact.Path, got, artifact.AfterSHA256)
		}
		if artifact.BeforeSHA256 != artifact.AfterSHA256 {
			changed = append(changed, artifact.Path)
		}
	}
	planned := append(append([]closedLoopV8SourceBinding(nil), plan.StaticEvidence.ProductionFiles...), plan.StaticEvidence.VerificationFiles...)
	for _, binding := range planned {
		artifact, found := artifacts[binding.Path]
		if !found || artifact.BeforeSHA256 != binding.SHA256 {
			t.Fatalf("V8 reviewed implementation omitted frozen source binding: %s", binding.Path)
		}
	}
	wantChanged := []string{
		"internal/capabilityfeedback/v5_reviewed_implementation_test.go",
		"internal/capabilityfeedback/v7_reviewed_implementation_test.go",
		"internal/capabilityfeedback/v8_round1_public_test.go",
		"internal/capabilityfeedback/v8_selection_test.go",
		"internal/opentopologysynthesis/simulation.go",
		"internal/opentopologysynthesis/simulation_test.go",
	}
	if !slices.Equal(changed, wantChanged) {
		t.Fatalf("V8 reviewed implementation changed paths = %#v, want %#v", changed, wantChanged)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Log("Git unavailable; current bytes and frozen implementation metadata verified without historical commit-path projection")
		return
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Log("Git metadata unavailable; current bytes and frozen implementation metadata verified without historical commit-path projection")
		return
	}
	if err := exec.Command("git", "-C", root, "cat-file", "-e", seal.ImplementationCommit+"^{commit}").Run(); err != nil {
		t.Log("implementation commit unreachable; current bytes and frozen implementation metadata verified without historical commit-path projection")
		return
	}
	output, err := exec.Command("git", "-C", root, "show", "--format=", "--name-only", "-z", seal.ImplementationCommit).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect V8 implementation commit: %v", err)
	}
	trimmed := bytes.TrimSuffix(output, []byte{0})
	if len(trimmed) == 0 {
		t.Fatal("V8 implementation commit contains no changed paths")
	}
	fields := bytes.Split(trimmed, []byte{0})
	commitPaths := make([]string, len(fields))
	for index := range fields {
		commitPaths[index] = string(fields[index])
	}
	slices.Sort(commitPaths)
	if !slices.Equal(commitPaths, wantChanged) {
		t.Fatalf("V8 implementation commit paths = %#v, want %#v", commitPaths, wantChanged)
	}
}

func hashClosedLoopV8ImplementationSeal(value closedLoopV8ImplementationSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}
