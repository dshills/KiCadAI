package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/componentonboarding"
)

func TestComponentOverlayLoaderRejectsTamperedArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlay.json")
	body := `{
	  "schema": "kicadai.component-catalog-overlay.v1",
	  "policy_version": "evidence-backed-component-onboarding-v1",
	  "status": "supported",
	  "requirement_hash": "` + strings.Repeat("1", 64) + `",
	  "candidate_hash": "` + strings.Repeat("2", 64) + `",
	  "promotion_hash": "` + strings.Repeat("3", 64) + `",
	  "records": [],
	  "models": {"schema":"kicadai.model-provenance-registry.v1","version":1,"records":[]},
	  "hash": "` + strings.Repeat("0", 64) + `"
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadComponentCatalogForOptions(context.Background(), cliOptions{componentOverlay: path}); err == nil ||
		!strings.Contains(err.Error(), "overlay hash mismatch") {
		t.Fatalf("tampered overlay load error = %v", err)
	}
}

func TestComponentPromotionRequiresExplicitExecute(t *testing.T) {
	var output bytes.Buffer
	err := runComponentOnboardingPromote(context.Background(), cliOptions{}, &output, nil)
	if err == nil || !strings.Contains(output.String(), "requires explicit --execute authorization") {
		t.Fatalf("promotion authorization result: err=%v output=%s", err, output.String())
	}
}

func TestLoadOnboardingDocumentsRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	_, err := loadOnboardingDocuments(root, []onboardingDocumentFile{{
		ID: "datasheet", Kind: componentonboarding.DocumentDatasheet,
		Publisher: "Example", Locator: "https://example.invalid/part",
		Revision: "A", Path: "../outside.pdf", ExpectedSHA256: strings.Repeat("0", 64),
	}})
	if err == nil {
		t.Fatal("onboarding document escaped its request directory")
	}
}

func TestDecodeOnboardingArtifactRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requirement.json")
	if err := os.WriteFile(path, []byte(`{"schema":"kicadai.component-onboarding-request.v1","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeOnboardingArtifactFile(path, componentonboarding.DecodeRequirement); err == nil {
		t.Fatal("unknown onboarding artifact field was accepted")
	}
}
