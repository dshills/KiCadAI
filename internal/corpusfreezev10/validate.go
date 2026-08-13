package corpusfreezev10

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"kicadai/internal/corpusassignment"
	"kicadai/internal/corpusfreeze"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	wantCasesPerRole          = 24
	wantDimensionPerRole      = 4
	wantSafetyPerRole         = 6
	wantSafetyCategory        = 12
	minimumMultiOutputPerRole = 6
	minimumOffNominalPerRole  = 6
)

func Validate(
	assignments map[string][]byte,
	bundles map[string]corpusfreeze.Bundle,
	binding Binding,
	historical HistoricalCommitments,
	policy Policy,
) (Report, error) {
	if err := ValidateHistoricalBoundary(historical); err != nil {
		return Report{}, err
	}
	if err := validateFrozenAssignmentPreflight(assignments, policy); err != nil {
		return Report{}, err
	}
	report, requirements, err := validateBase(assignments, bundles, binding, historical, policy)
	if err != nil {
		return Report{}, err
	}
	if err := validateAggregate(report, requirements, policy); err != nil {
		return Report{}, err
	}
	return report, nil
}

func validateFrozenAssignmentPreflight(assignments map[string][]byte, policy Policy) error {
	entries := make([]corpusassignment.Entry, 0, len(policy.AuthorSlots)*policy.CasesPerAuthor)
	for _, author := range policy.AuthorSlots {
		data, ok := assignments[author]
		if !ok {
			return fmt.Errorf("V10_ASSIGNMENT_MISSING: %s", author)
		}
		assignment, err := decodeAssignment(data)
		if err != nil {
			return err
		}
		if assignment.AuthorSlot != author {
			return fmt.Errorf("V10_ASSIGNMENT_HEADER_OR_COUNT: %s", author)
		}
		for _, entry := range assignment.Entries {
			entries = append(entries, corpusassignment.Entry{ID: entry.ID, Author: author, Partition: entry.Role, Domain: entry.Domain,
				CircuitRole: entry.CircuitRole, SafetyImpact: entry.SafetyImpact, PrimaryClass: entry.PrimaryClass,
				OutputMultiplicity: entry.OutputMultiplicity, RequireOffNominal: entry.RequireOffNominal})
		}
	}
	preflightPolicy := corpusassignment.Policy{Authors: append([]string(nil), policy.AuthorSlots...), Partitions: append([]string(nil), policy.Roles...),
		Domains: append([]string(nil), policy.Domains...), CircuitRoles: append([]string(nil), policy.CircuitRoles...),
		SafetyImpacts: append([]string(nil), policy.SafetyImpacts...), HighSafetyImpacts: []string{"safety_relevant", "safety_critical"},
		CasesPerAuthor: policy.CasesPerAuthor, CasesPerPartition: wantCasesPerRole, DimensionCountPerPartition: wantDimensionPerRole,
		SafetyCountPerPartition: wantSafetyPerRole, MinimumStaticPerAuthor: policy.MinimumStaticCasesPerAuthor,
		MinimumDynamicPerAuthor: policy.MinimumDynamicCasesPerAuthor, MinimumMultiOutputPerPartition: minimumMultiOutputPerRole,
		MinimumOffNominalPerAuthor: policy.MinimumOffNominalPerAuthor, RequireHighSafetyDomains: true, RequireHighSafetyCircuitRoles: true}
	if _, err := corpusassignment.Validate(entries, preflightPolicy); err != nil {
		return fmt.Errorf("V10_ASSIGNMENT_PREFLIGHT: %w", err)
	}
	return nil
}

