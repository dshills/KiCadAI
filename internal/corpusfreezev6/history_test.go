package corpusfreezev6

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/corpusfreeze"
)

func TestLoadHistoricalCommitments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	value := HistoricalCommitmentFile{
		Schema: HistoricalCommitmentSchema, Version: HistoricalCommitmentVersion,
		Raw:                []corpusfreeze.CommitmentRecord{{SHA256: strings.Repeat("a", 64), ID: "old_1"}},
		NeutralSemantic:    []corpusfreeze.CommitmentRecord{{SHA256: strings.Repeat("b", 64), ID: "old_1"}},
		NormalizedSemantic: []corpusfreeze.CommitmentRecord{{SHA256: strings.Repeat("c", 64), ID: "old_1"}},
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHistoricalCommitments(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base.RawSHA256[strings.Repeat("a", 64)] != "old_1" ||
		got.Base.NeutralSemanticSHA256[strings.Repeat("b", 64)] != "old_1" ||
		got.NormalizedSemanticSHA256[strings.Repeat("c", 64)] != "old_1" || got.Base.SourceSHA256 != hashBytes(data) {
		t.Fatalf("commitments = %#v", got)
	}
}

func TestLoadHistoricalCommitmentsFailsClosed(t *testing.T) {
	valid := HistoricalCommitmentFile{
		Schema: HistoricalCommitmentSchema, Version: HistoricalCommitmentVersion,
		Raw: []corpusfreeze.CommitmentRecord{
			{SHA256: strings.Repeat("a", 64), ID: "old_1"},
			{SHA256: strings.Repeat("b", 64), ID: "old_2"},
		},
		NeutralSemantic:    []corpusfreeze.CommitmentRecord{{SHA256: strings.Repeat("c", 64), ID: "old_1"}},
		NormalizedSemantic: []corpusfreeze.CommitmentRecord{{SHA256: strings.Repeat("d", 64), ID: "old_1"}},
	}
	for name, mutate := range map[string]func(*HistoricalCommitmentFile){
		"retired source opened": func(value *HistoricalCommitmentFile) { value.RetiredSourceOpened = true },
		"missing normalized":    func(value *HistoricalCommitmentFile) { value.NormalizedSemantic = nil },
		"invalid digest": func(value *HistoricalCommitmentFile) {
			value.NormalizedSemantic[0].SHA256 = "invalid"
		},
		"noncanonical order": func(value *HistoricalCommitmentFile) { value.Raw[0], value.Raw[1] = value.Raw[1], value.Raw[0] },
		"duplicate digest":   func(value *HistoricalCommitmentFile) { value.Raw[1].SHA256 = value.Raw[0].SHA256 },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			value.Raw = append([]corpusfreeze.CommitmentRecord(nil), valid.Raw...)
			value.NeutralSemantic = append([]corpusfreeze.CommitmentRecord(nil), valid.NeutralSemantic...)
			value.NormalizedSemantic = append([]corpusfreeze.CommitmentRecord(nil), valid.NormalizedSemantic...)
			mutate(&value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "history.json")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadHistoricalCommitments(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadHistoricalCommitmentsRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	data := []byte(`{"schema":"kicadai.behavior-corpus-historical-commitments.v2","version":2,"raw":[],"neutral_semantic":[],"normalized_semantic":[],"retired_source_opened":false,"unexpected":true}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHistoricalCommitments(path); err == nil {
		t.Fatal("expected error")
	}
}
