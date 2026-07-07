package designworkflow

import (
	"context"
	"testing"
	"time"

	"kicadai/internal/reports"
)

func TestBuildInterBlockRouteGroupsGroupsThreeEndpointNet(t *testing.T) {
	groups, issues := BuildInterBlockRouteGroups([]InterBlockRouteCandidate{{
		NetName:     "SDA",
		Status:      InterBlockRouteCandidateRoutable,
		InstanceIDs: []string{"sensor", "io"},
		BlockIDs:    []string{"i2c_sensor", "connector_breakout"},
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "U1", Pin: "SDA", InstanceID: "sensor", BlockID: "i2c_sensor"},
			{Ref: "J1", Pin: "3", InstanceID: "io", BlockID: "connector_breakout"},
			{Ref: "R1", Pin: "1", InstanceID: "sensor", BlockID: "i2c_sensor"},
		},
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want one SDA group", groups)
	}
	group := groups[0]
	if group.NetName != "SDA" || group.Status != InterBlockRouteCandidateRoutable {
		t.Fatalf("group = %#v, want routable SDA", group)
	}
	if len(group.RequiredEndpoints) != 3 {
		t.Fatalf("required endpoints = %#v, want sensor, connector, and pull-up endpoints", group.RequiredEndpoints)
	}
	if group.RequiredEndpoints[0].ID != "J1.3" || group.RequiredEndpoints[1].ID != "R1.1" || group.RequiredEndpoints[2].ID != "U1.SDA" {
		t.Fatalf("required endpoints sorted = %#v", group.RequiredEndpoints)
	}
}

func TestBuildInterBlockRouteGroupsDeduplicatesTargetsAndPreservesCandidateProvenance(t *testing.T) {
	groups, issues := BuildInterBlockRouteGroups([]InterBlockRouteCandidate{
		{
			NetName:     "VCC",
			Status:      InterBlockRouteCandidateRoutable,
			InstanceIDs: []string{"sensor", "io"},
			BlockIDs:    []string{"i2c_sensor", "connector_breakout"},
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "U1", Pin: "VCC", InstanceID: "sensor", BlockID: "i2c_sensor"},
				{Ref: "J1", Pin: "1", InstanceID: "io", BlockID: "connector_breakout"},
			},
		},
		{
			NetName:     "VCC",
			Status:      InterBlockRouteCandidateRoutable,
			InstanceIDs: []string{"sensor"},
			BlockIDs:    []string{"i2c_sensor"},
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "U1", Pin: "VCC", InstanceID: "sensor", BlockID: "i2c_sensor"},
				{Ref: "C1", Pin: "1", InstanceID: "sensor", BlockID: "i2c_sensor"},
			},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want merged VCC group", groups)
	}
	group := groups[0]
	if len(group.RequiredEndpoints) != 3 {
		t.Fatalf("required endpoints = %#v, want duplicate U1.VCC removed", group.RequiredEndpoints)
	}
	if len(group.SourceCandidateIndices) != 2 || group.SourceCandidateIndices[0] != 0 || group.SourceCandidateIndices[1] != 1 {
		t.Fatalf("source candidate indices = %#v, want [0 1]", group.SourceCandidateIndices)
	}
	if len(group.InstanceIDs) != 2 || group.InstanceIDs[0] != "io" || group.InstanceIDs[1] != "sensor" {
		t.Fatalf("instance IDs = %#v, want merged sorted IDs", group.InstanceIDs)
	}
}

func TestBuildInterBlockRouteGroupsReportsUnresolvedRequiredEndpoints(t *testing.T) {
	groups, issues := BuildInterBlockRouteGroups([]InterBlockRouteCandidate{
		{
			NetName:    "SCL",
			Status:     InterBlockRouteCandidateRoutable,
			Unresolved: 1,
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "U1", Pin: "SCL", InstanceID: "sensor", BlockID: "i2c_sensor"},
				{Ref: "J1", Pin: "4", InstanceID: "io", BlockID: "connector_breakout"},
			},
		},
		{
			NetName:    "SCL",
			Status:     InterBlockRouteCandidateRoutable,
			Unresolved: 1,
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "U1", Pin: "SCL", InstanceID: "sensor", BlockID: "i2c_sensor"},
				{Ref: "J1", Pin: "4", InstanceID: "io", BlockID: "connector_breakout"},
			},
		},
	})
	if len(groups) != 1 || groups[0].ExpectedRequired != 3 || groups[0].UnresolvedRequired != 1 {
		t.Fatalf("groups = %#v, want unresolved count preserved", groups)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one aggregated unresolved endpoint diagnostic", issues)
	}
	if issues[0].Code != reports.CodeValidationFailed || issues[0].Severity != reports.SeverityError || issues[0].Path != `design.inter_block_route_groups["SCL"].unresolved_required` {
		t.Fatalf("issue = %#v, want stable unresolved endpoint diagnostic", issues[0])
	}
	if issues[0].Message != "inter-block route group SCL has 1 unresolved required endpoint(s)" {
		t.Fatalf("issue message = %q, want conservative unresolved count", issues[0].Message)
	}
}

