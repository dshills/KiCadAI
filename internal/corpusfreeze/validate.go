package corpusfreeze

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	ots "kicadai/internal/opentopologysynthesis"
)

type validationState struct {
	policy             Policy
	binding            Binding
	historical         HistoricalCommitments
	entriesByID        map[string]bool
	sources            map[string]bool
	files              map[string]bool
	rawHashes          map[string]string
	neutralHashes      map[string]string
	normalizedHashes   map[string]string
	counts             map[string]map[string]int
	roleDiversity      map[string]*diversityEvidence
	authorDiversity    map[string]*diversityEvidence
	domainSignatures   map[string]map[string]map[[3]string]bool
	implementationText *regexp.Regexp
	report             Report
}

func Validate(assignments map[string][]byte, bundles map[string]Bundle, binding Binding, historical HistoricalCommitments, policy Policy) (Report, error) {
	if err := validatePolicy(policy); err != nil {
		return Report{}, err
	}
	if err := validateBinding(policy, binding, historical); err != nil {
		return Report{}, err
	}
	state, err := newValidationState(policy, binding, historical)
	if err != nil {
		return Report{}, err
	}
	if len(assignments) != len(policy.AuthorSlots) || len(bundles) != len(policy.AuthorSlots) {
		return Report{}, fmt.Errorf("assignment/bundle counts = %d/%d, want %d each", len(assignments), len(bundles), len(policy.AuthorSlots))
	}
	for _, authorSlot := range policy.AuthorSlots {
		assignmentData, ok := assignments[authorSlot]
		if !ok {
			return Report{}, fmt.Errorf("missing assignment for %s", authorSlot)
		}
		bundle, ok := bundles[authorSlot]
		if !ok {
			return Report{}, fmt.Errorf("missing bundle for %s", authorSlot)
		}
		if err := state.validateAuthor(authorSlot, assignmentData, bundle); err != nil {
			return Report{}, err
		}
	}
	if err := state.validateAggregate(); err != nil {
		return Report{}, err
	}
	sort.Slice(state.report.Entries, func(i, j int) bool { return state.report.Entries[i].ID < state.report.Entries[j].ID })
	return state.report, nil
}

func newValidationState(policy Policy, binding Binding, historical HistoricalCommitments) (*validationState, error) {
	terms := make([]string, len(policy.ProhibitedTerms))
	for index, term := range policy.ProhibitedTerms {
		terms[index] = regexp.QuoteMeta(term)
	}
	implementationText, err := regexp.Compile(`(?i)\b(?:` + strings.Join(terms, "|") + `)\b`)
	if err != nil {
		return nil, fmt.Errorf("compile prohibited-language policy: %w", err)
	}
	policySHA256, err := policyHash(policy)
	if err != nil {
		return nil, err
	}
	state := &validationState{
		policy: policy, binding: binding, historical: historical,
		entriesByID: map[string]bool{}, sources: map[string]bool{}, files: map[string]bool{},
		rawHashes: map[string]string{}, neutralHashes: map[string]string{}, normalizedHashes: map[string]string{},
		counts: map[string]map[string]int{}, roleDiversity: map[string]*diversityEvidence{},
		authorDiversity: map[string]*diversityEvidence{}, domainSignatures: map[string]map[string]map[[3]string]bool{},
		implementationText: implementationText,
		report: Report{
			Schema: "kicadai.behavior-corpus-validation-report.v1", Version: 1,
			PolicySHA256: policySHA256, ContractBindingSHA256: binding.ContractBindingSHA256,
			AuthorPacketSHA256: map[string]string{}, AssignmentSHA256: map[string]string{}, AuthorshipSHA256: map[string]string{},
			Counts: map[string]map[string]int{}, AuthorStartedAt: map[string]time.Time{}, AuthorEndedAt: map[string]time.Time{},
		},
	}
	for _, role := range policy.Roles {
		state.counts[role] = map[string]int{}
		state.report.Counts[role] = map[string]int{}
		state.roleDiversity[role] = newDiversityEvidence()
	}
	for _, author := range policy.AuthorSlots {
		state.authorDiversity[author] = newDiversityEvidence()
		state.domainSignatures[author] = map[string]map[[3]string]bool{}
	}
	return state, nil
}

