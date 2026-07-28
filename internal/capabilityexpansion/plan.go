package capabilityexpansion

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/capabilitygate"
)

var commonPromotionGates = []string{
	"deterministic_replay",
	"simulation",
	"workflow",
	"kicad_erc",
	"kicad_drc",
	"connectivity",
	"route_completion",
	"writer_correctness",
	"round_trip_zero_diff",
}

// Plan converts a fail-closed assessment into reusable expansion needs.
func Plan(assessment capabilitygate.Assessment) (ExpansionPlan, error) {
	if err := capabilitygate.Validate(assessment); err != nil {
		return ExpansionPlan{}, fmt.Errorf("validate source assessment: %w", err)
	}
	if assessment.Classification != capabilitygate.ClassificationUnsupported {
		return ExpansionPlan{}, fmt.Errorf("capability expansion requires an unsupported assessment")
	}
	if len(assessment.Gaps) == 0 {
		return ExpansionPlan{}, fmt.Errorf("unsupported assessment has no actionable gaps")
	}
	if len(assessment.Gaps) > MaxExpansionNeeds {
		return ExpansionPlan{}, fmt.Errorf("expansion assessment exceeds %d-gap limit", MaxExpansionNeeds)
	}
	byKey := map[string]ExpansionNeed{}
	for _, gap := range assessment.Gaps {
		kind := needKindForGap(gap)
		capabilityID := canonicalID(firstNonEmpty(gap.ID, string(gap.Kind)))
		if capabilityID == "" {
			return ExpansionPlan{}, fmt.Errorf("gap %q has no reusable capability identity", gap.Code)
		}
		key := string(kind) + ":" + capabilityID
		need := byKey[key]
		if need.ID == "" {
			need = ExpansionNeed{
				ID:                     key,
				CapabilityID:           capabilityID,
				Kind:                   kind,
				RequirementKind:        gap.Kind,
				Stage:                  strings.TrimSpace(gap.Stage),
				RequiredSourceKinds:    sourceKindsForNeed(kind),
				RequiredArtifact:       artifactForNeed(kind),
				RequiredPromotionGates: append([]string(nil), commonPromotionGates...),
				Action:                 firstNonEmpty(strings.TrimSpace(gap.Action), defaultActionForNeed(kind)),
			}
		}
		need.GapCodes = append(need.GapCodes, strings.TrimSpace(gap.Code))
		byKey[key] = need
	}
	needs := make([]ExpansionNeed, 0, len(byKey))
	for _, need := range byKey {
		need.GapCodes = normalizedStrings(need.GapCodes)
		need.RequiredSourceKinds = normalizedSourceKinds(need.RequiredSourceKinds)
		need.RequiredPromotionGates = normalizedStrings(need.RequiredPromotionGates)
		needs = append(needs, need)
	}
	slices.SortStableFunc(needs, func(left, right ExpansionNeed) int {
		return strings.Compare(left.ID, right.ID)
	})
	plan := ExpansionPlan{
		Schema: PlanSchema, PolicyVersion: PolicyVersion,
		SourceAssessmentHash: assessment.Hash,
		SourceClassification: assessment.Classification,
		Domains:              assessmentDomains(assessment),
		Needs:                needs,
		Risks: []string{
			"candidate evidence remains quarantined until explicit reviewed promotion",
			"the original unsupported assessment is immutable and requires a fresh run after promotion",
		},
	}
	plan.Risks = normalizedStrings(plan.Risks)
	hash, err := planHash(plan)
	if err != nil {
		return ExpansionPlan{}, err
	}
	plan.Hash = hash
	if err := ValidatePlan(plan); err != nil {
		return ExpansionPlan{}, err
	}
	return plan, nil
}

