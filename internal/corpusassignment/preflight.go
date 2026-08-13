// Package corpusassignment validates outcome-blind corpus assignment metadata
// before isolated authors are dispatched.
package corpusassignment

import (
	"fmt"
	"strings"
)

type Entry struct {
	ID                 string
	Author             string
	Partition          string
	Domain             string
	CircuitRole        string
	SafetyImpact       string
	PrimaryClass       string
	OutputMultiplicity string
	RequireOffNominal  bool
}

type Policy struct {
	Authors                        []string
	Partitions                     []string
	Domains                        []string
	CircuitRoles                   []string
	SafetyImpacts                  []string
	HighSafetyImpacts              []string
	CasesPerAuthor                 int
	CasesPerPartition              int
	DimensionCountPerPartition     int
	SafetyCountPerPartition        int
	MinimumStaticPerAuthor         int
	MinimumDynamicPerAuthor        int
	MinimumMultiOutputPerPartition int
	MinimumOffNominalPerAuthor     int
	RequireHighSafetyDomains       bool
	RequireHighSafetyCircuitRoles  bool
}

type PartitionCoverage struct {
	Partition                string
	MissingHighSafetyDomains []string
	MissingHighSafetyRoles   []string
}

type Report struct {
	EntryCount int
	Partitions []PartitionCoverage
}

func Validate(entries []Entry, policy Policy) (Report, error) {
	if err := validatePolicy(policy); err != nil {
		return Report{}, err
	}
	wantAuthors, wantPartitions := set(policy.Authors), set(policy.Partitions)
	wantDomains, wantRoles, wantSafety := set(policy.Domains), set(policy.CircuitRoles), set(policy.SafetyImpacts)
	highSafety := set(policy.HighSafetyImpacts)
	type triple struct{ partition, domain, circuitRole string }
	seenIDs, seenTriples := map[string]bool{}, map[triple]bool{}
	authorCount, staticCount, dynamicCount, offNominalCount := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	partitionCount := map[string]int{}
	domainCount, roleCount, safetyCount, multiCount := nestedCounts(policy.Partitions), nestedCounts(policy.Partitions), nestedCounts(policy.Partitions), map[string]int{}
	highDomain, highRole := nestedSeen(policy.Partitions), nestedSeen(policy.Partitions)

	for _, entry := range entries {
		if entry.ID == "" {
			return Report{}, fmt.Errorf("assignment ID is empty")
		}
		if seenIDs[entry.ID] {
			return Report{}, fmt.Errorf("assignment ID %q is duplicated", entry.ID)
		}
		if !wantAuthors[entry.Author] {
			return Report{}, fmt.Errorf("assignment %s has unknown author %q", entry.ID, entry.Author)
		}
		if !wantPartitions[entry.Partition] {
			return Report{}, fmt.Errorf("assignment %s has unknown partition %q", entry.ID, entry.Partition)
		}
		if !wantDomains[entry.Domain] {
			return Report{}, fmt.Errorf("assignment %s has unknown domain %q", entry.ID, entry.Domain)
		}
		if !wantRoles[entry.CircuitRole] {
			return Report{}, fmt.Errorf("assignment %s has unknown circuit role %q", entry.ID, entry.CircuitRole)
		}
		if !wantSafety[entry.SafetyImpact] {
			return Report{}, fmt.Errorf("assignment %s has unknown safety impact %q", entry.ID, entry.SafetyImpact)
		}
		identity := triple{partition: entry.Partition, domain: entry.Domain, circuitRole: entry.CircuitRole}
		if seenTriples[identity] {
			return Report{}, fmt.Errorf("duplicate partition/domain/circuit-role assignment %q/%q/%q", entry.Partition, entry.Domain, entry.CircuitRole)
		}
		seenIDs[entry.ID], seenTriples[identity] = true, true
		authorCount[entry.Author]++
		partitionCount[entry.Partition]++
		domainCount[entry.Partition][entry.Domain]++
		roleCount[entry.Partition][entry.CircuitRole]++
		safetyCount[entry.Partition][entry.SafetyImpact]++
		switch entry.PrimaryClass {
		case "static":
			staticCount[entry.Author]++
		case "dynamic":
			dynamicCount[entry.Author]++
		default:
			return Report{}, fmt.Errorf("assignment %s has invalid primary class %q", entry.ID, entry.PrimaryClass)
		}
		if entry.OutputMultiplicity == "multiple" {
			multiCount[entry.Partition]++
		} else if entry.OutputMultiplicity != "single" {
			return Report{}, fmt.Errorf("assignment %s has invalid output multiplicity %q", entry.ID, entry.OutputMultiplicity)
		}
		if entry.RequireOffNominal {
			offNominalCount[entry.Author]++
		}
		if highSafety[entry.SafetyImpact] {
			highDomain[entry.Partition][entry.Domain] = true
			highRole[entry.Partition][entry.CircuitRole] = true
		}
	}

	for _, author := range policy.Authors {
		if authorCount[author] != policy.CasesPerAuthor || staticCount[author] < policy.MinimumStaticPerAuthor ||
			dynamicCount[author] < policy.MinimumDynamicPerAuthor || offNominalCount[author] < policy.MinimumOffNominalPerAuthor {
			return Report{}, fmt.Errorf("author %s assignment quota is infeasible", author)
		}
	}
	report := Report{EntryCount: len(entries)}
	var coverageFailures []string
	for _, partition := range policy.Partitions {
		if partitionCount[partition] != policy.CasesPerPartition || multiCount[partition] < policy.MinimumMultiOutputPerPartition {
			return Report{}, fmt.Errorf("partition %s assignment quota is infeasible", partition)
		}
		for _, domain := range policy.Domains {
			if domainCount[partition][domain] != policy.DimensionCountPerPartition {
				return Report{}, fmt.Errorf("partition %s domain %s count is infeasible", partition, domain)
			}
		}
		for _, role := range policy.CircuitRoles {
			if roleCount[partition][role] != policy.DimensionCountPerPartition {
				return Report{}, fmt.Errorf("partition %s circuit role %s count is infeasible", partition, role)
			}
		}
		for _, safety := range policy.SafetyImpacts {
			if safetyCount[partition][safety] != policy.SafetyCountPerPartition {
				return Report{}, fmt.Errorf("partition %s safety %s count is infeasible", partition, safety)
			}
		}
		coverage := PartitionCoverage{Partition: partition}
		if policy.RequireHighSafetyDomains {
			coverage.MissingHighSafetyDomains = missing(policy.Domains, highDomain[partition])
		}
		if policy.RequireHighSafetyCircuitRoles {
			coverage.MissingHighSafetyRoles = missing(policy.CircuitRoles, highRole[partition])
		}
		report.Partitions = append(report.Partitions, coverage)
		if len(coverage.MissingHighSafetyDomains) != 0 || len(coverage.MissingHighSafetyRoles) != 0 {
			coverageFailures = append(coverageFailures, fmt.Sprintf("partition %s high-safety coverage is infeasible: domains=%v circuit_roles=%v", partition, coverage.MissingHighSafetyDomains, coverage.MissingHighSafetyRoles))
		}
	}
	if len(coverageFailures) != 0 {
		return report, fmt.Errorf("%s", strings.Join(coverageFailures, "; "))
	}
	return report, nil
}