func (state *validationState) validateAuthor(authorSlot string, assignmentData []byte, bundle Bundle) error {
	assignment, err := DecodeAssignmentStrict(assignmentData)
	if err != nil {
		return fmt.Errorf("%s: %w", authorSlot, err)
	}
	if assignment.Schema != state.policy.AssignmentSchema || assignment.Version != state.policy.Version || assignment.AuthorSlot != authorSlot {
		return fmt.Errorf("%s assignment header is invalid", authorSlot)
	}
	wantEntryCount := len(state.policy.Roles) * len(state.policy.Domains) * state.policy.CasesPerAuthorRoleDomain
	if len(assignment.Entries) != wantEntryCount {
		return fmt.Errorf("%s assignment entries = %d, want %d", authorSlot, len(assignment.Entries), wantEntryCount)
	}
	assignmentSHA256 := hashBytes(assignmentData)
	authorship, started, ended, err := state.validateAuthorship(authorSlot, assignmentSHA256, bundle.AuthorshipJSON)
	if err != nil {
		return err
	}
	state.report.AuthorStartedAt[authorSlot], state.report.AuthorEndedAt[authorSlot] = started, ended
	state.report.AuthorPacketSHA256[authorSlot] = state.binding.AuthorPacketSHA256[authorSlot]
	state.report.AssignmentSHA256[authorSlot] = assignmentSHA256
	state.report.AuthorshipSHA256[authorSlot] = hashBytes(bundle.AuthorshipJSON)

	wantFiles := map[string]AssignmentEntry{}
	localCounts := map[string]map[string]int{}
	for _, role := range state.policy.Roles {
		localCounts[role] = map[string]int{}
	}
	for _, entry := range assignment.Entries {
		if err := state.validateEntryMetadata(authorSlot, entry); err != nil {
			return err
		}
		wantFiles[entry.RequirementFile] = entry
		localCounts[entry.Role][entry.Domain]++
	}
	for _, role := range state.policy.Roles {
		for _, domain := range state.policy.Domains {
			if localCounts[role][domain] != state.policy.CasesPerAuthorRoleDomain {
				return fmt.Errorf("%s %s/%s count = %d, want %d", authorSlot, role, domain, localCounts[role][domain], state.policy.CasesPerAuthorRoleDomain)
			}
		}
	}
	if len(bundle.Requirements) != len(wantFiles) {
		return fmt.Errorf("%s bundle requirement count = %d, want %d", authorSlot, len(bundle.Requirements), len(wantFiles))
	}
	bundlePaths := make([]string, 0, len(bundle.Requirements))
	for bundlePath := range bundle.Requirements {
		bundlePaths = append(bundlePaths, bundlePath)
	}
	sort.Strings(bundlePaths)
	for _, bundlePath := range bundlePaths {
		if !validRelativePath(bundlePath) {
			return fmt.Errorf("%s bundle contains unsafe requirement path", authorSlot)
		}
	}
	if len(authorship.RequirementSourceSHA256) != len(assignment.Entries) {
		return fmt.Errorf("%s authorship source hash count = %d, want %d", authorSlot, len(authorship.RequirementSourceSHA256), len(assignment.Entries))
	}
	sourceHashes := map[string]string{}
	for index, source := range authorship.RequirementSourceSHA256 {
		if !validRelativePath(source.Path) || !validSHA256(source.SHA256) || sourceHashes[source.Path] != "" {
			return fmt.Errorf("%s authorship contains invalid or duplicate source hash path", authorSlot)
		}
		if source.Path != assignment.Entries[index].RequirementFile {
			return fmt.Errorf("%s authorship source hashes are not in assignment order", authorSlot)
		}
		sourceHashes[source.Path] = source.SHA256
	}
	for _, entry := range assignment.Entries {
		data, ok := bundle.Requirements[entry.RequirementFile]
		if !ok {
			return fmt.Errorf("%s bundle omits %s", authorSlot, entry.RequirementFile)
		}
		rawHash := hashBytes(data)
		if sourceHashes[entry.RequirementFile] != rawHash {
			return fmt.Errorf("%s source hash mismatch for %s", authorSlot, entry.RequirementFile)
		}
		if err := state.validateRequirement(authorSlot, entry, data, rawHash); err != nil {
			return err
		}
	}
	return nil
}