func ValidatePlan(plan ExpansionPlan) error {
	if plan.Schema != PlanSchema || plan.PolicyVersion != PolicyVersion {
		return fmt.Errorf("unsupported expansion plan schema or policy")
	}
	if plan.SourceClassification != capabilitygate.ClassificationUnsupported || !validSHA256(plan.SourceAssessmentHash) {
		return fmt.Errorf("expansion plan requires an unsupported source assessment hash")
	}
	if len(plan.Needs) == 0 || len(plan.Needs) > MaxExpansionNeeds {
		return fmt.Errorf("expansion plan requires at least one need")
	}
	if !canonicalSortedIDs(plan.Domains) {
		return fmt.Errorf("expansion plan requires at least one electrical domain")
	}
	seen := map[string]bool{}
	for _, need := range plan.Needs {
		if seen[need.ID] || need.ID != string(need.Kind)+":"+need.CapabilityID ||
			!validNeedKind(need.Kind) || canonicalID(need.CapabilityID) != need.CapabilityID ||
			len(need.GapCodes) == 0 || len(need.RequiredSourceKinds) == 0 ||
			strings.TrimSpace(need.RequiredArtifact) == "" || len(need.RequiredPromotionGates) == 0 {
			return fmt.Errorf("invalid expansion need %q", need.ID)
		}
		seen[need.ID] = true
		for _, sourceKind := range need.RequiredSourceKinds {
			if !validSourceKind(sourceKind) {
				return fmt.Errorf("need %q has invalid source kind %q", need.ID, sourceKind)
			}
		}
	}
	if !validSHA256(plan.Hash) {
		return fmt.Errorf("expansion plan hash is invalid")
	}
	expected, err := planHash(plan)
	if err != nil {
		return err
	}
	if plan.Hash != expected {
		return fmt.Errorf("expansion plan hash mismatch")
	}
	return nil
}

func assessmentDomains(assessment capabilitygate.Assessment) []string {
	var domains []string
	for _, requirement := range assessment.Requirements {
		if requirement.Kind == capabilitygate.RequirementDomain {
			domains = append(domains, canonicalID(requirement.ID))
		}
	}
	domains = normalizedStrings(domains)
	if len(domains) == 0 {
		domains = append(domains, "unspecified_electrical")
	}
	return domains
}

func canonicalSortedIDs(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for index, value := range values {
		if canonicalID(value) != value || (index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func planHash(plan ExpansionPlan) (string, error) {
	hashless := plan
	hashless.Hash = ""
	return digest(hashless)
}

func needKindForGap(gap capabilitygate.Gap) NeedKind {
	codeStage := strings.ToLower(gap.Code + " " + gap.Stage)
	switch gap.Kind {
	case capabilitygate.RequirementArchitecture, capabilitygate.RequirementDomain:
		return NeedArchitecture
	case capabilitygate.RequirementComponent:
		return NeedComponent
	case capabilitygate.RequirementModel:
		return NeedModel
	case capabilitygate.RequirementPhysical:
		if strings.Contains(codeStage, "rout") || strings.Contains(codeStage, "endpoint") || strings.Contains(codeStage, "layer") {
			return NeedRouting
		}
		return NeedPhysicalRule
	case capabilitygate.RequirementVerification:
		if strings.Contains(codeStage, "rout") {
			return NeedRouting
		}
		return NeedVerification
	default:
		return NeedVerification
	}
}

func sourceKindsForNeed(kind NeedKind) []SourceKind {
	switch kind {
	case NeedArchitecture:
		return []SourceKind{SourceEngineeringReference, SourceVerification}
	case NeedComponent:
		return []SourceKind{SourceDatasheet, SourceLibraryBinding}
	case NeedModel:
		return []SourceKind{SourceDatasheet, SourceModel}
	case NeedPhysicalRule, NeedRouting:
		return []SourceKind{SourceStandard, SourceVerification}
	case NeedVerification:
		return []SourceKind{SourceEngineeringReference, SourceVerification}
	default:
		return nil
	}
}

func artifactForNeed(kind NeedKind) string {
	switch kind {
	case NeedArchitecture:
		return "declarative_provider"
	case NeedComponent:
		return "catalog_record"
	case NeedModel:
		return "model_record"
	case NeedPhysicalRule:
		return "physical_rule"
	case NeedRouting:
		return "routing_adapter"
	case NeedVerification:
		return "verification_manifest"
	default:
		return "capability_artifact"
	}
}

func defaultActionForNeed(kind NeedKind) string {
	return "add source-backed reusable " + string(kind) + " evidence and pass promotion"
}

func canonicalID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastSeparator := false
	for _, r := range value {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if valid {
			builder.WriteRune(r)
			lastSeparator = false
			continue
		}
		if !lastSeparator && builder.Len() != 0 {
			builder.WriteByte('_')
			lastSeparator = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
