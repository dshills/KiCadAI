package repair

import (
	"strings"

	"kicadai/internal/boardvalidation"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

func NormalizeWriterCorrectnessResult(result writercorrectness.Result, adapter string) []NormalizedFinding {
	findings := []NormalizedFinding{}
	for _, check := range result.Checks {
		findings = append(findings, NormalizeWriterCorrectnessCheck(check, adapter)...)
	}
	SortNormalizedFindings(findings)
	return findings
}

func NormalizeWriterCorrectnessCheck(check writercorrectness.CheckResult, adapter string) []NormalizedFinding {
	findings := make([]NormalizedFinding, 0, len(check.Issues))
	for _, issue := range check.Issues {
		findings = append(findings, NormalizeIssue(issue, NormalizeFindingOptions{
			Source:        sourceForWriterCheck(check.Name),
			Adapter:       firstNonEmptyStringValue(adapter, postValidatorWriterCorrectness),
			Category:      categoryForWriterCheck(check.Name, issue),
			Repairability: repairabilityForBuiltInIssue(issue, categoryForWriterCheck(check.Name, issue)),
			EvidencePath:  firstArtifactPath(check.Artifacts),
			RawCode:       check.Name,
			Subject:       subjectFromIssue(issue),
		}))
	}
	return findings
}

func NormalizeBoardValidationResult(result boardvalidation.Result, adapter string) []NormalizedFinding {
	findings := []NormalizedFinding{}
	for _, check := range result.Checks {
		findings = append(findings, NormalizeBoardValidationCheck(check, adapter)...)
	}
	SortNormalizedFindings(findings)
	return findings
}

func NormalizeBoardValidationCheck(check boardvalidation.Check, adapter string) []NormalizedFinding {
	findings := make([]NormalizedFinding, 0, len(check.Issues))
	for _, issue := range check.Issues {
		findings = append(findings, NormalizeIssue(issue, NormalizeFindingOptions{
			Source:        sourceForBoardCheck(check.Name),
			Adapter:       firstNonEmptyStringValue(adapter, postValidatorBoardValidation),
			Category:      categoryForBoardCheck(check.Name, issue),
			Repairability: repairabilityForBuiltInIssue(issue, categoryForBoardCheck(check.Name, issue)),
			EvidencePath:  firstNonEmptyStringValue(check.Evidence, firstArtifactPath(check.Artifacts)),
			RawCode:       check.Name,
			Subject:       subjectFromIssue(issue),
		}))
	}
	return findings
}

func sourceForWriterCheck(checkName string) FindingSource {
	if checkName == writercorrectness.CheckKiCadRoundTrip {
		return FindingSourceRoundTrip
	}
	return FindingSourceWriter
}

func sourceForBoardCheck(checkName string) FindingSource {
	if checkName == boardvalidation.CheckKiCadDRC {
		return FindingSourceKiCadDRC
	}
	return FindingSourceBoard
}

func categoryForWriterCheck(checkName string, issue reports.Issue) FindingCategory {
	switch checkName {
	case writercorrectness.CheckProjectStructure, writercorrectness.CheckLibraryResolver:
		return FindingCategoryProjectStructure
	case writercorrectness.CheckSchematicParse, writercorrectness.CheckPCBParse:
		return FindingCategoryParse
	case writercorrectness.CheckSchematicConnectivity, writercorrectness.CheckGeneratedConnectivity:
		return FindingCategoryConnectivity
	case writercorrectness.CheckSchematicPCBTransfer:
		return FindingCategoryConnectivity
	case writercorrectness.CheckPCBNetTable, writercorrectness.CheckFootprintPadNets:
		return FindingCategoryPadNet
	case writercorrectness.CheckCopperNetReferences:
		return FindingCategoryRoute
	case writercorrectness.CheckZoneNetReferences:
		return FindingCategoryZone
	case writercorrectness.CheckKiCadRoundTrip:
		return FindingCategoryRoundTrip
	default:
		return defaultFindingCategory(issue)
	}
}

func categoryForBoardCheck(checkName string, issue reports.Issue) FindingCategory {
	switch checkName {
	case boardvalidation.CheckPCBStructuralValidation:
		if issue.Code == reports.CodeMissingBoardOutline || containsEvidenceText(issue.Message, "outline") {
			return FindingCategoryOutline
		}
		return FindingCategoryProjectStructure
	case boardvalidation.CheckNetToPadValidation:
		return FindingCategoryPadNet
	case boardvalidation.CheckGeneratedConnectivity:
		return FindingCategoryConnectivity
	case boardvalidation.CheckUnroutedNetValidation, boardvalidation.CheckRouteCompletion:
		return FindingCategoryRoute
	case boardvalidation.CheckZoneValidation:
		return FindingCategoryZone
	case boardvalidation.CheckKiCadDRC:
		return FindingCategoryBoardDRC
	default:
		return defaultFindingCategory(issue)
	}
}

func repairabilityForBuiltInIssue(issue reports.Issue, category FindingCategory) Repairability {
	if !issue.Blocking() {
		return RepairabilityInformational
	}
	switch category {
	case FindingCategoryExternalTool:
		return RepairabilityExternalToolBlocked
	case FindingCategoryPreservation:
		return RepairabilityPreservationBlocked
	case FindingCategoryUnsupported:
		return RepairabilityUnsupported
	default:
		return RepairabilityRepairable
	}
}

func subjectFromIssue(issue reports.Issue) FindingSubject {
	subject := FindingSubject{}
	if len(issue.Refs) > 0 {
		subject.Ref = strings.Join(sortedStrings(issue.Refs), ",")
	}
	if len(issue.Nets) > 0 {
		subject.Net = strings.Join(sortedStrings(issue.Nets), ",")
	}
	subject.File = fileSubjectFromPath(slashPathForEvidence(issue.Path))
	subject.Location = locationFromIssuePath(issue.Path)
	return subject
}

func locationFromIssuePath(path string) string {
	path = slashPathForEvidence(path)
	if path == "" {
		return ""
	}
	for _, marker := range []string{".footprints.", ".pads.", ".tracks.", ".vias.", ".zones.", ".symbols."} {
		if index := strings.Index(path, marker); index >= 0 {
			return path[index+1:]
		}
	}
	return ""
}

func firstArtifactPath(artifacts []reports.Artifact) string {
	if len(artifacts) == 0 {
		return ""
	}
	return artifacts[0].Path
}