func validateBase(assignments map[string][]byte, bundles map[string]corpusfreeze.Bundle, binding Binding, historical HistoricalCommitments, policy Policy) (Report, map[string]ots.Requirement, error) {
	if !validSHA256(policy.HistoricalCommitmentsSHA256) || !reflect.DeepEqual(policy, PolicyForHistory(policy.HistoricalCommitmentsSHA256)) ||
		len(assignments) != len(policy.AuthorSlots) || len(bundles) != len(policy.AuthorSlots) ||
		binding.PacketSetSHA256 != policy.PacketSetSHA256 || historical.Base.SourceSHA256 != policy.HistoricalCommitmentsSHA256 {
		return Report{}, nil, fmt.Errorf("V10_FROZEN_INPUT_BINDING")
	}
	if !validSHA256(binding.ContractBindingSHA256) || len(binding.AuthorPacketSHA256) != len(policy.AuthorSlots) || len(binding.AssignmentSHA256) != len(policy.AuthorSlots) {
		return Report{}, nil, fmt.Errorf("V10_FROZEN_INPUT_BINDING")
	}
	commitmentSets := []map[string]string{historical.Base.RawSHA256, historical.Base.NeutralSemanticSHA256, historical.NormalizedSemanticSHA256}
	for _, commitments := range commitmentSets {
		if len(commitments) == 0 {
			return Report{}, nil, fmt.Errorf("V10_HISTORICAL_COMMITMENT")
		}
		for digest, id := range commitments {
			if !validSHA256(digest) || strings.TrimSpace(id) == "" {
				return Report{}, nil, fmt.Errorf("V10_HISTORICAL_COMMITMENT")
			}
		}
	}
	policyDigest, err := policyHash(policy)
	if err != nil {
		return Report{}, nil, err
	}
	implementationPattern, err := prohibitedPattern(policy.ProhibitedTerms)
	if err != nil {
		return Report{}, nil, err
	}
	report := Report{Schema: "kicadai.behavior-corpus-validation-report.v10", Version: 10,
		PolicySHA256: policyDigest, PacketSetSHA256: binding.PacketSetSHA256,
		ContractBindingSHA256: binding.ContractBindingSHA256, HistoricalCommitmentsSHA256: historical.Base.SourceSHA256,
		AuthorPacketSHA256: map[string]string{}, AssignmentSHA256: map[string]string{}, AuthorshipSHA256: map[string]string{},
		Counts: map[string]map[string]int{}, AuthorStartedAt: map[string]time.Time{}, AuthorEndedAt: map[string]time.Time{},
	}
	for _, role := range policy.Roles {
		report.Counts[role] = map[string]int{}
	}
	identities, sources, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	rawHashes, neutralHashes, normalizedHashes := map[string]string{}, map[string]string{}, map[string]string{}
	decodedRequirements := map[string]ots.Requirement{}
	for _, author := range policy.AuthorSlots {
		assignmentData, exists := assignments[author]
		if !exists {
			return Report{}, nil, fmt.Errorf("V10_ASSIGNMENT_MISSING: %s", author)
		}
		bundle, exists := bundles[author]
		if !exists {
			return Report{}, nil, fmt.Errorf("V10_BUNDLE_MISSING: %s", author)
		}
		assignment, err := decodeAssignment(assignmentData)
		if err != nil {
			return Report{}, nil, err
		}
		if assignment.Schema != policy.AssignmentSchema || assignment.Version != policy.Version || assignment.AuthorSlot != author || len(assignment.Entries) != policy.CasesPerAuthor {
			return Report{}, nil, fmt.Errorf("V10_ASSIGNMENT_HEADER_OR_COUNT: %s", author)
		}
		assignmentHash := hashBytes(assignmentData)
		if assignmentHash != binding.AssignmentSHA256[author] || !validSHA256(binding.AssignmentSHA256[author]) || !validSHA256(binding.AuthorPacketSHA256[author]) {
			return Report{}, nil, fmt.Errorf("V10_ASSIGNMENT_BINDING: %s", author)
		}
		authorship, err := decodeAuthorship(bundle.AuthorshipJSON)
		if err != nil {
			return Report{}, nil, err
		}
		started, ended, err := validateAuthorship(author, authorship, assignment, assignmentHash, binding)
		if err != nil {
			return Report{}, nil, err
		}
		report.AuthorStartedAt[author], report.AuthorEndedAt[author] = started, ended
		report.AuthorPacketSHA256[author], report.AssignmentSHA256[author], report.AuthorshipSHA256[author] = binding.AuthorPacketSHA256[author], assignmentHash, hashBytes(bundle.AuthorshipJSON)
		if len(bundle.Requirements) != policy.CasesPerAuthor {
			return Report{}, nil, fmt.Errorf("V10_BUNDLE_COUNT: %s", author)
		}
		for index, entry := range assignment.Entries {
			if !slices.Contains(policy.Roles, entry.Role) || !slices.Contains(policy.Domains, entry.Domain) ||
				!slices.Contains(policy.CircuitRoles, entry.CircuitRole) || !slices.Contains(policy.SafetyImpacts, entry.SafetyImpact) ||
				!slices.Contains([]string{"static", "dynamic"}, entry.PrimaryClass) || !slices.Contains(analyses, entry.RequiredPrimaryAnalysis) ||
				!slices.Contains([]string{"single", "multiple"}, entry.OutputMultiplicity) ||
				entry.ID == "" || entry.SourceID == "" || !validRelativePath(entry.RequirementFile) || identities[entry.ID] || sources[entry.SourceID] || paths[entry.RequirementFile] {
				return Report{}, nil, fmt.Errorf("V10_ASSIGNMENT_ENTRY: %s", author)
			}
			if analysisClass(entry.RequiredPrimaryAnalysis) != entry.PrimaryClass {
				return Report{}, nil, fmt.Errorf("V10_ASSIGNMENT_PRIMARY_CLASS: %s", entry.ID)
			}
			identities[entry.ID], sources[entry.SourceID], paths[entry.RequirementFile] = true, true, true
			if index >= len(authorship.RequirementSourceSHA256) || authorship.RequirementSourceSHA256[index].Path != entry.RequirementFile || !validSHA256(authorship.RequirementSourceSHA256[index].SHA256) {
				return Report{}, nil, fmt.Errorf("V10_AUTHORSHIP_SOURCE_ORDER: %s", author)
			}
			data, exists := bundle.Requirements[entry.RequirementFile]
			if !exists {
				return Report{}, nil, fmt.Errorf("V10_REQUIREMENT_MISSING: %s", entry.RequirementFile)
			}
			rawHash := hashBytes(data)
			if rawHash != authorship.RequirementSourceSHA256[index].SHA256 || historical.Base.RawSHA256[rawHash] != "" || rawHashes[rawHash] != "" {
				return Report{}, nil, fmt.Errorf("V10_RAW_HASH_OR_DUPLICATE: %s", entry.ID)
			}
			requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
			if len(issues) != 0 {
				return Report{}, nil, fmt.Errorf("V10_REQUIREMENT_CONTRACT: %s (%d issues)", entry.ID, len(issues))
			}
			if len(requirement.Requirements.OperatingCases) < policy.MinimumOperatingCases || len(requirement.Requirements.BehavioralRequirements) < policy.MinimumAssertions || containsProhibitedRequirement(requirement, policy.ProhibitedIdentityPrefixes, implementationPattern) {
				return Report{}, nil, fmt.Errorf("V10_REQUIREMENT_BOUNDARY: %s", entry.ID)
			}
			for _, assertion := range requirement.Requirements.BehavioralRequirements {
				if assertion.Metric == "thd" {
					return Report{}, nil, fmt.Errorf("V10_LEGACY_THD: %s", entry.ID)
				}
			}
			primary := primaryAssertion(requirement)
			if primary.Analysis != entry.RequiredPrimaryAnalysis || analysisClass(primary.Analysis) != entry.PrimaryClass {
				return Report{}, nil, fmt.Errorf("V10_PRIMARY_ASSIGNMENT: %s", entry.ID)
			}
			multiOutput := hasMeaningfulMultiOutput(requirement)
			if multiOutput != (entry.OutputMultiplicity == "multiple") {
				return Report{}, nil, fmt.Errorf("V10_OUTPUT_MULTIPLICITY: %s", entry.ID)
			}
			if entry.RequireOffNominal && !hasBoundedOffNominalCase(requirement) {
				return Report{}, nil, fmt.Errorf("V10_OFF_NOMINAL_ASSIGNMENT: %s", entry.ID)
			}
			neutralHash, normalizedHash, err := semanticHashes(requirement)
			if err != nil {
				return Report{}, nil, err
			}
			if historical.Base.NeutralSemanticSHA256[neutralHash] != "" || historical.NormalizedSemanticSHA256[normalizedHash] != "" || neutralHashes[neutralHash] != "" || normalizedHashes[normalizedHash] != "" {
				return Report{}, nil, fmt.Errorf("V10_SEMANTIC_DUPLICATE: %s", entry.ID)
			}
			rawHashes[rawHash], neutralHashes[neutralHash], normalizedHashes[normalizedHash] = entry.ID, entry.ID, entry.ID
			report.Counts[entry.Role][entry.Domain]++
			report.Entries = append(report.Entries, EntryEvidence{ID: entry.ID, AuthorSlot: author, Role: entry.Role, Domain: entry.Domain,
				CircuitRole: entry.CircuitRole, SafetyImpact: entry.SafetyImpact, PrimaryClass: entry.PrimaryClass,
				RequiredPrimaryAnalysis: entry.RequiredPrimaryAnalysis, OutputMultiplicity: entry.OutputMultiplicity,
				RequireOffNominal: entry.RequireOffNominal, SourceID: entry.SourceID, RequirementFile: entry.RequirementFile,
				RequirementSHA256: rawHash, NeutralSemanticSHA256: neutralHash, NormalizedSemanticSHA256: normalizedHash})
			decodedRequirements[entry.ID] = requirement
		}
	}
	sort.Slice(report.Entries, func(i, j int) bool { return report.Entries[i].ID < report.Entries[j].ID })
	return report, decodedRequirements, nil
}

