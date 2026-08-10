package closedloopopensetcontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev6"
)

const updateV6HistoricalCommitments = "UPDATE_V6_HISTORICAL_COMMITMENTS"

type v6HistoricalCommitmentFile = corpusfreezev6.HistoricalCommitmentFile

type v6HistoricalCorpusManifest struct {
	Schema  string                    `json:"schema"`
	Version int                       `json:"version"`
	Entries []v6HistoricalCorpusEntry `json:"entries"`
}

type v6HistoricalCorpusEntry struct {
	ID                       string `json:"id"`
	AuthorSlot               string `json:"author_slot"`
	Role                     string `json:"role"`
	Domain                   string `json:"domain"`
	SafetyImpact             string `json:"safety_impact"`
	SourceID                 string `json:"source_id"`
	StablePath               string `json:"stable_path"`
	RequirementSHA256        string `json:"requirement_sha256"`
	NeutralSemanticSHA256    string `json:"neutral_semantic_sha256"`
	NormalizedSemanticSHA256 string `json:"normalized_semantic_sha256"`
	Sealed                   bool   `json:"sealed"`
}

func TestVersionSixHistoricalCommitmentsAreExactPublicExtension(t *testing.T) {
	directory := v6ContractDirectory(t)
	want := v6DeriveHistoricalCommitments(t, directory)
	var got v6HistoricalCommitmentFile
	v6DecodeStrict(t, filepath.Join(directory, "V6_HISTORICAL_COMMITMENTS.json"), &got)
	if !slices.Equal(got.Raw, want.Raw) || !slices.Equal(got.NeutralSemantic, want.NeutralSemantic) ||
		!slices.Equal(got.NormalizedSemantic, want.NormalizedSemantic) || got.Schema != want.Schema ||
		got.Version != want.Version || got.RetiredSourceOpened {
		t.Fatal("V6 historical commitments are not the exact public V1-V5 extension")
	}
	if len(got.Raw) != 132 || len(got.NeutralSemantic) != 60 || len(got.NormalizedSemantic) != 36 {
		t.Fatalf("V6 historical commitment counts = %d/%d/%d, want 132/60/36", len(got.Raw), len(got.NeutralSemantic), len(got.NormalizedSemantic))
	}
	loaded, err := corpusfreezev6.LoadHistoricalCommitments(filepath.Join(directory, "V6_HISTORICAL_COMMITMENTS.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Base.RawSHA256) != 132 || len(loaded.Base.NeutralSemanticSHA256) != 60 || len(loaded.NormalizedSemanticSHA256) != 36 {
		t.Fatal("V6 production history loader did not preserve every commitment")
	}
}

func TestUpdateVersionSixHistoricalCommitments(t *testing.T) {
	if os.Getenv(updateV6HistoricalCommitments) != "1" {
		t.Skip("set " + updateV6HistoricalCommitments + "=1 for the one-time no-replace update")
	}
	directory := v6ContractDirectory(t)
	value := v6DeriveHistoricalCommitments(t, directory)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join(directory, "V6_HISTORICAL_COMMITMENTS.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func v6DeriveHistoricalCommitments(t *testing.T, directory string) v6HistoricalCommitmentFile {
	t.Helper()
	var previous corpusfreeze.HistoricalCommitmentFile
	v6DecodeStrict(t, filepath.Join(directory, "V5_HISTORICAL_COMMITMENTS.json"), &previous)
	if previous.Schema != corpusfreeze.HistoricalCommitmentSchema || previous.Version != corpusfreeze.HistoricalCommitmentVersion || previous.RetiredSourceOpened {
		t.Fatal("V5 historical commitment boundary is invalid")
	}

	root := filepath.Clean(filepath.Join(directory, "..", ".."))
	var manifest v6HistoricalCorpusManifest
	manifestPath := filepath.Join(root, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v5_corpus", "manifest.json")
	if err := json.Unmarshal(v6ReadFile(t, manifestPath), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.closed-loop-open-set-corpus.v5" || manifest.Version != 5 || len(manifest.Entries) != 36 {
		t.Fatal("V5 public corpus manifest is invalid")
	}

	result := v6HistoricalCommitmentFile{
		Schema:             corpusfreezev6.HistoricalCommitmentSchema,
		Version:            corpusfreezev6.HistoricalCommitmentVersion,
		Raw:                slices.Clone(previous.Raw),
		NeutralSemantic:    slices.Clone(previous.NeutralSemantic),
		NormalizedSemantic: make([]corpusfreeze.CommitmentRecord, 0, len(manifest.Entries)),
	}
	for _, entry := range manifest.Entries {
		id := "v5:" + entry.SourceID
		result.Raw = append(result.Raw, corpusfreeze.CommitmentRecord{SHA256: entry.RequirementSHA256, ID: id})
		result.NeutralSemantic = append(result.NeutralSemantic, corpusfreeze.CommitmentRecord{SHA256: entry.NeutralSemanticSHA256, ID: id})
		result.NormalizedSemantic = append(result.NormalizedSemantic, corpusfreeze.CommitmentRecord{SHA256: entry.NormalizedSemanticSHA256, ID: id})
	}
	for name, records := range map[string][]corpusfreeze.CommitmentRecord{
		"raw": result.Raw, "neutral": result.NeutralSemantic, "normalized": result.NormalizedSemantic,
	} {
		sort.Slice(records, func(i, j int) bool {
			return records[i].SHA256+"\x00"+records[i].ID < records[j].SHA256+"\x00"+records[j].ID
		})
		for index := 1; index < len(records); index++ {
			if records[index-1].SHA256 == records[index].SHA256 {
				t.Fatalf("duplicate %s historical digest %s", name, records[index].SHA256)
			}
		}
	}
	return result
}
