package repairloop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const repairCorpusManifestHash = "578ccfe5389e926374d87afcb77389ba776c59edd4ce794b096da1708d429829"

type repairCorpusManifest struct {
	Schema     string `json:"schema"`
	Version    int    `json:"version"`
	BaseCommit string `json:"base_commit"`
	FrozenAt   string `json:"frozen_at"`
	Cases      []struct {
		ID             string `json:"id"`
		Stage          string `json:"stage"`
		Source         string `json:"source"`
		SHA256         string `json:"sha256"`
		Transformation string `json:"transformation,omitempty"`
		InitialStatus  string `json:"initial_status"`
		Acceptance     string `json:"acceptance"`
	} `json:"cases"`
}

func TestDiagnosisDrivenRepairCorpusIsFrozen(t *testing.T) {
	manifestPath := filepath.Join("testdata", "corpus", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := repairCorpusHash(data); got != repairCorpusManifestHash {
		t.Fatalf("repair corpus manifest hash = %s, want %s", got, repairCorpusManifestHash)
	}
	var manifest repairCorpusManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.diagnosis-driven-repair-corpus.v1" || manifest.Version != 1 ||
		manifest.BaseCommit != "2365b35c0be54ec1a3fa9bd89a39dda2be1e9a08" || len(manifest.Cases) != 2 {
		t.Fatalf("repair corpus identity = %#v", manifest)
	}
	repositoryRoot := filepath.Join("..", "..")
	for _, item := range manifest.Cases {
		content, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(item.Source)))
		if err != nil {
			t.Fatalf("%s: %v", item.ID, err)
		}
		if got := repairCorpusHash(content); got != item.SHA256 {
			t.Fatalf("%s source hash = %s, want %s", item.ID, got, item.SHA256)
		}
		if item.ID == "dense_multi_endpoint_route_conflict" && item.Transformation == "" {
			t.Fatal("dense physical case lacks its frozen identity-neutral transformation")
		}
	}
}

func repairCorpusHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
