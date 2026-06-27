package components

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestLoadSourcesValidFixture(t *testing.T) {
	sources, err := LoadSources(context.Background(), SourceLoadOptions{SourceDir: sourceFixtureDir("valid")})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", sources.Diagnostics)
	}
	if len(sources.Records) < 6 {
		t.Fatalf("records = %#v", sources.Records)
	}
	record, ok := sources.Find("diodes incorporated", "AP2112K-3.3")
	if !ok {
		t.Fatal("missing AP2112K source record")
	}
	if record.SourceID != "curated_seed_procurement" || record.Lifecycle == nil || record.Lifecycle.Status != LifecycleActive {
		t.Fatalf("record = %#v", record)
	}
}

func TestSourceFindNormalizesMPNPunctuation(t *testing.T) {
	sources, err := LoadSources(context.Background(), SourceLoadOptions{SourceDir: sourceFixtureDir("valid")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sources.Find("Diodes Incorporated", "AP2112K 3.3"); !ok {
		t.Fatal("missing AP2112K source record with punctuation-insensitive MPN lookup")
	}
}

func TestLoadSourcesInvalidStatus(t *testing.T) {
	sources, err := LoadSources(context.Background(), SourceLoadOptions{SourceDir: sourceFixtureDir("invalid_status")})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSourceIssue(sources.Diagnostics, CodeSourceInvalidStatus) {
		t.Fatalf("diagnostics = %#v", sources.Diagnostics)
	}
}

func TestLoadSourcesKeepsValidRecordsFromPartialFile(t *testing.T) {
	sources, err := LoadSources(context.Background(), SourceLoadOptions{SourceDir: sourceFixtureDir("partial")})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSourceIssue(sources.Diagnostics, CodeSourceInvalidStatus) {
		t.Fatalf("diagnostics = %#v", sources.Diagnostics)
	}
	if _, ok := sources.Find("Diodes Incorporated", "AP2112K-3.3"); !ok {
		t.Fatalf("valid record missing after partial load: %#v", sources.Records)
	}
	if _, ok := sources.Find("Example Semiconductor", "BAD-1"); ok {
		t.Fatalf("invalid record should not be indexed: %#v", sources.Records)
	}
}

func TestLoadSourcesRejectsUntrimmedMetadata(t *testing.T) {
	sources, err := LoadSources(context.Background(), SourceLoadOptions{SourceDir: sourceFixtureDir("duplicate")})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSourceIssue(sources.Diagnostics, CodeSourceInvalidMetadata) {
		t.Fatalf("expected trim issue from duplicate fixture diagnostics = %#v", sources.Diagnostics)
	}
}

func TestValidateSourcesDuplicateNormalizedRecord(t *testing.T) {
	collection := &SourceCollection{Records: []SourceRecord{
		{Manufacturer: "Diodes Incorporated", MPN: "AP2112K-3.3", Lifecycle: &LifecycleEvidence{Status: LifecycleActive, Source: "curated", SourceDate: "2026-06-26", Confidence: SourceConfidenceCurated}},
		{Manufacturer: "diodes incorporated", MPN: "ap2112k-3.3", Lifecycle: &LifecycleEvidence{Status: LifecycleActive, Source: "curated", SourceDate: "2026-06-26", Confidence: SourceConfidenceCurated}},
	}}
	result := ValidateSources(collection)
	if !hasSourceIssue(result.Issues, CodeSourceDuplicateRecord) {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestLoadSourcesEmptyDirIsAllowed(t *testing.T) {
	sources, err := LoadSources(context.Background(), SourceLoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources.Records) != 0 || len(sources.Diagnostics) != 0 {
		t.Fatalf("sources = %#v", sources)
	}
}

func sourceFixtureDir(name string) string {
	return filepath.Join("testdata", "sources", name)
}

func hasSourceIssue(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
