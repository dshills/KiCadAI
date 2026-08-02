package repairloop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const crossStageRepairCorpusManifestHash = "9a071ef14dcb85cdb6fece58f0f26bf9afb4898491e6c48efbc7e03604045e99"

type crossStageRepairCorpusManifest struct {
	Schema     string `json:"schema"`
	Version    int    `json:"version"`
	BaseCommit string `json:"base_commit"`
	FrozenAt   string `json:"frozen_at"`
	Cases      []struct {
		ID     string `json:"id"`
		Stage  string `json:"stage"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"cases"`
}

type crossStageRepairCorpusCase struct {
	Schema       string `json:"schema"`
	Version      int    `json:"version"`
	ID           string `json:"id"`
	FailureStage string `json:"failure_stage"`
	Domain       string `json:"domain"`
	Fault        struct {
		Code     string `json:"code"`
		Category string `json:"category"`
		Mutation string `json:"mutation"`
	} `json:"fault"`
	Expected struct {
		Operator     string   `json:"operator"`
		ReenterStage string   `json:"reenter_stage"`
		Preserve     []string `json:"preserve"`
	} `json:"expected"`
}

func TestCrossStageRepairCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := filepath.Join("testdata", "cross_stage_corpus")
	manifestData := crossStageReadFile(t, filepath.Join(root, "manifest.json"))
	if got := crossStageContentHash(manifestData); got != crossStageRepairCorpusManifestHash {
		t.Fatalf("cross-stage corpus manifest hash = %s, want %s", got, crossStageRepairCorpusManifestHash)
	}
	var manifest crossStageRepairCorpusManifest
	crossStageDecodeStrict(t, manifestData, &manifest)
	wantStages := []string{"simulation", "schematic", "erc", "placement", "routing", "connectivity", "drc", "writer", "roundtrip"}
	if manifest.Schema != "kicadai.cross-stage-repair-corpus.v1" || manifest.Version != 1 ||
		manifest.BaseCommit != "b6ea64c2f0b98cd749c5caba2a80e9e52960b292" || len(manifest.Cases) != len(wantStages) {
		t.Fatalf("cross-stage corpus identity = %#v", manifest)
	}
	gotStages := make([]string, 0, len(manifest.Cases))
	seenIDs := map[string]struct{}{}
	for _, item := range manifest.Cases {
		if _, exists := seenIDs[item.ID]; exists {
			t.Fatalf("duplicate cross-stage corpus ID %q", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		cleanPath := filepath.Clean(filepath.FromSlash(item.Path))
		if filepath.IsAbs(cleanPath) || cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
			t.Fatalf("case %q has unsafe path %q", item.ID, item.Path)
		}
		caseData := crossStageReadFile(t, filepath.Join(root, cleanPath))
		if got := crossStageContentHash(caseData); got != item.SHA256 {
			t.Fatalf("case %q hash = %s, want %s", item.ID, got, item.SHA256)
		}
		var frozen crossStageRepairCorpusCase
		crossStageDecodeStrict(t, caseData, &frozen)
		if frozen.Schema != "kicadai.cross-stage-repair-case.v1" || frozen.Version != 1 || frozen.ID != item.ID || frozen.FailureStage != item.Stage {
			t.Fatalf("case %q identity = %#v", item.ID, frozen)
		}
		if strings.TrimSpace(frozen.Domain) == "" || strings.TrimSpace(frozen.Fault.Code) == "" ||
			strings.TrimSpace(frozen.Fault.Category) == "" || strings.TrimSpace(frozen.Fault.Mutation) == "" ||
			strings.TrimSpace(frozen.Expected.Operator) == "" || strings.TrimSpace(frozen.Expected.ReenterStage) == "" || len(frozen.Expected.Preserve) == 0 {
			t.Fatalf("case %q lacks frozen fault or acceptance evidence: %#v", item.ID, frozen)
		}
		gotStages = append(gotStages, item.Stage)
	}
	if !reflect.DeepEqual(gotStages, wantStages) {
		t.Fatalf("cross-stage corpus stages = %#v, want %#v", gotStages, wantStages)
	}
}

func crossStageReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func crossStageDecodeStrict(t *testing.T, data []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("cross-stage corpus file contains trailing JSON data: %v", err)
	}
}

func crossStageContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
