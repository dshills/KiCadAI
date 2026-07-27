package capabilityevaluation

import (
	"slices"
	"testing"
)

func TestExplainClusterAndListAffectedCases(t *testing.T) {
	report, _ := improvementReports(t)
	cluster, err := ExplainCluster(report, "clock_fanout_loading", 0)
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Capability != "clock_fanout_loading" {
		t.Fatalf("cluster = %#v", cluster)
	}
	byRank, err := ExplainCluster(report, "", cluster.Rank)
	if err != nil {
		t.Fatal(err)
	}
	if byRank.Key != cluster.Key {
		t.Fatalf("rank lookup = %#v, want %#v", byRank, cluster)
	}
	affected, err := ListCasesAffectedByCapability(report, "clock_fanout_loading")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(affected.Cases, []string{"case_002"}) {
		t.Fatalf("affected cases = %#v", affected)
	}
	if _, err := ExplainCluster(report, "missing_capability", 0); err == nil {
		t.Fatal("expected missing cluster error")
	}
	if _, err := ListCasesAffectedByCapability(report, "not valid"); err == nil {
		t.Fatal("expected invalid capability error")
	}
}