func validatePolicy(policy Policy) error {
	stringFields := []struct {
		name   string
		values []string
	}{{"Authors", policy.Authors}, {"Partitions", policy.Partitions}, {"Domains", policy.Domains}, {"CircuitRoles", policy.CircuitRoles},
		{"SafetyImpacts", policy.SafetyImpacts}, {"HighSafetyImpacts", policy.HighSafetyImpacts}}
	for _, field := range stringFields {
		name, values := field.name, field.values
		if !uniqueNonempty(values) {
			return fmt.Errorf("policy %s must contain unique nonempty values", name)
		}
	}
	positiveFields := []struct {
		name  string
		value int
	}{{"CasesPerAuthor", policy.CasesPerAuthor}, {"CasesPerPartition", policy.CasesPerPartition},
		{"DimensionCountPerPartition", policy.DimensionCountPerPartition}, {"SafetyCountPerPartition", policy.SafetyCountPerPartition}}
	for _, field := range positiveFields {
		name, value := field.name, field.value
		if value <= 0 {
			return fmt.Errorf("policy %s must be positive", name)
		}
	}
	nonnegativeFields := []struct {
		name  string
		value int
	}{{"MinimumStaticPerAuthor", policy.MinimumStaticPerAuthor}, {"MinimumDynamicPerAuthor", policy.MinimumDynamicPerAuthor},
		{"MinimumMultiOutputPerPartition", policy.MinimumMultiOutputPerPartition}, {"MinimumOffNominalPerAuthor", policy.MinimumOffNominalPerAuthor}}
	for _, field := range nonnegativeFields {
		name, value := field.name, field.value
		if value < 0 {
			return fmt.Errorf("policy %s cannot be negative", name)
		}
	}
	safety := set(policy.SafetyImpacts)
	for _, impact := range policy.HighSafetyImpacts {
		if !safety[impact] {
			return fmt.Errorf("high-safety impact %q is not a safety category", impact)
		}
	}
	if len(policy.Authors)*policy.CasesPerAuthor != len(policy.Partitions)*policy.CasesPerPartition {
		return fmt.Errorf("policy author and partition case totals disagree")
	}
	if len(policy.Domains)*policy.DimensionCountPerPartition != policy.CasesPerPartition ||
		len(policy.CircuitRoles)*policy.DimensionCountPerPartition != policy.CasesPerPartition ||
		len(policy.SafetyImpacts)*policy.SafetyCountPerPartition != policy.CasesPerPartition {
		return fmt.Errorf("policy dimension or safety quotas do not fill a partition")
	}
	if policy.MinimumStaticPerAuthor+policy.MinimumDynamicPerAuthor > policy.CasesPerAuthor || policy.MinimumOffNominalPerAuthor > policy.CasesPerAuthor {
		return fmt.Errorf("policy per-author minima exceed cases per author")
	}
	if policy.MinimumMultiOutputPerPartition > policy.CasesPerPartition {
		return fmt.Errorf("policy multi-output minimum exceeds cases per partition")
	}
	highSafetyCases := len(policy.HighSafetyImpacts) * policy.SafetyCountPerPartition
	if (policy.RequireHighSafetyDomains && highSafetyCases < len(policy.Domains)) ||
		(policy.RequireHighSafetyCircuitRoles && highSafetyCases < len(policy.CircuitRoles)) {
		return fmt.Errorf("policy cannot cover required high-safety dimensions")
	}
	return nil
}

func uniqueNonempty(values []string) bool {
	if len(values) == 0 {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func set(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func nestedCounts(keys []string) map[string]map[string]int {
	result := make(map[string]map[string]int, len(keys))
	for _, key := range keys {
		result[key] = map[string]int{}
	}
	return result
}

func nestedSeen(keys []string) map[string]map[string]bool {
	result := make(map[string]map[string]bool, len(keys))
	for _, key := range keys {
		result[key] = map[string]bool{}
	}
	return result
}

func missing(values []string, seen map[string]bool) []string {
	var result []string
	for _, value := range values {
		if !seen[value] {
			result = append(result, value)
		}
	}
	return result
}