func (state *validationState) validateAuthorship(authorSlot, assignmentSHA256 string, data []byte) (Authorship, time.Time, time.Time, error) {
	authorship, err := DecodeAuthorshipStrict(data)
	if err != nil {
		return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s: %w", authorSlot, err)
	}
	if authorship.Schema != state.policy.AuthorshipSchema || authorship.Version != state.policy.Version || authorship.AuthorSlot != authorSlot {
		return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship header is invalid", authorSlot)
	}
	for _, field := range []struct{ name, value string }{
		{"author_context_identity", authorship.AuthorContextIdentity},
		{"authoring_tool_model_version", authorship.AuthoringToolModelVersion},
		{"returned_bundle_root", authorship.ReturnedBundleRoot},
	} {
		if strings.TrimSpace(field.value) == "" || strings.ContainsAny(field.value, "[]") {
			return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship %s is unresolved", authorSlot, field.name)
		}
	}
	started, err := time.Parse(time.RFC3339, authorship.AuthoringStartedUTC)
	if err != nil || !strings.HasSuffix(authorship.AuthoringStartedUTC, "Z") {
		return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship start is not RFC3339", authorSlot)
	}
	ended, err := time.Parse(time.RFC3339, authorship.AuthoringEndedUTC)
	if err != nil || !strings.HasSuffix(authorship.AuthoringEndedUTC, "Z") || ended.Before(started) {
		return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship end is invalid", authorSlot)
	}
	wantPacketManifest := "AUTHOR_" + strings.TrimPrefix(authorSlot, "author_") + "_PACKET.sha256"
	if authorship.PerAuthorPacketManifest != wantPacketManifest || authorship.PerAuthorPacketSHA256 != state.binding.AuthorPacketSHA256[authorSlot] ||
		authorship.ContractBindingSHA256 != state.binding.ContractBindingSHA256 || authorship.AssignmentSHA256 != assignmentSHA256 ||
		assignmentSHA256 != state.binding.AssignmentSHA256[authorSlot] {
		return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship binding is invalid", authorSlot)
	}
	if !validSHA256(authorship.PerAuthorPacketSHA256) || !validSHA256(authorship.ContractBindingSHA256) || !validSHA256(authorship.AssignmentSHA256) {
		return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship contains a malformed hash", authorSlot)
	}
	for _, uncertainty := range authorship.Uncertainties {
		if strings.TrimSpace(uncertainty) == "" || strings.ContainsAny(uncertainty, "[]") {
			return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship uncertainty is unresolved", authorSlot)
		}
	}
	if !authorship.Attestations.AllTrue() {
		return Authorship{}, time.Time{}, time.Time{}, fmt.Errorf("%s authorship attestations are incomplete", authorSlot)
	}
	return authorship, started, ended, nil
}

func (state *validationState) validateEntryMetadata(authorSlot string, entry AssignmentEntry) error {
	if entry.ID == "" || entry.SourceID == "" || !slices.Contains(state.policy.Roles, entry.Role) ||
		!slices.Contains(state.policy.Domains, entry.Domain) || !slices.Contains(state.policy.SafetyImpacts, entry.SafetyImpact) || !validRelativePath(entry.RequirementFile) {
		return fmt.Errorf("%s assignment contains invalid entry metadata", authorSlot)
	}
	if state.entriesByID[entry.ID] || state.sources[entry.SourceID] || state.files[entry.RequirementFile] {
		return fmt.Errorf("%s assignment duplicates a corpus identity or path", authorSlot)
	}
	state.entriesByID[entry.ID], state.sources[entry.SourceID], state.files[entry.RequirementFile] = true, true, true
	state.counts[entry.Role][entry.Domain]++
	state.report.Counts[entry.Role][entry.Domain]++
	return nil
}

