package fabrication

import (
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestCalculateStatus(t *testing.T) {
	if got := CalculateStatus(nil, nil); got != StatusCandidate {
		t.Fatalf("empty evidence status = %s, want candidate", got)
	}
	if got := CalculateStatus([]reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "bad"}}, nil); got != StatusBlocked {
		t.Fatalf("blocking status = %s", got)
	}
	if got := CalculateStatus(nil, map[string]EvidenceStatus{"writer": EvidencePass, "drc": EvidenceMissing}); got != StatusCandidate {
		t.Fatalf("candidate status = %s", got)
	}
	if got := CalculateStatus(nil, map[string]EvidenceStatus{"writer": EvidencePass, "drc": EvidencePass}); got != StatusReady {
		t.Fatalf("ready status = %s", got)
	}
	if got := CalculateStatus(nil, map[string]EvidenceStatus{"writer": EvidenceStatus("unknown")}); got != StatusBlocked {
		t.Fatalf("unknown evidence status = %s, want blocked", got)
	}
	if got := Score(map[string]EvidenceStatus{"a": EvidencePass, "b": EvidenceMissing, "c": EvidencePass}); got != 67 {
		t.Fatalf("score = %d, want 67", got)
	}
}

func TestMarshalManifestDeterministic(t *testing.T) {
	manifest := Manifest{
		Project: ProjectRef{Name: "demo"},
		Status:  StatusBlocked,
		Score:   50,
		Artifacts: []Artifact{
			{Kind: ArtifactDrill, Path: "fabrication/demo.drl", Status: ArtifactMissing},
			{Kind: ArtifactBOM, Path: "fabrication/demo.csv", Status: ArtifactGenerated},
		},
		Evidence: map[string]EvidenceStatus{"drc": EvidenceMissing, "writer_correctness": EvidencePass},
		Issues: []reports.Issue{
			{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: "z", Message: "missing z"},
			{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "a", Message: "bad a"},
		},
	}

	first, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("manifest serialization is not deterministic:\n%s\n---\n%s", first, second)
	}
	text := string(first)
	if !strings.Contains(text, `"schema": "kicadai.fabrication.package.v1"`) ||
		!strings.Contains(text, `"created_by": "kicadai"`) {
		t.Fatalf("manifest missing defaults:\n%s", text)
	}
	if strings.Index(text, `"kind": "bom"`) > strings.Index(text, `"kind": "drill"`) {
		t.Fatalf("artifacts not sorted:\n%s", text)
	}
	if manifest.Artifacts[0].Kind != ArtifactDrill {
		t.Fatalf("MarshalManifest mutated caller artifact order: %#v", manifest.Artifacts)
	}
	normalized := NormalizeManifest(manifest)
	normalized.Evidence["drc"] = EvidencePass
	if manifest.Evidence["drc"] != EvidenceMissing {
		t.Fatalf("NormalizeManifest aliased evidence map: %#v", manifest.Evidence)
	}
}

func TestValidateManifestReportsStructuralIssues(t *testing.T) {
	issues := ValidateManifest(Manifest{
		Schema:  "unknown",
		Project: ProjectRef{Name: "demo"},
		Artifacts: []Artifact{{
			Kind:   ArtifactKind("invalid"),
			Path:   "C:\\tmp\\bom.csv",
			Status: ArtifactStatus("invalid"),
		}},
		Evidence: map[string]EvidenceStatus{"bad": EvidenceStatus("invalid")},
	})
	if len(issues) < 5 {
		t.Fatalf("issues = %#v, want schema, drive-letter path, kind, status, and evidence issues", issues)
	}
	if issues[1].Path != "artifacts[0].path" {
		t.Fatalf("artifact issue path lacks context: %#v", issues)
	}
}

func TestReportArtifactsMapsKinds(t *testing.T) {
	artifacts := ReportArtifacts([]Artifact{
		{Kind: ArtifactBOM, Path: "fabrication/bom.csv"},
		{Kind: ArtifactCPL, Path: "fabrication/placements.csv"},
		{Kind: ArtifactDRC, Path: "fabrication/drc.json"},
	})
	if len(artifacts) != 3 ||
		artifacts[0].Kind != reports.ArtifactBOM ||
		artifacts[1].Kind != reports.ArtifactCPL ||
		artifacts[2].Kind != reports.ArtifactDRCReport {
		t.Fatalf("artifacts = %#v", artifacts)
	}
}
