package practicalboardeval

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/designworkflow"
)

func TestWriteNewDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence")
	if err := WriteNew(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(path, []byte("second")); err == nil {
		t.Fatal("overwrote evidence")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("evidence changed: %q, %v", data, err)
	}
}

func TestNativeNormalizationIsNarrow(t *testing.T) {
	build := func(root, suffix string, duration, actual float64) map[string]any {
		return map[string]any{"actual": actual, "duration_ms": actual, "workflow": map[string]any{"stages": []any{map[string]any{
			"name": "kicad_checks", "summary": map[string]any{"erc": map[string]any{"duration_ms": duration, "violations": 0, "report_path": root + "/project/.kicadai/checks/kicadai-check-erc-" + suffix + "/erc.json"}},
		}}}}
	}
	a, err := NormalizeEvidence(build("/first", "123", 200, 1.25), "/first")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NormalizeEvidence(build("/second", "456", 300, 1.25), "/second")
	if err != nil || string(a) != string(b) {
		t.Fatalf("native volatility differs: %s %s %v", a, b, err)
	}
	c, err := NormalizeEvidence(build("/second", "456", 300, 1.26), "/second")
	if err != nil || string(a) == string(c) {
		t.Fatal("measured duration or value normalized away")
	}
}

func TestCompressedEvidenceIsCompleteAndDeterministic(t *testing.T) {
	root := t.TempDir()
	value := map[string]any{"a": []int{1, 2, 3}, "proof": "retained"}
	for _, name := range []string{"a.gz", "b.gz"} {
		if err := WriteGzipJSON(filepath.Join(root, name), value); err != nil {
			t.Fatal(err)
		}
	}
	a, err := os.ReadFile(filepath.Join(root, "a.gz"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "b.gz"))
	if err != nil || string(a) != string(b) {
		t.Fatal("compressed evidence is not deterministic")
	}
	f, err := os.Open(filepath.Join(root, "a.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	decoded, err := io.ReadAll(z)
	if err != nil || string(decoded) != "{\"a\":[1,2,3],\"proof\":\"retained\"}\n" {
		t.Fatalf("incomplete gzip: %s %v", decoded, err)
	}
}

func TestProjectIdentityIncludesHierarchicalSheets(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sch"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(filepath.Join(root, "root.kicad_sch"), []byte("root")); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(filepath.Join(root, "sch", "child.kicad_sch"), []byte("child")); err != nil {
		t.Fatal(err)
	}
	identity, err := ProjectIdentity(root)
	if err != nil || len(identity) != 2 || identity["sch/child.kicad_sch"] == "" {
		t.Fatalf("hierarchy omitted: %v %v", identity, err)
	}
}

func TestCommonRequirementGateRejectsDroppedProofAndTemperature(t *testing.T) {
	r := architecturesearch.Requirement{}
	if got := CommonRequirementGate(r); got != "acceptance.require_erc" {
		t.Fatal(got)
	}
	r.Acceptance = architecturesearch.Acceptance{
		RequireERC: true, RequireStrictDRC: true, RequireCompleteRouting: true, RequireConnectivity: true,
		RequireWriterCorrectness: true, RequireRoundTripZeroDiff: true, RequireDeterministicReplay: true,
		RequireSimulation: true, RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true,
	}
	if got := CommonRequirementGate(r); got != "requirements.constraints.board_enclosure" {
		t.Fatal(got)
	}
	r.Requirements.Constraints = architecturesearch.BoardLimits{MaxWidthMM: 100, MaxHeightMM: 80}
	if got := CommonRequirementGate(r); got != "requirements.operating_cases.ambient_temperature" {
		t.Fatal(got)
	}
	low, high := -10.0, 50.0
	r.Requirements.OperatingCases = []architecturesearch.OperatingCase{{Conditions: []architecturesearch.OperatingCondition{{Axis: "ambient_temperature", Unit: "degC", Min: &low, Max: &high}}}}
	if got := CommonRequirementGate(r); got != "" {
		t.Fatal(got)
	}
	r.Requirements.OperatingCases[0].Conditions[0].Min = nil
	if CommonRequirementGate(r) == "" {
		t.Fatal("missing temperature bound accepted")
	}
}

func TestRequiredStagesFailClosed(t *testing.T) {
	var result designworkflow.WorkflowResult
	for _, name := range requiredStages {
		result.Stages = append(result.Stages, designworkflow.StageResult{Name: name, Status: designworkflow.StageStatusOK})
	}
	if got := FirstFailedStage(result); got != "" {
		t.Fatal(got)
	}
	for _, status := range []designworkflow.StageStatus{"", designworkflow.StageStatusSkipped, designworkflow.StageStatusWarning, designworkflow.StageStatusBlocked} {
		result.Stages[0].Status = status
		if got := FirstFailedStage(result); got != "schematic" {
			t.Fatalf("%s accepted: %s", status, got)
		}
	}
	result.Stages = nil
	if FirstFailedStage(result) == "" {
		t.Fatal("missing evidence passed")
	}
}

func TestNormalizationRetainsElectricalDifferences(t *testing.T) {
	a, err := NormalizeEvidence(map[string]any{"path": "/first/output.json", "actual": 1.25, "status": "pass"}, "/first")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NormalizeEvidence(map[string]any{"path": "/second/output.json", "actual": 1.25, "status": "pass"}, "/second")
	if err != nil || string(a) != string(b) {
		t.Fatalf("path normalization: %s %s %v", a, b, err)
	}
	changed, err := NormalizeEvidence(map[string]any{"path": "/second/output.json", "actual": 1.26, "status": "pass"}, "/second")
	if err != nil || string(a) == string(changed) {
		t.Fatal("electrical difference erased")
	}
}

func TestInventoryRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("/etc/hosts", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Inventory(root); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestFreezeDetectsMutation(t *testing.T) {
	root := t.TempDir()
	if err := WriteNew(filepath.Join(root, "input.json"), []byte("{}")); err != nil {
		t.Fatal(err)
	}
	files, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	f := Freeze{Schema: "kicadai.practical-board-freeze.v1", BaselineCommit: "87c411b7a13bdeb0efc6ff36b35e9a69c4e2706d", Files: files}
	if err := VerifyFreeze(root, f); err != nil {
		t.Fatal(err)
	}
	f.Files[0].SHA256 = SHA([]byte("changed"))
	if err := VerifyFreeze(root, f); err == nil {
		t.Fatal("changed seal accepted")
	}
	f.Files[0].Path = "../escape"
	if err := VerifyFreeze(root, f); err == nil {
		t.Fatal("escaping path accepted")
	}
}
