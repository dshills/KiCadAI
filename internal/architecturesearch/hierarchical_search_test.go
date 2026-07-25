package architecturesearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFrozenHierarchicalMultiDomainCorpusSearchesAndPlansDeterministically(t *testing.T) {
	root := frozenHierarchicalCorpusRoot()
	manifestData, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest frozenHierarchicalManifest
	decodeFrozenClosedLoopStrict(t, manifestData, &manifest)
	registry, issues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, row := range manifest.Fixtures {
		row := row
		t.Run(row.ID, func(t *testing.T) {
			data, readErr := os.ReadFile(filepath.Join(root, row.File))
			if readErr != nil {
				t.Fatal(readErr)
			}
			requirement, decodeIssues := DecodeStrict(bytes.NewReader(data))
			if len(decodeIssues) != 0 {
				t.Fatalf("decode issues = %#v", decodeIssues)
			}
			first := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "hierarchical-corpus"})
			if first.Status != SearchSelected || first.Selected == nil {
				t.Fatalf("search=%s issues=%#v rejections=%#v", first.Status, first.Issues, first.Rejections)
			}
			validateSelectedSystemPlan(t, requirement, first)
			firstBytes, marshalErr := json.Marshal(first)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			second := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "hierarchical-corpus"})
			secondBytes, marshalErr := json.Marshal(second)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if !bytes.Equal(firstBytes, secondBytes) {
				t.Fatal("V4 hierarchy, contracts, resources, physical plan, or search result changed on replay")
			}
		})
	}
}

func TestFrozenThermalSystemExposesCatalogProvenBallastedRegulatorBranch(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(frozenHierarchicalCorpusRoot(), "regulated_sensor_mcu_communications_system.json"))
	if err != nil {
		t.Fatal(err)
	}
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	obligations, obligationIssues := initialSearchObligations(requirement, EvidenceRuleInferred)
	if len(obligationIssues) != 0 {
		t.Fatal(obligationIssues)
	}
	index := slices.IndexFunc(obligations, func(obligation searchObligation) bool {
		return obligation.Path == "objective:regulate_logic"
	})
	if index < 0 {
		t.Fatal("regulated logic objective obligation is missing")
	}
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	expansions, err := provider.Expand(context.Background(), providerRequestFor(obligations[index], requirement.Requirements.Constraints))
	if err != nil {
		t.Fatal(err)
	}
	for _, expansion := range expansions {
		realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == "thermal_ballast"
		}) {
			result := Search(context.Background(), requirement, mustCatalogRegistry(t), SearchOptions{CatalogHash: "thermal-branch"})
			if result.Selected == nil {
				t.Fatalf("thermal system search did not select a candidate: %#v", result.Issues)
			}
			selectionIndex := slices.IndexFunc(result.Selected.Selections, func(selection FragmentSelection) bool {
				return selection.ObligationPath == "objective:regulate_logic"
			})
			if selectionIndex < 0 {
				t.Fatal("thermal system selected no regulator branch")
			}
			selectedRealization, decodeErr := DecodeFragmentRealization(result.Selected.Selections[selectionIndex].Payload)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if !slices.ContainsFunc(selectedRealization.Instances, func(instance RealizationInstance) bool { return instance.ID == "thermal_ballast" }) {
				summaries := []string{}
				for _, candidate := range append([]CandidateResult{*result.Selected}, result.Alternatives...) {
					for _, selection := range candidate.Selections {
						if selection.ObligationPath == "objective:regulate_logic" {
							summaries = append(summaries, selection.ExpansionID+":"+optionalFloatText(selection.Metrics.WorstMargin))
						}
					}
				}
				t.Fatalf("thermal system selected %s without ballast; regulator candidate summaries=%v rejections=%#v", result.Selected.Selections[selectionIndex].ExpansionID, summaries, result.Rejections)
			}
			return
		}
	}
	ids := make([]string, len(expansions))
	for index := range expansions {
		ids[index] = expansions[index].ID
	}
	request := providerRequestFor(obligations[index], requirement.Requirements.Constraints)
	t.Fatalf("thermal behavior did not expose a ballasted regulator branch: ids=%v ports=%#v constraints=%#v", ids, request.Ports, request.Constraints)
}

func mustCatalogRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, issues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	return registry
}

func validateSelectedSystemPlan(t *testing.T, requirement Requirement, result SearchResult) {
	t.Helper()
	if result.PolicyVersion != PolicyVersionV4 || result.Backtracking == nil {
		t.Fatalf("V4 policy or backtracking evidence is missing: %#v", result)
	}
	candidates := []CandidateResult{*result.Selected}
	candidates = append(candidates, result.Alternatives...)
	if err := ValidateBacktrackingEvidence(*result.Backtracking, candidates); err != nil {
		t.Fatal(err)
	}
	plan := result.Selected.SystemPlan
	if plan == nil || plan.Schema != SystemPlanSchema || plan.RequirementHash != result.RequirementHash ||
		plan.CandidateFingerprint != result.Selected.Fingerprint || plan.PlanHash == "" {
		t.Fatalf("system plan identity = %#v", plan)
	}
	if len(plan.Hierarchy.Subsystems) == 0 ||
		len(plan.Hierarchy.Blocks) < len(requirement.Requirements.Objectives)+len(requirement.Requirements.Participants) ||
		len(plan.Interfaces) == 0 || len(plan.Resources) == 0 ||
		len(plan.Physical.Partitions) != len(plan.Hierarchy.Subsystems) ||
		len(plan.Traceability) < len(requirement.Requirements.Objectives)+len(requirement.Requirements.Participants)+len(requirement.Requirements.BehavioralRequirements) {
		t.Fatalf("system plan is incomplete: %#v", plan)
	}
	for _, connection := range plan.Interfaces {
		if connection.Status != "pass" || len(connection.Endpoints) == 0 {
			t.Fatalf("interface contract is not proven: %#v", connection)
		}
	}
	for _, resource := range plan.Resources {
		if resource.Status != "pass" || resource.Source == "" {
			t.Fatalf("shared resource is not proven: %#v", resource)
		}
	}
	for _, partition := range plan.Physical.Partitions {
		if len(partition.BlockIDs) == 0 || len(partition.Rules) == 0 {
			t.Fatalf("physical partition is incomplete: %#v", partition)
		}
	}
	for _, block := range plan.Hierarchy.Blocks {
		if len(block.VerificationIDs) == 0 {
			t.Fatalf("hierarchy block lacks independent verification contracts: %#v", block)
		}
	}
	hashInput := *plan
	hashInput.PlanHash = ""
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	if got := hex.EncodeToString(digest[:]); got != plan.PlanHash {
		t.Fatalf("plan hash = %s, want %s", plan.PlanHash, got)
	}
}
