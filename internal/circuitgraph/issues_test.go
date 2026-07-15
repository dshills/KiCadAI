package circuitgraph

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/reports"
)

func TestValidateAggregatesIndependentPreflightStagesDeterministically(t *testing.T) {
	document := validTestDocument()
	document.Components[0].ComponentID = ""
	document.Nets[0].Role = NetRole("invalid")
	document.Schematic.Groups[0].Members = append(document.Schematic.Groups[0].Members, "missing")
	document.PCB.Zones = append(document.PCB.Zones, PCBZone{Net: "missing"})
	document.Policy.AllowRouteRetry = nil

	first := Validate(document)
	assertGraphIssueStages(t, first, []string{
		PreflightStageComponent,
		PreflightStageConnectivity,
		PreflightStageSchematic,
		PreflightStagePCB,
		PreflightStagePolicy,
	})

	second := Validate(document)
	if len(first) != len(second) {
		t.Fatalf("issue count changed between runs: %d != %d", len(first), len(second))
	}
	for index := range first {
		if first[index].IssueID != second[index].IssueID || first[index].Path != second[index].Path {
			t.Fatalf("issue order changed at %d\nfirst=%#v\nsecond=%#v", index, first[index], second[index])
		}
	}
}

func TestValidateDoesNotMutateInput(t *testing.T) {
	document := validTestDocument()
	document.Components[0], document.Components[1] = document.Components[1], document.Components[0]
	document.Nets[0], document.Nets[1] = document.Nets[1], document.Nets[0]
	before, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	_ = Validate(document)
	after, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("Validate mutated input\nbefore=%s\nafter=%s", before, after)
	}
}

func TestFinalizeGraphIssuesSuppressesOnlyExplicitDependents(t *testing.T) {
	root := graphIssue(CodeComponentUnresolved, "components[0]", "component is unresolved")
	dependent := graphDependentIssue(CodePinUnresolved, "nets[0].endpoints[0]", "endpoint depends on unresolved component", root.IssueID, root.RetryScope)
	independent := graphIssue(CodeNetInvalid, "nets[1]", "independent invalid net")
	issues := finalizeGraphIssues([]reports.Issue{dependent, independent, root})
	if len(issues) != 2 || !hasGraphIssueID(issues, root.IssueID) || !hasGraphIssueID(issues, independent.IssueID) {
		t.Fatalf("final issues = %#v", issues)
	}
	if unrooted := finalizeGraphIssues([]reports.Issue{dependent}); len(unrooted) != 1 || unrooted[0].RootCauseID != root.IssueID {
		t.Fatalf("dependent without present root was incorrectly suppressed: %#v", unrooted)
	}
}

func TestResolverContinuesIndependentComponentsAndSuppressesRootedEndpoints(t *testing.T) {
	document := minimalResolvedDocument()
	document.Components[0].ComponentID = "missing.component"
	resolved, issues := NewResolver(ResolveOptions{Catalog: minimalResolvedCatalog()}).Resolve(context.Background(), document)
	if len(resolved.Components) != 1 || resolved.Components[0].Instance.ID != "u2" {
		t.Fatalf("partial resolution = %#v", resolved.Components)
	}
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("expected component root issue: %#v", issues)
	}
	for _, issue := range issues {
		if issue.RootCauseID != "" || issue.Path == "nets[0].endpoints[0]" || issue.IssueID == "" || issue.RetryScope == "" || issue.Suggestion == "" {
			t.Fatalf("unexpected unresolved dependent or incomplete metadata: %#v", issue)
		}
	}
}

func assertGraphIssueStages(t *testing.T, issues []reports.Issue, expected []string) {
	t.Helper()
	found := map[string]bool{}
	for _, issue := range issues {
		if issue.IssueID == "" || issue.RetryScope == "" || issue.Suggestion == "" {
			t.Fatalf("issue metadata incomplete: %#v", issue)
		}
		found[issue.Stage] = true
	}
	for _, stage := range expected {
		if !found[stage] {
			t.Fatalf("missing stage %q in %#v", stage, issues)
		}
	}
}

func hasGraphIssueID(issues []reports.Issue, id string) bool {
	for _, issue := range issues {
		if issue.IssueID == id {
			return true
		}
	}
	return false
}
