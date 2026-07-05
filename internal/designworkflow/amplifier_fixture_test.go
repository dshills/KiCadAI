package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/schematiclayout"
)

func TestAmplifierDesignFixturesPlanToDeclaredAcceptance(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	paths, err := filepath.Glob(filepath.Join(repoRoot, "examples", "design", "amplifier", "*.json"))
	if err != nil {
		t.Fatalf("glob amplifier fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no amplifier design fixtures found")
	}
	expectedAcceptance := map[string]AcceptanceLevel{
		"class_ab_headphone_driver": AcceptanceConnectivity,
		"opamp_headphone_buffer":    AcceptanceDraft,
	}
	sort.Strings(paths)
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open amplifier fixture: %v", err)
			}
			defer file.Close()
			request, issues := DecodeRequestStrict(file)
			if len(issues) != 0 {
				t.Fatalf("decode issues:\n%s", formatDesignExampleIssues(issues))
			}
			if request.Validation.Acceptance == "" {
				t.Fatal("acceptance must be specified")
			}
			fixtureName := strings.TrimSuffix(filepath.Base(path), ".json")
			expected, ok := expectedAcceptance[fixtureName]
			if !ok {
				t.Fatalf("fixture %q is missing an expected acceptance entry", fixtureName)
			}
			if request.Name != fixtureName {
				t.Fatalf("request name = %q, want fixture name %q", request.Name, fixtureName)
			}
			if request.Validation.Acceptance != expected {
				t.Fatalf("acceptance = %q, want %q", request.Validation.Acceptance, expected)
			}
			ctx, cancel := context.WithTimeout(context.Background(), designExamplePlanningTimeout)
			defer cancel()
			stage := designExamplePlanStage(ctx, request)
			if stage.Status != StageStatusOK || len(stage.Issues) != 0 {
				t.Fatalf("block planning status = %q issues:\n%s", stage.Status, formatDesignExampleIssues(stage.Issues))
			}
		})
	}
}

func TestClassABHeadphoneFixtureSchematicReadability(t *testing.T) {
	request := readClassABHeadphoneFixture(t)
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues:\n%s", formatDesignExampleIssues(plan.Stage.Issues))
	}
	stage := schematicStageFromPlan(plan)
	readability, ok := stage.Summary["readability"].(map[string]any)
	if !ok {
		t.Fatalf("readability summary missing: %#v", stage.Summary)
	}
	if readability["rule_profile"] != schematiclayout.RuleProfileAmplifier {
		t.Fatalf("rule_profile = %#v, want amplifier; summary=%#v", readability["rule_profile"], readability)
	}
	for _, key := range []string{"diagonal_wire_count", "stage_order_violation_count", "power_placement_violation_count"} {
		if got := summaryInt(t, readability, key); got != 0 {
			t.Fatalf("%s = %d, want 0; summary=%#v", key, got, readability)
		}
	}
}

func TestClassABHeadphoneFixturePCBPlacementRoutingEvidence(t *testing.T) {
	request := readClassABHeadphoneFixture(t)
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan issues:\n%s", formatDesignExampleIssues(plan.Stage.Issues))
	}
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	if workflowStageBlocked(fragments.Stage) {
		t.Fatalf("PCB realization status = %s issues:\n%s", fragments.Stage.Status, formatDesignExampleIssues(fragments.Stage.Issues))
	}
	placed := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if workflowStageBlocked(placed.Stage) {
		t.Fatalf("placement status = %s issues:\n%s", placed.Stage.Status, formatDesignExampleIssues(placed.Stage.Issues))
	}
	allowPartial := true
	routed := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{AllowPartial: &allowPartial})
	local := requireStageSummary[LocalRouteConnectivitySummary](t, routed.Stage, "route_connectivity")
	if local.RoutesAttempted == 0 || local.EndpointContactsProven < local.RoutesAttempted*2 || local.IssueCount != 0 {
		t.Fatalf("local route connectivity = %#v, want clean local contact evidence", local)
	}
	interBlock := requireInterBlockRouteSummary(t, routed.Stage)
	if interBlock.Candidates == 0 || interBlock.RequiredEndpoints == 0 || interBlock.EndpointsResolved != interBlock.RequiredEndpoints {
		t.Fatalf("inter-block routing = %#v, want resolved amplifier inter-block endpoints", interBlock)
	}
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, routed.Stage)
	if routeTrees.GroupsPlanned == 0 || routeTrees.BranchesRouted == 0 {
		t.Fatalf("route-tree execution = %#v, want routed amplifier branch evidence", routeTrees)
	}
	access := requireStageSummary[RouteTreeEndpointAccessSummary](t, routed.Stage, "route_tree_access")
	if access.PadAccess == 0 || access.LocalRouteAnchors == 0 {
		t.Fatalf("route-tree access = %#v, want pad and local-route anchor access evidence", access)
	}
	contactGraph := requireStageSummary[RouteTreeContactGraphSummary](t, routed.Stage, "route_tree_contact_graph")
	if contactGraph.RequiredEndpoints == 0 || contactGraph.ProvenEndpoints == 0 || contactGraph.LocalRouteMerges == 0 {
		t.Fatalf("route-tree contact graph = %#v, want proven amplifier contact graph evidence", contactGraph)
	}
	contacts := requireInterBlockContactSummary(t, routed.Stage)
	if contacts.ContactsRequired == 0 || contacts.ContactsProven == 0 {
		t.Fatalf("inter-block contacts = %#v, want proven amplifier contact evidence", contacts)
	}
}

func readClassABHeadphoneFixture(t *testing.T) Request {
	t.Helper()
	repoRoot := designExampleRepoRoot(t)
	path := filepath.Join(repoRoot, "examples", "design", "amplifier", "class_ab_headphone_driver.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open amplifier fixture: %v", err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if len(issues) != 0 {
		t.Fatalf("decode issues:\n%s", formatDesignExampleIssues(issues))
	}
	return request
}