func (state *validationState) validateRequirement(authorSlot string, entry AssignmentEntry, data []byte, rawHash string) error {
	for _, prefix := range state.policy.ProhibitedIdentityPrefixes {
		if bytes.Contains(data, []byte(prefix)) {
			return fmt.Errorf("%s leaks a prohibited manifest identity prefix", entry.ID)
		}
	}
	if prior := state.historical.RawSHA256[rawHash]; prior != "" {
		return fmt.Errorf("%s duplicates historical raw requirement %s", entry.ID, prior)
	}
	if prior := state.rawHashes[rawHash]; prior != "" {
		return fmt.Errorf("%s duplicates raw requirement %s", entry.ID, prior)
	}
	requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		return fmt.Errorf("%s violates the public requirement contract (%d issues)", entry.ID, len(issues))
	}
	if state.containsImplementationLanguage(requirement) {
		return fmt.Errorf("%s contains prohibited implementation language", entry.ID)
	}
	if len(requirement.Requirements.OperatingCases) < state.policy.MinimumOperatingCases || len(requirement.Requirements.BehavioralRequirements) < state.policy.MinimumAssertions {
		return fmt.Errorf("%s is below the operating-case or assertion minimum", entry.ID)
	}
	analyses := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		analyses[assertion.Analysis] = true
		if assertion.Metric == "thd" {
			return fmt.Errorf("%s uses prohibited legacy metric thd", entry.ID)
		}
	}
	if len(analyses) < state.policy.MinimumAnalysesPerRequirement {
		return fmt.Errorf("%s analysis kinds = %d, want at least %d", entry.ID, len(analyses), state.policy.MinimumAnalysesPerRequirement)
	}
	if !allAcceptanceGates(requirement.Acceptance) {
		return fmt.Errorf("%s does not require all acceptance gates", entry.ID)
	}
	neutralHash, normalizedHash, err := semanticHashes(requirement)
	if err != nil {
		return fmt.Errorf("%s semantic hashes: %w", entry.ID, err)
	}
	if prior := state.historical.NeutralSemanticSHA256[neutralHash]; prior != "" {
		return fmt.Errorf("%s duplicates historical semantic requirement %s", entry.ID, prior)
	}
	if prior := state.neutralHashes[neutralHash]; prior != "" {
		return fmt.Errorf("%s duplicates neutral semantic requirement %s", entry.ID, prior)
	}
	if prior := state.normalizedHashes[normalizedHash]; prior != "" {
		return fmt.Errorf("%s duplicates normalized semantic requirement %s", entry.ID, prior)
	}
	state.rawHashes[rawHash], state.neutralHashes[neutralHash], state.normalizedHashes[normalizedHash] = entry.ID, entry.ID, entry.ID
	state.roleDiversity[entry.Role].observe(entry.Domain, requirement)
	state.authorDiversity[authorSlot].observe(entry.Domain, requirement)
	portSignature, assertionSignature, analysisSignature := requirementSignatures(requirement)
	current := [3]string{portSignature, assertionSignature, analysisSignature}
	if state.recordStructuralSignature(authorSlot, entry.Domain, current) {
		return fmt.Errorf("%s repeats all structural signatures within %s/%s", entry.ID, authorSlot, entry.Domain)
	}
	state.report.Entries = append(state.report.Entries, EntryEvidence{
		ID: entry.ID, AuthorSlot: authorSlot, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact,
		SourceID: entry.SourceID, RequirementFile: entry.RequirementFile, RequirementSHA256: rawHash,
		NeutralSemanticSHA256: neutralHash, NormalizedSemanticSHA256: normalizedHash,
	})
	return nil
}

func (state *validationState) recordStructuralSignature(authorSlot, domain string, signature [3]string) bool {
	if state.domainSignatures[authorSlot][domain] == nil {
		state.domainSignatures[authorSlot][domain] = map[[3]string]bool{}
	}
	duplicate := state.domainSignatures[authorSlot][domain][signature]
	state.domainSignatures[authorSlot][domain][signature] = true
	return duplicate
}

func (state *validationState) validateAggregate() error {
	for _, role := range state.policy.Roles {
		for _, domain := range state.policy.Domains {
			want := len(state.policy.AuthorSlots) * state.policy.CasesPerAuthorRoleDomain
			if state.counts[role][domain] != want {
				return fmt.Errorf("aggregate %s/%s count = %d, want %d", role, domain, state.counts[role][domain], want)
			}
		}
		if err := validateRoleDiversity(role, state.roleDiversity[role], state.policy); err != nil {
			return err
		}
	}
	for _, author := range state.policy.AuthorSlots {
		evidence := state.authorDiversity[author]
		if len(evidence.analysisKinds) < state.policy.MinimumAnalysisKindsPerAuthor {
			return fmt.Errorf("%s analysis kinds = %d, want at least %d", author, len(evidence.analysisKinds), state.policy.MinimumAnalysisKindsPerAuthor)
		}
		if len(evidence.events) < state.policy.MinimumEventKindsPerAuthor {
			return fmt.Errorf("%s event kinds = %d, want at least %d", author, len(evidence.events), state.policy.MinimumEventKindsPerAuthor)
		}
	}
	return nil
}