func validateAuthorship(author string, value Authorship, assignment Assignment, assignmentHash string, binding Binding) (time.Time, time.Time, error) {
	manifest := "AUTHOR_" + strings.TrimPrefix(author, "author_") + "_PACKET.sha256"
	if value.Schema != "kicadai.closed-loop-open-set-authorship.v10" || value.Version != 10 || value.AuthorSlot != author ||
		value.PerAuthorPacketManifest != manifest || value.PerAuthorPacketSHA256 != binding.AuthorPacketSHA256[author] ||
		value.ContractBindingSHA256 != binding.ContractBindingSHA256 || value.AssignmentSHA256 != assignmentHash ||
		len(value.RequirementSourceSHA256) != len(assignment.Entries) || !value.Attestations.allTrue() {
		return time.Time{}, time.Time{}, fmt.Errorf("V10_AUTHORSHIP_BINDING: %s", author)
	}
	for _, text := range []string{value.AuthorContextIdentity, value.AuthoringToolModelVersion, value.ReturnedBundleRoot} {
		if strings.TrimSpace(text) == "" || strings.ContainsAny(text, "[]") {
			return time.Time{}, time.Time{}, fmt.Errorf("V10_AUTHORSHIP_UNRESOLVED: %s", author)
		}
	}
	for _, uncertainty := range value.Uncertainties {
		if strings.TrimSpace(uncertainty) == "" || strings.ContainsAny(uncertainty, "[]") {
			return time.Time{}, time.Time{}, fmt.Errorf("V10_AUTHORSHIP_UNCERTAINTY: %s", author)
		}
	}
	started, err := time.Parse(time.RFC3339, value.AuthoringStartedUTC)
	if err != nil || !strings.HasSuffix(value.AuthoringStartedUTC, "Z") {
		return time.Time{}, time.Time{}, fmt.Errorf("V10_AUTHORSHIP_START: %s", author)
	}
	ended, err := time.Parse(time.RFC3339, value.AuthoringEndedUTC)
	if err != nil || !strings.HasSuffix(value.AuthoringEndedUTC, "Z") || ended.Before(started) {
		return time.Time{}, time.Time{}, fmt.Errorf("V10_AUTHORSHIP_END: %s", author)
	}
	return started, ended, nil
}

