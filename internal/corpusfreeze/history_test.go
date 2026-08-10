package corpusfreeze

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHistoricalCommitments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	data := []byte(`{
  "schema": "kicadai.behavior-corpus-historical-commitments.v1",
  "version": 1,
  "raw": [{"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","id":"old_1"}],
  "neutral_semantic": [{"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","id":"old_1"}],
  "retired_source_opened": false
}
`)
	mustWriteTestFile(t, path, data)
	got, err := LoadHistoricalCommitments(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawSHA256[strings.Repeat("a", 64)] != "old_1" || got.NeutralSemanticSHA256[strings.Repeat("b", 64)] != "old_1" {
		t.Fatalf("commitments = %#v", got)
	}
	if got.SourceSHA256 != hashBytes(data) {
		t.Fatalf("source hash = %q", got.SourceSHA256)
	}
}

func TestLoadHistoricalCommitmentsRejectsUnsafeMetadata(t *testing.T) {
	valid := HistoricalCommitmentFile{
		Schema: HistoricalCommitmentSchema, Version: HistoricalCommitmentVersion,
		Raw: []CommitmentRecord{
			{SHA256: strings.Repeat("a", 64), ID: "old_1"},
			{SHA256: strings.Repeat("b", 64), ID: "old_2"},
		},
		NeutralSemantic: []CommitmentRecord{{SHA256: strings.Repeat("c", 64), ID: "old_1"}},
	}
	for name, mutate := range map[string]func(*HistoricalCommitmentFile){
		"retired source opened": func(value *HistoricalCommitmentFile) { value.RetiredSourceOpened = true },
		"invalid digest":        func(value *HistoricalCommitmentFile) { value.Raw[0].SHA256 = "invalid" },
		"noncanonical order": func(value *HistoricalCommitmentFile) {
			value.Raw[0], value.Raw[1] = value.Raw[1], value.Raw[0]
		},
		"duplicate digest": func(value *HistoricalCommitmentFile) { value.Raw[1].SHA256 = value.Raw[0].SHA256 },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			value.Raw = append([]CommitmentRecord(nil), valid.Raw...)
			value.NeutralSemantic = append([]CommitmentRecord(nil), valid.NeutralSemantic...)
			mutate(&value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "history.json")
			mustWriteTestFile(t, path, data)
			if _, err := LoadHistoricalCommitments(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadHistoricalCommitmentsRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	mustWriteTestFile(t, path, []byte(`{"schema":"kicadai.behavior-corpus-historical-commitments.v1","version":1,"raw":[],"neutral_semantic":[],"retired_source_opened":false,"unexpected":true}`))
	if _, err := LoadHistoricalCommitments(path); err == nil {
		t.Fatal("expected error")
	}
}