func (state *validationState) containsImplementationLanguage(requirement ots.Requirement) bool {
	contains := func(values ...string) bool {
		for _, value := range values {
			if state.implementationText.MatchString(value) {
				return true
			}
		}
		return false
	}
	if contains(requirement.Project.Name, requirement.Project.Title, requirement.Project.Description) {
		return true
	}
	for _, domain := range requirement.Requirements.Domains {
		if contains(domain.ID, domain.Kind, domain.Source) {
			return true
		}
	}
	for _, port := range requirement.Requirements.Ports {
		if contains(port.ID, port.Kind, port.Direction, port.Domain, port.Electrical.DefaultState) {
			return true
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		if contains(operatingCase.ID) {
			return true
		}
		for _, condition := range operatingCase.Conditions {
			if contains(condition.Axis, condition.Target, condition.Unit) {
				return true
			}
		}
		for _, event := range operatingCase.Events {
			if contains(event.ID, event.Kind, event.Target, event.Unit) {
				return true
			}
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if contains(assertion.ID, assertion.Metric, assertion.Analysis, assertion.Unit, assertion.Observation.Kind, assertion.Observation.ID) ||
			assertion.Excitation != nil && contains(assertion.Excitation.Kind, assertion.Excitation.ID) || contains(assertion.OperatingCases...) {
			return true
		}
	}
	return false
}

func validatePolicy(policy Policy) error {
	if policy.AssignmentSchema == "" || policy.AuthorshipSchema == "" || policy.Version <= 0 || len(policy.AuthorSlots) == 0 || len(policy.Roles) == 0 || len(policy.Domains) == 0 || policy.CasesPerAuthorRoleDomain <= 0 {
		return fmt.Errorf("corpus-freeze policy is incomplete")
	}
	if len(policy.ProhibitedTerms) == 0 {
		return fmt.Errorf("corpus-freeze prohibited-language policy is empty")
	}
	for _, field := range []struct {
		name   string
		values []string
	}{
		{"author slots", policy.AuthorSlots}, {"roles", policy.Roles}, {"domains", policy.Domains},
		{"safety impacts", policy.SafetyImpacts}, {"prohibited terms", policy.ProhibitedTerms},
	} {
		if !uniqueNonempty(field.values) {
			return fmt.Errorf("corpus-freeze policy %s are empty or duplicated", field.name)
		}
	}
	return nil
}

func validateBinding(policy Policy, binding Binding, historical HistoricalCommitments) error {
	if !validSHA256(binding.ContractBindingSHA256) || len(binding.AuthorPacketSHA256) != len(policy.AuthorSlots) || len(binding.AssignmentSHA256) != len(policy.AuthorSlots) {
		return fmt.Errorf("corpus-freeze binding is incomplete")
	}
	for _, author := range policy.AuthorSlots {
		if !validSHA256(binding.AuthorPacketSHA256[author]) || !validSHA256(binding.AssignmentSHA256[author]) {
			return fmt.Errorf("corpus-freeze binding for %s is invalid", author)
		}
	}
	for _, group := range []struct {
		name        string
		commitments map[string]string
	}{
		{"raw", historical.RawSHA256}, {"semantic", historical.NeutralSemanticSHA256},
	} {
		digests := make([]string, 0, len(group.commitments))
		for digest := range group.commitments {
			digests = append(digests, digest)
		}
		sort.Strings(digests)
		for _, digest := range digests {
			id := group.commitments[digest]
			if !validSHA256(digest) || strings.TrimSpace(id) == "" {
				return fmt.Errorf("historical %s commitment is invalid", group.name)
			}
		}
	}
	return nil
}

func uniqueNonempty(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validRelativePath(value string) bool {
	return value != "" && value != "." && strings.TrimSpace(value) == value && !strings.ContainsAny(value, `\:`) &&
		!path.IsAbs(value) && path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../")
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func allAcceptanceGates(acceptance ots.Acceptance) bool {
	return acceptance.RequirePrimitiveOnly && acceptance.RequireTopologySearch && acceptance.RequireSimulation &&
		acceptance.RequireAllCorners && acceptance.RequireModelProvenance && acceptance.RequireClosedLoopEvidence &&
		acceptance.RequireCompleteRouting && acceptance.RequireConnectivity && acceptance.RequireWriterCorrectness &&
		acceptance.RequireRoundTripZeroDiff && acceptance.RequireERC && acceptance.RequireStrictDRC &&
		acceptance.RequireDeterministicReplay && acceptance.RequireFailClosed
}