func prohibitedPattern(terms []string) (*regexp.Regexp, error) {
	escaped := make([]string, len(terms))
	for index, term := range terms {
		escaped[index] = regexp.QuoteMeta(term)
	}
	return regexp.Compile(`(?i)\b(?:` + strings.Join(escaped, "|") + `)\b`)
}

func containsProhibitedRequirement(requirement ots.Requirement, identityPrefixes []string, implementationPattern *regexp.Regexp) bool {
	// Strict decoding has already fixed every object key. Scan the decoded
	// values once, excluding only the protocol-owned schema discriminator whose
	// canonical value contains the otherwise prohibited word "topology".
	requirement.Schema = ""
	stack := []reflect.Value{reflect.ValueOf(requirement)}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !current.IsValid() {
			continue
		}
		switch current.Kind() {
		case reflect.Interface, reflect.Pointer:
			if !current.IsNil() {
				stack = append(stack, current.Elem())
			}
		case reflect.String:
			item := current.String()
			for _, prefix := range identityPrefixes {
				if strings.Contains(item, prefix) {
					return true
				}
			}
			if implementationPattern.MatchString(item) {
				return true
			}
		case reflect.Array, reflect.Slice:
			for index := 0; index < current.Len(); index++ {
				stack = append(stack, current.Index(index))
			}
		case reflect.Struct:
			for index := 0; index < current.NumField(); index++ {
				stack = append(stack, current.Field(index))
			}
		case reflect.Map:
			iterator := current.MapRange()
			for iterator.Next() {
				stack = append(stack, iterator.Key(), iterator.Value())
			}
		}
	}
	return false
}

