package corpusassignment

import (
	"strings"
	"testing"
)

func TestValidateAcceptsAssignmentWithCompletePreauthoringCoverage(t *testing.T) {
	entries, policy := balancedFixture()
	report, err := Validate(entries, policy)
	if err != nil {
		t.Fatal(err)
	}
	if report.EntryCount != 8 || len(report.Partitions) != 2 {
		t.Fatalf("unexpected preflight report: %+v", report)
	}
}

func TestValidateRejectsMissingHighSafetyRoleBeforeAuthoring(t *testing.T) {
	entries, policy := balancedFixture()
	entries[3].SafetyImpact = "low"
	entries[2].SafetyImpact = "high"
	report, err := Validate(entries, policy)
	if err == nil || !strings.Contains(err.Error(), "high-safety coverage is infeasible") {
		t.Fatalf("error=%v report=%+v", err, report)
	}
	if len(report.Partitions) != 2 || len(report.Partitions[0].MissingHighSafetyRoles) != 1 || report.Partitions[0].MissingHighSafetyRoles[0] != "role_b" {
		t.Fatalf("missing-role evidence=%+v", report)
	}
}

func TestValidateRejectsDuplicateTriplesAndAuthorQuotaDrift(t *testing.T) {
	entries, policy := balancedFixture()
	entries[1].Domain, entries[1].CircuitRole = entries[0].Domain, entries[0].CircuitRole
	if _, err := Validate(entries, policy); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error=%v", err)
	}
	entries, policy = balancedFixture()
	entries[0].Author = "author_b"
	if _, err := Validate(entries, policy); err == nil || !strings.Contains(err.Error(), "author author_a") {
		t.Fatalf("quota error=%v", err)
	}
}

func TestValidateRejectsNonAdjacentDuplicatePolicyValues(t *testing.T) {
	entries, policy := balancedFixture()
	policy.Domains = []string{"domain_a", "domain_b", "domain_a"}
	if _, err := Validate(entries, policy); err == nil || !strings.Contains(err.Error(), "policy Domains must contain unique nonempty values") {
		t.Fatalf("duplicate policy error=%v", err)
	}
}

func balancedFixture() ([]Entry, Policy) {
	policy := Policy{Authors: []string{"author_a", "author_b"}, Partitions: []string{"discovery", "held_out"}, Domains: []string{"domain_a", "domain_b"},
		CircuitRoles: []string{"role_a", "role_b"}, SafetyImpacts: []string{"low", "high"}, HighSafetyImpacts: []string{"high"},
		CasesPerAuthor: 4, CasesPerPartition: 4, DimensionCountPerPartition: 2, SafetyCountPerPartition: 2,
		MinimumStaticPerAuthor: 2, MinimumDynamicPerAuthor: 2, MinimumMultiOutputPerPartition: 2, MinimumOffNominalPerAuthor: 2,
		RequireHighSafetyDomains: true, RequireHighSafetyCircuitRoles: true}
	entries := []Entry{
		{ID: "d1", Author: "author_a", Partition: "discovery", Domain: "domain_a", CircuitRole: "role_a", SafetyImpact: "high", PrimaryClass: "static", OutputMultiplicity: "multiple", RequireOffNominal: true},
		{ID: "d2", Author: "author_a", Partition: "discovery", Domain: "domain_a", CircuitRole: "role_b", SafetyImpact: "low", PrimaryClass: "dynamic", OutputMultiplicity: "single"},
		{ID: "d3", Author: "author_b", Partition: "discovery", Domain: "domain_b", CircuitRole: "role_a", SafetyImpact: "low", PrimaryClass: "static", OutputMultiplicity: "multiple", RequireOffNominal: true},
		{ID: "d4", Author: "author_b", Partition: "discovery", Domain: "domain_b", CircuitRole: "role_b", SafetyImpact: "high", PrimaryClass: "dynamic", OutputMultiplicity: "single"},
		{ID: "h1", Author: "author_a", Partition: "held_out", Domain: "domain_b", CircuitRole: "role_b", SafetyImpact: "high", PrimaryClass: "static", OutputMultiplicity: "multiple", RequireOffNominal: true},
		{ID: "h2", Author: "author_a", Partition: "held_out", Domain: "domain_a", CircuitRole: "role_a", SafetyImpact: "high", PrimaryClass: "dynamic", OutputMultiplicity: "single"},
		{ID: "h3", Author: "author_b", Partition: "held_out", Domain: "domain_a", CircuitRole: "role_b", SafetyImpact: "low", PrimaryClass: "static", OutputMultiplicity: "multiple", RequireOffNominal: true},
		{ID: "h4", Author: "author_b", Partition: "held_out", Domain: "domain_b", CircuitRole: "role_a", SafetyImpact: "low", PrimaryClass: "dynamic", OutputMultiplicity: "single"},
	}
	return entries, policy
}
