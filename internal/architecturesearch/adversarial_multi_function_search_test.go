package architecturesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFrozenAdversarialMultiFunctionCorpusSearchesAndReplays(t *testing.T) {
	root := frozenAdversarialMultiFunctionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Fixtures []struct {
			ID   string `json:"id"`
			File string `json:"file"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	registry, issues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			first := Search(context.Background(), requirement, registry, SearchOptions{})
			if first.Status != SearchSelected || first.Selected == nil {
				t.Fatalf("search status = %s; issues=%#v rejections=%#v coverage=%#v", first.Status, first.Issues, first.Rejections, first.Coverage)
			}
			if first.Coverage == nil || first.Coverage.Metrics.Selected == 0 || first.Coverage.Metrics.Selected != first.Coverage.Metrics.Total {
				t.Fatalf("coverage = %#v", first.Coverage)
			}
			if first.Rationale == nil || first.Rationale.SelectedFingerprint != first.Selected.Fingerprint {
				t.Fatalf("selection rationale = %#v", first.Rationale)
			}
			if requirement.Acceptance.RequireAlternatives && len(first.Alternatives) == 0 {
				t.Fatal("required materially distinct alternative architecture is missing")
			}
			if requirement.Acceptance.RequireGlobalReasoning {
				for _, constraint := range requirement.Requirements.SystemConstraints {
					path := "candidate.system_constraints." + constraint.Name
					if !slices.ContainsFunc(first.Selected.GlobalChecks, func(check GlobalCheck) bool { return check.Path == path }) {
						t.Fatalf("global checks = %#v, missing proof for %s", first.Selected.GlobalChecks, path)
					}
				}
			}
			firstBytes, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			second := Search(context.Background(), requirement, registry, SearchOptions{})
			secondBytes, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(firstBytes, secondBytes) {
				t.Fatal("search replay differs")
			}
		})
	}
}