func TestBuildInterBlockRouteGroupsDoesNotOvercountComplementaryResolvedEndpoints(t *testing.T) {
	groups, issues := BuildInterBlockRouteGroups([]InterBlockRouteCandidate{
		{
			NetName:    "VCC",
			Status:     InterBlockRouteCandidateRoutable,
			Unresolved: 1,
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "U1", Pin: "VCC"},
				{Ref: "J1", Pin: "1"},
			},
		},
		{
			NetName:    "VCC",
			Status:     InterBlockRouteCandidateRoutable,
			Unresolved: 0,
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "U1", Pin: "VCC"},
				{Ref: "J1", Pin: "1"},
				{Ref: "C1", Pin: "1"},
			},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want no unresolved issue after complementary endpoints merge", issues)
	}
	if len(groups) != 1 || groups[0].ExpectedRequired != 3 || groups[0].UnresolvedRequired != 0 {
		t.Fatalf("groups = %#v, want complementary endpoints to satisfy expected count", groups)
	}
}

func TestBuildInterBlockRouteGroupsKeepsMostSevereMergedStatus(t *testing.T) {
	groups, issues := BuildInterBlockRouteGroups([]InterBlockRouteCandidate{
		{
			NetName: "VCC",
			Status:  InterBlockRouteCandidateRoutable,
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "U1", Pin: "VCC"},
				{Ref: "J1", Pin: "1"},
			},
		},
		{
			NetName: "VCC",
			Status:  InterBlockRouteCandidateBlocked,
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "C1", Pin: "1"},
			},
		},
		{
			NetName: "VCC",
			Status:  InterBlockRouteCandidateFailed,
			Endpoints: []InterBlockRouteEndpoint{
				{Ref: "C2", Pin: "1"},
			},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(groups) != 1 || groups[0].Status != InterBlockRouteCandidateFailed {
		t.Fatalf("groups = %#v, want merged group to keep failed status", groups)
	}
}

func TestBuildInterBlockRouteGroupsReportsMissingNetName(t *testing.T) {
	groups, issues := BuildInterBlockRouteGroups([]InterBlockRouteCandidate{{
		Status: InterBlockRouteCandidateRoutable,
		Endpoints: []InterBlockRouteEndpoint{
			{Ref: "U1", Pin: "1"},
			{Ref: "J1", Pin: "1"},
		},
	}})
	if len(groups) != 0 {
		t.Fatalf("groups = %#v, want no group for missing net name", groups)
	}
	if len(issues) != 1 || issues[0].Path != "design.inter_block_route_groups.candidates[0].net_name" {
		t.Fatalf("issues = %#v, want missing net diagnostic", issues)
	}
}

func TestBuildInterBlockRouteGroupsI2CSensorBreakoutPrunesLocallyRoutedPassives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	groups, issues := BuildInterBlockRouteGroups(candidates)
	if len(issues) != 0 {
		t.Fatalf("group issues = %#v", issues)
	}
	byNet := interBlockRouteGroupsByNetForTest(groups)
	for _, net := range []string{"VCC", "GND", "SDA", "SCL"} {
		group, ok := byNet[net]
		if !ok {
			t.Fatalf("groups = %#v, missing %s", groups, net)
		}
		if len(group.RequiredEndpoints) != 2 {
			t.Fatalf("%s group = %#v, want connector and sensor endpoints with locally routed passives pruned", net, group)
		}
		refs := map[string]bool{}
		for _, endpoint := range group.RequiredEndpoints {
			refs[endpoint.InstanceID] = true
		}
		if !refs["io"] || !refs["sensor"] {
			t.Fatalf("%s group = %#v, want io and sensor endpoint coverage", net, group)
		}
	}
}

func interBlockRouteGroupsByNetForTest(groups []InterBlockRouteGroup) map[string]InterBlockRouteGroup {
	byNet := map[string]InterBlockRouteGroup{}
	for _, group := range groups {
		byNet[group.NetName] = group
	}
	return byNet
}
