package fabrication

import (
	"strings"

	"kicadai/internal/reports"
)

func applyReportEvidence(result *Result, data ReportData, opts Options) {
	result.Summary.ComponentIdentity = componentIdentityEvidence(data.BOM)
	result.Summary.ComponentReadiness = componentReadinessEvidence(data.BOM)
	if result.Summary.ComponentReadiness != EvidenceMissing {
		result.Issues = removeExactIssuePath(result.Issues, "component_readiness")
	}
	result.Summary.BOMCPLConsistency = consistencyEvidence(data)
	result.Summary.ManufacturerProfile = EvidenceSkipped
	if profileID := strings.TrimSpace(opts.ManufacturerProfile); profileID != "" {
		profile, ok := LookupManufacturerProfile(profileID)
		if !ok {
			result.Summary.ManufacturerProfile = EvidenceFail
			result.Issues = append(result.Issues, reports.Issue{
				Code:       reports.CodeInvalidArgument,
				Severity:   reports.SeverityError,
				Path:       "manufacturer_profile",
				Message:    "unknown manufacturer profile " + profileID,
				Suggestion: "use a built-in profile such as generic_assembly",
			})
		} else {
			profileIssues := ValidateManufacturerProfile(profile, data)
			result.Issues = append(result.Issues, profileIssues...)
			result.Summary.ManufacturerProfile = evidenceForIssues(profileIssues, EvidencePass)
		}
	}
	result.Summary.AssemblyReadiness = assemblyEvidence(result.Summary)
}

func componentReadinessEvidence(rows []BOMRow) EvidenceStatus {
	if len(rows) == 0 {
		return EvidenceMissing
	}
	status := EvidencePass
	for _, row := range rows {
		if skipComponentIdentityWarning(row) {
			continue
		}
		if row.IdentityBlockingCount > 0 || row.IdentityStatus == IdentityFail || strings.TrimSpace(row.Manufacturer) == "" || strings.TrimSpace(row.MPN) == "" {
			return EvidenceFail
		}
		switch strings.ToLower(strings.TrimSpace(row.Lifecycle)) {
		case "active", "production", "recommended":
		case "obsolete", "eol", "end_of_life", "nrnd", "not_recommended_for_new_designs":
			return EvidenceFail
		default:
			status = EvidenceWarning
		}
		switch strings.ToLower(strings.TrimSpace(row.ProcurementOutcome)) {
		case "blocked", "rejected":
			return EvidenceFail
		case "warning", "stale":
			status = EvidenceWarning
		}
		if row.IdentityIssueCount > 0 || row.IdentityStatus == IdentityWarning || row.IdentityStatus == IdentityMissing || row.IdentityStatus == IdentitySkipped {
			status = EvidenceWarning
		}
	}
	return status
}

func componentIdentityEvidence(rows []BOMRow) EvidenceStatus {
	if len(rows) == 0 {
		return EvidenceMissing
	}
	status := EvidencePass
	for _, row := range rows {
		if row.IdentityBlockingCount > 0 || row.IdentityStatus == IdentityFail {
			return EvidenceFail
		}
		if skipComponentIdentityWarning(row) {
			continue
		}
		if row.IdentityIssueCount > 0 ||
			row.IdentityStatus == IdentityWarning ||
			row.IdentityStatus == IdentityMissing ||
			row.IdentityStatus == IdentitySkipped ||
			strings.TrimSpace(row.Manufacturer) == "" ||
			strings.TrimSpace(row.MPN) == "" {
			status = EvidenceWarning
		}
	}
	return status
}

func skipComponentIdentityWarning(row BOMRow) bool {
	switch strings.ToLower(strings.TrimSpace(row.ComponentClass)) {
	case "mechanical", "virtual", "documentation", "pcb", "logo", "mounting":
		return true
	}
	for _, ref := range row.References {
		switch referencePrefix(ref) {
		case "MH", "H", "FID", "LOGO":
			return true
		}
	}
	return false
}

func consistencyEvidence(data ReportData) EvidenceStatus {
	if len(data.BOM) == 0 || len(data.CPL) == 0 {
		return EvidenceMissing
	}
	if data.Consistency.BlockingCount > 0 {
		return EvidenceFail
	}
	if data.Consistency.WarningCount > 0 {
		return EvidenceWarning
	}
	return EvidencePass
}

func evidenceForIssues(issues []reports.Issue, pass EvidenceStatus) EvidenceStatus {
	if reports.HasBlockingIssue(issues) {
		return EvidenceFail
	}
	if len(issues) > 0 {
		return EvidenceWarning
	}
	return pass
}

func assemblyEvidence(summary Summary) EvidenceStatus {
	statuses := []EvidenceStatus{
		summary.ComponentReadiness,
		summary.ComponentIdentity,
		summary.BOMCPLConsistency,
		summary.ManufacturerProfile,
	}
	status := EvidencePass
	for _, evidence := range statuses {
		switch evidence {
		case EvidenceFail:
			return EvidenceFail
		case EvidenceMissing:
			if status != EvidenceWarning {
				status = EvidenceMissing
			}
		case EvidenceWarning:
			status = EvidenceWarning
		case EvidenceSkipped:
			if status == EvidencePass {
				status = EvidenceSkipped
			}
		}
	}
	return status
}
