package componentonboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"index/suffixarray"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

var canonicalIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
var claimFieldPattern = regexp.MustCompile(`^[A-Za-z0-9]+(?:[._-][A-Za-z0-9]+)*$`)

func Onboard(
	ctx context.Context,
	requirement BehavioralRequirement,
	documents []DocumentInput,
	extractor Extractor,
	base *components.Catalog,
	libraries libraryresolver.LibraryIndex,
) (Candidate, error) {
	if err := ValidateRequirementAgainstCatalog(requirement, base); err != nil {
		return Candidate{}, err
	}
	if extractor == nil {
		return Candidate{}, fmt.Errorf("component onboarding requires an extractor")
	}
	documentRecords, documentContent, err := ingestDocuments(documents)
	if err != nil {
		return Candidate{}, err
	}
	extraction, err := extractor.Extract(ctx, requirement, cloneDocumentInputs(documents))
	if err != nil {
		return Candidate{}, fmt.Errorf("extract manufacturer evidence: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Candidate{}, err
	}
	if extraction.Schema != ExtractionSchema || len(extraction.Claims) == 0 || len(extraction.Claims) > MaxClaims ||
		len(extraction.Candidates) == 0 || len(extraction.Candidates) > MaxCandidates {
		return Candidate{}, fmt.Errorf("extractor returned an invalid bounded extraction")
	}
	claims, err := validateAndNormalizeClaims(extraction.Claims, documentRecords, documentContent)
	if err != nil {
		return Candidate{}, err
	}
	proposals := append([]ComponentProposal(nil), extraction.Candidates...)
	slices.SortStableFunc(proposals, func(left, right ComponentProposal) int {
		return strings.Compare(left.Record.ID, right.Record.ID)
	})
	seen := map[string]bool{}
	scores := make([]CandidateScore, 0, len(proposals))
	for index := range proposals {
		if err := ctx.Err(); err != nil {
			return Candidate{}, err
		}
		proposal := &proposals[index]
		if proposal.Record.ID == "" || seen[proposal.Record.ID] {
			return Candidate{}, fmt.Errorf("extraction has empty or duplicate component id %q", proposal.Record.ID)
		}
		seen[proposal.Record.ID] = true
		normalizeProposal(proposal)
		if err := validateProposal(requirement, *proposal, claims, documentRecords, base, libraries); err != nil {
			return Candidate{}, fmt.Errorf("candidate %q: %w", proposal.Record.ID, err)
		}
		score, err := scoreProposal(requirement, *proposal)
		if err != nil {
			return Candidate{}, fmt.Errorf("candidate %q: %w", proposal.Record.ID, err)
		}
		scores = append(scores, score)
	}
	if err := validateProposalSelections(requirement, proposals, base); err != nil {
		return Candidate{}, err
	}
	rankScores(scores)
	candidate := Candidate{
		Schema: CandidateSchema, PolicyVersion: PolicyVersion, Status: StatusQuarantined,
		Requirement: requirement, Documents: documentRecords, Claims: claims,
		Proposals: proposals, Ranking: scores, SelectedID: scores[0].ComponentID,
	}
	candidate.Hash, err = hashWithoutField(candidate)
	if err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

func ValidateRequirement(requirement BehavioralRequirement) error {
	if requirement.Schema != RequestSchema || !canonicalIDPattern.MatchString(requirement.ID) ||
		!canonicalIDPattern.MatchString(requirement.Family) {
		return fmt.Errorf("requirement must use schema %s and canonical identity", RequestSchema)
	}
	if len(requirement.RequiredFunctions) == 0 || len(requirement.RequiredRatings) == 0 ||
		len(requirement.RequiredAnalyses) == 0 {
		return fmt.Errorf("requirement needs functions, ratings, and analyses")
	}
	if requirement.RequiredTemperature.MinimumC == nil || requirement.RequiredTemperature.MaximumC == nil ||
		*requirement.RequiredTemperature.MinimumC >= *requirement.RequiredTemperature.MaximumC {
		return fmt.Errorf("requirement temperature interval must be ordered")
	}
	if !finite(requirement.MinimumDerating) || requirement.MinimumDerating < 1 {
		return fmt.Errorf("minimum derating ratio must be finite and at least one")
	}
	if err := canonicalUnique(requirement.RequiredFunctions, "required functions"); err != nil {
		return err
	}
	if err := canonicalUnique(requirement.RequiredAnalyses, "required analyses"); err != nil {
		return err
	}
	if err := canonicalUnique(requirement.AllowedPackages, "allowed packages"); err != nil {
		return err
	}
	for _, rating := range requirement.RequiredRatings {
		if strings.TrimSpace(rating.Kind) == "" || strings.TrimSpace(rating.Unit) == "" ||
			strings.TrimSpace(rating.Value) == "" {
			return fmt.Errorf("required ratings must have positive finite values and units")
		}
		value, ok := components.ParseEngineeringValue(rating.Value)
		if !ok || !finite(value) || value <= 0 {
			return fmt.Errorf("required rating %q has an invalid value", rating.Kind)
		}
	}
	for _, analysis := range requirement.RequiredAnalyses {
		if !slices.Contains(simmodel.SupportedAnalysisKinds(simmodel.ModelLinearCircuitMNAV1), analysis) &&
			!registeredAnalysis(analysis) {
			return fmt.Errorf("required analysis %q is not registered", analysis)
		}
	}
	return nil
}

func ValidateRequirementAgainstCatalog(requirement BehavioralRequirement, catalog *components.Catalog) error {
	if err := ValidateRequirement(requirement); err != nil {
		return err
	}
	if catalog == nil {
		return fmt.Errorf("base component catalog is required")
	}
	if !slices.ContainsFunc(catalog.Families, func(family components.FamilyDefinition) bool {
		return family.ID == requirement.Family
	}) {
		return fmt.Errorf("required family %q is not registered", requirement.Family)
	}
	return nil
}

func ingestDocuments(inputs []DocumentInput) ([]DocumentRecord, map[string][]byte, error) {
	if len(inputs) == 0 || len(inputs) > MaxDocuments {
		return nil, nil, fmt.Errorf("onboarding requires a bounded nonempty document set")
	}
	records := make([]DocumentRecord, 0, len(inputs))
	content := make(map[string][]byte, len(inputs))
	var total int64
	for _, input := range inputs {
		input.ID = strings.TrimSpace(input.ID)
		input.Publisher = strings.TrimSpace(input.Publisher)
		input.Locator = strings.TrimSpace(input.Locator)
		input.Revision = strings.TrimSpace(input.Revision)
		input.License = strings.TrimSpace(input.License)
		input.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(input.ExpectedSHA256))
		if !canonicalIDPattern.MatchString(input.ID) || !validDocumentKind(input.Kind) ||
			input.Publisher == "" || input.Locator == "" || input.Revision == "" ||
			len(input.Content) == 0 || len(input.Content) > MaxDocumentBytes {
			return nil, nil, fmt.Errorf("document %q is incomplete or outside ingestion bounds", input.ID)
		}
		if err := validateLocator(input.Locator); err != nil {
			return nil, nil, fmt.Errorf("document %q: %w", input.ID, err)
		}
		if input.Kind == DocumentModel && input.License == "" {
			return nil, nil, fmt.Errorf("model document %q requires license provenance", input.ID)
		}
		if _, duplicate := content[input.ID]; duplicate {
			return nil, nil, fmt.Errorf("document %q is duplicated", input.ID)
		}
		total += int64(len(input.Content))
		if total > MaxDocumentSetBytes {
			return nil, nil, fmt.Errorf("document set exceeds aggregate byte bound")
		}
		sum := sha256.Sum256(input.Content)
		actual := hex.EncodeToString(sum[:])
		if actual != input.ExpectedSHA256 {
			return nil, nil, fmt.Errorf("document %q digest mismatch", input.ID)
		}
		content[input.ID] = append([]byte(nil), input.Content...)
		records = append(records, DocumentRecord{
			ID: input.ID, Kind: input.Kind, Publisher: input.Publisher,
			Locator: input.Locator, Revision: input.Revision, License: input.License,
			SHA256: actual, Bytes: len(input.Content),
		})
	}
	slices.SortStableFunc(records, func(left, right DocumentRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	return records, content, nil
}

func validateAndNormalizeClaims(
	inputs []Claim,
	documents []DocumentRecord,
	content map[string][]byte,
) ([]Claim, error) {
	documentByID := make(map[string]DocumentRecord, len(documents))
	documentIndexByID := make(map[string]*suffixarray.Index, len(documents))
	for _, document := range documents {
		documentByID[document.ID] = document
	}
	claims := append([]Claim(nil), inputs...)
	for index := range claims {
		claim := &claims[index]
		claim.ID = strings.TrimSpace(claim.ID)
		claim.DocumentID = strings.TrimSpace(claim.DocumentID)
		claim.Subject = strings.TrimSpace(claim.Subject)
		claim.Field = strings.TrimSpace(claim.Field)
		claim.Relation = strings.TrimSpace(claim.Relation)
		claim.Value = strings.TrimSpace(claim.Value)
		claim.Unit = strings.TrimSpace(claim.Unit)
		claim.Excerpt = strings.TrimSpace(claim.Excerpt)
		claim.Location = strings.TrimSpace(claim.Location)
		if !canonicalIDPattern.MatchString(claim.ID) || claim.Subject == "" ||
			!claimFieldPattern.MatchString(claim.Field) || claim.Value == "" ||
			claim.Excerpt == "" || claim.Location == "" {
			return nil, fmt.Errorf("claim %q is incomplete", claim.ID)
		}
		if _, ok := documentByID[claim.DocumentID]; !ok {
			return nil, fmt.Errorf("claim %q references unknown document %q", claim.ID, claim.DocumentID)
		}
		documentIndex, ok := documentIndexByID[claim.DocumentID]
		if !ok {
			documentIndex = suffixarray.New(content[claim.DocumentID])
			documentIndexByID[claim.DocumentID] = documentIndex
		}
		if len(documentIndex.Lookup([]byte(claim.Excerpt), 1)) == 0 {
			return nil, fmt.Errorf("claim %q excerpt is absent from immutable source document", claim.ID)
		}
		if !claimValueAnchored(*claim) {
			return nil, fmt.Errorf("claim %q value is not anchored by its source excerpt", claim.ID)
		}
	}
	slices.SortStableFunc(claims, func(left, right Claim) int {
		return strings.Compare(left.ID, right.ID)
	})
	seenID := map[string]bool{}
	seenField := map[string]Claim{}
	for _, claim := range claims {
		if seenID[claim.ID] {
			return nil, fmt.Errorf("claim %q is duplicated", claim.ID)
		}
		seenID[claim.ID] = true
		key := strings.ToLower(claim.Subject) + "\x00" + claim.Field
		if previous, ok := seenField[key]; ok &&
			!strings.HasPrefix(claim.Field, "pin.") &&
			(previous.Relation != claim.Relation || previous.Value != claim.Value || previous.Unit != claim.Unit) {
			return nil, fmt.Errorf("claims %q and %q conflict for %s", previous.ID, claim.ID, claim.Field)
		}
		seenField[key] = claim
	}
	return claims, nil
}

func validateProposal(
	requirement BehavioralRequirement,
	proposal ComponentProposal,
	claims []Claim,
	documents []DocumentRecord,
	base *components.Catalog,
	libraries libraryresolver.LibraryIndex,
) error {
	record := proposal.Record
	if record.ID == "" || record.Family != requirement.Family || record.Generic ||
		strings.TrimSpace(record.Manufacturer) == "" || strings.TrimSpace(record.MPN) == "" {
		return fmt.Errorf("record must be a concrete member of required family %q", requirement.Family)
	}
	documentByID := make(map[string]DocumentRecord, len(documents))
	for _, document := range documents {
		documentByID[document.ID] = document
	}
	if slices.ContainsFunc(base.Records, func(existing components.ComponentRecord) bool {
		return existing.ID == record.ID ||
			(strings.EqualFold(existing.Manufacturer, record.Manufacturer) && strings.EqualFold(existing.MPN, record.MPN))
	}) {
		return fmt.Errorf("record is already present in the base catalog")
	}
	claimByID := make(map[string]Claim, len(claims))
	for _, claim := range claims {
		claimByID[claim.ID] = claim
	}
	requiredCategories := []string{"derating", "identity", "model", "package", "pin_mapping", "provenance", "ratings", "temperature"}
	categoryClaims := map[string][]Claim{}
	seenBinding := map[string]bool{}
	for _, binding := range proposal.Evidence {
		if !slices.Contains(requiredCategories, binding.Path) || seenBinding[binding.Path] || len(binding.ClaimIDs) == 0 {
			return fmt.Errorf("evidence binding %q is unknown, duplicated, or empty", binding.Path)
		}
		seenBinding[binding.Path] = true
		for _, claimID := range binding.ClaimIDs {
			claim, ok := claimByID[claimID]
			if !ok || !strings.EqualFold(claim.Subject, record.MPN) {
				return fmt.Errorf("evidence binding %q references absent or wrong-subject claim %q", binding.Path, claimID)
			}
			categoryClaims[binding.Path] = append(categoryClaims[binding.Path], claim)
		}
	}
	for _, category := range requiredCategories {
		if !seenBinding[category] {
			return fmt.Errorf("missing %s evidence", category)
		}
	}
	if err := validateIdentityClaims(record, categoryClaims["identity"]); err != nil {
		return err
	}
	if err := validateManufacturerDocuments(record, categoryClaims["identity"], documentByID); err != nil {
		return err
	}
	if err := validateRatingClaims(record, requirement, categoryClaims["ratings"]); err != nil {
		return err
	}
	if err := validateTemperatureClaims(record, requirement, categoryClaims["temperature"]); err != nil {
		return err
	}
	if err := validatePinAndPackageClaims(record, requirement, categoryClaims, libraries); err != nil {
		return err
	}
	if len(record.DeratingRules) == 0 {
		return fmt.Errorf("record lacks derating rules")
	}
	for _, rule := range record.DeratingRules {
		if !claimHasValue(categoryClaims["derating"], "derating."+rule.Kind, rule.Expression, "") {
			return fmt.Errorf("derating rule %q lacks matching source claim", rule.Kind)
		}
	}
	if err := validateModelProposal(record, requirement, proposal.Model, categoryClaims["model"], documentByID); err != nil {
		return err
	}
	if err := validateVerificationSources(record, documentByID, categoryClaims["provenance"]); err != nil {
		return err
	}
	return nil
}

func validateProposalSelections(
	requirement BehavioralRequirement,
	proposals []ComponentProposal,
	base *components.Catalog,
) error {
	records := make([]components.ComponentRecord, 0, len(proposals))
	for _, proposal := range proposals {
		records = append(records, proposal.Record)
	}
	merged, err := mergeCatalog(base, records)
	if err != nil {
		return err
	}
	for _, proposal := range proposals {
		record := proposal.Record
		selection, result := components.Select(context.Background(), merged, components.SelectionRequest{
			Query:      components.Query{Text: record.ID, Family: requirement.Family},
			Acceptance: components.AcceptanceERCDRC, RequiredRatings: requirement.RequiredRatings,
			RequiredTemperature: &requirement.RequiredTemperature, RequiredFunctions: requirement.RequiredFunctions,
			RequireConcrete: true,
		})
		if !result.OK || selection.Component.ID != record.ID {
			return fmt.Errorf("candidate %q: %w", record.ID, firstSelectionError(result))
		}
	}
	return nil
}

func validateIdentityClaims(record components.ComponentRecord, claims []Claim) error {
	required := map[string]string{
		"identity.family":       record.Family,
		"identity.manufacturer": record.Manufacturer,
		"identity.mpn":          record.MPN,
	}
	for field, value := range required {
		if !claimHasValue(claims, field, value, "") {
			return fmt.Errorf("%s lacks matching source claim", field)
		}
	}
	return nil
}

func validateRatingClaims(record components.ComponentRecord, requirement BehavioralRequirement, claims []Claim) error {
	for _, required := range requirement.RequiredRatings {
		ratingIndex := slices.IndexFunc(record.Ratings, func(value components.RatingConstraint) bool {
			return value.Kind == required.Kind
		})
		if ratingIndex < 0 {
			return fmt.Errorf("required rating %q is absent", required.Kind)
		}
		if !ratingConstraintClaimed(record.Ratings[ratingIndex], claims) {
			return fmt.Errorf("rating %q lacks matching source claim", required.Kind)
		}
	}
	return nil
}

func ratingConstraintClaimed(rating components.RatingConstraint, claims []Claim) bool {
	for suffix, value := range map[string]string{"min": rating.Min, "typ": rating.Typ, "max": rating.Max} {
		if value != "" && claimHasValue(claims, "rating."+rating.Kind+"."+suffix, value, rating.Unit) {
			return true
		}
	}
	return false
}

func validateTemperatureClaims(
	record components.ComponentRecord,
	requirement BehavioralRequirement,
	claims []Claim,
) error {
	if record.Temperature == nil {
		return fmt.Errorf("record lacks temperature evidence")
	}
	minimum, minOK := components.ParseEngineeringValue(record.Temperature.Min)
	maximum, maxOK := components.ParseEngineeringValue(record.Temperature.Max)
	if !minOK || !maxOK || minimum > *requirement.RequiredTemperature.MinimumC ||
		maximum < *requirement.RequiredTemperature.MaximumC {
		return fmt.Errorf("record temperature evidence does not cover requirement")
	}
	if !claimHasValue(claims, "temperature.min", record.Temperature.Min, record.Temperature.Unit) ||
		!claimHasValue(claims, "temperature.max", record.Temperature.Max, record.Temperature.Unit) {
		return fmt.Errorf("temperature range lacks matching source claims")
	}
	return nil
}

func validatePinAndPackageClaims(
	record components.ComponentRecord,
	requirement BehavioralRequirement,
	categoryClaims map[string][]Claim,
	libraries libraryresolver.LibraryIndex,
) error {
	if err := validateRecordLibraryBindings(record, libraries); err != nil {
		return err
	}
	for _, variant := range record.Packages {
		if len(requirement.AllowedPackages) != 0 && !slices.Contains(requirement.AllowedPackages, variant.PackageType) {
			continue
		}
		if !claimHasValue(categoryClaims["package"], "package.footprint", variant.FootprintID, "") {
			return fmt.Errorf("footprint %q lacks matching source claim", variant.FootprintID)
		}
		for _, function := range requirement.RequiredFunctions {
			if err := validateFunctionPinClaims(
				record.Symbols,
				variant.PadFunctions,
				function,
				categoryClaims["pin_mapping"],
			); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("no package satisfies the allowed package set")
}

func validateRecordLibraryBindings(record components.ComponentRecord, libraries libraryresolver.LibraryIndex) error {
	if len(record.Symbols) == 0 || len(record.Packages) == 0 {
		return fmt.Errorf("record lacks symbol or package binding")
	}
	for _, symbol := range record.Symbols {
		librarySymbol, ok := libraries.Symbols[symbol.SymbolID]
		if !ok {
			return fmt.Errorf("KiCad symbol %q is unavailable", symbol.SymbolID)
		}
		for _, pin := range symbol.FunctionPins {
			if !slices.ContainsFunc(librarySymbol.Pins, func(candidate libraryresolver.SymbolPin) bool {
				return candidate.Number == pin.SymbolPin
			}) {
				return fmt.Errorf("symbol %q lacks pin %q", symbol.SymbolID, pin.SymbolPin)
			}
		}
	}
	for _, variant := range record.Packages {
		footprint, ok := libraries.Footprints[variant.FootprintID]
		if !ok {
			return fmt.Errorf("KiCad footprint %q is unavailable", variant.FootprintID)
		}
		for _, function := range variant.PadFunctions {
			if !slices.ContainsFunc(footprint.Pads, func(candidate libraryresolver.FootprintPad) bool {
				return candidate.Name == function.Pad
			}) {
				return fmt.Errorf("footprint %q lacks pad %q", variant.FootprintID, function.Pad)
			}
		}
	}
	return nil
}

func validateFunctionPinClaims(
	symbols []components.SymbolBinding,
	padFunctions []components.PadFunction,
	function string,
	claims []Claim,
) error {
	symbolPins := functionSymbolPins(symbols, function)
	pads := functionPads(padFunctions, function)
	if len(symbolPins) == 0 || len(pads) == 0 {
		return fmt.Errorf("required function %q lacks symbol pins or package pads", function)
	}
	allowedPins := make(map[string]struct{}, len(symbolPins))
	allowedPads := make(map[string]struct{}, len(pads))
	for _, pin := range symbolPins {
		allowedPins[pin] = struct{}{}
	}
	for _, pad := range pads {
		allowedPads[pad] = struct{}{}
	}
	seenPins := make(map[string]struct{}, len(symbolPins))
	seenPads := make(map[string]struct{}, len(pads))
	seenMappings := make(map[string]struct{}, len(symbolPins)+len(pads))
	field := "pin." + function
	for _, claim := range claims {
		if claim.Field != field {
			continue
		}
		mapping := strings.Split(claim.Value, ":")
		if len(mapping) != 2 {
			return fmt.Errorf("function %q pin-map claim %q is malformed", function, claim.ID)
		}
		if _, ok := allowedPins[mapping[0]]; !ok {
			return fmt.Errorf("function %q claim references unknown symbol pin %q", function, mapping[0])
		}
		if _, ok := allowedPads[mapping[1]]; !ok {
			return fmt.Errorf("function %q claim references unknown package pad %q", function, mapping[1])
		}
		mappingKey := mapping[0] + "\x00" + mapping[1]
		if _, duplicate := seenMappings[mappingKey]; duplicate {
			return fmt.Errorf("function %q repeats mapping %q", function, claim.Value)
		}
		seenMappings[mappingKey] = struct{}{}
		seenPins[mapping[0]] = struct{}{}
		seenPads[mapping[1]] = struct{}{}
	}
	if len(seenPins) != len(symbolPins) || len(seenPads) != len(pads) {
		return fmt.Errorf("function %q pin map lacks source claims for every symbol pin and package pad", function)
	}
	return nil
}

func validateModelProposal(
	record components.ComponentRecord,
	requirement BehavioralRequirement,
	proposal ModelProposal,
	claims []Claim,
	documents map[string]DocumentRecord,
) error {
	if proposal.Kind != ModelManufacturer && proposal.Kind != ModelBounded {
		return fmt.Errorf("model kind is unsupported")
	}
	if proposal.ModelID == "" || proposal.Provenance.CatalogID != record.ID ||
		proposal.Provenance.Family != record.Family || proposal.Provenance.ModelID != proposal.ModelID {
		return fmt.Errorf("model proposal identity does not match component")
	}
	if !claimHasValue(claims, "model.id", proposal.ModelID, "") {
		return fmt.Errorf("model id lacks matching source claim")
	}
	if proposal.Kind == ModelBounded && len(proposal.BoundedAssumptions) == 0 {
		return fmt.Errorf("bounded analytic substitute requires explicit assumptions")
	}
	if len(proposal.ClaimIDs) == 0 {
		return fmt.Errorf("model proposal lacks evidence claims")
	}
	for _, claimID := range proposal.ClaimIDs {
		if !slices.ContainsFunc(claims, func(claim Claim) bool { return claim.ID == claimID }) {
			return fmt.Errorf("model proposal references unrelated claim %q", claimID)
		}
	}
	modelHash, ok := simmodel.ModelContentHash(proposal.ModelID)
	if !ok || proposal.Provenance.Provenance.SHA256 != modelHash {
		return fmt.Errorf("model provenance does not reference the canonical trusted model definition")
	}
	for _, analysis := range requirement.RequiredAnalyses {
		if !simmodel.SupportsCatalogAnalysis(proposal.ModelID, analysis) {
			return fmt.Errorf("model %q does not support required analysis %q", proposal.ModelID, analysis)
		}
	}
	if diagnostics := simmodel.ValidateRequiredModelProvenance(&proposal.Provenance.Provenance, requirement.RequiredAnalyses); len(diagnostics) != 0 {
		return fmt.Errorf("model provenance: %s", diagnostics[0].Message)
	}
	if diagnostics := simmodel.ValidateCatalogEvidence(record.Family, record.SimulationModels); len(diagnostics) != 0 {
		return fmt.Errorf("catalog model evidence: %s", diagnostics[0].Message)
	}
	sourceID := strings.TrimPrefix(proposal.Provenance.Provenance.Source, "document:")
	document, found := documents[sourceID]
	if !found {
		return fmt.Errorf("model provenance source must reference an ingested document")
	}
	if proposal.Kind == ModelManufacturer && document.Kind != DocumentModel {
		return fmt.Errorf("manufacturer model must reference a licensed model document")
	}
	return nil
}

func validateVerificationSources(record components.ComponentRecord, documents map[string]DocumentRecord, claims []Claim) error {
	if record.Verification.Confidence != components.ConfidenceVerified ||
		!record.Verification.ResolverChecked || !record.Verification.PinMapChecked ||
		len(record.Verification.Sources) == 0 {
		return fmt.Errorf("record verification is incomplete")
	}
	expected := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		expected["document:"+document.ID+"@sha256:"+document.SHA256] = struct{}{}
	}
	for _, source := range record.Verification.Sources {
		if _, ok := expected[source]; !ok {
			return fmt.Errorf("verification source %q is not content-addressed to an ingested document", source)
		}
	}
	if len(claims) == 0 {
		return fmt.Errorf("provenance evidence is absent")
	}
	if !slices.ContainsFunc(claims, func(claim Claim) bool {
		document, found := documents[claim.DocumentID]
		return found && claim.Field == "provenance.revision" && claim.Value == document.Revision
	}) {
		return fmt.Errorf("document revision lacks matching provenance claim")
	}
	return nil
}

func validateManufacturerDocuments(record components.ComponentRecord, claims []Claim, documents map[string]DocumentRecord) error {
	for _, claim := range claims {
		document, found := documents[claim.DocumentID]
		if !found {
			continue
		}
		if document.Kind == DocumentDatasheet || document.Kind == DocumentPackage || document.Kind == DocumentModel {
			if !strings.EqualFold(document.Publisher, record.Manufacturer) {
				return fmt.Errorf("manufacturer document publisher %q does not match record manufacturer %q", document.Publisher, record.Manufacturer)
			}
			return nil
		}
	}
	return fmt.Errorf("identity evidence is not backed by manufacturer documentation")
}

func mergeCatalog(base *components.Catalog, records []components.ComponentRecord) (*components.Catalog, error) {
	generatedAt := base.GeneratedAt
	if base.GeneratedAt != nil {
		value := *base.GeneratedAt
		generatedAt = &value
	}
	merged := components.Catalog{
		Version:      base.Version,
		GeneratedAt:  generatedAt,
		Records:      append([]components.ComponentRecord(nil), base.Records...),
		Families:     append([]components.FamilyDefinition(nil), base.Families...),
		ThermalPaths: append([]components.ThermalPathRecord(nil), base.ThermalPaths...),
		Diagnostics:  append([]reports.Issue(nil), base.Diagnostics...),
	}
	merged.Records = append(merged.Records, records...)
	slices.SortStableFunc(merged.Records, func(left, right components.ComponentRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	components.RebuildCatalogIndexes(&merged)
	result := components.ValidateCatalog(&merged)
	if !result.OK {
		return nil, firstReportError(result)
	}
	return &merged, nil
}

func firstSelectionError(result reports.Result) error {
	if len(result.Issues) == 0 {
		return fmt.Errorf("component was not deterministically selected")
	}
	return fmt.Errorf("component selection failed: %s: %s", result.Issues[0].Path, result.Issues[0].Message)
}

func firstReportError(result reports.Result) error {
	if len(result.Issues) == 0 {
		return fmt.Errorf("catalog validation failed")
	}
	return fmt.Errorf("catalog validation failed: %s: %s", result.Issues[0].Path, result.Issues[0].Message)
}

func normalizeProposal(proposal *ComponentProposal) {
	slices.SortStableFunc(proposal.Evidence, func(left, right EvidenceBinding) int {
		return strings.Compare(left.Path, right.Path)
	})
	for index := range proposal.Evidence {
		slices.Sort(proposal.Evidence[index].ClaimIDs)
	}
	slices.Sort(proposal.Model.ClaimIDs)
	slices.Sort(proposal.Model.BoundedAssumptions)
}

func scoreProposal(requirement BehavioralRequirement, proposal ComponentProposal) (CandidateScore, error) {
	margin := math.Inf(1)
	for _, required := range requirement.RequiredRatings {
		index := slices.IndexFunc(proposal.Record.Ratings, func(rating components.RatingConstraint) bool {
			return rating.Kind == required.Kind
		})
		if index < 0 {
			return CandidateScore{}, fmt.Errorf("required rating %q is absent", required.Kind)
		}
		value := proposal.Record.Ratings[index].Max
		if value == "" {
			value = proposal.Record.Ratings[index].Typ
		}
		if value == "" {
			value = proposal.Record.Ratings[index].Min
		}
		numeric, ok := components.ParseEngineeringValue(value)
		if !ok {
			return CandidateScore{}, fmt.Errorf("rating %q is not numeric", required.Kind)
		}
		requiredNumeric, ok := components.ParseEngineeringValue(required.Value)
		if !ok || requiredNumeric <= 0 {
			return CandidateScore{}, fmt.Errorf("required rating %q is not numeric", required.Kind)
		}
		margin = math.Min(margin, numeric/requiredNumeric)
	}
	if !finite(margin) || margin < requirement.MinimumDerating {
		return CandidateScore{}, fmt.Errorf("minimum rating margin %.6g is below required %.6g", margin, requirement.MinimumDerating)
	}
	variantID := ""
	if len(proposal.Record.Packages) != 0 {
		variantID = proposal.Record.Packages[0].ID
	}
	coverage := 0
	for _, binding := range proposal.Evidence {
		coverage += len(binding.ClaimIDs)
	}
	return CandidateScore{
		ComponentID: proposal.Record.ID, VariantID: variantID,
		MinimumMargin: margin, EvidenceCoverage: coverage,
	}, nil
}

func rankScores(scores []CandidateScore) {
	slices.SortStableFunc(scores, func(left, right CandidateScore) int {
		if left.MinimumMargin != right.MinimumMargin {
			if left.MinimumMargin > right.MinimumMargin {
				return -1
			}
			return 1
		}
		if left.EvidenceCoverage != right.EvidenceCoverage {
			return right.EvidenceCoverage - left.EvidenceCoverage
		}
		if order := strings.Compare(left.ComponentID, right.ComponentID); order != 0 {
			return order
		}
		return strings.Compare(left.VariantID, right.VariantID)
	})
	for index := range scores {
		scores[index].Rank = index + 1
	}
}

func claimHasValue(claims []Claim, field, value, unit string) bool {
	return slices.ContainsFunc(claims, func(claim Claim) bool {
		return claim.Field == field && claim.Value == value && claim.Unit == unit
	})
}

func claimValueAnchored(claim Claim) bool {
	excerpt := strings.ToLower(claim.Excerpt)
	value := strings.ToLower(claim.Value)
	if strings.HasPrefix(claim.Field, "pin.") {
		parts := strings.Split(claim.Value, ":")
		if len(parts) != 2 {
			return false
		}
		pinCount := anchoredTokenCount(excerpt, strings.ToLower(parts[0]))
		padCount := anchoredTokenCount(excerpt, strings.ToLower(parts[1]))
		if strings.EqualFold(parts[0], parts[1]) {
			return pinCount >= 2
		}
		return pinCount >= 1 && padCount >= 1
	}
	if anchoredTokenCount(excerpt, value) == 0 {
		return false
	}
	return claim.Unit == "" || anchoredTokenCount(excerpt, strings.ToLower(claim.Unit)) != 0
}

func anchoredTokenCount(text, token string) int {
	if token == "" {
		return 0
	}
	count := 0
	offset := 0
	for offset <= len(text)-len(token) {
		relative := strings.Index(text[offset:], token)
		if relative < 0 {
			break
		}
		start := offset + relative
		end := start + len(token)
		beforeBoundary := start == 0
		if !beforeBoundary {
			before, _ := utf8.DecodeLastRuneInString(text[:start])
			beforeBoundary = !claimTokenRune(before)
		}
		afterBoundary := end == len(text)
		if !afterBoundary {
			after, _ := utf8.DecodeRuneInString(text[end:])
			afterBoundary = !claimTokenRune(after)
		}
		if beforeBoundary && afterBoundary {
			count++
		}
		offset = end
	}
	return count
}

func claimTokenRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsNumber(value) ||
		value == '_' || value == '.' || value == '-'
}

func functionSymbolPins(symbols []components.SymbolBinding, function string) []string {
	var result []string
	for _, symbol := range symbols {
		for _, pin := range symbol.FunctionPins {
			if pin.Function == function {
				result = append(result, pin.SymbolPin)
			}
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func functionPads(functions []components.PadFunction, function string) []string {
	var result []string
	for _, pad := range functions {
		if pad.Function == function {
			result = append(result, pad.Pad)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func hashWithoutField(value any) (string, error) {
	var payload any = value
	// The explicit nil hash field shadows only the top-level artifact hash.
	// Nested hashes remain part of the canonical approval chain.
	switch typed := value.(type) {
	case Candidate:
		type alias Candidate
		payload = struct {
			alias
			Hash *struct{} `json:"hash,omitempty"`
		}{alias: alias(typed)}
	case Promotion:
		type alias Promotion
		payload = struct {
			alias
			Hash *struct{} `json:"hash,omitempty"`
		}{alias: alias(typed)}
	case SupportedOverlay:
		type alias SupportedOverlay
		payload = struct {
			alias
			Hash *struct{} `json:"hash,omitempty"`
		}{alias: alias(typed)}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalUnique(values []string, label string) error {
	if !slices.IsSorted(values) {
		return fmt.Errorf("%s must be canonically sorted", label)
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" || index != 0 && value == values[index-1] {
			return fmt.Errorf("%s must contain unique nonempty values", label)
		}
	}
	return nil
}

func validDocumentKind(kind DocumentKind) bool {
	return kind == DocumentDatasheet || kind == DocumentModel ||
		kind == DocumentLibrary || kind == DocumentPackage
}

func validateLocator(locator string) error {
	if strings.HasPrefix(locator, "kicadai:") {
		identity := strings.TrimPrefix(locator, "kicadai:")
		if identity == "" || strings.TrimSpace(identity) != identity ||
			strings.ContainsAny(identity, " \t\r\n") {
			return fmt.Errorf("kicadai locator requires a stable internal identity")
		}
		return nil
	}
	parsed, err := url.Parse(locator)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("locator must use publisher HTTPS or a stable kicadai identity")
	}
	return nil
}

func cloneDocumentInputs(inputs []DocumentInput) []DocumentInput {
	cloned := append([]DocumentInput(nil), inputs...)
	for index := range cloned {
		cloned[index].Content = append([]byte(nil), inputs[index].Content...)
	}
	return cloned
}

func registeredAnalysis(analysis string) bool {
	for _, modelID := range simmodel.ModelIDs() {
		if slices.Contains(simmodel.SupportedAnalysisKinds(modelID), analysis) {
			return true
		}
	}
	return false
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
