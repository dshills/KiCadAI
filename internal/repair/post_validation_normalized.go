package repair

import "strings"

func NormalizeStageIssues(groups []StageIssues) []NormalizedFinding {
	var findings []NormalizedFinding
	for _, group := range groups {
		adapter := strings.TrimSpace(group.Stage)
		source, _ := findingMetadataForAdapter(adapter)
		findings = append(findings, NormalizeIssues(group.Issues, NormalizeFindingOptions{
			Source:  source,
			Adapter: adapter,
		})...)
	}
	SortNormalizedFindings(findings)
	return findings
}

func NormalizePostApplyValidations(validations []PostApplyValidation) []NormalizedFinding {
	var findings []NormalizedFinding
	for _, validation := range validations {
		adapter := strings.TrimSpace(validation.Name)
		source, category := findingMetadataForAdapter(adapter)
		findings = append(findings, NormalizeIssues(validation.Issues, NormalizeFindingOptions{
			Source:   source,
			Adapter:  adapter,
			Category: category,
		})...)
	}
	SortNormalizedFindings(findings)
	return findings
}

func findingMetadataForAdapter(adapter string) (FindingSource, FindingCategory) {
	switch adapter {
	case "transaction":
		return FindingSourceTransaction, ""
	case postValidatorWriterCorrectness:
		return FindingSourceWriter, ""
	case postValidatorBoardValidation, "validation":
		return FindingSourceBoard, ""
	case postValidatorKiCadERC:
		return FindingSourceKiCadERC, ""
	case postValidatorKiCadDRC, "drc":
		return FindingSourceKiCadDRC, ""
	case postValidatorRoundTrip, "roundtrip", "round_trip":
		return FindingSourceRoundTrip, ""
	case "workflow":
		return FindingSourceWorkflow, ""
	default:
		if strings.HasPrefix(adapter, "zone_refill") {
			return FindingSourceKiCadDRC, FindingCategoryZone
		}
		return FindingSourceRepair, ""
	}
}