type aggregateEvidence struct {
	roleCount          map[string]int
	domainRoleCount    map[string]map[string]int
	circuitRoleCount   map[string]map[string]int
	safetyRoleCount    map[string]map[string]int
	highSafetyDomain   map[string]map[string]bool
	highSafetyCircuit  map[string]map[string]bool
	analysesByRole     map[string]map[string]bool
	multiOutputByRole  map[string]int
	offNominalByRole   map[string]int
	authorDomains      map[string]map[string]bool
	authorCircuitRoles map[string]map[string]bool
	authorRoles        map[string]map[string]int
	authorAnalyses     map[string]map[string]bool
	authorMultiOutput  map[string]map[string]int
	authorStatic       map[string]int
	authorDynamic      map[string]int
	authorOffNominal   map[string]int
	authorSpecial      map[string]bool
	pairs              map[string]string
	signatures         map[string]string
}

func newAggregateEvidence(policy Policy) *aggregateEvidence {
	e := &aggregateEvidence{
		roleCount: map[string]int{}, domainRoleCount: map[string]map[string]int{},
		circuitRoleCount: map[string]map[string]int{}, safetyRoleCount: map[string]map[string]int{},
		highSafetyDomain: map[string]map[string]bool{}, highSafetyCircuit: map[string]map[string]bool{},
		analysesByRole: map[string]map[string]bool{}, multiOutputByRole: map[string]int{}, offNominalByRole: map[string]int{},
		authorDomains: map[string]map[string]bool{}, authorCircuitRoles: map[string]map[string]bool{},
		authorRoles: map[string]map[string]int{}, authorAnalyses: map[string]map[string]bool{},
		authorMultiOutput: map[string]map[string]int{}, authorStatic: map[string]int{}, authorDynamic: map[string]int{},
		authorOffNominal: map[string]int{}, authorSpecial: map[string]bool{}, pairs: map[string]string{}, signatures: map[string]string{},
	}
	for _, role := range policy.Roles {
		e.domainRoleCount[role], e.circuitRoleCount[role], e.safetyRoleCount[role] = map[string]int{}, map[string]int{}, map[string]int{}
		e.highSafetyDomain[role], e.highSafetyCircuit[role], e.analysesByRole[role] = map[string]bool{}, map[string]bool{}, map[string]bool{}
	}
	for _, author := range policy.AuthorSlots {
		e.authorDomains[author], e.authorCircuitRoles[author], e.authorRoles[author] = map[string]bool{}, map[string]bool{}, map[string]int{}
		e.authorAnalyses[author], e.authorMultiOutput[author] = map[string]bool{}, map[string]int{}
	}
	return e
}

