package corpusfreeze

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReportIsStableAndPrivate(t *testing.T) {
	digest := strings.Repeat("a", 64)
	report := Report{
		Schema: "kicadai.behavior-corpus-validation-report.v1", Version: 1,
		PolicySHA256: digest, PacketSetSHA256: digest, ContractBindingSHA256: digest, HistoricalCommitmentsSHA256: digest,
		AuthorPacketSHA256: map[string]string{"author_1": digest},
		AssignmentSHA256:   map[string]string{"author_1": digest},
		AuthorshipSHA256:   map[string]string{"author_1": digest},
		Entries: []EntryEvidence{{
			ID: "case_001", AuthorSlot: "author_1", RequirementSHA256: digest,
			NeutralSemanticSHA256: digest, NormalizedSemanticSHA256: digest,
		}},
		Counts: map[string]map[string]int{"role": {"discovery": 1}},
	}
	want, err := report.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) || len(got) == 0 || got[len(got)-1] != '\n' {
		t.Fatal("written report is not stable")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("report mode = %o", info.Mode().Perm())
	}
}

func TestMarshalJSONStableRejectsIncompleteReport(t *testing.T) {
	digest := strings.Repeat("a", 64)
	if _, err := (Report{Schema: "kicadai.behavior-corpus-validation-report.v1", Version: 1, PolicySHA256: digest, PacketSetSHA256: digest, ContractBindingSHA256: digest, HistoricalCommitmentsSHA256: digest}).MarshalJSONStable(); err == nil {
		t.Fatal("expected error")
	}
}