func validateAggregate(report Report, requirements map[string]ots.Requirement, policy Policy) error {
	if len(report.Entries) != len(policy.AuthorSlots)*policy.CasesPerAuthor {
		return fmt.Errorf("V10_CASE_TOTAL: got %d", len(report.Entries))
	}
	e := newAggregateEvidence(policy)
	for _, entry := range report.Entries {
		requirement, ok := requirements[entry.ID]
		if !ok {
			return fmt.Errorf("V10_REQUIREMENT_MISSING: %s", entry.ID)
		}
		if err := e.observe(entry, requirement); err != nil {
			return err
		}
	}
	return e.check(policy)
}

func (e *aggregateEvidence) observe(entry EntryEvidence, requirement ots.Requirement) error {
	e.roleCount[entry.Role]++
	e.domainRoleCount[entry.Role][entry.Domain]++
	e.circuitRoleCount[entry.Role][entry.CircuitRole]++
	e.safetyRoleCount[entry.Role][entry.SafetyImpact]++
	e.authorDomains[entry.AuthorSlot][entry.Domain] = true
	e.authorCircuitRoles[entry.AuthorSlot][entry.CircuitRole] = true
	e.authorRoles[entry.AuthorSlot][entry.Role]++
	pair := entry.Role + "\x00" + entry.Domain + "\x00" + entry.CircuitRole
	if prior := e.pairs[pair]; prior != "" {
		return fmt.Errorf("V10_DOMAIN_CIRCUIT_ROLE_PAIR_DUPLICATE: %s and %s", prior, entry.ID)
	}
	e.pairs[pair] = entry.ID
	if entry.SafetyImpact == "safety_relevant" || entry.SafetyImpact == "safety_critical" {
		e.highSafetyDomain[entry.Role][entry.Domain] = true
		e.highSafetyCircuit[entry.Role][entry.CircuitRole] = true
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		e.analysesByRole[entry.Role][assertion.Analysis] = true
		e.authorAnalyses[entry.AuthorSlot][assertion.Analysis] = true
		if slices.Contains([]string{"noise", "thermal", "electrothermal", "stability"}, assertion.Analysis) {
			e.authorSpecial[entry.AuthorSlot] = true
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "tolerance_corner" || condition.Axis == "model_corner" {
				e.authorSpecial[entry.AuthorSlot] = true
			}
		}
	}
	if err := validateSafetyEvidence(entry, requirement); err != nil {
		return err
	}
	multiOutput := hasMeaningfulMultiOutput(requirement)
	if multiOutput {
		e.multiOutputByRole[entry.Role]++
		e.authorMultiOutput[entry.AuthorSlot][entry.Role]++
	}
	if hasBoundedOffNominalCase(requirement) {
		e.offNominalByRole[entry.Role]++
		e.authorOffNominal[entry.AuthorSlot]++
	}
	primary := primaryAssertion(requirement)
	if analysisClass(primary.Analysis) == "static" {
		e.authorStatic[entry.AuthorSlot]++
	} else {
		e.authorDynamic[entry.AuthorSlot]++
	}
	signature := behaviorSignature(requirement)
	if prior := e.signatures[signature]; prior != "" {
		return fmt.Errorf("V10_NORMALIZED_BEHAVIOR_SIGNATURE_DUPLICATE: %s and %s", prior, entry.ID)
	}
	e.signatures[signature] = entry.ID
	return nil
}

func (e *aggregateEvidence) check(policy Policy) error {
	for _, role := range policy.Roles {
		if e.roleCount[role] != wantCasesPerRole {
			return fmt.Errorf("V10_ROLE_TOTAL: %s=%d", role, e.roleCount[role])
		}
		for _, domain := range policy.Domains {
			if e.domainRoleCount[role][domain] != wantDimensionPerRole || !e.highSafetyDomain[role][domain] {
				return fmt.Errorf("V10_DOMAIN_ROLE_BALANCE: %s/%s", role, domain)
			}
		}
		for _, circuitRole := range policy.CircuitRoles {
			if e.circuitRoleCount[role][circuitRole] != wantDimensionPerRole || !e.highSafetyCircuit[role][circuitRole] {
				return fmt.Errorf("V10_CIRCUIT_ROLE_BALANCE: %s/%s", role, circuitRole)
			}
		}
		for _, analysis := range analyses {
			if !e.analysesByRole[role][analysis] {
				return fmt.Errorf("V10_ANALYSIS_COVERAGE: %s omits %s", role, analysis)
			}
		}
		if e.multiOutputByRole[role] < minimumMultiOutputPerRole || e.offNominalByRole[role] < minimumOffNominalPerRole {
			return fmt.Errorf("V10_ROLE_BEHAVIOR_DIVERSITY: %s multi=%d off_nominal=%d", role, e.multiOutputByRole[role], e.offNominalByRole[role])
		}
	}
	for _, safety := range policy.SafetyImpacts {
		total := 0
		for _, role := range policy.Roles {
			count := e.safetyRoleCount[role][safety]
			if count != wantSafetyPerRole {
				return fmt.Errorf("V10_SAFETY_ROLE_BALANCE: %s/%s=%d", role, safety, count)
			}
			total += count
		}
		if total != wantSafetyCategory {
			return fmt.Errorf("V10_SAFETY_TOTAL: %s=%d", safety, total)
		}
	}
	for _, author := range policy.AuthorSlots {
		if len(e.authorDomains[author]) != len(policy.Domains) || len(e.authorCircuitRoles[author]) < 5 ||
			e.authorRoles[author]["discovery"] != 4 || e.authorRoles[author]["held_out"] != 4 {
			return fmt.Errorf("V10_AUTHOR_ASSIGNMENT_BALANCE: %s", author)
		}
		if len(e.authorAnalyses[author]) < policy.MinimumAnalysisKindsPerAuthor || e.authorStatic[author] < policy.MinimumStaticCasesPerAuthor ||
			e.authorDynamic[author] < policy.MinimumDynamicCasesPerAuthor || e.authorOffNominal[author] < policy.MinimumOffNominalPerAuthor || !e.authorSpecial[author] {
			return fmt.Errorf("V10_AUTHOR_BEHAVIOR_DIVERSITY: %s", author)
		}
		if e.authorMultiOutput[author]["discovery"] < policy.MinimumMultiOutputPerRoleAuthor || e.authorMultiOutput[author]["held_out"] < policy.MinimumMultiOutputPerRoleAuthor ||
			e.authorMultiOutput[author]["discovery"]+e.authorMultiOutput[author]["held_out"] < 2 {
			return fmt.Errorf("V10_AUTHOR_MULTI_OUTPUT: %s", author)
		}
	}
	return nil
}

func analysisClass(analysis string) string {
	if slices.Contains([]string{"dc_operating_point", "dc_sweep", "thermal", "electrothermal"}, analysis) {
		return "static"
	}
	return "dynamic"
}

func primaryAssertion(requirement ots.Requirement) ots.BehavioralAssertion {
	primary := requirement.Requirements.BehavioralRequirements[0]
	for _, assertion := range requirement.Requirements.BehavioralRequirements[1:] {
		if assertion.ID < primary.ID {
			primary = assertion
		}
	}
	return primary
}

func hasMeaningfulMultiOutput(requirement ots.Requirement) bool {
	outputs := map[string]bool{}
	for _, port := range requirement.Requirements.Ports {
		if port.Direction == "source" && port.Kind != "power" && port.Kind != "reference" {
			outputs[port.ID] = false
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind == "port" {
			if _, exists := outputs[assertion.Observation.ID]; exists {
				outputs[assertion.Observation.ID] = true
			}
		}
	}
	observed := 0
	for _, value := range outputs {
		if value {
			observed++
		}
	}
	return observed >= 2
}

func hasBoundedOffNominalCase(requirement ots.Requirement) bool {
	return len(offNominalCases(requirement)) != 0
}

func offNominalCases(requirement ots.Requirement) map[string]bool {
	result := map[string]bool{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		if len(operatingCase.Events) != 0 {
			result[operatingCase.ID] = true
		}
		for _, condition := range operatingCase.Conditions {
			if condition.Min != condition.Max && condition.Axis != "input_voltage" && condition.Axis != "input_current" {
				result[operatingCase.ID] = true
			}
		}
	}
	return result
}

func validateSafetyEvidence(entry EntryEvidence, requirement ots.Requirement) error {
	offNominal := offNominalCases(requirement)
	critical, criticalOffNominal := 0, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if !assertion.Critical {
			continue
		}
		critical++
		for _, operatingCase := range assertion.OperatingCases {
			criticalOffNominal = criticalOffNominal || offNominal[operatingCase]
		}
	}
	switch entry.SafetyImpact {
	case "non_safety":
		if critical != 0 {
			return fmt.Errorf("V10_NON_SAFETY_CRITICAL_ASSERTION: %s", entry.ID)
		}
	case "review_required":
		if len(offNominal) == 0 {
			return fmt.Errorf("V10_REVIEW_REQUIRED_OFF_NOMINAL: %s", entry.ID)
		}
	case "safety_relevant":
		if critical < 1 {
			return fmt.Errorf("V10_SAFETY_RELEVANT_CRITICAL_ASSERTION: %s", entry.ID)
		}
	case "safety_critical":
		if critical < 2 || !criticalOffNominal {
			return fmt.Errorf("V10_SAFETY_CRITICAL_EVIDENCE: %s", entry.ID)
		}
	}
	return nil
}

func behaviorSignature(requirement ots.Requirement) string {
	parts := []string{}
	for _, domain := range requirement.Requirements.Domains {
		parts = append(parts, "d:"+strings.Join([]string{domain.Kind, floatPointer(domain.MinVoltageV), floatPointer(domain.NominalVoltageV), floatPointer(domain.MaxVoltageV), floatPointer(domain.MaxCurrentA), domain.Source}, ":"))
	}
	for _, port := range requirement.Requirements.Ports {
		e := port.Electrical
		parts = append(parts, "p:"+strings.Join([]string{port.Kind, port.Direction, floatPointer(e.MinVoltageV), floatPointer(e.NominalVoltageV), floatPointer(e.MaxVoltageV), floatPointer(e.MaxCurrentA), floatPointer(e.InputImpedanceMinOhm), e.DefaultState}, ":"))
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			parts = append(parts, "c:"+strings.Join([]string{condition.Axis, floatValue(condition.Min), floatValue(condition.Max), condition.Unit}, ":"))
		}
		for _, event := range operatingCase.Events {
			parts = append(parts, "e:"+strings.Join([]string{event.Kind, floatValue(event.TriggerTimeS), floatValue(event.Initial), floatValue(event.Applied), event.Unit}, ":"))
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		excitation := "none"
		if assertion.Excitation != nil {
			excitation = assertion.Excitation.Kind
		}
		parts = append(parts, "a:"+strings.Join([]string{assertion.Analysis, assertion.Metric, assertion.Unit, excitation, assertion.Observation.Kind, floatPointer(assertion.Min), floatPointer(assertion.Max), floatPointer(assertion.FrequencyHz), strconv.FormatBool(assertion.Critical)}, ":"))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func floatPointer(value *float64) string {
	if value == nil {
		return "-"
	}
	return floatValue(*value)
}

func floatValue(value float64) string {
	if value == 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}
